/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package jnum

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// The characters java.text.DecimalFormat.applyPattern gives a meaning to. A pattern is never
// localized here, because Pebble builds its formats with new DecimalFormat(pattern, symbols),
// which applies the pattern with these fixed characters and only writes the locale's symbols.
const (
	patZero      = '0'
	patGrouping  = ','
	patDecimal   = '.'
	patPerCent   = '%'
	patPerMille  = '‰'
	patDigit     = '#'
	patSeparator = ';'
	patExponent  = 'E'
	patMinus     = '-'
	patCurrency  = '¤'
	patQuote     = '\''
)

// decimalFormat is a java.text.DecimalFormat: a pattern reduced to digit counts, affixes and a
// multiplier. The rounding mode is always HALF_EVEN, DecimalFormat's default.
type decimalFormat struct {
	sym                   *symbols
	posPrefix, posSuffix  string
	negPrefix, negSuffix  string
	multiplier            int
	groupingSize          int
	groupingUsed          bool
	minInt, maxInt        int
	minFrac, maxFrac      int
	useExponent           bool
	minExponentDigits     int
	decimalSepAlwaysShown bool
}

// maximumIntegerDigits is DecimalFormat's DOUBLE_INTEGER_DIGITS ceiling for a pattern without an
// explicit limit.
const maximumIntegerDigits = 2147483647

