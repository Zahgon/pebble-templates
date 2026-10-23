/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package jtime

import (
	"strconv"
	"strings"
)

// Format is new SimpleDateFormat(pattern, locale).format(date) with the format's time zone set to
// zone: every pattern letter of SimpleDateFormat (G y Y M L w W D d F E u a H k K h m s S z Z X),
// the text forms for counts >= 4, and the locale's names and first day of week.
func Format(date Millis, pattern, locale, zone string) (string, error) {
	items, err := compilePattern(pattern)
	if err != nil {
		return "", err
	}
	tag, ld := resolveLocale(locale)
	z := resolveZone(zone)
	offset := z.offset(date)
	f := computeFields(int64(date)+int64(offset), ld.firstDayOfWeek, ld.minimalDays)

	// CLDR gives some locales a different month name for a month standing on its own than for
	// one beside another field: German is "Nov" alone but "Nov." in "d MMM". Java switches to the
	// stand-alone names exactly when the pattern holds no other field.
	fields := 0
	for _, it := range items {
		if it.letter != 0 {
			fields++
		}
	}
	standalone := fields == 1

	var b strings.Builder
	for _, it := range items {
		if it.letter == 0 {
			b.WriteString(it.text)
			continue
		}
		if err := formatField(&b, it, f, ld, z, tag, date, offset, standalone); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// formatField is SimpleDateFormat.subFormat for one run of pattern letters.
func formatField(b *strings.Builder, it patternItem, f calFields, ld localeData, z *javaZone, tag string, date Millis, offset int, standalone bool) error {
	count := it.count
	switch it.letter {
	case 'G':
		// The era is a text field whose count never matters: Java has only the one set of names.
		b.WriteString(ld.eras[f.era])
	case 'y':
		writeYear(b, f.year, count)
	case 'Y':
		writeYear(b, f.weekYear, count)
	case 'M':
		if standalone {
			writeMonth(b, f.month, count, ld.monthsStandShort, ld.monthsStandLong)
		} else {
			writeMonth(b, f.month, count, ld.monthsShort, ld.monthsLong)
		}
	case 'L':
		writeMonth(b, f.month, count, ld.monthsStandShort, ld.monthsStandLong)
	case 'd':
		writeNumber(b, int64(f.day), count)
	case 'k':
		// k is the hour of day counted from 1, so midnight prints as 24.
		h := f.hour
		if h == 0 {
			h = 24
		}
		writeNumber(b, int64(h), count)
	case 'H':
		writeNumber(b, int64(f.hour), count)
	case 'm':
		writeNumber(b, int64(f.minute), count)
	case 's':
		writeNumber(b, int64(f.second), count)
	case 'S':
		// S is the millisecond field zero padded to the count, not a fraction: SSSS of 678 ms is
		// "0678", and S of 678 ms is "678".
		writeNumber(b, int64(f.millis), count)
	case 'E':
		if count >= 4 {
			b.WriteString(ld.weekdaysLong[f.dayOfWeek-1])
		} else {
			b.WriteString(ld.weekdaysShort[f.dayOfWeek-1])
		}
	case 'D':
		writeNumber(b, int64(f.dayOfYear), count)
	case 'F':
		writeNumber(b, int64(f.dayOfWeekInMonth), count)
	case 'w':
		writeNumber(b, int64(f.weekOfYear), count)
	case 'W':
		writeNumber(b, int64(f.weekOfMonth), count)
	case 'a':
		b.WriteString(ld.amPM[f.hour/12])
	case 'h':
		h := f.hour % 12
		if h == 0 {
			h = 12
		}
		writeNumber(b, int64(h), count)
	case 'K':
		writeNumber(b, int64(f.hour%12), count)
	case 'u':
		// Calendar's ISO day of week: Monday is 1, Sunday is 7.
		iso := f.dayOfWeek - 1
		if iso == 0 {
			iso = 7
		}
		writeNumber(b, int64(iso), count)
	case 'z':
		b.WriteString(z.displayName(tag, z.inDST(date), count >= 4))
	case 'Z':
		// Z is always RFC 822, however many letters there are.
		writeRFC822(b, offset)
	case 'X':
		if count >= 4 {
			// SimpleDateFormat.encode lets the constructor through and fails here instead.
			return &IllegalArgumentError{Message: "invalid ISO 8601 format: length=" + strconv.Itoa(count)}
		}
		writeISO8601(b, offset, count)
	}
	return nil
}

// writeYear is the year rule: two letters mean exactly the last two digits, any other count is
// the whole year zero padded to that width.
func writeYear(b *strings.Builder, year int64, count int) {
	if count == 2 {
		b.WriteString(zeroPad(year, 2, 2))
		return
	}
	writeNumber(b, year, count)
}

func writeMonth(b *strings.Builder, month, count int, short, long [12]string) {
	switch {
	case count >= 4:
		b.WriteString(long[month])
	case count == 3:
		b.WriteString(short[month])
	default:
		writeNumber(b, int64(month+1), count)
	}
}

func writeNumber(b *strings.Builder, value int64, minDigits int) {
	b.WriteString(zeroPad(value, minDigits, 0))
}

// zeroPad is SimpleDateFormat.zeroPaddingNumber: the digits of the value, padded on the left with
// zeroes to minDigits and, when maxDigits is positive, cut down to its lowest maxDigits digits.
// A negative value keeps its sign in front of the padding, as Java's NumberFormat writes it.
func zeroPad(value int64, minDigits, maxDigits int) string {
	neg := value < 0
	digits := strconv.FormatUint(uint64(abs64(value)), 10)
	if maxDigits > 0 && len(digits) > maxDigits {
		digits = digits[len(digits)-maxDigits:]
	}
	if n := minDigits - len(digits); n > 0 {
		digits = strings.Repeat("0", n) + digits
	}
	if neg {
		return "-" + digits
	}
	return digits
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// writeRFC822 is the Z form: a sign and four digits, "+0530" or "-0500".
func writeRFC822(b *strings.Builder, offset int) {
	minutes := offset / 60000
	if minutes >= 0 {
		b.WriteByte('+')
	} else {
		b.WriteByte('-')
		minutes = -minutes
	}
	b.WriteString(zeroPad(int64(minutes/60*100+minutes%60), 4, 0))
}

// writeISO8601 is the X form: "Z" for UTC, otherwise a sign and the hours, with the minutes
// appended for two letters and separated by a colon for three. One letter drops the minutes, so
// Asia/Kolkata prints as "+05".
func writeISO8601(b *strings.Builder, offset, count int) {
	if offset == 0 {
		b.WriteByte('Z')
		return
	}
	minutes := offset / 60000
	if minutes < 0 {
		b.WriteByte('-')
		minutes = -minutes
	} else {
		b.WriteByte('+')
	}
	b.WriteString(zeroPad(int64(minutes/60), 2, 0))
	if count == 1 {
		return
	}
	if count == 3 {
		b.WriteByte(':')
	}
	b.WriteString(zeroPad(int64(minutes%60), 2, 0))
}
