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
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/PebbleTemplates/pebble/internal/jnum"
	"github.com/PebbleTemplates/pebble/internal/jstr"
)

// Char is java.lang.Character, held as a single UTF-16 code unit. The core extension produces it
// in two places: the first and last filters applied to a String return String.charAt, and the
// range function walks characters when both of its bounds are one character long.
type Char uint16

// ToStr is io.pebbletemplates.pebble.utils.StringUtils.toString, which is how a value reaches the
// output: PrintNode writes nothing for null, a BigDecimal through toPlainString (the specialized
// writers of extension/writer do the same) and everything else through its toString.
func ToStr(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case *BigDecimal:
		return v.PlainString()
	default:
		return valueOf(value)
	}
}

// valueOf is java.lang.String.valueOf of the boxed Java value, that is what StringBuilder.append
// and the string concatenation of OperatorUtils see. It differs from ToStr for BigDecimal, which
// prints in scientific notation here ("1E+10") but plainly when printed on its own.
func valueOf(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case bool:
		if v {
			return "true"
		}
		return "false"
	case string:
		return v
	case SafeString:
		return string(v)
	case Char:
		return string(rune(v))
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case float32:
		return jnum.FloatString(v)
	case float64:
		return jnum.DoubleString(v)
	case *big.Int:
		return v.String()
	case *BigDecimal:
		return v.String()
	case Locale:
		return string(v)
	case *List:
		return listString(v)
	case *Map:
		return mapString(v)
	case fmt.Stringer:
		// The loop variable, a map entry and the macro attribute provider print through toString.
		return v.String()
	default:
		// Object.toString of a value with no interesting text: Java prints the class name and an
		// identity hash code, which no output can depend on.
		return javaClassName(value) + "@0"
	}
}

// listString is java.util.AbstractCollection.toString.
func listString(l *List) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, item := range l.Items {
		if i > 0 {
			b.WriteString(", ")
		}
		if item == l {
			b.WriteString("(this Collection)")
			continue
		}
		b.WriteString(valueOf(item))
	}
	b.WriteByte(']')
	return b.String()
}

// mapString is java.util.AbstractMap.toString, which walks the map in its own iteration order.
func mapString(m *Map) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, key := range m.Keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(valueOf(key))
		b.WriteByte('=')
		val := m.Vals[key]
		if val == m {
			b.WriteString("(this Map)")
			continue
		}
		b.WriteString(valueOf(val))
	}
	b.WriteByte('}')
	return b.String()
}

// Truthy is how Pebble decides a condition: io.pebbletemplates.pebble.utils.TypeUtils
// compatibleCast(value, Boolean.class). A null is false, a Boolean itself, a Number true when it
// is neither NaN nor zero and a String true when it is not empty.
//
// Any other type makes the Java cast to Boolean fail; this function reports false for it, so a
// caller that has to raise the Java ClassCastException or the "Unsupported value type" message
// (the if tag, the ternary operator, the not operator) checks the type itself first. BoolCast
// reports whether the value is one Java could cast.
func Truthy(value any) bool {
	truthy, _ := BoolCast(value)
	return truthy
}

// BoolCast is Truthy with the extra bit of information that Java's cast to Boolean would have
// succeeded; ok is false for the types that make it throw.
func BoolCast(value any) (truthy, ok bool) {
	switch v := value.(type) {
	case nil:
		return false, true
	case bool:
		return v, true
	case string:
		// String.isEmpty, which needs no UTF-16 length.
		return v != "", true
	}
	if d, isNumber := javaDoubleValue(value); isNumber {
		return !math.IsNaN(d) && d != 0, true
	}
	return false, false
}

// Equal is io.pebbletemplates.pebble.utils.OperatorUtils.equals, the == and equals operators. It
// is not plain Java equals: two Numbers are compared as doubles, so 1 equals 1.0, and everything
// else falls back to Object.equals. (Pebble also compares an enum with a String by name; the
// reference command has no enums in its value space.)
func Equal(left, right any) bool {
	_, leftNumber := javaDoubleValue(left)
	_, rightNumber := javaDoubleValue(right)
	if leftNumber && rightNumber {
		l, _ := javaDoubleValue(left)
		r, _ := javaDoubleValue(right)
		return l == r
	}
	return javaEquals(left, right)
}

