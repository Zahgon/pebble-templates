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
	"math"
	"math/big"
	"strings"

	"github.com/PebbleTemplates/pebble/internal/jnum"
	"github.com/PebbleTemplates/pebble/internal/jstr"
)

// arithOp names the five operations of OperatorUtils.
type arithOp int

const (
	opAdd arithOp = iota
	opSub
	opMul
	opDiv
	opMod
)

// ApplyBinary applies a binary operator of the core extension to two evaluated operands. It is
// the node/expression/*Expression classes and io.pebbletemplates.pebble.utils.OperatorUtils
// together: each operator wraps the failures of OperatorUtils in the PebbleException its
// expression class builds.
//
// The "is", "is not" and "|" operators of the table never reach here: the parser turns them into
// test and filter expressions. The right operand of "and" and "or" has already been evaluated
// here, so the caller is the one that short-circuits by not evaluating it at all.
func ApplyBinary(op string, left, right any, ctx Ctx, line int) (any, error) {
	switch op {
	case "or":
		return applyOr(left, right, ctx, line)
	case "and":
		return applyAnd(left, right, ctx, line)
	case "contains":
		return applyContains(left, right, ctx, line)
	case "==", "equals":
		return Equal(left, right), nil
	case "!=":
		return !Equal(left, right), nil
	case ">":
		return relational(left, right, ctx, line, "GREATER_THAN",
			"Could not perform greater than comparison")
	case ">=":
		return relational(left, right, ctx, line, "GREATER_THAN_EQUALS",
			"Could not perform greater than or equals comparison")
	case "<":
		// LessThanExpression really does report the modulus message.
		return relational(left, right, ctx, line, "LESS_THAN", "Could not perform greater modulus")
	case "<=":
		return relational(left, right, ctx, line, "LESS_THAN_EQUALS",
			"Could not perform less than or equals comparison")
	case "+":
		return wrap(ctx, line, "Could not perform addition", func() (any, error) {
			return add(left, right)
		})
	case "-":
		return wrap(ctx, line, "Could not perform subtraction", func() (any, error) {
			if l, ok := left.(*List); ok {
				return subtractFromList(l, right), nil
			}
			return arith(left, right, opSub)
		})
	case "*":
		return wrap(ctx, line, "Could not perform multiplication", func() (any, error) {
			return arith(left, right, opMul)
		})
	case "/":
		return wrap(ctx, line, "Could not perform division", func() (any, error) {
			return arith(left, right, opDiv)
		})
	case "%":
		return wrap(ctx, line, "Could not perform greater modulus", func() (any, error) {
			return arith(left, right, opMod)
		})
	case "~":
		return concatenate(left, right), nil
	case "..":
		return rangeOf(left, right, ctx, line)
	default:
		return nil, NewError("Unexpected operator ["+op+"]", ctx.Name(), line)
	}
}

// ApplyUnary applies "not", "+" or "-".
func ApplyUnary(op string, value any, ctx Ctx, line int) (any, error) {
	switch op {
	case "not":
		return applyNot(value, ctx, line)
	case "+":
		// OperatorUtils.unaryPlus multiplies by the Integer 1, so the Java promotion of the
		// operand with an int decides the result type.
		return arith(int32(1), value, opMul)
	case "-":
		return arith(int32(-1), value, opMul)
	default:
		return nil, NewError("Unexpected operator ["+op+"]", ctx.Name(), line)
	}
}

// wrap runs an operation and wraps whatever it reports in the PebbleException of the expression
// class, which keeps the original exception as its cause.
func wrap(ctx Ctx, line int, message string, run func() (any, error)) (any, error) {
	value, err := run()
	if err != nil {
		return nil, &Error{Kind: "PebbleException", Message: message, File: ctx.Name(),
			Line: line, Cause: err}
	}
	return value, nil
}