// newDecimalFormat is new DecimalFormat(pattern, symbols): it parses the pattern in the three
// phases applyPattern uses (prefix, digits, suffix) for the positive subpattern and, after a
// ';', the negative one.
func newDecimalFormat(pattern string, sym *symbols) (*decimalFormat, error) {
	f := &decimalFormat{sym: sym, multiplier: 1, minInt: 1}
	if pattern == "" {
		// An empty pattern leaves the JDK's own defaults in place.
		f.minInt, f.maxInt, f.minFrac, f.maxFrac = 0, maximumIntegerDigits, 0, 340
		return f, nil
	}
	p := []rune(pattern)
	malformed := illegalArgumentError(`Malformed pattern "` + pattern + `"`)
	gotNegative := false
	start := 0
	for j := 1; j >= 0 && start < len(p); j-- {
		var prefix, suffix strings.Builder
		affix := &prefix
		phase := 0
		inQuote := false
		decimalPos := -1
		multiplier := 1
		digitLeft, zeroDigit, digitRight := 0, 0, 0
		groupingCount := -1
		useExponent := false
		minExponentDigits := 0
		endOfSubpattern := len(p)
		nextStart := len(p)

		for pos := start; pos < endOfSubpattern; pos++ {
			ch := p[pos]
			if phase == 1 {
				switch {
				case ch == patDigit:
					if zeroDigit > 0 {
						digitRight++
					} else {
						digitLeft++
					}
					if groupingCount >= 0 && decimalPos < 0 {
						groupingCount++
					}
				case ch >= patZero && ch <= patZero+9:
					if digitRight > 0 {
						return nil, illegalArgumentError(`Unexpected '0' in pattern "` + pattern + `"`)
					}
					zeroDigit++
					if groupingCount >= 0 && decimalPos < 0 {
						groupingCount++
					}
				case ch == patGrouping:
					groupingCount = 0
				case ch == patDecimal:
					if decimalPos >= 0 {
						return nil, illegalArgumentError(`Multiple decimal separators in pattern "` + pattern + `"`)
					}
					decimalPos = digitLeft + zeroDigit + digitRight
				case ch == patExponent:
					if useExponent {
						return nil, illegalArgumentError(`Multiple exponential symbols in pattern "` + pattern + `"`)
					}
					useExponent = true
					minExponentDigits = 0
					// The exponent digits follow the mark immediately; read them here and
					// continue with the suffix.
					pos++
					for pos < len(p) && p[pos] == patZero {
						minExponentDigits++
						pos++
					}
					if digitLeft+zeroDigit < 1 || minExponentDigits < 1 {
						return nil, illegalArgumentError(`Malformed exponential pattern "` + pattern + `"`)
					}
					phase = 2
					affix = &suffix
					pos--
				default:
					phase = 2
					affix = &suffix
					pos--
				}
				continue
			}
			// Phase 0 (prefix) and phase 2 (suffix).
			if inQuote {
				if ch == patQuote {
					if pos+1 < len(p) && p[pos+1] == patQuote {
						pos++
						affix.WriteRune(patQuote)
					} else {
						inQuote = false
					}
					continue
				}
				affix.WriteRune(ch)
				continue
			}
			switch {
			case ch == patDigit || ch == patGrouping || ch == patDecimal || (ch >= patZero && ch <= patZero+9):
				// Any of these begins the digit part; reprocess the character there. A suffix
				// that runs into one goes back to the digit phase too, which is how the JDK
				// accepts "#0b0" and rejects "#0b#".
				phase = 1
				pos--
			case ch == patQuote:
				if pos+1 < len(p) && p[pos+1] == patQuote {
					pos++
					affix.WriteRune(patQuote)
				} else {
					inQuote = true
				}
			case ch == patSeparator:
				if phase == 0 || j == 0 {
					return nil, illegalArgumentError(fmt.Sprintf("Unquoted special character '%c' in pattern %q", ch, pattern))
				}
				nextStart = pos + 1
				endOfSubpattern = pos
			case ch == patCurrency:
				affix.WriteString(sym.currency)
			case ch == patPerCent:
				if multiplier != 1 {
					return nil, illegalArgumentError(`Too many percent/per mille characters in pattern "` + pattern + `"`)
				}
				multiplier = 100
				affix.WriteString(sym.percent)
			case ch == patPerMille:
				if multiplier != 1 {
					return nil, illegalArgumentError(`Too many percent/per mille characters in pattern "` + pattern + `"`)
				}
				multiplier = 1000
				affix.WriteString(sym.perMille)
			case ch == patMinus:
				affix.WriteString(sym.minus)
			default:
				affix.WriteRune(ch)
			}
		}
		if inQuote {
			return nil, malformed
		}
		// A pattern with no '0' is read as if it had one: "##.###" is "#0.###", ".###" is ".0##".
		if zeroDigit == 0 && digitLeft > 0 && decimalPos >= 0 {
			n := decimalPos
			if n == 0 {
				n++
			}
			digitRight = digitLeft - n
			digitLeft = n - 1
			zeroDigit = 1
		}
		if (decimalPos < 0 && digitRight > 0) ||
			(decimalPos >= 0 && (decimalPos < digitLeft || decimalPos > digitLeft+zeroDigit)) ||
			groupingCount == 0 {
			return nil, malformed
		}
		if j == 1 {
			f.posPrefix, f.posSuffix = prefix.String(), suffix.String()
			f.negPrefix, f.negSuffix = f.posPrefix, f.posSuffix
			digitTotal := digitLeft + zeroDigit + digitRight
			effectiveDecimalPos := decimalPos
			if decimalPos < 0 {
				effectiveDecimalPos = digitTotal
			}
			f.minInt = effectiveDecimalPos - digitLeft
			if useExponent {
				f.maxInt = digitLeft + f.minInt
			} else {
				f.maxInt = maximumIntegerDigits
			}
			if decimalPos >= 0 {
				f.maxFrac = digitTotal - decimalPos
				f.minFrac = digitLeft + zeroDigit - decimalPos
			} else {
				f.maxFrac, f.minFrac = 0, 0
			}
			f.groupingUsed = groupingCount > 0
			if groupingCount > 0 {
				f.groupingSize = groupingCount
			}
			f.multiplier = multiplier
			f.decimalSepAlwaysShown = decimalPos == 0 || decimalPos == digitTotal
			f.useExponent = useExponent
			f.minExponentDigits = minExponentDigits
		} else {
			// Only the affixes of the negative subpattern matter; its digit counts are ignored.
			f.negPrefix, f.negSuffix = prefix.String(), suffix.String()
			gotNegative = true
		}
		start = nextStart
	}
	if !gotNegative || (f.negPrefix == f.posPrefix && f.negSuffix == f.posSuffix) {
		f.negSuffix = f.posSuffix
		f.negPrefix = f.sym.minus + f.posPrefix
	}
	return f, nil
}

