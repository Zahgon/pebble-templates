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
	"math/big"
	"strconv"
	"strings"
)

// BigDecimal is java.math.BigDecimal: an arbitrary precision unscaled value and a scale. The
// value it denotes is unscaled * 10^-scale, so 1.00, 1.0 and 1 are three different BigDecimals
// that compare equal; the scale survives every operation and shows up in the output.
type BigDecimal struct {
	unscaled *big.Int
	scale    int32
}

// newBigDecimal builds a BigDecimal from an unscaled value and a scale, taking ownership of i.
func newBigDecimal(i *big.Int, scale int32) *BigDecimal {
	return &BigDecimal{unscaled: i, scale: scale}
}

// ParseBigDecimal is new BigDecimal(String): it keeps the scale of the literal and reports a
// NumberFormatException with the JDK's text as an error. The JDK throws some of them with no
// message at all (an empty exponent, for instance); those come back with the message "null",
// which is how Java prints a null message in a concatenation.
func ParseBigDecimal(text string) (*BigDecimal, error) {
	if text == "" {
		return nil, numberFormatError("null")
	}
	runes := []rune(text)
	i := 0
	sign := 1
	if runes[0] == '-' || runes[0] == '+' {
		if runes[0] == '-' {
			sign = -1
		}
		i = 1
	}
	var digits strings.Builder
	dot := -1
	nDigits := 0
	sawExponent := false
	for ; i < len(runes); i++ {
		r := runes[i]
		if d := javaDigit(r); d >= 0 {
			digits.WriteByte(byte('0' + d))
			nDigits++
			continue
		}
		if r == '.' {
			if dot >= 0 {
				return nil, numberFormatError("Character array contains more than one decimal point.")
			}
			dot = nDigits
			continue
		}
		if r == 'e' || r == 'E' {
			sawExponent = true
			i++
			break
		}
		return nil, numberFormatError("Character " + string(r) +
			` is neither a decimal digit number, decimal point, nor "e" notation exponential mark.`)
	}
	exp := int64(0)
	if sawExponent {
		rest := runes[i:]
		esign := int64(1)
		j := 0
		if len(rest) > 0 && (rest[0] == '-' || rest[0] == '+') {
			if rest[0] == '-' {
				esign = -1
			}
			j = 1
		}
		if j >= len(rest) {
			// An exponent mark with no digits: the JDK throws with no message at all.
			return nil, numberFormatError("null")
		}
		for ; j < len(rest); j++ {
			d := javaDigit(rest[j])
			if d < 0 {
				return nil, numberFormatError("Not a digit.")
			}
			exp = exp*10 + int64(d)
			if exp > 1<<40 {
				return nil, numberFormatError("Too many nonzero exponent digits.")
			}
		}
		exp *= esign
	}
	if nDigits == 0 {
		return nil, numberFormatError("No digits found.")
	}
	scale := int64(0)
	if dot >= 0 {
		scale = int64(nDigits - dot)
	}
	scale -= exp
	if scale > 2147483647 || scale < -2147483648 {
		return nil, numberFormatError("Scale out of range.")
	}
	unscaled, _ := new(big.Int).SetString(digits.String(), 10)
	if sign < 0 {
		unscaled.Neg(unscaled)
	}
	return newBigDecimal(unscaled, int32(scale)), nil
}

// NewBigDecimalFromFloat is BigDecimal.valueOf(double), i.e. new BigDecimal(Double.toString(d)).
// It is not new BigDecimal(double): valueOf(0.1) is exactly 0.1, not the binary value's full
// 0.1000000000000000055511151231257827021181583404541015625.
func NewBigDecimalFromFloat(f float64) *BigDecimal {
	b, err := ParseBigDecimal(DoubleString(f))
	if err != nil {
		// An infinity or a NaN: BigDecimal.valueOf hands "Infinity" or "NaN" to the string
		// constructor, which throws a NumberFormatException. There is no error to return here,
		// so the caller gets nil and has to treat it as that exception.
		return nil
	}
	return b
}

// NewBigDecimalFromInt is BigDecimal.valueOf(long).
func NewBigDecimalFromInt(i int64) *BigDecimal {
	return newBigDecimal(big.NewInt(i), 0)
}

