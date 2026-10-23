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
	"math/big"
	"strconv"
	"strings"

	"github.com/PebbleTemplates/pebble/internal/jstr"
)

// getAttributeExpr is GetAttributeExpression.evaluate.
func (c *evalCtx) getAttributeExpr(e *Expr, self *Template) (any, error) {
	object, err := c.eval(e.Args[0], self)
	if err != nil {
		return nil, err
	}
	name, err := c.eval(e.Args[1], self)
	if err != nil {
		return nil, err
	}
	return c.getAttribute(object, name, e.Arguments, e.Line, self, e.Args[0])
}

// getAttribute resolves obj.name with the attribute resolvers of the extensions, in their order:
// the default resolver, which tries maps, then arrays, then lists, then the macros of an imported
// template and finally the members of the object itself.
//
// objExpr is the expression the object came from, which decides which message a null object gets
// when strict variables are on; it may be nil.
func (c *evalCtx) getAttribute(object any, nameValue any, args *Arguments, line int,
	self *Template, objExpr *Expr) (any, error) {
	name := stringValueOf(nameValue)

	// The argument values are null when the attribute was not written as a call, which is what
	// lets the map, array and list resolvers run at all.
	var argValues []any
	if args != nil {
		argValues = make([]any, 0, len(args.Positional))
		for _, arg := range args.Positional {
			value, err := c.eval(arg, self)
			if err != nil {
				return nil, err
			}
			argValues = append(argValues, value)
		}
	}

	if object == nil && c.strict {
		if objExpr != nil && objExpr.Kind == ExprContextVariable {
			return nil, NewRootAttributeError(fmt.Sprintf(
				"Root attribute [%s] does not exist or can not be accessed and strict variables is set to true.",
				objExpr.Name), objExpr.Name, self.Name, line)
		}
		return nil, NewRootAttributeError(
			"Attempt to get attribute of null object and strict variables is set to true.",
			name, self.Name, line)
	}

	if object != nil {
		if argValues == nil {
			switch typed := object.(type) {
			case *Map:
				return c.resolveMap(typed, nameValue, line, self)
			case *globalContext:
				// GlobalContext is a Map that implements get only: a numeric key reaches
				// keySet and strict variables reach containsKey, both unsupported.
				if isNumber(nameValue) || c.strict {
					return nil, unsupported()
				}
				return typed.get(name), nil
			case *List:
				value, resolved, err := c.resolveList(typed, name, line, self)
				if err != nil || resolved {
					return value, err
				}
			}
		}
		if provider, ok := object.(*macroProvider); ok {
			return provider.macro(c, name, args, line)
		}
		if value, resolved, err := c.resolveMember(object, name, argValues, line, self); err != nil || resolved {
			return value, err
		}
	}

	if c.strict {
		return nil, NewAttributeError(fmt.Sprintf(
			"Attribute [%s] of [%s] does not exist or can not be accessed and strict variables is set to true.",
			name, javaClassName(object)), name, self.Name, line)
	}
	return nil, nil
}

// resolveMap is io.pebbletemplates.pebble.attributes.MapResolver: the attribute is the key. A
// numeric attribute is cast to the type of the first key of the map, since a Long written in a
// template would never equal the Integer key of a map.
func (c *evalCtx) resolveMap(object *Map, nameValue any, line int, self *Template) (any, error) {
	if len(object.Keys) == 0 && !c.strict {
		return nil, nil
	}
	key := nameValue
	if isNumber(nameValue) {
		if len(object.Keys) == 0 {
			// keySet().iterator().next() on an empty map.
			return nil, &JavaError{Kind: "NoSuchElementException"}
		}
		cast, err := castNumberToKey(nameValue, object.Keys[0], line, self)
		if err != nil {
			return nil, err
		}
		key = cast
	}
	if _, found := object.Vals[key]; c.strict && !found {
		text := stringValueOf(nameValue)
		return nil, NewAttributeError(fmt.Sprintf(
			"Attribute [%s] of [%s] does not exist or can not be accessed and strict variables is set to true.",
			text, javaClassName(object)), text, self.Name, line)
	}
	return object.Vals[key], nil
}

