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
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// JavaError is a Java runtime exception the JDK's number code throws. Error reports the Java
// message, which is what Pebble puts in its own messages; Class is the exception's simple name,
// for a caller that reproduces a stack trace or an exception type test.
type JavaError struct {
	Class   string
	Message string
}

func (e *JavaError) Error() string { return e.Message }

func numberFormatError(msg string) error {
	return &JavaError{Class: "NumberFormatException", Message: msg}
}

func illegalArgumentError(msg string) error {
	return &JavaError{Class: "IllegalArgumentException", Message: msg}
}

func arithmeticError(msg string) error {
	return &JavaError{Class: "ArithmeticException", Message: msg}
}

// forInputString is the message of most of the JDK's NumberFormatExceptions.
func forInputString(s string) error {
	return numberFormatError(`For input string: "` + s + `"`)
}

// javaDoubleRe is the grammar Double.valueOf documents: a decimal or hexadecimal floating point
// literal with an optional type suffix, or NaN or Infinity.
var javaDoubleRe = regexp.MustCompile(`^[+-]?(NaN|Infinity|` +
	`(([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?|` +
	`0[xX](([0-9a-fA-F]+(\.[0-9a-fA-F]*)?)|(\.[0-9a-fA-F]+))[pP][+-]?[0-9]+)[fFdD]?)$`)

// ParseDouble is Double.parseDouble. Java trims the ASCII control characters and spaces around
// the text, accepts a trailing type suffix and a hexadecimal literal, and overflows to infinity
// instead of failing.
func ParseDouble(text string) (float64, error) {
	s := strings.Trim(text, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\t\n\x0b\x0c\r\x0e\x0f"+
		"\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f ")
	if s == "" {
		return 0, numberFormatError("empty String")
	}
	if !javaDoubleRe.MatchString(s) {
		// FloatingDecimal reports a second decimal point before it reports the string.
		if strings.Count(s, ".") > 1 && strings.Trim(s, "+-.0123456789") == "" {
			return 0, numberFormatError("multiple points")
		}
		return 0, forInputString(text)
	}
	body := strings.TrimRight(s, "fFdD")
	switch {
	case strings.HasSuffix(body, "NaN"):
		return strconv.ParseFloat("NaN", 64)
	case strings.HasSuffix(body, "Infinity"):
		if body[0] == '-' {
			return strconv.ParseFloat("-Inf", 64)
		}
		return strconv.ParseFloat("+Inf", 64)
	}
	v, err := strconv.ParseFloat(body, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			// Java saturates to infinity and flushes to zero without complaining.
			return v, nil
		}
		return 0, forInputString(text)
	}
	return v, nil
}

// ParseLong is Long.parseLong and ParseInt is Integer.parseInt. Both accept any Unicode decimal
// digit, as Character.digit does, and reject everything else with the one message the JDK uses
// for a malformed string and for an out of range value alike.
func ParseLong(text string) (int64, error) {
	return parseIntegral(text, 64)
}

// ParseInt is Integer.parseInt.
func ParseInt(text string) (int32, error) {
	v, err := parseIntegral(text, 32)
	return int32(v), err
}

func parseIntegral(text string, bits int) (int64, error) {
	digits, negative, err := javaDigitString(text)
	if err != nil {
		return 0, err
	}
	mag, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return 0, forInputString(text)
	}
	if negative {
		mag.Neg(mag)
	}
	if !mag.IsInt64() {
		return 0, forInputString(text)
	}
	v := mag.Int64()
	if bits == 32 && (v < -2147483648 || v > 2147483647) {
		return 0, forInputString(text)
	}
	return v, nil
}

// javaDigitString translates the Unicode digits of an integer literal to ASCII ones and reports
// the sign, failing exactly where Integer.parseInt does.
func javaDigitString(text string) (string, bool, error) {
	if text == "" {
		return "", false, forInputString(text)
	}
	runes := []rune(text)
	negative := false
	i := 0
	switch runes[0] {
	case '-':
		negative = true
		i = 1
	case '+':
		i = 1
	}
	if i >= len(runes) {
		return "", false, forInputString(text)
	}
	var b strings.Builder
	for ; i < len(runes); i++ {
		d := javaDigit(runes[i])
		if d < 0 {
			return "", false, forInputString(text)
		}
		b.WriteByte(byte('0' + d))
	}
	return b.String(), negative, nil
}

// javaDigit is Character.digit(c, 10): the value of any Unicode decimal digit, or -1.
func javaDigit(r rune) int {
	if r >= '0' && r <= '9' {
		return int(r - '0')
	}
	if !unicode.IsDigit(r) {
		return -1
	}
	for _, rng := range unicode.Nd.R16 {
		if rng.Stride == 1 && rune(rng.Lo) <= r && r <= rune(rng.Hi) {
			return int(r-rune(rng.Lo)) % 10
		}
	}
	for _, rng := range unicode.Nd.R32 {
		if rng.Stride == 1 && rune(rng.Lo) <= r && r <= rune(rng.Hi) {
			return int(r-rune(rng.Lo)) % 10
		}
	}
	return -1
}

// ParseBigInt is new BigInteger(String). Its messages are the JDK's: a sign anywhere but at the
// front is an "Illegal embedded sign character", and an illegal digit names the nine digit group
// the JDK was parsing rather than the whole string.
func ParseBigInt(text string) (*big.Int, error) {
	if text == "" {
		return nil, numberFormatError("Zero length BigInteger")
	}
	runes := []rune(text)
	minus := lastIndexOfRune(runes, '-')
	plus := lastIndexOfRune(runes, '+')
	sign := 1
	cursor := 0
	if minus >= 0 {
		if minus != 0 || plus >= 0 {
			return nil, numberFormatError("Illegal embedded sign character")
		}
		sign, cursor = -1, 1
	} else if plus >= 0 {
		if plus != 0 {
			return nil, numberFormatError("Illegal embedded sign character")
		}
		cursor = 1
	}
	if cursor == len(runes) {
		return nil, numberFormatError("Zero length BigInteger")
	}
	for cursor < len(runes) && javaDigit(runes[cursor]) == 0 {
		cursor++
	}
	if cursor == len(runes) {
		return new(big.Int), nil
	}
	// BigInteger parses nine digits at a time with Integer.parseInt, so a bad character is
	// reported with its group as the input string.
	const digitsPerInt = 9
	first := (len(runes) - cursor) % digitsPerInt
	if first == 0 {
		first = digitsPerInt
	}
	value := new(big.Int)
	group := big.NewInt(1000000000)
	for n := first; cursor < len(runes); n = digitsPerInt {
		part := string(runes[cursor : cursor+n])
		cursor += n
		var d strings.Builder
		for _, r := range part {
			v := javaDigit(r)
			if v < 0 {
				return nil, forInputString(part)
			}
			d.WriteByte(byte('0' + v))
		}
		chunk, _ := new(big.Int).SetString(d.String(), 10)
		if n == digitsPerInt {
			value.Mul(value, group)
		} else {
			value.Mul(value, pow10(n))
		}
		value.Add(value, chunk)
	}
	if sign < 0 {
		value.Neg(value)
	}
	return value, nil
}

func lastIndexOfRune(runes []rune, r rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == r {
			return i
		}
	}
	return -1
}
