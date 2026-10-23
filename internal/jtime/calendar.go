/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package jtime

// This file is java.util.GregorianCalendar reduced to what SimpleDateFormat asks of it: the
// conversion between a local millisecond value and the broken down fields, in the hybrid Julian /
// Gregorian calendar Java uses. Day numbers here are "fixed dates" rebased on the epoch: day 0 is
// 1970-01-01, which makes the millisecond arithmetic a plain floor division.

const millisPerDay = 86400000

// gregorianCutoverDay is the day number of 1582-10-15, the day GregorianCalendar leaves the
// Julian calendar behind. Java's cutover is the instant -12219292800000, and that is exactly
// -141427 days; the calendar system is chosen by comparing the *local* date with it, so a zone
// offset never moves the cutover.
const gregorianCutoverDay = -141427

// gregorianCutoverYear is GregorianCalendar.gregorianCutoverYear: the year the cutover falls in.
// It picks which calendar system a year is converted with before the result is double checked.
const gregorianCutoverYear = 1582

// Era values, as java.util.GregorianCalendar.BC and .AD.
const (
	eraBC = 0
	eraAD = 1
)

// floorDiv and floorMod are Java's Math.floorDiv and Math.floorMod: they round towards negative
// infinity, which is what date arithmetic before the epoch needs.
func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func floorMod(a, b int64) int64 { return a - floorDiv(a, b)*b }

// isGregorianLeap is CalendarUtils.isGregorianLeapYear on an astronomical year number (year 0 is
// 1 BC), and isJulianLeap is CalendarUtils.isJulianLeapYear: every fourth year, year 0 included.
func isGregorianLeap(y int64) bool {
	return floorMod(y, 4) == 0 && (floorMod(y, 100) != 0 || floorMod(y, 400) == 0)
}

func isJulianLeap(y int64) bool { return floorMod(y, 4) == 0 }

// monthLengths is the length of each month of a common year.
var monthLengths = [12]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

// daysFromGregorian is the day number of a date in the proleptic Gregorian calendar, with the
// month 1 based and astronomical year numbering. It accepts any day number, including out of
// range ones: a lenient parse relies on 2021-01-32 meaning the day after 2021-01-31.
func daysFromGregorian(y int64, m, d int) int64 {
	n := 365*(y-1) + floorDiv(y-1, 4) - floorDiv(y-1, 100) + floorDiv(y-1, 400)
	n += int64((367*m - 362) / 12)
	if m > 2 {
		if isGregorianLeap(y) {
			n--
		} else {
			n -= 2
		}
	}
	return n + int64(d) - 719163
}

// daysFromJulian is the day number of a date in the Julian calendar. The Julian epoch is two days
// before the Gregorian one, which is the whole of the difference between the two formulas.
func daysFromJulian(y int64, m, d int) int64 {
	n := -2 + 365*(y-1) + floorDiv(y-1, 4)
	n += int64((367*m - 362) / 12)
	if m > 2 {
		if isJulianLeap(y) {
			n--
		} else {
			n -= 2
		}
	}
	return n + int64(d) - 719163
}

// daysFromYMD converts a date to a day number the way GregorianCalendar.computeTime does: the
// calendar system is chosen from the year, and if the resulting day falls on the other side of
// the cutover the conversion is redone with the other system. Dates inside the ten day gap of
// October 1582 therefore end up with the Gregorian reading, as they do in Java.
func daysFromYMD(y int64, m, d int) int64 {
	if y > gregorianCutoverYear {
		if n := daysFromGregorian(y, m, d); n >= gregorianCutoverDay {
			return n
		}
		return daysFromJulian(y, m, d)
	}
	j := daysFromJulian(y, m, d)
	if j < gregorianCutoverDay {
		return j
	}
	// The Julian reading lands on or after the cutover, so the date is read as Gregorian -- but
	// only if that reading is itself on or after the cutover. The ten days October 1582 lost keep
	// their Julian reading, which is how Java resolves 1582-10-10.
	if g := daysFromGregorian(y, m, d); g >= gregorianCutoverDay {
		return g
	}
	return j
}

// gregorianFromDays is the inverse of daysFromGregorian.
func gregorianFromDays(n int64) (y int64, m, d int) {
	// Shift the era to start at 0000-03-01 so that the leap day is the last day of the cycle.
	z := n + 719468
	era := floorDiv(z, 146097)
	doe := z - era*146097                                  // [0, 146096]
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365 // [0, 399]
	yr := yoe + era*400                                    // March based year
	doy := doe - (365*yoe + yoe/4 - yoe/100)               // [0, 365]
	mp := (5*doy + 2) / 153                                // [0, 11], 0 is March
	d = int(doy-(153*mp+2)/5) + 1                          // [1, 31]
	m = int(mp) + 3                                        // [3, 14]
	if m > 12 {
		m -= 12
		yr++
	}
	return yr, m, d
}

