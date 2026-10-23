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
	"math"
	"math/big"
	"strconv"
	"strings"
)

// DoubleString is Double.toString and FloatString is Float.toString: a decimal point is always
// present, the exponent form is used outside [1e-3, 1e7), "Infinity", "-Infinity" and "NaN" are
// spelled out and -0.0 keeps its sign.
func DoubleString(f float64) string {
	if s, ok := specialFloatString(f); ok {
		return s
	}
	digits, exp := shortestDigits(math.Abs(f), 64)
	return signOf(f) + layoutJavaFloat(digits, exp)
}

// FloatString is Float.toString. It asks for the shortest decimal that round-trips through a
// 32 bit float, which is usually shorter than the double one (0.1f prints as "0.1").
func FloatString(f float32) string {
	if s, ok := specialFloatString(float64(f)); ok {
		return s
	}
	digits, exp := shortestDigits(math.Abs(float64(f)), 32)
	return signOf(float64(f)) + layoutJavaFloat(digits, exp)
}

// specialFloatString reports the Java spelling of the values that have no digits.
func specialFloatString(f float64) (string, bool) {
	switch {
	case math.IsNaN(f):
		return "NaN", true
	case math.IsInf(f, 1):
		return "Infinity", true
	case math.IsInf(f, -1):
		return "-Infinity", true
	case f == 0:
		if math.Signbit(f) {
			return "-0.0", true
		}
		return "0.0", true
	}
	return "", false
}

func signOf(f float64) string {
	if math.Signbit(f) {
		return "-"
	}
	return ""
}

// shortestDigits returns the shortest decimal digit string that round-trips to the finite,
// positive value v at the given bit size, together with the decimal exponent e for which
// v == 0.<digits> * 10^e. That is the (digits, decExponent) pair of the JDK's FloatingDecimal,
// whose shortest-representation algorithm agrees with the one Go's strconv uses.
func shortestDigits(v float64, bitSize int) (string, int) {
	s := strconv.FormatFloat(v, 'e', -1, bitSize)
	e := strings.IndexByte(s, 'e')
	exp, err := strconv.Atoi(s[e+1:])
	if err != nil {
		panic("jnum: unparsable float form " + s)
	}
	digits := strings.Replace(s[:e], ".", "", 1)
	for len(digits) > 1 && digits[len(digits)-1] == '0' {
		digits = digits[:len(digits)-1]
	}
	decExp := exp + 1
	if len(digits) == 1 && v != 0 {
		// Java never settles for a single digit: where the shortest round-tripping decimal has
		// one digit, Double.toString takes the two digit decimal closest to the value instead
		// (ties to the even last digit), which is why Double.MIN_VALUE prints as 4.9E-324 and
		// not 5.0E-324. Stripping the trailing zero afterwards leaves the usual answer for
		// everything that is exactly one digit, such as 0.1 -> "1".
		trueExp := trueDecExp(v, bitSize)
		digits = twoDigitForm(v, trueExp)
		decExp = trueExp
		if digits == "100" {
			digits, decExp = "1", decExp+1
		}
		for len(digits) > 1 && digits[len(digits)-1] == '0' {
			digits = digits[:len(digits)-1]
		}
	}
	return digits, decExp
}

// trueDecExp is the decimal exponent of the value itself, the e with 0.1 <= v/10^e < 1. It is not
// always the exponent of the shortest representation: the double just below 1e-323 rounds to
// 1e-323, whose exponent is one larger.
func trueDecExp(v float64, bitSize int) int {
	precision := 17
	if bitSize == 32 {
		precision = 9
	}
	_, exp := fixedDigits(v, precision)
	return exp
}

// twoDigitForm is the two digit decimal closest to v, given that v == 0.d*10^decExp.
func twoDigitForm(v float64, decExp int) string {
	r := new(big.Rat).SetFloat64(v)
	if k := 2 - decExp; k >= 0 {
		r.Mul(r, new(big.Rat).SetInt(pow10(k)))
	} else {
		r.Quo(r, new(big.Rat).SetInt(pow10(-k)))
	}
	return divRoundHalfEven(r.Num(), r.Denom()).String()
}