// javaEquals is Object.equals of the Java value space of the reference command.
func javaEquals(left, right any) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	switch l := left.(type) {
	case bool:
		r, ok := right.(bool)
		return ok && l == r
	case string:
		r, ok := right.(string)
		return ok && l == r
	case SafeString:
		r, ok := right.(SafeString)
		return ok && l == r
	case Char:
		r, ok := right.(Char)
		return ok && l == r
	case int32:
		r, ok := right.(int32)
		return ok && l == r
	case int64:
		r, ok := right.(int64)
		return ok && l == r
	case float32:
		r, ok := right.(float32)
		return ok && floatBitsEqual(float64(l), float64(r))
	case float64:
		r, ok := right.(float64)
		return ok && floatBitsEqual(l, r)
	case *big.Int:
		r, ok := right.(*big.Int)
		return ok && l.Cmp(r) == 0
	case *BigDecimal:
		r, ok := right.(*BigDecimal)
		return ok && l.Equals(r)
	case Locale:
		r, ok := right.(Locale)
		return ok && l == r
	case *List:
		r, ok := right.(*List)
		if !ok || len(l.Items) != len(r.Items) {
			return false
		}
		for i := range l.Items {
			// java.util.AbstractList.equals compares the elements with Object.equals, not with
			// the numeric comparison of the == operator.
			if !javaEquals(l.Items[i], r.Items[i]) {
				return false
			}
		}
		return true
	case *Map:
		r, ok := right.(*Map)
		if !ok || len(l.Keys) != len(r.Keys) {
			return false
		}
		for _, key := range l.Keys {
			other, found := r.Vals[key]
			if !found || !javaEquals(l.Vals[key], other) {
				return false
			}
		}
		return true
	default:
		return left == right
	}
}

// floatBitsEqual is Double.equals, which compares the bit patterns: NaN equals NaN and 0.0 does
// not equal -0.0.
func floatBitsEqual(a, b float64) bool {
	return math.Float64bits(canonicalNaN(a)) == math.Float64bits(canonicalNaN(b))
}

func canonicalNaN(f float64) float64 {
	if math.IsNaN(f) {
		return math.NaN()
	}
	return f
}

// Compare is the relational operators of the core extension: Java widens both operands to double
// and compares them, so it reports an error for anything that is not a java.lang.Number. A null
// operand makes every relational operator false rather than raise, so ApplyBinary checks for null
// before calling this.
func Compare(left, right any) (int, error) {
	l, leftNumber := javaDoubleValue(left)
	r, rightNumber := javaDoubleValue(right)
	if !leftNumber || !rightNumber {
		return 0, &JavaError{Kind: "RuntimeException",
			Message: "invalid operands for mathematical comparison"}
	}
	switch {
	case l < r:
		return -1, nil
	case l > r:
		return 1, nil
	case l == r:
		return 0, nil
	default:
		// A NaN operand: every ordering is false, which the callers spell out themselves.
		return 0, nil
	}
}

// javaDoubleValue is Number.doubleValue, and reports whether the value is a java.lang.Number at
// all. java.lang.Character and java.lang.Boolean are not.
func javaDoubleValue(value any) (float64, bool) {
	switch v := value.(type) {
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case *big.Int:
		f, _ := new(big.Float).SetInt(v).Float64()
		return f, true
	case *BigDecimal:
		return v.Float64(), true
	default:
		return 0, false
	}
}