// NewBigDecimalFromBigInt is new BigDecimal(BigInteger).
func NewBigDecimalFromBigInt(i *big.Int) *BigDecimal {
	return newBigDecimal(new(big.Int).Set(i), 0)
}

// String is BigDecimal.toString, which uses scientific notation when the adjusted exponent asks
// for it ("1E+3", "0.001", "1.5E-8").
func (b *BigDecimal) String() string {
	coeff := b.unscaled.String()
	sign := ""
	if strings.HasPrefix(coeff, "-") {
		sign, coeff = "-", coeff[1:]
	}
	if b.scale == 0 {
		return sign + coeff
	}
	adjusted := -int64(b.scale) + int64(len(coeff)) - 1
	var out strings.Builder
	out.WriteString(sign)
	if b.scale >= 0 && adjusted >= -6 {
		// Plain notation: the point falls inside or just before the coefficient.
		pad := int64(b.scale) - int64(len(coeff))
		if pad >= 0 {
			out.WriteString("0.")
			out.WriteString(strings.Repeat("0", int(pad)))
			out.WriteString(coeff)
		} else {
			// The point falls after coeffLen-scale digits.
			split := len(coeff) - int(b.scale)
			out.WriteString(coeff[:split])
			out.WriteByte('.')
			out.WriteString(coeff[split:])
		}
		return out.String()
	}
	// Scientific notation. A zero coefficient keeps the exponent of its scale ("0E-10").
	out.WriteByte(coeff[0])
	if len(coeff) > 1 {
		out.WriteByte('.')
		out.WriteString(coeff[1:])
	}
	out.WriteByte('E')
	if adjusted > 0 {
		out.WriteByte('+')
	}
	out.WriteString(strconv.FormatInt(adjusted, 10))
	return out.String()
}

// PlainString is BigDecimal.toPlainString: never an exponent, however big the scale.
func (b *BigDecimal) PlainString() string {
	coeff := b.unscaled.String()
	sign := ""
	if strings.HasPrefix(coeff, "-") {
		sign, coeff = "-", coeff[1:]
	}
	switch {
	case b.scale == 0:
		return sign + coeff
	case b.scale < 0:
		if b.unscaled.Sign() == 0 {
			return coeff
		}
		return sign + coeff + strings.Repeat("0", int(-int64(b.scale)))
	default:
		pad := int64(b.scale) - int64(len(coeff))
		if pad >= 0 {
			return sign + "0." + strings.Repeat("0", int(pad)) + coeff
		}
		split := len(coeff) - int(b.scale)
		return sign + coeff[:split] + "." + coeff[split:]
	}
}

// Float64 is doubleValue, Int64 longValue (truncating), Int32 intValue.
func (b *BigDecimal) Float64() float64 {
	// BigDecimal.doubleValue hands its own toString to Double.parseDouble, so the result is the
	// correctly rounded double and an overflow becomes an infinity.
	v, _ := ParseDouble(b.String())
	return v
}

// Int64 is BigDecimal.longValue: the fraction is truncated and a value out of range keeps its low
// order 64 bits, as Java's narrowing conversion does.
func (b *BigDecimal) Int64() int64 {
	i := b.toBigIntTruncated()
	if i.IsInt64() {
		return i.Int64()
	}
	// Two's complement low 64 bits.
	mod := new(big.Int).Lsh(big.NewInt(1), 64)
	r := new(big.Int).Mod(i, mod)
	if r.Cmp(new(big.Int).Lsh(big.NewInt(1), 63)) >= 0 {
		r.Sub(r, mod)
	}
	return r.Int64()
}

// Int32 is BigDecimal.intValue.
func (b *BigDecimal) Int32() int32 {
	return int32(uint32(b.Int64()))
}

// toBigIntTruncated is BigDecimal.toBigInteger: the value with its fraction dropped.
func (b *BigDecimal) toBigIntTruncated() *big.Int {
	switch {
	case b.scale == 0:
		return new(big.Int).Set(b.unscaled)
	case b.scale < 0:
		return new(big.Int).Mul(b.unscaled, pow10(int(-b.scale)))
	default:
		return new(big.Int).Quo(b.unscaled, pow10(int(b.scale)))
	}
}