// relational is one of >, >=, < and <=: a null operand makes them all false and a non-numeric one
// raises, as OperatorUtils.wideningConversionBinaryComparison does.
func relational(left, right any, ctx Ctx, line int, comparison, message string) (any, error) {
	if left == nil || right == nil {
		return false, nil
	}
	l, leftNumber := javaDoubleValue(left)
	r, rightNumber := javaDoubleValue(right)
	if !leftNumber || !rightNumber {
		return nil, &Error{Kind: "PebbleException", Message: message, File: ctx.Name(), Line: line,
			Cause: &JavaError{Kind: "RuntimeException",
				Message: "invalid operands for mathematical comparison [" + comparison + "]"}}
	}
	switch comparison {
	case "GREATER_THAN":
		return l > r, nil
	case "GREATER_THAN_EQUALS":
		return l >= r, nil
	case "LESS_THAN":
		return l < r, nil
	default:
		return l <= r, nil
	}
}

// applyAnd and applyOr are AndExpression and OrExpression. They cast each operand to a Boolean
// the way TypeUtils.compatibleCast does and treat a null as false unless strict variables are on.
func applyAnd(left, right any, ctx Ctx, line int) (any, error) {
	l, err := booleanOperand(left, ctx, line)
	if err != nil {
		return nil, err
	}
	if !l {
		return false, nil
	}
	return booleanOperand(right, ctx, line)
}

func applyOr(left, right any, ctx Ctx, line int) (any, error) {
	l, err := booleanOperand(left, ctx, line)
	if err != nil {
		return nil, err
	}
	if l {
		return true, nil
	}
	return booleanOperand(right, ctx, line)
}

func booleanOperand(value any, ctx Ctx, line int) (bool, error) {
	if value == nil {
		if ctx.Strict() {
			// Both operators report the "and" message; OrExpression copied it.
			return false, NewError("null value used in and operator and strict variables is set to true",
				ctx.Name(), line)
		}
		return false, nil
	}
	truthy, ok := BoolCast(value)
	if !ok {
		return false, castError(value, "java.lang.Boolean")
	}
	return truthy, nil
}

// applyNot is UnaryNotExpression, which accepts only a Boolean, a String or a Number.
func applyNot(value any, ctx Ctx, line int) (any, error) {
	if value == nil {
		if ctx.Strict() {
			return nil, NewError("null value given to not() and strict variables is set to true",
				ctx.Name(), line)
		}
		return true, nil
	}
	truthy, ok := BoolCast(value)
	if !ok {
		return nil, NewError("Unsupported value type "+javaSimpleName(value)+
			". Expected Boolean, String, Number in \"if\" statement", ctx.Name(), line)
	}
	return !truthy, nil
}

// concatenate is ConcatenateExpression: a null operand contributes nothing and every other one
// its toString, so a BigDecimal appears in scientific notation here.
func concatenate(left, right any) string {
	var b strings.Builder
	if left != nil {
		b.WriteString(valueOf(left))
	}
	if right != nil {
		b.WriteString(valueOf(right))
	}
	return b.String()
}

// rangeOf is RangeExpression, which simply calls the range function with two positional
// arguments.
func rangeOf(left, right any, ctx Ctx, line int) (any, error) {
	return Functions["range"].Apply(map[string]any{"start": left, "end": right}, ctx, line)
}

// applyContains is ContainsExpression.
func applyContains(left, right any, ctx Ctx, line int) (any, error) {
	if left == nil {
		return false, nil
	}
	switch l := left.(type) {
	case *List:
		if r, ok := right.(*List); ok {
			// Collection.containsAll
			for _, want := range r.Items {
				if !listContains(l, want) {
					return false, nil
				}
			}
			return true, nil
		}
		return listContains(l, right), nil
	case *Map:
		_, found := l.Vals[right]
		return found, nil
	case string:
		return strings.Contains(l, valueOf(right)), nil
	default:
		return nil, NewError(
			"Contains operator can only be used on Collections, Maps and arrays. Actual type was: "+
				javaClassName(left), ctx.Name(), line)
	}
}

