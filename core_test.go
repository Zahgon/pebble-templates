/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package pebble

import (
	"strings"
	"testing"

	"github.com/PebbleTemplates/pebble/internal/jnum"
)

// testCtx is the minimum of Ctx the core extension needs. The reference command renders with the
// en-US locale and UTC.
type testCtx struct {
	strict   bool
	escaping string
}

func (c testCtx) Locale() string               { return "en-US" }
func (c testCtx) Zone() string                 { return "UTC" }
func (c testCtx) Strict() bool                 { return c.strict }
func (c testCtx) Name() string                 { return "i.peb" }
func (c testCtx) Escaping() (bool, string)     { return true, c.escaping }
func (c testCtx) Render(*Stmt) (string, error) { return "", nil }
func (c testCtx) Lookup(string) (any, bool, error) {
	return nil, false, nil
}
func (c testCtx) Eval(*Expr) (any, error) { return nil, nil }
func (c testCtx) GetAttribute(any, any, *Arguments, int) (any, error) {
	return nil, nil
}

var ctx Ctx = testCtx{}

// checkRuntime groups assertions that exercise the Java-compatible runtime helper packages.
func checkRuntime(t *testing.T, run func()) {
	t.Helper()
	run()
}

func list(items ...any) *List { return &List{Items: items} }

func linkedMap(pairs ...any) *Map {
	m := &Map{Vals: map[any]any{}}
	for i := 0; i < len(pairs); i += 2 {
		m.Keys = append(m.Keys, pairs[i])
		m.Vals[pairs[i]] = pairs[i+1]
	}
	return m
}

func TestApplyBinaryArithmetic(t *testing.T) {
	cases := []struct {
		op    string
		left  any
		right any
		want  any
	}{
		// A Long stays a long and overflows there.
		{"+", int64(9223372036854775807), int64(1), int64(-9223372036854775808)},
		// Two Integers are added as ints but the Java method reports a long.
		{"+", int32(2147483647), int32(1), int64(-2147483648)},
		{"+", int64(2147483647), int64(1), int64(2147483648)},
		// Any double makes the operation a double one.
		{"+", float64(1), int64(1), float64(2)},
		{"-", int64(1), int64(3), int64(-2)},
		{"*", int64(6), int64(7), int64(42)},
		// Integer division truncates towards zero and the modulus keeps the dividend's sign.
		{"/", int64(-7), int64(2), int64(-3)},
		{"%", int64(-7), int64(3), int64(-1)},
		{"%", int64(7), int64(-3), int64(1)},
		// A string operand of + concatenates rather than adds.
		{"+", "5", int64(5), "55"},
		{"+", int64(5), "5", "55"},
		{"~", int64(1), int64(2), "12"},
		{"~", nil, int64(2), "2"},
		{"==", int64(1), float64(1), true},
		{"==", "1", int64(1), false},
		{"==", nil, nil, true},
		{"!=", int64(1), int64(2), true},
		{">", nil, int64(1), false},
		{"<", int64(1), int64(2), true},
		{">=", int64(2), int64(2), true},
		{"contains", list(int64(1), int64(2)), int64(1), true},
		{"contains", list(int64(1)), list(int64(1)), true},
		{"contains", nil, int64(1), false},
		{"and", true, false, false},
		{"or", false, true, true},
		{"or", int64(0), "a", true},
	}
	for _, c := range cases {
		got, err := ApplyBinary(c.op, c.left, c.right, ctx, 1)
		if err != nil {
			t.Errorf("%v %s %v: %v", c.left, c.op, c.right, err)
			continue
		}
		if got != c.want {
			t.Errorf("%v %s %v = %#v, want %#v", c.left, c.op, c.right, got, c.want)
		}
	}
}