// Scale and Unscaled expose the representation.
func (b *BigDecimal) Scale() int32 { return b.scale }

// Unscaled is unscaledValue; the caller must not modify the result.
func (b *BigDecimal) Unscaled() *big.Int { return b.unscaled }

// Precision is BigDecimal.precision: the number of digits of the unscaled value, 1 for zero.
func (b *BigDecimal) Precision() int32 {
	if b.unscaled.Sign() == 0 {
		return 1
	}
	s := b.unscaled.String()
	if s[0] == '-' {
		return int32(len(s) - 1)
	}
	return int32(len(s))
}

// align returns the two unscaled values brought to a common scale, the larger of the two.
func align(x, y *BigDecimal) (*big.Int, *big.Int, int32) {
	switch {
	case x.scale == y.scale:
		return x.unscaled, y.unscaled, x.scale
	case x.scale > y.scale:
		return x.unscaled, new(big.Int).Mul(y.unscaled, pow10(int(x.scale-y.scale))), x.scale
	default:
		return new(big.Int).Mul(x.unscaled, pow10(int(y.scale-x.scale))), y.unscaled, y.scale
	}
}

// Add, Sub, Mul are BigDecimal.add/subtract/multiply: exact, with the Java scale rules (the
// larger scale of the two for add and subtract, the sum of the scales for multiply).
func (b *BigDecimal) Add(o *BigDecimal) *BigDecimal {
	x, y, scale := align(b, o)
	return newBigDecimal(new(big.Int).Add(x, y), scale)
}

// Sub is BigDecimal.subtract.
func (b *BigDecimal) Sub(o *BigDecimal) *BigDecimal {
	x, y, scale := align(b, o)
	return newBigDecimal(new(big.Int).Sub(x, y), scale)
}

// Mul is BigDecimal.multiply.
func (b *BigDecimal) Mul(o *BigDecimal) *BigDecimal {
	return newBigDecimal(new(big.Int).Mul(b.unscaled, o.unscaled), saturateScale(int64(b.scale)+int64(o.scale)))
}

// saturateScale clamps a computed scale to the int32 range, as BigDecimal.saturateLong does.
func saturateScale(s int64) int32 {
	switch {
	case s > 2147483647:
		return 2147483647
	case s < -2147483648:
		return -2147483648
	}
	return int32(s)
}

// Div is BigDecimal.divide(BigDecimal): the exact quotient, or an ArithmeticException error when
// the quotient does not terminate.
func (b *BigDecimal) Div(o *BigDecimal) (*BigDecimal, error) {
	if o.unscaled.Sign() == 0 {
		if b.unscaled.Sign() == 0 {
			return nil, arithmeticError("Division undefined")
		}
		return nil, arithmeticError("Division by zero")
	}
	preferred := saturateScale(int64(b.scale) - int64(o.scale))
	if b.unscaled.Sign() == 0 {
		return newBigDecimal(new(big.Int), preferred), nil
	}
	q, scale, ok := exactQuotient(b, o)
	if !ok {
		return nil, arithmeticError("Non-terminating decimal expansion; no exact representable decimal result.")
	}
	if preferred > scale {
		q = new(big.Int).Mul(q, pow10(int(preferred-scale)))
		scale = preferred
	}
	// Java also sheds the trailing zeros the preferred scale does not ask for, so 1E+3 divided
	// by 1E-3 is 1E+6 and not 1000000.
	return newBigDecimal(q, scale).stripZerosTo(preferred), nil
}