// castNumberToKey is MapResolver.cast: it narrows the number to the type of the map's keys.
func castNumberToKey(number any, key any, line int, self *Template) (any, error) {
	switch key.(type) {
	case int64:
		return numberInt64(number), nil
	case int32:
		return int32(numberInt64(number)), nil
	case float64:
		return numberFloat64(number), nil
	case float32:
		return float32(numberFloat64(number)), nil
	default:
		return nil, &Error{Kind: "PebbleException", File: self.Name, Line: line,
			Message: fmt.Sprintf("type %s not supported for key %s", javaClassLiteral(key),
				stringValueOf(number))}
	}
}

// resolveList is io.pebbletemplates.pebble.attributes.ListResolver: the attribute is an index.
// An attribute that is not an integer leaves the list unresolved, so that the members of
// java.util.List can still be reached.
func (c *evalCtx) resolveList(object *List, name string, line int, self *Template) (any, bool, error) {
	index, ok := javaParseInt(name)
	if !ok {
		return nil, false, nil
	}
	if index < 0 || int(index) >= len(object.Items) {
		if c.strict {
			return nil, true, NewAttributeError(
				"Index out of bounds while accessing array with strict variables on.",
				name, self.Name, line)
		}
		return nil, true, nil
	}
	return object.Items[index], true, nil
}

// resolveMember is the reflective part of the DefaultAttributeResolver. There are no Java beans
// in the context of this port, so only the members of the values of the template value model are
// known: the methods of java.lang.String, java.util.List and java.util.Map, the getters of the
// loop variable, of a map entry, of the template global and of the locale global, and the value
// conversions of java.lang.Number.
//
// Java looks a member up by the attribute name itself, then by the name prefixed with get, is and
// has, and matches method names without regard for case.
func (c *evalCtx) resolveMember(object any, name string, args []any, line int,
	self *Template) (any, bool, error) {
	if name == "" {
		return nil, false, nil
	}
	methods := javaMethods(object)
	if methods == nil {
		return nil, false, nil
	}
	capitalized := strings.ToUpper(name[:1]) + name[1:]
	for _, candidate := range []string{name, "get" + capitalized, "is" + capitalized,
		"has" + capitalized} {
		method, ok := methods[strings.ToLower(candidate)]
		if !ok || method.arity != len(args) {
			continue
		}
		value, err := method.call(c, object, args, line, self)
		if err != nil {
			return nil, true, err
		}
		return value, true, nil
	}
	return nil, false, nil
}

// javaMethod is one member of a value of the template value model.
type javaMethod struct {
	arity int
	call  func(c *evalCtx, object any, args []any, line int, self *Template) (any, error)
}

// javaMethods is the member table of a value, keyed by the lower case Java method name.
func javaMethods(object any) map[string]javaMethod {
	switch object.(type) {
	case string, SafeString:
		return stringMethods
	case *List:
		return listMethods
	case *Map:
		return mapMethods
	case *mapEntry:
		return entryMethods
	case *loopVars:
		return loopMethods
	case *Template:
		return templateMethods
	case Locale:
		return localeMethods
	case bool:
		return boolMethods
	case int32, int64, float32, float64, *big.Int, *BigDecimal:
		return numberMethods
	}
	return nil
}

// receiverString is the string a String member acts on; a SafeString behaves like its content.
func receiverString(object any) string {
	if safe, ok := object.(SafeString); ok {
		return string(safe)
	}
	text, _ := object.(string)
	return text
}