// digitList is java.text.DigitList: the significant decimal digits of a value, with no leading or
// trailing zeros, and the position of the decimal point among them (value == 0.digits*10^decimalAt).
type digitList struct {
	digits    []byte
	count     int
	decimalAt int
}

func (d *digitList) isZero() bool { return d.count == 0 }

// setString is DigitList.set: it reads a Java number string (plain or exponential), drops the
// leading zeros and rounds to maximumDigits. With fixedPoint the limit counts fraction digits,
// otherwise significant ones. roundedUp and exact describe how the string relates to the value it
// came from, which only a tie needs (see shouldRoundUp).
func (d *digitList) setString(s string, roundedUp, exact bool, maximumDigits int, fixedPoint bool) {
	d.digits = d.digits[:0]
	d.count = 0
	d.decimalAt = -1
	exponent := 0
	leadingZerosAfterDecimal := 0
	nonZeroSeen := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '.':
			d.decimalAt = d.count
		case c == 'e' || c == 'E':
			exponent = parseExponent(s[i+1:])
			i = len(s)
		default:
			if !nonZeroSeen {
				nonZeroSeen = c != '0'
				if !nonZeroSeen && d.decimalAt != -1 {
					leadingZerosAfterDecimal++
				}
			}
			if nonZeroSeen {
				d.digits = append(d.digits, c)
				d.count++
			}
		}
		if i == len(s) {
			break
		}
	}
	if d.decimalAt == -1 {
		d.decimalAt = d.count
	}
	if nonZeroSeen {
		d.decimalAt += exponent - leadingZerosAfterDecimal
	}
	if fixedPoint {
		// A value too small for the fraction digits on offer rounds to nothing, or to a single
		// one in the last place.
		if -d.decimalAt > maximumDigits {
			d.count = 0
			d.digits = d.digits[:0]
			return
		}
		if -d.decimalAt == maximumDigits {
			if d.shouldRoundUp(0, roundedUp, exact) {
				d.digits = append(d.digits[:0], '1')
				d.count = 1
				d.decimalAt++
			} else {
				d.count = 0
				d.digits = d.digits[:0]
			}
			return
		}
	}
	for d.count > 1 && d.digits[d.count-1] == '0' {
		d.count--
	}
	d.digits = d.digits[:d.count]
	if fixedPoint {
		d.round(maximumDigits+d.decimalAt, roundedUp, exact)
	} else {
		d.round(maximumDigits, roundedUp, exact)
	}
}

// parseExponent is DigitList.parseInt: a signed run of digits, stopping at anything else.
func parseExponent(s string) int {
	positive := true
	i := 0
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		positive = s[i] == '+'
		i++
	}
	value := 0
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			break
		}
		value = value*10 + int(s[i]-'0')
	}
	if positive {
		return value
	}
	return -value
}

// round keeps at most maximumDigits digits, rounding HALF_EVEN. A negative maximumDigits means no
// limit, which is what the integer paths of DecimalFormat ask for.
func (d *digitList) round(maximumDigits int, alreadyRounded, exact bool) {
	if maximumDigits < 0 || maximumDigits >= d.count {
		return
	}
	if d.shouldRoundUp(maximumDigits, alreadyRounded, exact) {
		// Carry from the least significant kept digit upwards; all nines becomes a single one
		// with a larger exponent.
		for {
			maximumDigits--
			if maximumDigits < 0 {
				d.digits[0] = '1'
				d.decimalAt++
				maximumDigits = 0
				break
			}
			d.digits[maximumDigits]++
			if d.digits[maximumDigits] <= '9' {
				break
			}
		}
		maximumDigits++
	}
	d.count = maximumDigits
	for d.count > 1 && d.digits[d.count-1] == '0' {
		d.count--
	}
	d.digits = d.digits[:d.count]
}