func TestApplyBinaryDoubleDivision(t *testing.T) {
	// A double division by zero is an infinity rather than a failure.
	got, err := ApplyBinary("/", float64(1), int64(0), ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := got.(float64); !ok || f <= 0 || f*0 == 0 {
		t.Errorf("1.0 / 0 = %#v, want +Inf", got)
	}
}

func TestApplyBinaryErrors(t *testing.T) {
	cases := []struct {
		op    string
		left  any
		right any
		want  string
		cause string
	}{
		{"/", int64(1), int64(0), "Could not perform division (i.peb:1)", "/ by zero"},
		{"%", int64(1), int64(0), "Could not perform greater modulus (i.peb:1)", "/ by zero"},
		{"+", true, int64(1), "Could not perform addition (i.peb:1)",
			"Cannot convert to Number: true"},
		{"+", nil, int64(1), "Could not perform addition (i.peb:1)",
			"Cannot convert to Number: null"},
		{">", "a", "b", "Could not perform greater than comparison (i.peb:1)",
			"invalid operands for mathematical comparison [GREATER_THAN]"},
		{"<", "a", "b", "Could not perform greater modulus (i.peb:1)",
			"invalid operands for mathematical comparison [LESS_THAN]"},
		{"contains", int64(5), int64(1),
			"Contains operator can only be used on Collections, Maps and arrays. " +
				"Actual type was: java.lang.Long (i.peb:1)", ""},
	}
	for _, c := range cases {
		_, err := ApplyBinary(c.op, c.left, c.right, ctx, 1)
		if err == nil {
			t.Errorf("%v %s %v: no error", c.left, c.op, c.right)
			continue
		}
		if err.Error() != c.want {
			t.Errorf("%v %s %v: %q, want %q", c.left, c.op, c.right, err.Error(), c.want)
		}
		if c.cause == "" {
			continue
		}
		cause, ok := err.(*Error)
		if !ok || cause.Cause == nil || cause.Cause.Error() != c.cause {
			t.Errorf("%v %s %v: cause %v, want %q", c.left, c.op, c.right, cause, c.cause)
		}
	}
}

func TestApplyBinaryLists(t *testing.T) {
	sum, err := ApplyBinary("+", list(int64(1), int64(2)), int64(3), ctx, 1)
	if err != nil || ToStr(sum) != "[1, 2, 3]" {
		t.Errorf("[1, 2] + 3 = %v (%v)", ToStr(sum), err)
	}
	rest, err := ApplyBinary("-", list(int64(1), int64(2)), int64(1), ctx, 1)
	if err != nil || ToStr(rest) != "[2]" {
		t.Errorf("[1, 2] - 1 = %v (%v)", ToStr(rest), err)
	}
}

func TestApplyUnary(t *testing.T) {
	cases := []struct {
		op    string
		value any
		want  any
	}{
		{"not", int64(0), true},
		{"not", int64(1), false},
		{"not", "", true},
		{"not", "a", false},
		{"not", nil, true},
		{"not", true, false},
		{"-", int64(5), int64(-5)},
		{"-", float64(5), float64(-5)},
		{"+", int64(5), int64(5)},
	}
	for _, c := range cases {
		got, err := ApplyUnary(c.op, c.value, ctx, 1)
		if err != nil {
			t.Errorf("%s %v: %v", c.op, c.value, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s %v = %#v, want %#v", c.op, c.value, got, c.want)
		}
	}
	// A list is none of Boolean, String or Number.
	_, err := ApplyUnary("not", list(), ctx, 1)
	want := "Unsupported value type ArrayList. Expected Boolean, String, Number in \"if\" statement (i.peb:1)"
	if err == nil || err.Error() != want {
		t.Errorf("not []: %v, want %q", err, want)
	}
	// With strict variables a null operand is refused instead.
	_, err = ApplyUnary("not", nil, testCtx{strict: true}, 1)
	if err == nil || !strings.Contains(err.Error(), "null value given to not()") {
		t.Errorf("not null under strict variables: %v", err)
	}
}

func TestToStr(t *testing.T) {
	cases := []struct {
		value any
		want  string
	}{
		{nil, ""},
		{int64(10), "10"},
		{int32(10), "10"},
		{true, "true"},
		{"a", "a"},
		{SafeString("<b>"), "<b>"},
		{Char('a'), "a"},
		{list(int64(1), list(int64(2)), linkedMap("a", int64(1))), "[1, [2], {a=1}]"},
		{linkedMap("a", int64(1), "b", list(int64(1))), "{a=1, b=[1]}"},
	}
	for _, c := range cases {
		if got := ToStr(c.value); got != c.want {
			t.Errorf("ToStr(%#v) = %q, want %q", c.value, got, c.want)
		}
	}
}

func TestTruthy(t *testing.T) {
	cases := []struct {
		value any
		want  bool
	}{
		{nil, false},
		{true, true},
		{false, false},
		{int64(0), false},
		{int64(1), true},
		{float64(0), false},
		{"", false},
		{"a", true},
	}
	for _, c := range cases {
		if got := Truthy(c.value); got != c.want {
			t.Errorf("Truthy(%#v) = %v", c.value, got)
		}
	}
	if _, ok := BoolCast(list()); ok {
		t.Error("BoolCast of a list should report that Java could not cast it")
	}
}

func TestCoreTests(t *testing.T) {
	cases := []struct {
		name  string
		input any
		want  bool
	}{
		{"even", int64(2), true},
		{"even", int64(1), false},
		{"odd", int64(1), true},
		{"iterable", list(), true},
		{"iterable", linkedMap(), false},
		{"iterable", "a", false},
		{"map", linkedMap(), true},
		{"map", list(), false},
		{"null", nil, true},
		{"defined", nil, false},
		{"defined", int64(1), true},
		{"empty", nil, true},
		{"empty", list(), true},
		{"empty", linkedMap(), true},
		{"empty", int64(0), false},
	}
	for _, c := range cases {
		got, err := Tests[c.name].Apply(c.input, nil, ctx, 1)
		if err != nil {
			t.Errorf("%v is %s: %v", c.input, c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%v is %s = %v", c.input, c.name, got)
		}
	}
	if _, err := Tests["even"].Apply(nil, nil, ctx, 1); err == nil ||
		err.Error() != "Can not pass null value to \"even\" test. (i.peb:1)" {
		t.Errorf("null is even: %v", err)
	}
	if _, err := Tests["odd"].Apply(nil, nil, ctx, 1); err == nil ||
		err.Error() != "Can not pass null value to \"odd\" test." {
		t.Errorf("null is odd: %v", err)
	}
}

func TestRangeFunction(t *testing.T) {
	cases := []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"start": int64(1), "end": int64(3)}, "[1, 2, 3]"},
		{map[string]any{"start": int64(3), "end": int64(1), "increment": int64(-1)}, "[3, 2, 1]"},
		// A fractional increment is truncated to a long.
		{map[string]any{"start": int64(1), "end": int64(3), "increment": float64(1.9)}, "[1, 2, 3]"},
	}
	for _, c := range cases {
		got, err := Functions["range"].Apply(c.args, ctx, 1)
		if err != nil {
			t.Errorf("range(%v): %v", c.args, err)
			continue
		}
		if ToStr(got) != c.want {
			t.Errorf("range(%v) = %v, want %v", c.args, ToStr(got), c.want)
		}
	}
	_, err := Functions["range"].Apply(
		map[string]any{"start": int64(1), "end": int64(3), "increment": int64(0)}, ctx, 1)
	if err == nil || err.Error() !=
		"The increment of the range function must be different than 0 (i.peb:1)" {
		t.Errorf("range with a zero increment: %v", err)
	}
	_, err = Functions["range"].Apply(map[string]any{"start": int64(1), "end": "a"}, ctx, 1)
	if err == nil || err.Error() !=
		"Arguments of range function must be of type Number or String with a length of 1 (i.peb:1)" {
		t.Errorf("range(1, \"a\"): %v", err)
	}
	checkRuntime(t, func() {
		chars, err := Functions["range"].Apply(map[string]any{"start": "a", "end": "c"}, ctx, 1)
		if err != nil || ToStr(chars) != "[a, b, c]" {
			t.Errorf("range(\"a\", \"c\") = %v (%v)", ToStr(chars), err)
		}
	})
}