// exactQuotient divides b by o exactly, reporting the smallest scale that represents the
// quotient. It fails when the quotient has a non-terminating decimal expansion, which is the case
// exactly when the reduced denominator has a prime factor other than 2 and 5.
func exactQuotient(b, o *BigDecimal) (*big.Int, int32, bool) {
	num := new(big.Int).Set(b.unscaled)
	den := new(big.Int).Set(o.unscaled)
	// this/o == (bu / ou) * 10^(os - bs)
	shift := int64(o.scale) - int64(b.scale)
	if shift >= 0 {
		num.Mul(num, pow10(int(shift)))
	} else {
		den.Mul(den, pow10(int(-shift)))
	}
	if den.Sign() < 0 {
		num.Neg(num)
		den.Neg(den)
	}
	g := new(big.Int).GCD(nil, nil, new(big.Int).Abs(num), den)
	num.Quo(num, g)
	den.Quo(den, g)
	// Strip the factors of 2 and 5 from the denominator, counting the scale they need.
	scale := int64(0)
	two, five, ten := big.NewInt(2), big.NewInt(5), big.NewInt(10)
	zero := new(big.Int)
	m := new(big.Int)
	for {
		if m.Mod(den, ten).Cmp(zero) == 0 {
			den.Quo(den, ten)
			scale++
			continue
		}
		if m.Mod(den, two).Cmp(zero) == 0 {
			den.Quo(den, two)
			num.Mul(num, five)
			scale++
			continue
		}
		if m.Mod(den, five).Cmp(zero) == 0 {
			den.Quo(den, five)
			num.Mul(num, two)
			scale++
			continue
		}
		break
	}
	if den.CmpAbs(big.NewInt(1)) != 0 {
		return nil, 0, false
	}
	q := new(big.Int).Quo(num, den)
	// A negative scale means trailing zeros the quotient does not need; Java keeps the smallest
	// scale, so strip them.
	for scale < 0 {
		q.Mul(q, ten)
		scale++
	}
	return q, saturateScale(scale), true
}

// Mod is BigDecimal.remainder(BigDecimal): this minus its integral quotient times the divisor.
func (b *BigDecimal) Mod(o *BigDecimal) (*BigDecimal, error) {
	q, err := b.divideToIntegralValue(o)
	if err != nil {
		return nil, err
	}
	return b.Sub(q.Mul(o)), nil
}

// divideToIntegralValue is BigDecimal.divideToIntegralValue: the integer part of the quotient,
// with the preferred scale this.scale - divisor.scale when the value allows it.
func (b *BigDecimal) divideToIntegralValue(o *BigDecimal) (*BigDecimal, error) {
	q, _, err := b.integralQuotient(o)
	return q, err
}

// integralQuotient is divideToIntegralValue together with the digit count of the quotient before
// it is padded out to the preferred scale, which is what the MathContext precision applies to.
func (b *BigDecimal) integralQuotient(o *BigDecimal) (*BigDecimal, int, error) {
	if o.unscaled.Sign() == 0 {
		if b.unscaled.Sign() == 0 {
			return nil, 0, arithmeticError("Division undefined")
		}
		return nil, 0, arithmeticError("Division by zero")
	}
	preferred := saturateScale(int64(b.scale) - int64(o.scale))
	num, den, _ := align(b, o)
	q := new(big.Int).Quo(num, den) // Go and Java both truncate towards zero.
	scale := int32(0)
	// Java strips trailing zeros of the integral quotient down to the preferred scale, then pads
	// with zeros if the preferred scale is larger.
	ten := big.NewInt(10)
	m := new(big.Int)
	for scale > preferred && q.Sign() != 0 && m.Mod(q, ten).Sign() == 0 {
		q.Quo(q, ten)
		scale--
	}
	digits := digitCount(q)
	if scale < preferred {
		q = new(big.Int).Mul(q, pow10(int(int64(preferred)-int64(scale))))
		scale = preferred
	}
	return newBigDecimal(q, scale), digits, nil
}

// MathContext is a java.math.MathContext with HALF_EVEN rounding, the only rounding mode Pebble
// asks for: Precision is the number of significant digits to keep, 0 meaning unlimited.
type MathContext struct {
	Precision int
}

// Decimal128 is java.math.MathContext.DECIMAL128, the context io.pebbletemplates.pebble.utils.
// OperatorUtils uses for BigDecimal multiplication, division and remainder.
var Decimal128 = MathContext{Precision: 34}

// MulContext is BigDecimal.multiply(BigDecimal, MathContext).
func (b *BigDecimal) MulContext(o *BigDecimal, mc MathContext) *BigDecimal {
	return b.Mul(o).round(mc)
}