func listContains(l *List, want any) bool {
	for _, item := range l.Items {
		if javaEquals(want, item) {
			return true
		}
	}
	return false
}

// add is OperatorUtils.add: a String or a SafeString operand concatenates, a List on the left
// appends and everything else goes through the Java numeric promotion.
func add(left, right any) (any, error) {
	if isStringish(left) || isStringish(right) {
		return valueOf(left) + valueOf(right), nil
	}
	if l, ok := left.(*List); ok {
		return addToList(l, right), nil
	}
	return arith(left, right, opAdd)
}

func isStringish(value any) bool {
	switch value.(type) {
	case string, SafeString:
		return true
	default:
		return false
	}
}

// addToList and subtractFromList are the undocumented list arithmetic of OperatorUtils; they copy
// the list rather than change it.
func addToList(left *List, right any) *List {
	items := append([]any{}, left.Items...)
	if r, ok := right.(*List); ok {
		items = append(items, r.Items...)
	} else {
		items = append(items, right)
	}
	return &List{Items: items}
}

func subtractFromList(left *List, right any) *List {
	var remove []any
	if r, ok := right.(*List); ok {
		// List.removeAll drops every occurrence of every element.
		remove = r.Items
	} else {
		// List.remove(Object) drops only the first occurrence.
		for i, item := range left.Items {
			if javaEquals(right, item) {
				items := append([]any{}, left.Items[:i]...)
				return &List{Items: append(items, left.Items[i+1:]...)}
			}
		}
		return &List{Items: append([]any{}, left.Items...)}
	}
	items := []any{}
	for _, item := range left.Items {
		drop := false
		for _, unwanted := range remove {
			if javaEquals(unwanted, item) {
				drop = true
				break
			}
		}
		if !drop {
			items = append(items, item)
		}
	}
	return &List{Items: items}
}

// arith is OperatorUtils.wideningConversionBinaryOperation: section 5.6.2 of the Java language
// specification, with BigDecimal added on top. A BigDecimal operand makes both sides a
// BigDecimal.valueOf of their double value, which is lossy but is what Pebble does; otherwise the
// widest of double, float, long and int wins, and two ints still produce a Long because the Java
// method returns long.
func arith(left, right any, op arithOp) (any, error) {
	num1, err := toNumber(left)
	if err != nil {
		return nil, err
	}
	num2, err := toNumber(right)
	if err != nil {
		return nil, err
	}

	if isBigDecimal(num1) || isBigDecimal(num2) {
		d1, _ := javaDoubleValue(num1)
		d2, _ := javaDoubleValue(num2)
		return bigDecimalOp(jnum.NewBigDecimalFromFloat(d1), jnum.NewBigDecimalFromFloat(d2), op)
	}
	if isFloat64(num1) || isFloat64(num2) {
		d1, _ := javaDoubleValue(num1)
		d2, _ := javaDoubleValue(num2)
		return doubleOp(d1, d2, op), nil
	}
	if isFloat32(num1) || isFloat32(num2) {
		return floatOp(javaFloatValue(num1), javaFloatValue(num2), op), nil
	}
	if isInt64(num1) || isInt64(num2) {
		return longOp(javaLongValue(num1), javaLongValue(num2), op)
	}
	return intOp(javaIntValue(num1), javaIntValue(num2), op)
}

func isBigDecimal(value any) bool { _, ok := value.(*BigDecimal); return ok }
func isFloat64(value any) bool    { _, ok := value.(float64); return ok }
func isFloat32(value any) bool    { _, ok := value.(float32); return ok }
func isInt64(value any) bool      { _, ok := value.(int64); return ok }

// toNumber is OperatorUtils.toNumber: a Number stays as it is, a String is parsed as a double
// when it looks like one and as a long otherwise, and anything else raises.
func toNumber(value any) (any, error) {
	switch v := value.(type) {
	case int32, int64, float32, float64, *big.Int, *BigDecimal:
		return v, nil
	case string:
		if strings.ContainsAny(v, ".eE") {
			d, err := jnum.ParseDouble(v)
			if err != nil {
				return nil, err
			}
			return d, nil
		}
		l, err := jnum.ParseLong(v)
		if err != nil {
			return nil, err
		}
		return l, nil
	default:
		return nil, &JavaError{Kind: "IllegalArgumentException",
			Message: "Cannot convert to Number: " + valueOf(value)}
	}
}