func TestMinMaxFunctions(t *testing.T) {
	got, err := Functions["max"].Apply(map[string]any{"0": int64(1), "1": float64(2.5)}, ctx, 1)
	if err != nil || got != float64(2.5) {
		t.Errorf("max(1, 2.5) = %#v (%v)", got, err)
	}
	got, err = Functions["min"].Apply(map[string]any{"0": int64(1), "1": int64(2)}, ctx, 1)
	if err != nil || got != int64(1) {
		t.Errorf("min(1, 2) = %#v (%v)", got, err)
	}
	// No arguments at all is a null.
	if got, err = Functions["max"].Apply(map[string]any{}, ctx, 1); err != nil || got != nil {
		t.Errorf("max() = %#v (%v)", got, err)
	}
	_, err = Functions["max"].Apply(map[string]any{"0": "a", "1": "b"}, ctx, 1)
	if err == nil || err.Error() != "invalid operands for mathematical comparison [GREATER_THAN]" {
		t.Errorf("max(\"a\", \"b\"): %v", err)
	}
}

func TestSortFilters(t *testing.T) {
	items := list(int64(3), int64(1), int64(2))
	got, err := Filters["sort"].Apply(items, nil, ctx, 1)
	if err != nil || ToStr(got) != "[1, 2, 3]" {
		t.Fatalf("sort = %v (%v)", ToStr(got), err)
	}
	// Collections.sort works on the list itself.
	if ToStr(items) != "[1, 2, 3]" {
		t.Errorf("sort did not sort the input list: %v", ToStr(items))
	}
	got, err = Filters["rsort"].Apply(list(int64(1), int64(3), int64(2)), nil, ctx, 1)
	if err != nil || ToStr(got) != "[3, 2, 1]" {
		t.Errorf("rsort = %v (%v)", ToStr(got), err)
	}
	got, err = Filters["reverse"].Apply(list(int64(1), int64(2)), nil, ctx, 1)
	if err != nil || ToStr(got) != "[2, 1]" {
		t.Errorf("reverse = %v (%v)", ToStr(got), err)
	}
	// A list of mixed types fails on the first comparison the sort makes, and which operand is
	// the receiver of compareTo differs between sort and rsort.
	_, err = Filters["sort"].Apply(list(int64(3), "a"), nil, ctx, 1)
	want := "class java.lang.Long cannot be cast to class java.lang.String " +
		"(java.lang.Long and java.lang.String are in module java.base of loader 'bootstrap')"
	if err == nil || err.Error() != want {
		t.Errorf("sort of a mixed list: %v", err)
	}
	_, err = Filters["rsort"].Apply(list(int64(3), "a"), nil, ctx, 1)
	want = "class java.lang.String cannot be cast to class java.lang.Long " +
		"(java.lang.String and java.lang.Long are in module java.base of loader 'bootstrap')"
	if err == nil || err.Error() != want {
		t.Errorf("rsort of a mixed list: %v", err)
	}
	_, err = Filters["sort"].Apply(list(list(int64(1)), list(int64(2))), nil, ctx, 1)
	if err == nil || !strings.Contains(err.Error(), "cannot be cast to class java.lang.Comparable") {
		t.Errorf("sort of a list of lists: %v", err)
	}
	_, err = Filters["sort"].Apply("abc", nil, ctx, 1)
	if err == nil || err.Error() != "Unsupported input type for sort filter (i.peb:1)" {
		t.Errorf("sort of a string: %v", err)
	}
}