// DivContext is BigDecimal.divide(BigDecimal, MathContext): the quotient rounded HALF_EVEN to the
// context's precision, with the trailing zeros an exact quotient does not need removed down to
// the preferred scale this.scale - divisor.scale.
func (b *BigDecimal) DivContext(o *BigDecimal, mc MathContext) (*BigDecimal, error) {
	if mc.Precision == 0 {
		return b.Div(o)
	}
	if o.unscaled.Sign() == 0 {
		if b.unscaled.Sign() == 0 {
			return nil, arithmeticError("Division undefined")
		}
		return nil, arithmeticError("Division by zero")
	}
	preferred := saturateScale(int64(b.scale) - int64(o.scale))
	if b.unscaled.Sign() == 0 {
		return newBigDecimal(new(big.Int), preferred), nil
	}
	if q, scale, ok := exactQuotient(b, o); ok {
		r := newBigDecimal(q, scale).round(mc)
		if r.scale < preferred {
			r = newBigDecimal(new(big.Int).Mul(r.unscaled, pow10(int(int64(preferred)-int64(r.scale)))), preferred)
		}
		return r.stripZerosTo(preferred), nil
	}
	// The quotient does not terminate, so produce exactly mc.Precision significant digits of it
	// and let the remainder of the division decide the HALF_EVEN rounding.
	num := new(big.Int).Set(b.unscaled)
	den := new(big.Int).Set(o.unscaled)
	if shift := int64(o.scale) - int64(b.scale); shift >= 0 {
		num.Mul(num, pow10(int(shift)))
	} else {
		den.Mul(den, pow10(int(-shift)))
	}
	if den.Sign() < 0 {
		num.Neg(num)
		den.Neg(den)
	}
	scale := int64(0)
	ten := big.NewInt(10)
	for {
		q := new(big.Int).Quo(num, den)
		if q.Sign() == 0 || digitCount(q) < mc.Precision {
			num.Mul(num, ten)
			scale++
			continue
		}
		if digitCount(q) > mc.Precision {
			den.Mul(den, ten)
			scale--
			continue
		}
		break
	}
	q := divRoundHalfEven(num, den)
	if digitCount(q) > mc.Precision {
		// 0.999 rounded to two digits is 1.0, one digit too wide; shed the zero.
		q.Quo(q, ten)
		scale--
	}
	return newBigDecimal(q, saturateScale(scale)), nil
}

// ModContext is BigDecimal.remainder(BigDecimal, MathContext).
func (b *BigDecimal) ModContext(o *BigDecimal, mc MathContext) (*BigDecimal, error) {
	q, digits, err := b.integralQuotient(o)
	if err != nil {
		return nil, err
	}
	if mc.Precision > 0 && digits > mc.Precision {
		return nil, arithmeticError("Division impossible")
	}
	return b.Sub(q.Mul(o)), nil
}

// round is MathContext rounding: keep mc.Precision significant digits, HALF_EVEN.
func (b *BigDecimal) round(mc MathContext) *BigDecimal {
	if mc.Precision <= 0 {
		return b
	}
	digits := int(b.Precision())
	if digits <= mc.Precision || b.unscaled.Sign() == 0 {
		return b
	}
	drop := digits - mc.Precision
	q := divRoundHalfEven(b.unscaled, pow10(drop))
	scale := int64(b.scale) - int64(drop)
	// Rounding 9.99 to two digits gives 10.0, one digit too many; drop the zero it ends with.
	if digitCount(q) > mc.Precision {
		q.Quo(q, big.NewInt(10))
		scale--
	}
	return newBigDecimal(q, saturateScale(scale))
}

// stripZerosTo removes trailing zeros of the unscaled value while the scale stays at or above
// target, which is BigDecimal.stripZerosToMatchScale.
func (b *BigDecimal) stripZerosTo(target int32) *BigDecimal {
	q := new(big.Int).Set(b.unscaled)
	scale := b.scale
	ten := big.NewInt(10)
	m := new(big.Int)
	for scale > target && q.Sign() != 0 && m.Mod(q, ten).Sign() == 0 {
		q.Quo(q, ten)
		scale--
	}
	return newBigDecimal(q, scale)
}