var stringMethods = map[string]javaMethod{
	"length": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return int32(utf16Len(receiverString(o))), nil
	}},
	"isempty": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return receiverString(o) == "", nil
	}},
	"isblank": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return jstr.Strip(receiverString(o)) == "", nil
	}},
	"trim": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return jstr.Trim(receiverString(o)), nil
	}},
	"strip": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return jstr.Strip(receiverString(o)), nil
	}},
	"touppercase": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return jstr.ToUpper(receiverString(o), c.locale), nil
	}},
	"tolowercase": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return jstr.ToLower(receiverString(o), c.locale), nil
	}},
	"tostring": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return receiverString(o), nil
	}},
	"hashcode": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return jstr.HashCode(receiverString(o)), nil
	}},
	"contains": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return strings.Contains(receiverString(o), stringValueOf(a[0])), nil
	}},
	"startswith": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return strings.HasPrefix(receiverString(o), stringValueOf(a[0])), nil
	}},
	"endswith": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return strings.HasSuffix(receiverString(o), stringValueOf(a[0])), nil
	}},
	"concat": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return receiverString(o) + stringValueOf(a[0]), nil
	}},
	"equals": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		other, ok := a[0].(string)
		return ok && other == receiverString(o), nil
	}},
	"indexof": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		at := strings.Index(receiverString(o), stringValueOf(a[0]))
		if at < 0 {
			return int32(-1), nil
		}
		return int32(utf16Len(receiverString(o)[:at])), nil
	}},
	"substring": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		text := receiverString(o)
		return javaSubstring(text, int(numberInt64(a[0])), utf16Len(text))
	}},
	"replace": {2, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return jstr.Replace(receiverString(o), stringValueOf(a[0]), stringValueOf(a[1])), nil
	}},
}

var listMethods = map[string]javaMethod{
	"size": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return int32(len(o.(*List).Items)), nil
	}},
	"isempty": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return len(o.(*List).Items) == 0, nil
	}},
	"get": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		items := o.(*List).Items
		index := int(numberInt64(a[0]))
		if index < 0 || index >= len(items) {
			return nil, &JavaError{Kind: "IndexOutOfBoundsException",
				Message: fmt.Sprintf("Index %d out of bounds for length %d", index, len(items))}
		}
		return items[index], nil
	}},
	"contains": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		for _, item := range o.(*List).Items {
			if Equal(item, a[0]) {
				return true, nil
			}
		}
		return false, nil
	}},
	"indexof": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		for index, item := range o.(*List).Items {
			if Equal(item, a[0]) {
				return int32(index), nil
			}
		}
		return int32(-1), nil
	}},
}

var mapMethods = map[string]javaMethod{
	"size": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return int32(len(o.(*Map).Keys)), nil
	}},
	"isempty": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return len(o.(*Map).Keys) == 0, nil
	}},
	"get": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return o.(*Map).Vals[a[0]], nil
	}},
	"containskey": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		_, found := o.(*Map).Vals[a[0]]
		return found, nil
	}},
}

var entryMethods = map[string]javaMethod{
	"getkey": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return o.(*mapEntry).Key, nil
	}},
	"getvalue": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return o.(*mapEntry).Value, nil
	}},
	"tostring": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return o.(*mapEntry).String(), nil
	}},
}

var loopMethods = map[string]javaMethod{
	"getindex": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return o.(*loopVars).index, nil
	}},
	"getlength": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return o.(*loopVars).length, nil
	}},
	"getrevindex": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return o.(*loopVars).revindex, nil
	}},
	"isfirst": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return o.(*loopVars).first, nil
	}},
	"islast": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return o.(*loopVars).last, nil
	}},
	"tostring": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return o.(*loopVars).String(), nil
	}},
}

var templateMethods = map[string]javaMethod{
	"getname": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return o.(*Template).Name, nil
	}},
	"hasmacro": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		_, found := o.(*Template).macros[stringValueOf(a[0])]
		return found, nil
	}},
	"hasblock": {1, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		_, found := o.(*Template).blocks[stringValueOf(a[0])]
		return found, nil
	}},
}

var localeMethods = map[string]javaMethod{
	"getlanguage": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		tag := string(o.(Locale))
		if at := strings.IndexAny(tag, "-_"); at >= 0 {
			return tag[:at], nil
		}
		return tag, nil
	}},
	"getcountry": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		tag := string(o.(Locale))
		if at := strings.IndexAny(tag, "-_"); at >= 0 {
			return strings.ToUpper(tag[at+1:]), nil
		}
		return "", nil
	}},
	"tostring": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return ToStr(o), nil
	}},
}

var boolMethods = map[string]javaMethod{
	"booleanvalue": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return o.(bool), nil
	}},
	"tostring": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return ToStr(o), nil
	}},
}