func TestSliceFilter(t *testing.T) {
	got, err := Filters["slice"].Apply(list(int64(1), int64(2), int64(3)),
		map[string]any{"fromIndex": int64(1)}, ctx, 1)
	if err != nil || ToStr(got) != "[2, 3]" {
		t.Errorf("slice(1) = %v (%v)", ToStr(got), err)
	}
	cases := []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"fromIndex": int64(0), "toIndex": int64(5)},
			"toIndex must be smaller than input size: 2 (i.peb:1)"},
		{map[string]any{"fromIndex": int64(2), "toIndex": int64(2)},
			"toIndex must be greater than fromIndex (i.peb:1)"},
		{map[string]any{"fromIndex": int64(-1), "toIndex": int64(1)},
			"fromIndex must be greater than 0"},
		{map[string]any{"fromIndex": "a", "toIndex": int64(1)},
			"Argument fromIndex must be a number. Actual type: java.lang.String (i.peb:1)"},
	}
	for _, c := range cases {
		_, err := Filters["slice"].Apply(list(int64(1), int64(2)), c.args, ctx, 1)
		if err == nil || err.Error() != c.want {
			t.Errorf("slice(%v): %v, want %q", c.args, err, c.want)
		}
	}
	_, err = Filters["slice"].Apply(int64(5), map[string]any{"fromIndex": int64(0)}, ctx, 1)
	want := "Slice filter can only be applied to String, List and array inputs. " +
		"Actual type was: java.lang.Long (i.peb:1)"
	if err == nil || err.Error() != want {
		t.Errorf("slice of a Long: %v", err)
	}
}