// layoutJavaFloat lays the digits out the way FloatingDecimal.toJavaFormatString does: a plain
// decimal while the decimal exponent is in (-3, 8], scientific notation outside it, and always at
// least one digit on either side of the point.
func layoutJavaFloat(digits string, exp int) string {
	var b strings.Builder
	switch {
	case exp > 0 && exp < 8:
		if len(digits) <= exp {
			b.WriteString(digits)
			b.WriteString(strings.Repeat("0", exp-len(digits)))
			b.WriteString(".0")
		} else {
			b.WriteString(digits[:exp])
			b.WriteByte('.')
			b.WriteString(digits[exp:])
		}
	case exp <= 0 && exp > -3:
		b.WriteString("0.")
		b.WriteString(strings.Repeat("0", -exp))
		b.WriteString(digits)
	default:
		b.WriteByte(digits[0])
		b.WriteByte('.')
		if len(digits) > 1 {
			b.WriteString(digits[1:])
		} else {
			b.WriteByte('0')
		}
		b.WriteByte('E')
		b.WriteString(strconv.Itoa(exp - 1))
	}
	return b.String()
}

// legacyDigits is FloatingDecimal.getBinaryToASCIIConverter, the digit generator java.text.
// DigitList is fed from. It is not the one behind Double.toString: that one was replaced in JDK 19
// and now accepts a decimal that sits exactly on the boundary of the value's rounding interval,
// while this one still rejects it. The double nearest 1e23 shows the difference, printing as
// "1.0E23" but formatting through sixteen digits. The reported flags say whether the digits are
// the exact value and, if not, whether they lie above it; DigitList needs them to break a tie.
func legacyDigits(v float64) (string, int, bool, bool) {
	lo, hi := roundingInterval(v)
	shortest, _ := shortestDigits(math.Abs(v), 64)
	for p := len(shortest); p <= 17; p++ {
		digits, exp := fixedDigits(math.Abs(v), p)
		d := decimalRat(digits, exp)
		if d.Cmp(lo) > 0 && d.Cmp(hi) < 0 {
			cmp := d.Cmp(new(big.Rat).SetFloat64(math.Abs(v)))
			for len(digits) > 1 && digits[len(digits)-1] == '0' {
				digits = digits[:len(digits)-1]
			}
			return digits, exp, cmp > 0, cmp == 0
		}
	}
	digits, exp := fixedDigits(math.Abs(v), 17)
	return digits, exp, false, false
}

// fixedDigits is the p digit decimal closest to the positive value v, with the decimal exponent
// for which v == 0.<digits>*10^exp.
func fixedDigits(v float64, p int) (string, int) {
	s := strconv.FormatFloat(v, 'e', p-1, 64)
	e := strings.IndexByte(s, 'e')
	exp, err := strconv.Atoi(s[e+1:])
	if err != nil {
		panic("jnum: unparsable float form " + s)
	}
	return strings.Replace(s[:e], ".", "", 1), exp + 1
}

// roundingInterval is the open interval of reals that round to the double v: the midpoints
// between v and its two neighbours.
func roundingInterval(v float64) (*big.Rat, *big.Rat) {
	a := math.Abs(v)
	prev := math.Nextafter(a, 0)
	next := math.Nextafter(a, math.Inf(1))
	exact := new(big.Rat).SetFloat64(a)
	half := big.NewRat(1, 2)
	lo := new(big.Rat).Mul(new(big.Rat).Add(new(big.Rat).SetFloat64(prev), exact), half)
	var hi *big.Rat
	if math.IsInf(next, 1) {
		// The largest finite double has no upper neighbour; its gap above matches the one below.
		hi = new(big.Rat).Add(exact, new(big.Rat).Sub(exact, lo))
	} else {
		hi = new(big.Rat).Mul(new(big.Rat).Add(exact, new(big.Rat).SetFloat64(next)), half)
	}
	return lo, hi
}

// decimalRat is the rational 0.<digits>*10^exp.
func decimalRat(digits string, exp int) *big.Rat {
	num, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		panic("jnum: bad digits " + digits)
	}
	d := new(big.Rat).SetInt(num)
	if p := exp - len(digits); p >= 0 {
		d.Mul(d, new(big.Rat).SetInt(pow10(p)))
	} else {
		d.Quo(d, new(big.Rat).SetInt(pow10(-p)))
	}
	return d
}

// decimalRatCmp compares the decimal 0.<digits>*10^exp with the exact value of the double v.
// java.text.DigitList needs to know whether the shortest decimal representation of a double is
// exact, and if not whether it lies above or below the double, to break a rounding tie the same
// way the JDK does (see shouldRoundUp).
func decimalRatCmp(digits string, exp int, v float64) int {
	return decimalRat(digits, exp).Cmp(new(big.Rat).SetFloat64(v))
}

// pow10 is 10^n as a big integer, for n >= 0.
func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}