var numberMethods = map[string]javaMethod{
	"intvalue": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return int32(numberInt64(o)), nil
	}},
	"longvalue": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return numberInt64(o), nil
	}},
	"doublevalue": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return numberFloat64(o), nil
	}},
	"floatvalue": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return float32(numberFloat64(o)), nil
	}},
	"tostring": {0, func(c *evalCtx, o any, a []any, l int, s *Template) (any, error) {
		return ToStr(o), nil
	}},
}

// javaSubstring is String.substring over UTF-16 code units.
func javaSubstring(text string, begin, end int) (string, error) {
	units := utf16Runes(text)
	if begin < 0 || end > len(units) || begin > end {
		return "", &JavaError{Kind: "StringIndexOutOfBoundsException",
			Message: fmt.Sprintf("begin %d, end %d, length %d", begin, end, len(units))}
	}
	return utf16String(units[begin:end]), nil
}

// javaParseInt is Integer.parseInt, which the list and array resolvers use to decide whether an
// attribute is an index. An overflow is a NumberFormatException, that is: not an index.
func javaParseInt(text string) (int32, bool) {
	value, err := strconv.ParseInt(text, 10, 32)
	if err != nil {
		return 0, false
	}
	return int32(value), true
}

// isNumber reports whether the value is a java.lang.Number.
func isNumber(value any) bool {
	switch value.(type) {
	case int32, int64, float32, float64, *big.Int, *BigDecimal:
		return true
	}
	return false
}

// numberInt64 is Number.longValue.
func numberInt64(value any) int64 {
	switch typed := value.(type) {
	case int32:
		return int64(typed)
	case int64:
		return typed
	case float32:
		return int64(typed)
	case float64:
		return int64(typed)
	case *big.Int:
		return typed.Int64()
	case *BigDecimal:
		return typed.Int64()
	}
	return 0
}

// numberFloat64 is Number.doubleValue.
func numberFloat64(value any) float64 {
	switch typed := value.(type) {
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case float32:
		return float64(typed)
	case float64:
		return typed
	case *big.Int:
		result, _ := new(big.Float).SetInt(typed).Float64()
		return result
	case *BigDecimal:
		return typed.Float64()
	}
	return 0
}

// stringValueOf is String.valueOf(Object).
func stringValueOf(value any) string {
	if value == nil {
		return "null"
	}
	return concatString(value)
}

// javaClassName is Class.getName of the Java class a template value has, which the attribute
// errors print. A null object prints as "null", as the Java conditional in the message does.
func javaClassName(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case bool:
		return "java.lang.Boolean"
	case string:
		return "java.lang.String"
	case Char:
		return "java.lang.Character"
	case SafeString:
		return "io.pebbletemplates.pebble.extension.escaper.SafeString"
	case int32:
		return "java.lang.Integer"
	case int64:
		return "java.lang.Long"
	case float32:
		return "java.lang.Float"
	case float64:
		return "java.lang.Double"
	case *big.Int:
		return "java.math.BigInteger"
	case *BigDecimal:
		return "java.math.BigDecimal"
	case *List:
		return "java.util.ArrayList"
	case *Map:
		if typed.Hash {
			return "java.util.HashMap"
		}
		return "java.util.LinkedHashMap"
	case *mapEntry:
		if typed.hash {
			return "java.util.HashMap$Node"
		}
		return "java.util.LinkedHashMap$Entry"
	case *loopVars:
		return "io.pebbletemplates.pebble.node.ForNode$LoopVariables"
	case *Template:
		return "io.pebbletemplates.pebble.template.PebbleTemplateImpl"
	case *MacroDef:
		return "io.pebbletemplates.pebble.template.Macro"
	case *macroProvider:
		return "io.pebbletemplates.pebble.template.MacroAttributeProvider"
	case *globalContext:
		return "io.pebbletemplates.pebble.template.GlobalContext"
	case Locale:
		return "java.util.Locale"
	}
	return fmt.Sprintf("%T", value)
}

// javaSimpleName is Class.getSimpleName, which the if tag prints for an unsupported value.
func javaSimpleName(value any) string {
	name := javaClassName(value)
	if at := strings.LastIndexAny(name, ".$"); at >= 0 {
		return name[at+1:]
	}
	return name
}

// javaClassLiteral is Class.toString, the form the map resolver prints for an unsupported key
// type.
func javaClassLiteral(value any) string {
	return "class " + javaClassName(value)
}