// shouldRoundUp is DigitList.shouldRoundUp for RoundingMode.HALF_EVEN. A '5' that is the last
// digit of a double's shortest representation is only half way if that representation is the
// exact value; otherwise the sign of the error decides, which is why DecimalFormat("#.##")
// rounds the double 1.005 down (its exact value is below 1.005) but the BigDecimal 1.005 to even.
func (d *digitList) shouldRoundUp(maximumDigits int, alreadyRounded, exact bool) bool {
	if maximumDigits >= d.count {
		return false
	}
	switch {
	case d.digits[maximumDigits] > '5':
		return true
	case d.digits[maximumDigits] == '5':
		if maximumDigits == d.count-1 {
			if alreadyRounded {
				return false
			}
			if !exact {
				return true
			}
			return maximumDigits > 0 && d.digits[maximumDigits-1]%2 != 0
		}
		for i := maximumDigits + 1; i < d.count; i++ {
			if d.digits[i] != '0' {
				return true
			}
		}
	}
	return false
}

// Format formats a number with java.text.DecimalFormat(pattern, DecimalFormatSymbols(locale)).
// The rounding is HALF_EVEN, as java.text.DecimalFormat's default. An invalid pattern is an
// IllegalArgumentException error with the JDK's text.
func Format(pattern, locale string, value any) (string, error) {
	f, err := newDecimalFormat(pattern, symbolsFor(locale))
	if err != nil {
		return "", err
	}
	return f.format(value)
}

// FormatDefault formats with NumberFormat.getInstance(locale), whose pattern is "#,##0.###" in
// every locale the JDK the reference command runs on knows about.
func FormatDefault(locale string, value any) string {
	f, err := newDecimalFormat("#,##0.###", symbolsFor(locale))
	if err != nil {
		panic("jnum: " + err.Error())
	}
	s, err := f.format(value)
	if err != nil {
		panic("jnum: " + err.Error())
	}
	return s
}

// format is DecimalFormat.format(Object): the boxed type picks the path, and the integer types go
// through one that never rounds away digits.
func (f *decimalFormat) format(value any) (string, error) {
	switch v := value.(type) {
	case int64:
		return f.formatInt(v)
	case int32:
		return f.formatInt(int64(v))
	case int16:
		return f.formatInt(int64(v))
	case int8:
		return f.formatInt(int64(v))
	case int:
		return f.formatInt(int64(v))
	case float64:
		return f.formatFloat(v), nil
	case float32:
		// Java widens a Float to a double before formatting, so 3.1f formats through the double
		// 3.0999999046325684.
		return f.formatFloat(float64(v)), nil
	case *BigDecimal:
		return f.formatBigDecimal(v), nil
	case *big.Int:
		if v.IsInt64() {
			return f.formatInt(v.Int64())
		}
		return f.formatBigInt(v), nil
	case *big.Rat:
		fl, _ := v.Float64()
		return f.formatFloat(fl), nil
	default:
		return "", illegalArgumentError("Cannot format given Object as a Number")
	}
}

