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
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestGolden replays testdata/expected.txt, whose every line is a case and the answer the JDK of
// the reference command gave for it (see testdata/README). The cases cover Double.toString,
// Float.toString, DecimalFormat over a cross product of patterns, locales and boxed types,
// NumberFormat.getInstance, BigDecimal arithmetic and printing, the parse functions and the boxed
// hashCodes.
func TestGolden(t *testing.T) {
	file, err := os.Open("testdata/expected.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	goldenT = t
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	var failed, total int
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		if text == "" {
			continue
		}
		fields := strings.Split(text, "\t")
		want := fields[len(fields)-1]
		if fields[0] == "BADPAT" && !strings.HasPrefix(want, "!") {
			want = "ok"
		}
		got := evalCase(fields[:len(fields)-1])
		total++
		if got != want {
			failed++
			if failed <= 25 {
				t.Errorf("line %d: %s\n got %q\nwant %q", line, strings.Join(fields[:len(fields)-1], " | "), got, want)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if failed > 0 {
		t.Errorf("%d of %d golden cases disagree with the JDK", failed, total)
	}
}

// result reports a value or, for a Java exception, "!" and its message.
func result(s string, err error) string {
	if err != nil {
		return "!" + err.Error()
	}
	return s
}

// spec parses a typed value of a case: "L:" a Long, "D:" a Double by its bits, "F:" a Float by
// its bits, "BD:" a BigDecimal, "BI:" a BigInteger, "B:" a Boolean, "S:" a Short, "Y:" a Byte.
func spec(t *testing.T, s string) any {
	kind, text, _ := strings.Cut(s, ":")
	switch kind {
	case "L":
		v, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		return v
	case "D":
		bits, err := strconv.ParseUint(text[2:], 16, 64)
		if err != nil {
			t.Fatal(err)
		}
		return math.Float64frombits(bits)
	case "F":
		bits, err := strconv.ParseUint(text[2:], 16, 32)
		if err != nil {
			t.Fatal(err)
		}
		return math.Float32frombits(uint32(bits))
	case "BD":
		v, err := ParseBigDecimal(text)
		if err != nil {
			t.Fatal(err)
		}
		return v
	case "BI":
		v, err := ParseBigInt(text)
		if err != nil {
			t.Fatal(err)
		}
		return v
	case "B":
		return text == "true"
	case "S":
		v, _ := strconv.ParseInt(text, 10, 16)
		return int16(v)
	case "Y":
		v, _ := strconv.ParseInt(text, 10, 8)
		return int8(v)
	}
	t.Fatalf("unknown spec %q", s)
	return nil
}

func evalCase(fields []string) string {
	t := goldenT
	switch fields[0] {
	case "D":
		bits, _ := strconv.ParseUint(fields[1][2:], 16, 64)
		return DoubleString(math.Float64frombits(bits))
	case "F":
		bits, _ := strconv.ParseUint(fields[1][2:], 16, 32)
		return FloatString(math.Float32frombits(uint32(bits)))
	case "FMT":
		return result(Format(fields[1], fields[2], spec(t, fields[3])))
	case "DEF":
		return FormatDefault(fields[1], spec(t, fields[2]))
	case "BADPAT":
		// Only the rejections carry a message to compare; an accepted pattern is compared
		// through the FMT cases instead, since toPattern is not part of this package.
		_, err := newDecimalFormat(fields[1], symbolsFor("en-US"))
		if err != nil {
			return "!" + err.Error()
		}
		return "ok"
	case "BD":
		return evalBinaryBigDecimal(fields[1], fields[2], fields[3])
	case "BD1":
		return evalUnaryBigDecimal(fields[1], fields[2])
	case "P":
		return evalParse(fields[1], fields[2])
	case "H":
		return strconv.FormatInt(int64(HashCode(spec(t, fields[1]))), 10)
	}
	return "unknown case " + fields[0]
}

func evalBinaryBigDecimal(op, left, right string) string {
	x, err := ParseBigDecimal(left)
	if err != nil {
		return "!" + err.Error()
	}
	y, err := ParseBigDecimal(right)
	if err != nil {
		return "!" + err.Error()
	}
	switch op {
	case "add":
		return x.Add(y).String()
	case "sub":
		return x.Sub(y).String()
	case "mul":
		return x.Mul(y).String()
	case "div":
		q, err := x.Div(y)
		if err != nil {
			return "!" + err.Error()
		}
		return q.String()
	case "mod":
		q, err := x.Mod(y)
		if err != nil {
			return "!" + err.Error()
		}
		return q.String()
	case "mulmc":
		return x.MulContext(y, Decimal128).String()
	case "divmc":
		q, err := x.DivContext(y, Decimal128)
		if err != nil {
			return "!" + err.Error()
		}
		return q.String()
	case "modmc":
		q, err := x.ModContext(y, Decimal128)
		if err != nil {
			return "!" + err.Error()
		}
		return q.String()
	case "cmp":
		return strconv.Itoa(x.Cmp(y))
	case "eq":
		return strconv.FormatBool(x.Equals(y))
	case "hash":
		return strconv.FormatInt(int64(x.HashCode()), 10) + "," + strconv.FormatInt(int64(y.HashCode()), 10)
	}
	return "unknown op " + op
}

func evalUnaryBigDecimal(op, text string) string {
	x, err := ParseBigDecimal(text)
	if err != nil {
		return "!" + err.Error()
	}
	switch op {
	case "str":
		return x.String()
	case "plain":
		return x.PlainString()
	case "scale":
		return strconv.FormatInt(int64(x.Scale()), 10)
	case "unscaled":
		return x.Unscaled().String()
	case "dbl":
		return DoubleString(x.Float64())
	case "lng":
		return strconv.FormatInt(x.Int64(), 10)
	case "int":
		return strconv.FormatInt(int64(x.Int32()), 10)
	case "hash":
		return strconv.FormatInt(int64(x.HashCode()), 10)
	case "prec":
		return strconv.FormatInt(int64(x.Precision()), 10)
	case "abs":
		return x.Abs().String()
	case "neg":
		return x.Neg().String()
	case "signum":
		return strconv.Itoa(x.Signum())
	case "vd":
		v := NewBigDecimalFromFloat(x.Float64())
		if v == nil {
			// BigDecimal.valueOf of an infinity throws; this package reports it with a nil.
			return `!Character I is neither a decimal digit number, decimal point, nor "e" notation exponential mark.`
		}
		return v.String()
	}
	return "unknown op " + op
}

func evalParse(op, text string) string {
	switch op {
	case "pd":
		v, err := ParseDouble(text)
		if err != nil {
			return "!" + err.Error()
		}
		return DoubleString(v)
	case "pl":
		v, err := ParseLong(text)
		if err != nil {
			return "!" + err.Error()
		}
		return strconv.FormatInt(v, 10)
	case "pi":
		v, err := ParseInt(text)
		if err != nil {
			return "!" + err.Error()
		}
		return strconv.FormatInt(int64(v), 10)
	case "pbi":
		v, err := ParseBigInt(text)
		if err != nil {
			return "!" + err.Error()
		}
		return v.String()
	case "pbd":
		v, err := ParseBigDecimal(text)
		if err != nil {
			return "!" + err.Error()
		}
		return v.String()
	}
	return "unknown op " + op
}

// goldenT lets the case evaluators report a broken case file; TestGolden sets it.
var goldenT *testing.T