// castError is the ClassCastException the JVM raises when Pebble casts a template value to the
// type a filter, a test or an operator expects. Every class of the value space but SafeString is
// in java.base, which the message spells out.
func castError(value any, target string) error {
	from := javaClassName(value)
	var where string
	if inJavaBase(from) && inJavaBase(target) {
		where = from + " and " + target + " are in module java.base of loader 'bootstrap'"
	} else {
		where = moduleOf(from) + "; " + moduleOf(target)
	}
	return &JavaError{Kind: "ClassCastException",
		Message: "class " + from + " cannot be cast to class " + target + " (" + where + ")"}
}

func inJavaBase(class string) bool {
	return strings.HasPrefix(class, "java.")
}

func moduleOf(class string) string {
	if inJavaBase(class) {
		return class + " is in module java.base of loader 'bootstrap'"
	}
	return class + " is in unnamed module of loader 'app'"
}

// javaHashCode is Object.hashCode of a template value, which decides the iteration order of a
// java.util.HashMap.
func javaHashCode(value any) int32 {
	switch v := value.(type) {
	case nil:
		return 0
	case bool:
		if v {
			return 1231
		}
		return 1237
	case string:
		return jstr.HashCode(v)
	case SafeString:
		return jstr.HashCode(string(v))
	case Char:
		return int32(v)
	case *List:
		var hash int32 = 1
		for _, item := range v.Items {
			hash = 31*hash + javaHashCode(item)
		}
		return hash
	case *Map:
		var hash int32
		for _, key := range v.Keys {
			hash += javaHashCode(key) ^ javaHashCode(v.Vals[key])
		}
		return hash
	default:
		return jnum.HashCode(value)
	}
}

// hashMap builds a java.util.HashMap: it reproduces the bucket order of the JDK's implementation
// so that the map iterates, and prints, exactly as the Java one does. initialCapacity is the
// argument the Java code passes to the constructor; zero means the no-argument constructor.
type hashMap struct {
	table     [][]any // the keys of each bucket, in insertion order within the bucket
	vals      map[any]any
	size      int
	threshold int
}

func newHashMap(initialCapacity int) *hashMap {
	m := &hashMap{vals: map[any]any{}}
	// HashMap(int) stores tableSizeFor(initialCapacity) in threshold and allocates a table of
	// that length on the first put; HashMap() allocates 16 with a threshold of 12.
	if initialCapacity > 0 {
		m.threshold = tableSizeFor(initialCapacity)
	}
	return m
}

func tableSizeFor(capacity int) int {
	n := 1
	for n < capacity && n < 1<<30 {
		n <<= 1
	}
	return n
}

// spread is java.util.HashMap.hash: the high bits of the hash code are folded into the low ones.
func spread(h int32) uint32 {
	u := uint32(h)
	return u ^ (u >> 16)
}

func (m *hashMap) put(key, value any) {
	if _, exists := m.vals[key]; exists {
		m.vals[key] = value
		return
	}
	if m.table == nil {
		capacity := m.threshold
		if capacity == 0 {
			capacity = 16
		}
		m.table = make([][]any, capacity)
		m.threshold = capacity * 3 / 4
	}
	i := int(spread(javaHashCode(key))) & (len(m.table) - 1)
	m.table[i] = append(m.table[i], key)
	m.vals[key] = value
	m.size++
	if m.size > m.threshold {
		m.resize()
	}
}

// resize doubles the table, splitting every bucket into the entries that stay at their index and
// the ones that move up by the old length, each keeping its relative order, as the JDK does.
func (m *hashMap) resize() {
	oldCap := len(m.table)
	table := make([][]any, oldCap*2)
	for i, bucket := range m.table {
		for _, key := range bucket {
			if int(spread(javaHashCode(key)))&oldCap == 0 {
				table[i] = append(table[i], key)
			} else {
				table[i+oldCap] = append(table[i+oldCap], key)
			}
		}
	}
	m.table = table
	m.threshold = len(table) * 3 / 4
}

// build returns the map in its iteration order.
func (m *hashMap) build() *Map {
	out := &Map{Vals: m.vals, Hash: true}
	for _, bucket := range m.table {
		out.Keys = append(out.Keys, bucket...)
	}
	return out
}