// formatFloat is DecimalFormat.format(double).
func (f *decimalFormat) formatFloat(v float64) string {
	if math.IsNaN(v) {
		return f.sym.nan
	}
	negative := math.Signbit(v)
	if f.multiplier != 1 {
		v *= float64(f.multiplier)
	}
	if math.IsInf(v, 0) {
		var b strings.Builder
		b.WriteString(f.prefix(negative))
		b.WriteString(f.sym.infinity)
		b.WriteString(f.suffix(negative))
		return b.String()
	}
	v = math.Abs(v)
	maxDigits := f.maxFrac
	if f.useExponent {
		maxDigits = f.maxInt + f.maxFrac
	}
	var d digitList
	if v == 0 {
		d.setString("0", false, true, maxDigits, !f.useExponent)
	} else {
		digits, exp, roundedUp, exact := legacyDigits(v)
		d.setString(javaDigitsString(digits, exp), roundedUp, exact, maxDigits, !f.useExponent)
	}
	return f.subformat(&d, negative, false)
}

// javaDigitsString spells a digit string and its decimal exponent the way DigitList's parser
// expects: a plain integer part with the point where it belongs.
func javaDigitsString(digits string, exp int) string {
	return "0." + digits + "E" + strconv.Itoa(exp)
}

// formatInt is DecimalFormat.format(long): the integer digits are never dropped, so a pattern
// without an exponent sees the value in full.
func (f *decimalFormat) formatInt(v int64) (string, error) {
	negative := v < 0
	// Long.MIN_VALUE has no positive counterpart, so take the magnitude unsigned; DigitList
	// keeps a hard coded representation of it for the same reason.
	magnitude := uint64(v)
	if negative {
		magnitude = -magnitude
	}
	if f.multiplier != 1 {
		// Java moves to the BigInteger path when the multiplier would overflow the long, which
		// keeps every digit of Long.MAX_VALUE * 100.
		if magnitude > uint64(math.MaxInt64)/uint64(f.multiplier) {
			return f.formatBigInt(big.NewInt(v)), nil
		}
		magnitude *= uint64(f.multiplier)
	}
	maxDigits := -1
	if f.useExponent {
		maxDigits = f.maxInt + f.maxFrac
	}
	var d digitList
	d.setString(strconv.FormatUint(magnitude, 10), false, true, maxDigits, false)
	return f.subformat(&d, negative, true), nil
}

// formatBigInt is DecimalFormat.format(BigInteger).
func (f *decimalFormat) formatBigInt(v *big.Int) string {
	negative := v.Sign() < 0
	magnitude := new(big.Int).Abs(v)
	if f.multiplier != 1 {
		magnitude.Mul(magnitude, big.NewInt(int64(f.multiplier)))
	}
	maxDigits := -1
	if f.useExponent {
		maxDigits = f.maxInt + f.maxFrac
	}
	var d digitList
	d.setString(magnitude.String(), false, true, maxDigits, false)
	return f.subformat(&d, negative, true)
}

// formatBigDecimal is DecimalFormat.format(BigDecimal), which keeps the value's full precision
// where format(double) only ever sees the digits Double.toString would print.
func (f *decimalFormat) formatBigDecimal(v *BigDecimal) string {
	negative := v.Signum() < 0
	value := v
	if f.multiplier != 1 {
		value = value.Mul(NewBigDecimalFromInt(int64(f.multiplier)))
	}
	if negative {
		value = value.Neg()
	}
	maxDigits := f.maxFrac
	if f.useExponent {
		maxDigits = f.maxInt + f.maxFrac
	}
	var d digitList
	d.setString(value.String(), false, true, maxDigits, !f.useExponent)
	return f.subformat(&d, negative, false)
}

func (f *decimalFormat) prefix(negative bool) string {
	if negative {
		return f.negPrefix
	}
	return f.posPrefix
}

func (f *decimalFormat) suffix(negative bool) string {
	if negative {
		return f.negSuffix
	}
	return f.posSuffix
}

// subformat is DecimalFormat.subformat: it lays the digits out between the affixes, either in
// scientific notation or as a grouped plain number.
func (f *decimalFormat) subformat(d *digitList, negative, isInteger bool) string {
	zeroDelta := f.sym.zero - '0'
	if d.isZero() {
		d.decimalAt = 0
	}
	var b strings.Builder
	b.WriteString(f.prefix(negative))
	if f.useExponent {
		f.subformatExponential(&b, d, zeroDelta)
	} else {
		f.subformatPlain(&b, d, zeroDelta, isInteger)
	}
	b.WriteString(f.suffix(negative))
	return b.String()
}