// javaFloatValue, javaLongValue and javaIntValue are Number.floatValue, longValue and intValue.
func javaFloatValue(value any) float32 {
	d, _ := javaDoubleValue(value)
	return float32(d)
}

func javaLongValue(value any) int64 {
	switch v := value.(type) {
	case int32:
		return int64(v)
	case int64:
		return v
	case float32:
		return doubleToLong(float64(v))
	case float64:
		return doubleToLong(v)
	case *big.Int:
		return bigIntToLong(v)
	case *BigDecimal:
		return v.Int64()
	default:
		return 0
	}
}

func javaIntValue(value any) int32 {
	switch v := value.(type) {
	case int32:
		return v
	case *BigDecimal:
		return v.Int32()
	case float32:
		return doubleToInt(float64(v))
	case float64:
		return doubleToInt(v)
	default:
		// Long and BigInteger narrow by keeping the low 32 bits.
		return int32(uint32(uint64(javaLongValue(value))))
	}
}

// doubleToLong and doubleToInt are the Java (long) and (int) casts of a double: NaN is zero and
// an out of range value saturates.
func doubleToLong(f float64) int64 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt64:
		return math.MaxInt64
	case f <= math.MinInt64:
		return math.MinInt64
	default:
		return int64(f)
	}
}

func doubleToInt(f float64) int32 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt32:
		return math.MaxInt32
	case f <= math.MinInt32:
		return math.MinInt32
	default:
		return int32(f)
	}
}

// bigIntToLong is BigInteger.longValue, which keeps the low 64 bits of the two's complement
// representation.
func bigIntToLong(v *big.Int) int64 {
	low := new(big.Int).And(v, maskInt64)
	return int64(low.Uint64())
}

var maskInt64 = new(big.Int).SetUint64(math.MaxUint64)

func doubleOp(a, b float64, op arithOp) float64 {
	switch op {
	case opAdd:
		return a + b
	case opSub:
		return a - b
	case opMul:
		return a * b
	case opDiv:
		return a / b
	default:
		return math.Mod(a, b)
	}
}

func floatOp(a, b float32, op arithOp) float32 {
	switch op {
	case opAdd:
		return a + b
	case opSub:
		return a - b
	case opMul:
		return a * b
	case opDiv:
		return a / b
	default:
		return float32(math.Mod(float64(a), float64(b)))
	}
}

// longOp is the long arithmetic of Java: it overflows silently and raises on an integer division
// or modulus by zero.
func longOp(a, b int64, op arithOp) (any, error) {
	switch op {
	case opAdd:
		return a + b, nil
	case opSub:
		return a - b, nil
	case opMul:
		return a * b, nil
	case opDiv:
		if b == 0 {
			return nil, divByZero()
		}
		if a == math.MinInt64 && b == -1 {
			return a, nil
		}
		return a / b, nil
	default:
		if b == 0 {
			return nil, divByZero()
		}
		if b == -1 {
			return int64(0), nil
		}
		// Java's % keeps the sign of the dividend, which Go's % does too.
		return a % b, nil
	}
}

// intOp is OperatorUtils.integerOperation: the arithmetic happens in int, so it overflows within
// 32 bits, but the method returns a long so the result is boxed as a Long.
func intOp(a, b int32, op arithOp) (any, error) {
	switch op {
	case opAdd:
		return int64(a + b), nil
	case opSub:
		return int64(a - b), nil
	case opMul:
		return int64(a * b), nil
	case opDiv:
		if b == 0 {
			return nil, divByZero()
		}
		if a == math.MinInt32 && b == -1 {
			return int64(a), nil
		}
		return int64(a / b), nil
	default:
		if b == 0 {
			return nil, divByZero()
		}
		if b == -1 {
			return int64(0), nil
		}
		return int64(a % b), nil
	}
}

