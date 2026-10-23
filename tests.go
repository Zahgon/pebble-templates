/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package pebble

import "github.com/PebbleTemplates/pebble/internal/jstr"

func init() {
	Tests["empty"] = Test{Apply: emptyTest}
	Tests["even"] = Test{Apply: evenTest}
	Tests["iterable"] = Test{Apply: iterableTest}
	Tests["map"] = Test{Apply: mapTest}
	Tests["null"] = Test{Apply: nullTest}
	Tests["odd"] = Test{Apply: oddTest}
	Tests["defined"] = Test{Apply: definedTest}
}

// emptyTest is EmptyTest: null, a String of nothing but whitespace, an empty Collection and an
// empty Map are empty; every other value, a zero included, is not.
func emptyTest(input any, args map[string]any, ctx Ctx, line int) (bool, error) {
	return isEmptyValue(input), nil
}

func isEmptyValue(input any) bool {
	switch v := input.(type) {
	case nil:
		return true
	case string:
		return jstr.Trim(v) == ""
	case *List:
		return len(v.Items) == 0
	case *Map:
		return len(v.Keys) == 0
	default:
		return false
	}
}

// evenTest is EvenTest, which accepts an Integer or a Long and nothing else.
func evenTest(input any, args map[string]any, ctx Ctx, line int) (bool, error) {
	switch v := input.(type) {
	case nil:
		return false, NewError("Can not pass null value to \"even\" test.", ctx.Name(), line)
	case int32:
		return v%2 == 0, nil
	case int64:
		return v%2 == 0, nil
	default:
		return false, castError(input, "java.lang.Long")
	}
}

// oddTest is OddTest, which rejects a null with a plain IllegalArgumentException rather than with
// the PebbleException the even test reports.
func oddTest(input any, args map[string]any, ctx Ctx, line int) (bool, error) {
	if input == nil {
		return false, &JavaError{Kind: "IllegalArgumentException",
			Message: "Can not pass null value to \"odd\" test."}
	}
	even, err := evenTest(input, args, ctx, line)
	if err != nil {
		return false, err
	}
	return !even, nil
}

// iterableTest is IterableTest: a java.lang.Iterable or an Object[]. A Map is neither, and neither
// is a String.
func iterableTest(input any, args map[string]any, ctx Ctx, line int) (bool, error) {
	_, ok := input.(*List)
	return ok, nil
}

// mapTest is MapTest.
func mapTest(input any, args map[string]any, ctx Ctx, line int) (bool, error) {
	_, ok := input.(*Map)
	return ok, nil
}

// nullTest is NullTest and definedTest its negation, DefinedTest.
func nullTest(input any, args map[string]any, ctx Ctx, line int) (bool, error) {
	return input == nil, nil
}

func definedTest(input any, args map[string]any, ctx Ctx, line int) (bool, error) {
	return input != nil, nil
}