func TestMergeFilter(t *testing.T) {
	got, err := Filters["merge"].Apply(list(int64(1)),
		map[string]any{"items": list(int64(2))}, ctx, 1)
	if err != nil || ToStr(got) != "[1, 2]" {
		t.Errorf("[1]|merge([2]) = %v (%v)", ToStr(got), err)
	}
	// A map merged into a list contributes its entries, which print as "key=value".
	got, err = Filters["merge"].Apply(list(int64(1)),
		map[string]any{"items": linkedMap("a", int64(2))}, ctx, 1)
	if err != nil || ToStr(got) != "[1, a=2]" {
		t.Errorf("[1]|merge({a:2}) = %v (%v)", ToStr(got), err)
	}
	_, err = Filters["merge"].Apply(list(int64(1)), map[string]any{"items": "x"}, ctx, 1)
	want := "Currently, only Maps and Lists can be merged with a List. " +
		"Arg2: java.lang.String (i.peb:1)"
	if err == nil || err.Error() != want {
		t.Errorf("[1]|merge(\"x\"): %v", err)
	}
	_, err = Filters["merge"].Apply(nil, map[string]any{"items": nil}, ctx, 1)
	if err == nil || err.Error() != "The two arguments to be merged are null (i.peb:1)" {
		t.Errorf("null|merge(null): %v", err)
	}
	// The left hand side alone is the result when the argument is null.
	got, err = Filters["merge"].Apply(list(int64(1)), map[string]any{"items": nil}, ctx, 1)
	if err != nil || ToStr(got) != "[1]" {
		t.Errorf("[1]|merge(null) = %v (%v)", ToStr(got), err)
	}
	checkRuntime(t, func() {
		// Two maps merge into a java.util.HashMap, which iterates in bucket order.
		got, err := Filters["merge"].Apply(linkedMap("a", int64(1)),
			map[string]any{"items": linkedMap("b", int64(2))}, ctx, 1)
		if err != nil || ToStr(got) != "{a=1, b=2}" {
			t.Errorf("{a:1}|merge({b:2}) = %v (%v)", ToStr(got), err)
		}
	})
}