func divByZero() error {
	return &JavaError{Kind: "ArithmeticException", Message: "/ by zero"}
}

// bigDecimalOp is OperatorUtils.bigDecimalOperation: add and subtract are exact, while multiply,
// divide and remainder round to the 34 significant digits of MathContext.DECIMAL128.
func bigDecimalOp(a, b *BigDecimal, op arithOp) (any, error) {
	var value *BigDecimal
	var err error
	switch op {
	case opAdd:
		return a.Add(b), nil
	case opSub:
		return a.Sub(b), nil
	case opMul:
		return a.MulContext(b, jnum.Decimal128), nil
	case opDiv:
		value, err = a.DivContext(b, jnum.Decimal128)
	default:
		value, err = a.ModContext(b, jnum.Decimal128)
	}
	if numberErr, ok := err.(*jnum.JavaError); ok {
		return nil, &JavaError{Kind: numberErr.Class, Message: numberErr.Message}
	}
	return value, err
}

// compareNatural is Comparable.compareTo of the Java value space, which the sort filter needs.
// The receiver decides the type the argument is cast to, so a mixed list reports the
// ClassCastException of the first comparison the sort happens to make.
func compareNatural(receiver, argument any) (int, error) {
	if receiver == nil {
		// Java calls compareTo on the element itself, so a null element is a NullPointerException.
		// Its helpful message names the variable of the sort that held the null; "a[runHi]" is the
		// one of the first comparison, which is where a null in a short list is always found.
		return 0, &JavaError{Kind: "NullPointerException",
			Message: "Cannot invoke \"java.lang.Comparable.compareTo(Object)\" " +
				"because \"a[runHi]\" is null"}
	}
	switch l := receiver.(type) {
	case string:
		r, ok := argument.(string)
		if !ok {
			return 0, castError(argument, "java.lang.String")
		}
		return jstr.CompareTo(l, r), nil
	case bool:
		r, ok := argument.(bool)
		if !ok {
			return 0, castError(argument, "java.lang.Boolean")
		}
		return boolCompare(l, r), nil
	case Char:
		r, ok := argument.(Char)
		if !ok {
			return 0, castError(argument, "java.lang.Character")
		}
		return int(l) - int(r), nil
	case int32:
		r, ok := argument.(int32)
		if !ok {
			return 0, castError(argument, "java.lang.Integer")
		}
		return intCompare(int64(l), int64(r)), nil
	case int64:
		r, ok := argument.(int64)
		if !ok {
			return 0, castError(argument, "java.lang.Long")
		}
		return intCompare(l, r), nil
	case float32:
		r, ok := argument.(float32)
		if !ok {
			return 0, castError(argument, "java.lang.Float")
		}
		return floatCompare(float64(l), float64(r)), nil
	case float64:
		r, ok := argument.(float64)
		if !ok {
			return 0, castError(argument, "java.lang.Double")
		}
		return floatCompare(l, r), nil
	case *big.Int:
		r, ok := argument.(*big.Int)
		if !ok {
			return 0, castError(argument, "java.math.BigInteger")
		}
		return l.Cmp(r), nil
	case *BigDecimal:
		r, ok := argument.(*BigDecimal)
		if !ok {
			return 0, castError(argument, "java.math.BigDecimal")
		}
		return l.Cmp(r), nil
	default:
		// Not a Comparable at all: the cast in the sort itself is the one that fails.
		return 0, castError(receiver, "java.lang.Comparable")
	}
}

func boolCompare(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	default:
		return -1
	}
}

func intCompare(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// floatCompare is Double.compare, which orders NaN above everything and -0.0 below 0.0.
func floatCompare(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	aBits := int64(math.Float64bits(canonicalNaN(a)))
	bBits := int64(math.Float64bits(canonicalNaN(b)))
	return intCompare(aBits, bBits)
}
