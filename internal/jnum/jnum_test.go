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
	"testing"
)

// The cases of every table below were read off the JDK of the reference command. The exhaustive
// comparison lives in TestGolden; these are the landmarks, so that a failure says what broke.

func TestDoubleString(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0.0"},
		{math.Copysign(0, -1), "-0.0"},
		{1, "1.0"},
		{-1.5, "-1.5"},
		{0.1, "0.1"},
		{1.0 / 3.0, "0.3333333333333333"},
		{0.001, "0.001"},
		{0.0001, "1.0E-4"},
		{9999999, "9999999.0"},
		{1e7, "1.0E7"},
		{1e23, "1.0E23"},
		{100, "100.0"},
		{5e-324, "4.9E-324"},
		{math.MaxFloat64, "1.7976931348623157E308"},
		{math.NaN(), "NaN"},
		{math.Inf(1), "Infinity"},
		{math.Inf(-1), "-Infinity"},
	}
	for _, c := range cases {
		if got := DoubleString(c.in); got != c.want {
			t.Errorf("DoubleString(%g) = %q, want %q", c.in, got, c.want)
		}
	}
	floats := []struct {
		in   float32
		want string
	}{
		{0.1, "0.1"},
		{3.1, "3.1"},
		{1e-45, "1.4E-45"},
		{16777216, "1.6777216E7"},
		{-0.5, "-0.5"},
	}
	for _, c := range floats {
		if got := FloatString(c.in); got != c.want {
			t.Errorf("FloatString(%g) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		pattern, locale string
		value           any
		want            string
	}{
		{"#,##0.###", "en-US", 1234567.8915, "1,234,567.891"},
		{"#,##0.###", "de-DE", 1234567.8915, "1.234.567,891"},
		{"#,##0.###", "fr-FR", 1234567.8915, "1 234 567,891"},
		{"#,##0.###", "tr-TR", 1234567.8915, "1.234.567,891"},
		{"#,##0.###", "ja-JP", 1234567.8915, "1,234,567.891"},
		{"#,##0.00", "en-US", int64(1234567890123), "1,234,567,890,123.00"},
		{"#", "en-US", 0.5, "0"}, // HALF_EVEN
		{"#", "en-US", 1.5, "2"},
		{"#", "en-US", 2.5, "2"},
		{"#.##", "en-US", 1.005, "1"}, // the double is just below the tie
		{"#.##%", "en-US", 0.1, "10%"},
		{"0.##E0", "en-US", 1234.5678, "1.23E3"},
		{"##0.##E0", "en-US", 0.000012345, "12.345E-6"},
		{"#;(#)", "en-US", -1234.5678, "(1235)"},
		{"¤#,##0.00", "en-US", 1234.5, "$1,234.50"},
		{"#,##0.###", "en-US", math.Inf(-1), "-∞"},
		{"#,##0.###", "en-US", math.NaN(), "NaN"},
	}
	for _, c := range cases {
		got, err := Format(c.pattern, c.locale, c.value)
		if err != nil {
			t.Errorf("Format(%q, %q, %v): %v", c.pattern, c.locale, c.value, err)
			continue
		}
		if got != c.want {
			t.Errorf("Format(%q, %q, %v) = %q, want %q", c.pattern, c.locale, c.value, got, c.want)
		}
	}
	// A BigDecimal keeps its precision where the same value as a double does not.
	big1, _ := ParseBigDecimal("123456789012345678901234567890.5")
	if got, _ := Format("#", "en-US", big1); got != "123456789012345678901234567890" {
		t.Errorf("Format of a BigDecimal = %q", got)
	}
	bad := []struct{ pattern, message string }{
		{"#0.0#0", `Unexpected '0' in pattern "#0.0#0"`},
		{"#.#.#", `Multiple decimal separators in pattern "#.#.#"`},
		{"#,", `Malformed pattern "#,"`},
		{"0E", `Malformed exponential pattern "0E"`},
		{"##%%", `Too many percent/per mille characters in pattern "##%%"`},
		{"0;0;0", `Unquoted special character ';' in pattern "0;0;0"`},
	}
	for _, c := range bad {
		_, err := Format(c.pattern, "en-US", int64(1))
		if err == nil || err.Error() != c.message {
			t.Errorf("Format(%q) error = %v, want %q", c.pattern, err, c.message)
		}
	}
}

func TestBigDecimal(t *testing.T) {
	mustParse := func(s string) *BigDecimal {
		b, err := ParseBigDecimal(s)
		if err != nil {
			t.Fatalf("ParseBigDecimal(%q): %v", s, err)
		}
		return b
	}
	if got := mustParse("1.000").Add(mustParse("2.5")).String(); got != "3.500" {
		t.Errorf("add = %q, want 3.500", got)
	}
	if got := mustParse("1.5").Mul(mustParse("2.50")).String(); got != "3.750" {
		t.Errorf("multiply = %q, want 3.750", got)
	}
	if _, err := mustParse("1").Div(mustParse("3")); err == nil ||
		err.Error() != "Non-terminating decimal expansion; no exact representable decimal result." {
		t.Errorf("1/3 error = %v", err)
	}
	if _, err := mustParse("1").Div(mustParse("0")); err == nil || err.Error() != "Division by zero" {
		t.Errorf("1/0 error = %v", err)
	}
	if q, _ := mustParse("1E+3").Div(mustParse("1E-3")); q.String() != "1E+6" {
		t.Errorf("1E+3/1E-3 = %q, want 1E+6", q)
	}
	if q, _ := mustParse("1").DivContext(mustParse("3"), Decimal128); q.String() != "0.3333333333333333333333333333333333" {
		t.Errorf("1/3 in DECIMAL128 = %q", q)
	}
	if r, _ := mustParse("1.000").Mod(mustParse("0.3")); r.String() != "0.100" {
		t.Errorf("remainder = %q, want 0.100", r)
	}
	if mustParse("1.0").Equals(mustParse("1.00")) {
		t.Error("equals ignores the scale")
	}
	if mustParse("1.0").Cmp(mustParse("1.00")) != 0 {
		t.Error("compareTo must ignore the scale")
	}
	if got := mustParse("1.5E-8").PlainString(); got != "0.000000015" {
		t.Errorf("toPlainString = %q", got)
	}
	if got := NewBigDecimalFromFloat(0.0001).String(); got != "0.00010" {
		t.Errorf("valueOf(1e-4) = %q, want 0.00010", got)
	}
	if got := mustParse("1234.5000").HashCode(); got != 382695004 {
		t.Errorf("hashCode = %d, want 382695004", got)
	}
}

func TestParseAndHash(t *testing.T) {
	if _, err := ParseDouble("x"); err == nil || err.Error() != `For input string: "x"` {
		t.Errorf("ParseDouble error = %v", err)
	}
	if _, err := ParseDouble(""); err == nil || err.Error() != "empty String" {
		t.Errorf("ParseDouble(\"\") error = %v", err)
	}
	if v, err := ParseDouble(" 1d "); err != nil || v != 1 {
		t.Errorf("ParseDouble(\" 1d \") = %v, %v", v, err)
	}
	if _, err := ParseInt("2147483648"); err == nil || err.Error() != `For input string: "2147483648"` {
		t.Errorf("ParseInt error = %v", err)
	}
	if _, err := ParseBigInt("1e-400"); err == nil || err.Error() != "Illegal embedded sign character" {
		t.Errorf("ParseBigInt error = %v", err)
	}
	hashes := []struct {
		value any
		want  int32
	}{
		{int64(123456789012), -1097262584},
		{int32(-7), -7},
		{1.5, 1073217536},
		{float32(0.1), 1036831949},
		{math.Copysign(0, -1), -2147483648},
		{true, 1231},
		{false, 1237},
		{int16(-3), -3},
	}
	for _, c := range hashes {
		if got := HashCode(c.value); got != c.want {
			t.Errorf("HashCode(%v) = %d, want %d", c.value, got, c.want)
		}
	}
}