func TestMiscFilters(t *testing.T) {
	// length reports an Integer and falls back to zero for a value with no length.
	cases := []struct {
		input any
		want  any
	}{
		{nil, int32(0)},
		{list(int64(1), int64(2)), int32(2)},
		{linkedMap("a", int64(1)), int32(1)},
		{int64(5), int32(0)},
		{true, int32(0)},
	}
	for _, c := range cases {
		got, err := Filters["length"].Apply(c.input, nil, ctx, 1)
		if err != nil || got != c.want {
			t.Errorf("length of %#v = %#v (%v)", c.input, got, err)
		}
	}
	// abs keeps the type of its input.
	for _, c := range []struct{ in, want any }{
		{int64(-5), int64(5)}, {int32(-5), int32(5)}, {float64(-5.5), float64(5.5)},
	} {
		got, err := Filters["abs"].Apply(c.in, nil, ctx, 1)
		if err != nil || got != c.want {
			t.Errorf("abs of %#v = %#v (%v)", c.in, got, err)
		}
	}
	if _, err := Filters["abs"].Apply(nil, nil, ctx, 1); err == nil ||
		err.Error() != "Can not pass null value to \"abs\" filter. (i.peb:1)" {
		t.Errorf("abs of null: %v", err)
	}
	if _, err := Filters["abs"].Apply("x", nil, ctx, 1); err == nil ||
		err.Error() != "The 'abs' filter does require as input a number. (i.peb:1)" {
		t.Errorf("abs of a string: %v", err)
	}
	// join uses String.valueOf of each element and no separator when none is given.
	got, err := Filters["join"].Apply(list(int64(1), int64(2)), map[string]any{}, ctx, 1)
	if err != nil || got != "12" {
		t.Errorf("join = %#v (%v)", got, err)
	}
	got, err = Filters["join"].Apply(list(int64(1), int64(2)),
		map[string]any{"separator": ","}, ctx, 1)
	if err != nil || got != "1,2" {
		t.Errorf("join(\",\") = %#v (%v)", got, err)
	}
	if _, err := Filters["join"].Apply(linkedMap(), map[string]any{}, ctx, 1); err == nil ||
		err.Error() != "The 'join' filter expects that the input is either a collection "+
			"or an array. (i.peb:1)" {
		t.Errorf("join of a map: %v", err)
	}
	// first and last on a map are a cast failure, as a Map is not a Collection.
	if _, err := Filters["first"].Apply(linkedMap("a", int64(1)), nil, ctx, 1); err == nil ||
		!strings.Contains(err.Error(), "cannot be cast to class java.util.Collection") {
		t.Errorf("first of a map: %v", err)
	}
	got, err = Filters["first"].Apply(list(), nil, ctx, 1)
	if err != nil || got != nil {
		t.Errorf("first of an empty list = %#v (%v)", got, err)
	}
	got, err = Filters["last"].Apply(list(int64(1), int64(2)), nil, ctx, 1)
	if err != nil || got != int64(2) {
		t.Errorf("last = %#v (%v)", got, err)
	}
	// default replaces anything the empty test considers empty, but not a zero.
	got, err = Filters["default"].Apply(nil, map[string]any{"default": "x"}, ctx, 1)
	if err != nil || got != "x" {
		t.Errorf("null|default = %#v (%v)", got, err)
	}
	got, err = Filters["default"].Apply(int64(0), map[string]any{"default": "x"}, ctx, 1)
	if err != nil || got != int64(0) {
		t.Errorf("0|default = %#v (%v)", got, err)
	}
	// raw declares its input safe; the escape filter lets a SafeString through untouched.
	got, err = Filters["raw"].Apply("<b>", nil, ctx, 1)
	if err != nil || got != SafeString("<b>") {
		t.Errorf("raw = %#v (%v)", got, err)
	}
	got, err = Filters["escape"].Apply(SafeString("<b>"), nil, ctx, 1)
	if err != nil || got != SafeString("<b>") {
		t.Errorf("escape of a SafeString = %#v (%v)", got, err)
	}
	if _, err := Filters["escape"].Apply("x",
		map[string]any{"strategy": "bogus"}, ctx, 1); err == nil ||
		err.Error() != "Unknown escaping strategy [bogus] (i.peb:1)" {
		t.Errorf("escape with an unknown strategy: %v", err)
	}
	// Every filter but a handful reports null for a null input.
	for _, name := range []string{"abbreviate", "capitalize", "date", "first", "format", "join",
		"last", "lower", "numberformat", "slice", "sort", "rsort", "reverse", "title", "trim",
		"upper", "urlencode", "replace", "split", "base64encode", "base64decode", "sha256",
		"nl2br", "escape", "raw"} {
		got, err := Filters[name].Apply(nil, map[string]any{}, ctx, 1)
		if err != nil || got != nil {
			t.Errorf("%s of null = %#v (%v)", name, got, err)
		}
	}
	// nl2br is the only filter that refuses a non-String outright.
	if _, err := Filters["nl2br"].Apply(int64(5), nil, ctx, 1); err == nil ||
		err.Error() != "nl2br filters only supports String input." {
		t.Errorf("nl2br of a Long: %v", err)
	}
	got, err = Filters["nl2br"].Apply("a\r\nb\nc", nil, ctx, 1)
	if err != nil || got != "a<br />b<br />c" {
		t.Errorf("nl2br = %#v (%v)", got, err)
	}
}

func TestEscapeJSON(t *testing.T) {
	// Verified against the reference command: the ampersand and every non-ASCII code unit become
	// \uXXXX with upper case digits, while the slash, the apostrophe and the angle brackets stay.
	cases := []struct{ in, want string }{
		{"<b>&\"' /", "<b>\\u0026\\\"' /"},
		{"\x00\x01\x0b", "\\u0000\\u0001\\u000B"},
		{"\b\t\n\f\r", "\\b\\t\\n\\f\\r"},
		{"é€", "\\u00E9\\u20AC"},
		{"\U0001F600", "\\uD83D\\uDE00"},
		{"a\\b", "a\\\\b"},
	}
	for _, c := range cases {
		got, err := Escape(c.in, "json")
		if err != nil || got != c.want {
			t.Errorf("Escape(%q, json) = %q (%v), want %q", c.in, got, err, c.want)
		}
	}
	if _, err := Escape("x", "bogus"); err == nil {
		t.Error("an unknown strategy should be an error")
	}
}