// divRoundHalfEven divides n by d (d > 0) rounding half to even, the way RoundingMode.HALF_EVEN
// does, including for a negative n (the magnitude is what rounds).
func divRoundHalfEven(n, d *big.Int) *big.Int {
	q, r := new(big.Int).QuoRem(n, d, new(big.Int))
	if r.Sign() == 0 {
		return q
	}
	twice := new(big.Int).Abs(r)
	twice.Lsh(twice, 1)
	cmp := twice.Cmp(d)
	up := cmp > 0 || (cmp == 0 && q.Bit(0) == 1)
	if up {
		if n.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return q
}

// digitCount is the number of decimal digits of the magnitude of i, 1 for zero.
func digitCount(i *big.Int) int {
	if i.Sign() == 0 {
		return 1
	}
	s := i.String()
	if s[0] == '-' {
		return len(s) - 1
	}
	return len(s)
}

// Neg is negate, Abs abs, Cmp compareTo, Signum signum, Equals the Java equals (scale included).
func (b *BigDecimal) Neg() *BigDecimal {
	return newBigDecimal(new(big.Int).Neg(b.unscaled), b.scale)
}

// Abs is BigDecimal.abs.
func (b *BigDecimal) Abs() *BigDecimal {
	return newBigDecimal(new(big.Int).Abs(b.unscaled), b.scale)
}

// Cmp is BigDecimal.compareTo: the scale does not take part, so 1.0 and 1.00 compare equal.
func (b *BigDecimal) Cmp(o *BigDecimal) int {
	if b.unscaled.Sign() != o.unscaled.Sign() {
		if b.unscaled.Sign() < o.unscaled.Sign() {
			return -1
		}
		return 1
	}
	x, y, _ := align(b, o)
	return x.Cmp(y)
}

// Signum is BigDecimal.signum.
func (b *BigDecimal) Signum() int { return b.unscaled.Sign() }

// Equals is BigDecimal.equals, which is scale sensitive: 1.0 does not equal 1.00.
func (b *BigDecimal) Equals(o *BigDecimal) bool {
	if b == nil || o == nil {
		return b == o
	}
	return b.scale == o.scale && b.unscaled.Cmp(o.unscaled) == 0
}

// HashCode is BigDecimal.hashCode. The JDK has two code paths, and they do not agree: a value
// whose unscaled part fits in a long mixes the two halves of the long, a bigger one uses
// BigInteger.hashCode. Both are reproduced because java.util.HashMap's iteration order depends
// on them.
func (b *BigDecimal) HashCode() int32 {
	if b.unscaled.IsInt64() {
		compact := b.unscaled.Int64()
		// Long.MIN_VALUE is BigDecimal's INFLATED marker and takes the BigInteger path.
		if compact != -9223372036854775808 {
			val2 := compact
			if val2 < 0 {
				val2 = -val2
			}
			temp := int32(uint32(val2>>32))*31 + int32(uint32(uint64(val2)&0xffffffff))
			if compact < 0 {
				temp = -temp
			}
			return 31*temp + b.scale
		}
	}
	return 31*bigIntHashCode(b.unscaled) + b.scale
}

// bigIntHashCode is java.math.BigInteger.hashCode: the magnitude's 32 bit words, big endian,
// folded with the usual 31 multiplier and then signed.
func bigIntHashCode(i *big.Int) int32 {
	mag := new(big.Int).Abs(i).Bytes()
	// Pad to a multiple of four bytes so the words line up the way Java's int[] magnitude does.
	if pad := len(mag) % 4; pad != 0 {
		mag = append(make([]byte, 4-pad), mag...)
	}
	var h int32
	for w := 0; w < len(mag); w += 4 {
		word := uint32(mag[w])<<24 | uint32(mag[w+1])<<16 | uint32(mag[w+2])<<8 | uint32(mag[w+3])
		h = h*31 + int32(word)
	}
	if i.Sign() < 0 {
		h = -h
	}
	return h
}