func (f *decimalFormat) subformatExponential(b *strings.Builder, d *digitList, zeroDelta rune) {
	exponent := d.decimalAt
	repeat := f.maxInt
	minimumIntegerDigits := f.minInt
	if repeat > 1 && repeat > f.minInt {
		// The integer part repeats in blocks of maxInt digits (engineering notation).
		if exponent >= 1 {
			exponent = ((exponent - 1) / repeat) * repeat
		} else {
			exponent = ((exponent - repeat) / repeat) * repeat
		}
		minimumIntegerDigits = 1
	} else {
		exponent -= minimumIntegerDigits
	}
	minimumDigits := f.minInt + f.minFrac
	integerDigits := d.decimalAt - exponent
	if d.isZero() {
		integerDigits = minimumIntegerDigits
	}
	totalDigits := d.count
	if totalDigits < minimumDigits {
		totalDigits = minimumDigits
	}
	if integerDigits > totalDigits {
		totalDigits = integerDigits
	}
	for i := 0; i < totalDigits; i++ {
		if i == integerDigits {
			b.WriteRune(f.sym.decimal)
		}
		if i < d.count {
			b.WriteRune(rune(d.digits[i]) + zeroDelta)
		} else {
			b.WriteRune(f.sym.zero)
		}
	}
	if f.decimalSepAlwaysShown && totalDigits == integerDigits {
		b.WriteRune(f.sym.decimal)
	}
	b.WriteString(f.sym.exponent)
	if d.isZero() {
		exponent = 0
	}
	if exponent < 0 {
		exponent = -exponent
		b.WriteString(f.sym.minus)
	}
	digits := strconv.Itoa(exponent)
	for i := len(digits); i < f.minExponentDigits; i++ {
		b.WriteRune(f.sym.zero)
	}
	for i := 0; i < len(digits); i++ {
		b.WriteRune(rune(digits[i]) + zeroDelta)
	}
}

func (f *decimalFormat) subformatPlain(b *strings.Builder, d *digitList, zeroDelta rune, isInteger bool) {
	count := f.minInt
	digitIndex := 0
	if d.decimalAt > 0 && count < d.decimalAt {
		count = d.decimalAt
	}
	if count > f.maxInt {
		count = f.maxInt
		digitIndex = d.decimalAt - count
	}
	sizeBeforeIntegerPart := b.Len()
	for i := count - 1; i >= 0; i-- {
		if i < d.decimalAt && digitIndex < d.count {
			b.WriteRune(rune(d.digits[digitIndex]) + zeroDelta)
			digitIndex++
		} else {
			b.WriteRune(f.sym.zero)
		}
		if f.groupingUsed && i > 0 && f.groupingSize != 0 && i%f.groupingSize == 0 {
			b.WriteRune(f.sym.grouping)
		}
	}
	fractionPresent := f.minFrac > 0 || (!isInteger && digitIndex < d.count)
	if !fractionPresent && b.Len() == sizeBeforeIntegerPart {
		b.WriteRune(f.sym.zero)
	}
	if f.decimalSepAlwaysShown || fractionPresent {
		b.WriteRune(f.sym.decimal)
	}
	for i := 0; i < f.maxFrac; i++ {
		if i >= f.minFrac && (isInteger || digitIndex >= d.count) {
			break
		}
		if -1-i > d.decimalAt-1 {
			// A leading zero of the fraction, before the first significant digit.
			b.WriteRune(f.sym.zero)
			continue
		}
		if !isInteger && digitIndex < d.count {
			b.WriteRune(rune(d.digits[digitIndex]) + zeroDelta)
			digitIndex++
		} else {
			b.WriteRune(f.sym.zero)
		}
	}
}