func TestVisitEscaper(t *testing.T) {
	text := func(s string) *Expr { return &Expr{Kind: ExprLiteralString, Value: s} }
	variable := func(name string) *Expr { return &Expr{Kind: ExprContextVariable, Name: name} }
	escaped := func(e *Expr) bool { return e.Kind == ExprFilter && e.Name == "escape" }

	// A plain print node of a variable is wrapped, a string literal is not.
	print1 := &Stmt{Kind: StmtPrint, Expr: variable("x")}
	print2 := &Stmt{Kind: StmtPrint, Expr: text("<b>")}
	concat := &Stmt{Kind: StmtPrint,
		Expr: &Expr{Kind: ExprConcat, Args: []*Expr{text("a"), text("b")}}}
	mixed := &Stmt{Kind: StmtPrint,
		Expr: &Expr{Kind: ExprConcat, Args: []*Expr{text("a"), variable("x")}}}
	interpolated := &Stmt{Kind: StmtPrint,
		Expr: &Expr{Kind: ExprLiteralString, Value: "a", Extra: "interpolated"}}
	root := &Stmt{Kind: StmtRoot, Body: &Stmt{Kind: StmtBody,
		Children: []*Stmt{print1, print2, concat, mixed, interpolated}}}
	VisitEscaper(root, &Options{AutoEscaping: true})
	if !escaped(print1.Expr) {
		t.Error("a variable should be escaped")
	}
	if escaped(print2.Expr) || escaped(concat.Expr) {
		t.Error("a string literal and a concatenation of two literals are safe")
	}
	if !escaped(mixed.Expr) || !escaped(interpolated.Expr) {
		t.Error("a concatenation with a variable and an interpolated string are not safe")
	}

	// With auto escaping off nothing is wrapped.
	plain := &Stmt{Kind: StmtPrint, Expr: variable("x")}
	VisitEscaper(&Stmt{Kind: StmtRoot, Body: &Stmt{Kind: StmtBody, Children: []*Stmt{plain}}},
		&Options{AutoEscaping: false})
	if escaped(plain.Expr) {
		t.Error("auto escaping is off")
	}

	// The autoescape tag pushes its own state and strategy.
	inner := &Stmt{Kind: StmtPrint, Expr: variable("x")}
	after := &Stmt{Kind: StmtPrint, Expr: variable("x")}
	tag := &Stmt{Kind: StmtAutoEscape, Flag: true, Extra: "js",
		Body: &Stmt{Kind: StmtBody, Children: []*Stmt{inner}}}
	VisitEscaper(&Stmt{Kind: StmtRoot,
		Body: &Stmt{Kind: StmtBody, Children: []*Stmt{tag, after}}},
		&Options{AutoEscaping: true})
	if !escaped(inner.Expr) || len(inner.Expr.Arguments.Names) != 1 ||
		inner.Expr.Arguments.Names[0] != "strategy" ||
		inner.Expr.Arguments.Named[0].Value != "js" {
		t.Errorf("the autoescape strategy should become the filter argument: %#v", inner.Expr)
	}
	if !escaped(after.Expr) || len(after.Expr.Arguments.Names) != 0 {
		t.Error("outside the tag the filter takes no argument")
	}
	off := &Stmt{Kind: StmtPrint, Expr: variable("x")}
	VisitEscaper(&Stmt{Kind: StmtRoot, Body: &Stmt{Kind: StmtBody, Children: []*Stmt{
		{Kind: StmtAutoEscape, Flag: false, Body: &Stmt{Kind: StmtBody,
			Children: []*Stmt{off}}}}}}, &Options{AutoEscaping: true})
	if escaped(off.Expr) {
		t.Error("{% autoescape false %} turns escaping off")
	}

	// The bodies of a cache and of an embed tag are invisible to the Java visitor.
	cached := &Stmt{Kind: StmtPrint, Expr: variable("x")}
	VisitEscaper(&Stmt{Kind: StmtRoot, Body: &Stmt{Kind: StmtBody, Children: []*Stmt{
		{Kind: StmtCache, Body: &Stmt{Kind: StmtBody, Children: []*Stmt{cached}}}}}},
		&Options{AutoEscaping: true})
	if escaped(cached.Expr) {
		t.Error("a cache body is never escaped")
	}

	// A ternary operator is escaped branch by branch.
	ternary := &Stmt{Kind: StmtPrint, Expr: &Expr{Kind: ExprTernary,
		Args: []*Expr{variable("c"), variable("x"), text("<i>")}}}
	VisitEscaper(&Stmt{Kind: StmtRoot, Body: &Stmt{Kind: StmtBody, Children: []*Stmt{ternary}}},
		&Options{AutoEscaping: true})
	if !escaped(ternary.Expr.Args[1]) || escaped(ternary.Expr.Args[2]) {
		t.Error("only the unsafe branch of a ternary is escaped")
	}
}