// julianFromDays is the inverse of daysFromJulian.
func julianFromDays(n int64) (y int64, m, d int) {
	// Estimate the year from the mean Julian year, then walk at most one step either way.
	y = floorDiv(n+719163+2, 366) + 1
	for daysFromJulian(y+1, 1, 1) <= n {
		y++
	}
	for daysFromJulian(y, 1, 1) > n {
		y--
	}
	doy := int(n - daysFromJulian(y, 1, 1)) // 0 based day of year
	m = 1
	for {
		length := monthLengths[m-1]
		if m == 2 && isJulianLeap(y) {
			length++
		}
		if doy < length {
			break
		}
		doy -= length
		m++
	}
	return y, m, doy + 1
}

// ymdFromDays converts a day number to a date, using whichever calendar system Java would.
func ymdFromDays(n int64) (y int64, m, d int) {
	if n >= gregorianCutoverDay {
		return gregorianFromDays(n)
	}
	return julianFromDays(n)
}

// dayOfWeek is Calendar.DAY_OF_WEEK: 1 is Sunday. Day 0, 1970-01-01, was a Thursday.
func dayOfWeek(n int64) int { return int(floorMod(n+4, 7)) + 1 }

// calFields are the java.util.Calendar fields SimpleDateFormat reads while formatting.
type calFields struct {
	era              int   // eraBC or eraAD
	year             int64 // positive year within the era, as Calendar.YEAR
	astroYear        int64 // year in astronomical numbering, used by the week based year
	month            int   // 0 based, as Calendar.MONTH
	day              int
	hour             int // 0..23
	minute           int
	second           int
	millis           int
	dayOfWeek        int // 1 is Sunday
	dayOfYear        int
	weekOfYear       int
	weekOfMonth      int
	weekYear         int64
	dayOfWeekInMonth int
}

// weekStart is the day the week containing n starts on, for a locale whose week starts on
// firstDayOfWeek (1 is Sunday).
func weekStart(n int64, firstDayOfWeek int) int64 {
	return n - int64((dayOfWeek(n)-firstDayOfWeek+7)%7)
}

// firstWeekStart is the day week 1 of a period starting at first begins on: the week containing
// first, unless it holds fewer than minimalDays days of the period, in which case week 1 is the
// week after. This is java.util.Calendar's generalisation of the ISO week rules.
func firstWeekStart(first int64, firstDayOfWeek, minimalDays int) int64 {
	start := weekStart(first, firstDayOfWeek)
	if 7-int(first-start) < minimalDays {
		start += 7
	}
	return start
}

// computeFields breaks a local millisecond value down into calendar fields, as
// GregorianCalendar.computeFields does for a calendar with the given week rules.
func computeFields(localMillis int64, firstDayOfWeek, minimalDays int) calFields {
	var f calFields
	n := floorDiv(localMillis, millisPerDay)
	ms := int(localMillis - n*millisPerDay)
	f.millis = ms % 1000
	f.second = (ms / 1000) % 60
	f.minute = (ms / 60000) % 60
	f.hour = ms / 3600000

	y, m, d := ymdFromDays(n)
	f.astroYear, f.month, f.day = y, m-1, d
	if y <= 0 {
		f.era, f.year = eraBC, 1-y
	} else {
		f.era, f.year = eraAD, y
	}
	f.dayOfWeek = dayOfWeek(n)
	jan1 := daysFromYMD(y, 1, 1)
	f.dayOfYear = int(n-jan1) + 1

	// Day of week in month counts from the first day the month actually has, which is the same
	// as (day-1)/7+1 everywhere except October 1582, where the month starts in the Julian
	// calendar and ten of its days do not exist.
	month1 := daysFromYMD(y, m, 1)
	f.dayOfWeekInMonth = int((n-month1)/7) + 1

	// Week of month can legitimately be 0 in Java: unlike week of year it is never pushed back
	// into the previous month.
	f.weekOfMonth = int((weekStart(n, firstDayOfWeek)-firstWeekStart(month1, firstDayOfWeek, minimalDays))/7) + 1

	// Week of year, with the roll into the neighbouring year that gives the week based year.
	start := firstWeekStart(jan1, firstDayOfWeek, minimalDays)
	switch {
	case weekStart(n, firstDayOfWeek) < start:
		prevJan1 := daysFromYMD(y-1, 1, 1)
		prevStart := firstWeekStart(prevJan1, firstDayOfWeek, minimalDays)
		f.weekOfYear = int((weekStart(n, firstDayOfWeek)-prevStart)/7) + 1
		f.weekYear = y - 1
	default:
		nextJan1 := daysFromYMD(y+1, 1, 1)
		nextStart := firstWeekStart(nextJan1, firstDayOfWeek, minimalDays)
		if weekStart(n, firstDayOfWeek) >= nextStart {
			f.weekOfYear = 1
			f.weekYear = y + 1
		} else {
			f.weekOfYear = int((weekStart(n, firstDayOfWeek)-start)/7) + 1
			f.weekYear = y
		}
	}
	return f
}