func TestStringFiltersAgainstJava(t *testing.T) {
	// These all go through the jstr package; the cases are the ones the reference command printed.
	cases := []struct {
		filter string
		input  any
		args   map[string]any
		want   any
	}{
		{"abbreviate", "ab", map[string]any{"length": int64(1)}, "ab"},
		{"abbreviate", "abcdef", map[string]any{"length": int64(5)}, "ab..."},
		{"abbreviate", "abcd", map[string]any{"length": int64(4)}, "a..."},
		{"trim", "  a  ", nil, "a"},
		{"upper", "ab", nil, "AB"},
		{"upper", int64(5), nil, "5"},
		{"lower", "AB", nil, "ab"},
		{"capitalize", "ab cd", nil, "Ab cd"},
		{"title", "ab cd", nil, "Ab Cd"},
		{"length", "ab", nil, int32(2)},
		{"urlencode", "a b", nil, "a+b"},
		{"base64encode", "ab", nil, "YWI="},
		{"base64decode", "YWI=", nil, "ab"},
		{"sha256", "a", nil,
			"ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"},
		{"slice", "hello", map[string]any{"fromIndex": int64(1), "toIndex": int64(3)}, "el"},
		{"first", "abc", nil, Char('a')},
		{"last", "abc", nil, Char('c')},
		{"replace", "a", map[string]any{"replace_pairs": linkedMap("a", "b")}, "b"},
		{"format", "a %s b", map[string]any{"0": "x"}, "a x b"},
		{"escape", "<b>", nil, SafeString("&lt;b&gt;")},
		{"numberformat", float64(1234.5), map[string]any{}, "1,234.5"},
		{"numberformat", float64(1234.5), map[string]any{"format": "#.##"}, "1234.5"},
		{"date", int64(0), map[string]any{"format": "yyyy-MM-dd HH:mm:ss"},
			SafeString("1970-01-01 00:00:00")},
	}
	for _, c := range cases {
		c := c
		t.Run(c.filter, func(t *testing.T) {
			checkRuntime(t, func() {
				got, err := Filters[c.filter].Apply(c.input, c.args, ctx, 1)
				if err != nil {
					t.Fatalf("%s: %v", c.filter, err)
				}
				if got != c.want {
					t.Errorf("%s of %#v = %#v, want %#v", c.filter, c.input, got, c.want)
				}
			})
		})
	}
}

func TestSplitFilter(t *testing.T) {
	checkRuntime(t, func() {
		got, err := Filters["split"].Apply("a,b",
			map[string]any{"delimiter": ","}, ctx, 1)
		if err != nil || ToStr(got) != "[a, b]" {
			t.Errorf("split = %v (%v)", ToStr(got), err)
		}
	})
	_, err := Filters["split"].Apply("a", map[string]any{}, ctx, 1)
	if err == nil || err.Error() != "missing delimiter parameter in split filter (i.peb:1)" {
		t.Errorf("split without a delimiter: %v", err)
	}
}

func TestBigDecimalArithmetic(t *testing.T) {
	checkRuntime(t, func() {
		cases := []struct {
			op    string
			left  string
			right any
			want  string
		}{
			// Verified against the reference command with --big-decimals.
			{"+", "1.0E10", int64(1), "10000000001.0"},
			{"*", "1.0E10", int64(3), "30000000000"},
			{"/", "1.0E10", int64(3), "3333333333.333333333333333333333333"},
			{"%", "1.0E10", int64(3), "1.0"},
			{"/", "1", int64(3), "0.3333333333333333333333333333333333"},
		}
		for _, c := range cases {
			left, err := parseBigDecimalForTest(c.left)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ApplyBinary(c.op, left, c.right, ctx, 1)
			if err != nil {
				t.Errorf("%s %s %v: %v", c.left, c.op, c.right, err)
				continue
			}
			if ToStr(got) != c.want {
				t.Errorf("%s %s %v = %v, want %v", c.left, c.op, c.right, ToStr(got), c.want)
			}
		}
	})
}

// parseBigDecimalForTest builds a BigDecimal from its Java text.
func parseBigDecimalForTest(text string) (*BigDecimal, error) {
	return jnum.ParseBigDecimal(text)
}
