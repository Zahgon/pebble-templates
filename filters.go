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
	"strconv"
	"strings"

	"github.com/PebbleTemplates/pebble/internal/jnum"
	"github.com/PebbleTemplates/pebble/internal/jstr"
	"github.com/PebbleTemplates/pebble/internal/jtime"
)

// defaultDatePattern is the pattern DateFilter falls back to, both for parsing a String input and
// for formatting the result.
const defaultDatePattern = "yyyy-MM-dd'T'HH:mm:ssZ"

func init() {
	Filters["abbreviate"] = Filter{ArgNames: []string{"length"}, Apply: abbreviateFilter}
	Filters["abs"] = Filter{Apply: absFilter}
	Filters["capitalize"] = Filter{Apply: capitalizeFilter}
	Filters["date"] = Filter{
		ArgNames: []string{"format", "existingFormat", "timeZone"}, Apply: dateFilter}
	Filters["default"] = Filter{ArgNames: []string{"default"}, Apply: defaultFilter}
	Filters["first"] = Filter{Apply: firstFilter}
	// FormatFilter declares an empty argument name list, so its arguments arrive numbered.
	Filters["format"] = Filter{ArgNames: []string{}, Apply: formatFilter}
	Filters["join"] = Filter{ArgNames: []string{"separator"}, Apply: joinFilter}
	Filters["last"] = Filter{Apply: lastFilter}
	Filters["lower"] = Filter{Apply: lowerFilter}
	Filters["numberformat"] = Filter{ArgNames: []string{"format"}, Apply: numberFormatFilter}
	Filters["slice"] = Filter{ArgNames: []string{"fromIndex", "toIndex"}, Apply: sliceFilter}
	Filters["sort"] = Filter{Apply: sortFilter}
	Filters["rsort"] = Filter{Apply: rsortFilter}
	Filters["reverse"] = Filter{Apply: reverseFilter}
	Filters["title"] = Filter{Apply: titleFilter}
	Filters["trim"] = Filter{Apply: trimFilter}
	Filters["upper"] = Filter{Apply: upperFilter}
	Filters["urlencode"] = Filter{Apply: urlEncodeFilter}
	Filters["length"] = Filter{Apply: lengthFilter}
	Filters["replace"] = Filter{ArgNames: []string{"replace_pairs"}, Apply: replaceFilter}
	Filters["merge"] = Filter{ArgNames: []string{"items"}, Apply: mergeFilter}
	Filters["split"] = Filter{ArgNames: []string{"delimiter", "limit"}, Apply: splitFilter}
	Filters["base64encode"] = Filter{Apply: base64EncodeFilter}
	Filters["base64decode"] = Filter{Apply: base64DecodeFilter}
	Filters["sha256"] = Filter{Apply: sha256Filter}
	Filters["nl2br"] = Filter{Apply: nl2brFilter}
	// The escaper extension's two filters.
	Filters["escape"] = Filter{ArgNames: []string{"strategy"}, Apply: escapeFilter}
	Filters["raw"] = Filter{Apply: rawFilter}
}

// asString is the (String) cast the string filters apply to their input. The filter expression has
// already replaced a SafeString input with its text, so only a genuinely wrong type fails here.
func asString(input any) (string, error) {
	if s, ok := input.(string); ok {
		return s, nil
	}
	return "", castError(input, "java.lang.String")
}

// abbreviateFilter is AbbreviateFilter.
func abbreviateFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	value, err := asString(input)
	if err != nil {
		return nil, err
	}
	length, ok := args["length"].(int64)
	if !ok {
		// The filter casts the argument to a Long before calling intValue on it.
		if args["length"] == nil {
			return nil, &JavaError{Kind: "NullPointerException",
				Message: "Cannot invoke \"java.lang.Long.intValue()\" because the return value of " +
					"\"java.util.Map.get(Object)\" is null"}
		}
		return nil, castError(args["length"], "java.lang.Long")
	}
	maxWidth := int(int32(length))
	if maxWidth < 0 {
		return nil, NewError("Invalid argument to abbreviate filter; must be greater than zero",
			ctx.Name(), line)
	}
	return jstr.Abbreviate(value, maxWidth)
}

// absFilter is AbsFilter, which keeps the type of its input.
func absFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	switch v := input.(type) {
	case nil:
		return nil, NewError("Can not pass null value to \"abs\" filter.", ctx.Name(), line)
	case int32:
		// Math.abs of Integer.MIN_VALUE is Integer.MIN_VALUE again.
		if v == math.MinInt32 {
			return v, nil
		}
		if v < 0 {
			return -v, nil
		}
		return v, nil
	case float32:
		return float32(math.Abs(float64(v))), nil
	case int64:
		if v == math.MinInt64 {
			return v, nil
		}
		if v < 0 {
			return -v, nil
		}
		return v, nil
	case float64:
		return math.Abs(v), nil
	case *BigDecimal:
		return v.Abs(), nil
	case *big.Int:
		return new(big.Int).Abs(v), nil
	default:
		return nil, NewError("The 'abs' filter does require as input a number.", ctx.Name(), line)
	}
}

// capitalizeFilter is CapitalizeFilter: the first non-whitespace character becomes title case.
func capitalizeFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	value, err := asString(input)
	if err != nil {
		return nil, err
	}
	if value == "" {
		return value, nil
	}
	return jstr.Capitalize(value, ctx.Locale()), nil
}

// dateFilter is DateFilter. The value space of the reference command holds no TemporalAccessor, so
// only the java.util.Date branch of the filter can be reached.
func dateFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	format, err := optionalString(args, "format")
	if err != nil {
		return nil, err
	}
	timeZone, err := optionalString(args, "timeZone")
	if err != nil {
		return nil, err
	}
	existingFormat, err := optionalString(args, "existingFormat")
	if err != nil {
		return nil, err
	}

	var date jtime.Millis
	switch v := input.(type) {
	case string:
		pattern := defaultDatePattern
		if existingFormat != nil {
			pattern = *existingFormat
		}
		parsed, parseErr := jtime.Parse(v, pattern, ctx.Locale(), ctx.Zone())
		if parseErr != nil {
			return nil, &Error{Kind: "PebbleException",
				Message: "Could not parse the string '" + v + "' into a date, with formatting: " + pattern,
				File:    ctx.Name(), Line: line, Cause: parseErr}
		}
		date = parsed
	default:
		if _, isNumber := javaDoubleValue(input); !isNumber {
			return nil, &JavaError{Kind: "IllegalArgumentException",
				Message: "Unsupported argument type: " + javaClassName(input) +
					" (value: " + valueOf(input) + ")"}
		}
		date = jtime.Millis(javaLongValue(input))
	}

	pattern := defaultDatePattern
	if format != nil {
		pattern = *format
	}
	zone := ctx.Zone()
	if timeZone != nil {
		zone = *timeZone
	}
	text, err := jtime.Format(date, pattern, ctx.Locale(), zone)
	if err != nil {
		return nil, err
	}
	return SafeString(text), nil
}

// optionalString is the (String) cast of an argument that may be absent or null.
func optionalString(args map[string]any, name string) (*string, error) {
	value, present := args[name]
	if !present || value == nil {
		return nil, nil
	}
	s, ok := value.(string)
	if !ok {
		return nil, castError(value, "java.lang.String")
	}
	return &s, nil
}

// defaultFilter is DefaultFilter, which replaces anything the empty test considers empty.
func defaultFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if isEmptyValue(input) {
		return args["default"], nil
	}
	return input, nil
}

// firstFilter is FirstFilter.
func firstFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	switch v := input.(type) {
	case nil:
		return nil, nil
	case string:
		return charAt(v, 0)
	case *List:
		if len(v.Items) > 0 {
			return v.Items[0], nil
		}
		return nil, nil
	default:
		return nil, castError(input, "java.util.Collection")
	}
}

// lastFilter is LastFilter.
func lastFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	switch v := input.(type) {
	case nil:
		return nil, nil
	case string:
		return charAt(v, jstr.Length(v)-1)
	case *List:
		if len(v.Items) > 0 {
			return v.Items[len(v.Items)-1], nil
		}
		return nil, nil
	default:
		return nil, castError(input, "java.util.Collection")
	}
}

func charAt(s string, index int) (any, error) {
	unit, err := jstr.CharAt(s, index)
	if err != nil {
		return nil, err
	}
	return Char(unit), nil
}

// formatFilter is FormatFilter: the input is the format string and the arguments arrive numbered
// from zero, so they are read back in that order.
func formatFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	values := make([]any, 0, len(args))
	for i := 0; i < len(args); i++ {
		values = append(values, args[strconv.Itoa(i)])
	}
	return jstr.Format(ctx.Locale(), valueOf(input), values)
}

// joinFilter is JoinFilter.
func joinFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	glue := ""
	if value, present := args["separator"]; present && value != nil {
		s, ok := value.(string)
		if !ok {
			return nil, castError(value, "java.lang.String")
		}
		glue = s
	}
	items, ok := input.(*List)
	if !ok {
		return nil, NewError(
			"The 'join' filter expects that the input is either a collection or an array.",
			ctx.Name(), line)
	}
	var b strings.Builder
	for i, item := range items.Items {
		if i > 0 {
			b.WriteString(glue)
		}
		b.WriteString(valueOf(item))
	}
	return b.String(), nil
}

// lowerFilter is LowerFilter, upperFilter is UpperFilter; both accept any input through toString.
func lowerFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	return jstr.ToLower(valueOf(input), ctx.Locale()), nil
}

func upperFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	return jstr.ToUpper(valueOf(input), ctx.Locale()), nil
}

// numberFormatFilter is NumberFormatFilter.
func numberFormatFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	if _, isNumber := javaDoubleValue(input); !isNumber {
		return nil, NewError("The input for the 'NumberFormat' filter has to be a number.",
			ctx.Name(), line)
	}
	if pattern, present := args["format"]; present && pattern != nil {
		text, ok := pattern.(string)
		if !ok {
			return nil, castError(pattern, "java.lang.String")
		}
		return jnum.Format(text, ctx.Locale(), input)
	}
	return jnum.FormatDefault(ctx.Locale(), input), nil
}

// sliceFilter is SliceFilter.
func sliceFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}

	from := 0
	if argFrom, present := args["fromIndex"]; present && argFrom != nil {
		if _, isNumber := javaDoubleValue(argFrom); !isNumber {
			return nil, NewError("Argument fromIndex must be a number. Actual type: "+
				javaClassName(argFrom), ctx.Name(), line)
		}
		from = int(javaIntValue(argFrom))
	}
	if from < 0 {
		return nil, &JavaError{Kind: "IllegalArgumentException",
			Message: "fromIndex must be greater than 0"}
	}

	argTo, hasTo := args["toIndex"]
	if hasTo && argTo != nil {
		if _, isNumber := javaDoubleValue(argTo); !isNumber {
			return nil, NewError("Argument toIndex must be a number. Actual type: "+
				javaClassName(argTo), ctx.Name(), line)
		}
	} else {
		argTo = nil
	}

	var length int
	switch v := input.(type) {
	case *List:
		length = len(v.Items)
	case string:
		length = jstr.Length(v)
	default:
		return nil, NewError(
			"Slice filter can only be applied to String, List and array inputs. Actual type was: "+
				javaClassName(input), ctx.Name(), line)
	}

	to := length
	if argTo != nil {
		to = int(javaIntValue(argTo))
		if to > length {
			return nil, NewError("toIndex must be smaller than input size: "+strconv.Itoa(length),
				ctx.Name(), line)
		}
		if from >= to {
			return nil, NewError("toIndex must be greater than fromIndex", ctx.Name(), line)
		}
	}

	if items, ok := input.(*List); ok {
		if from > to {
			return nil, &JavaError{Kind: "IllegalArgumentException",
				Message: "fromIndex(" + strconv.Itoa(from) + ") > toIndex(" + strconv.Itoa(to) + ")"}
		}
		return &List{Items: append([]any{}, items.Items[from:to]...)}, nil
	}
	text, err := jstr.Substring(input.(string), from, to)
	if err != nil {
		return nil, err
	}
	return text, nil
}

// sortFilter is SortFilter: java.util.Collections.sort with the natural ordering, in place.
func sortFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	items, ok := input.(*List)
	if !ok {
		return nil, NewError("Unsupported input type for sort filter", ctx.Name(), line)
	}
	if err := timSort(items.Items, compareNatural); err != nil {
		return nil, err
	}
	return items, nil
}

// rsortFilter is RsortFilter: List.sort with Collections.reverseOrder, which swaps the operands of
// every comparison, in place.
func rsortFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	items, ok := input.(*List)
	if !ok {
		return nil, castError(input, "java.util.List")
	}
	reverse := func(a, b any) (int, error) { return compareNatural(b, a) }
	if err := timSort(items.Items, reverse); err != nil {
		return nil, err
	}
	return items, nil
}

// reverseFilter is ReverseFilter: java.util.Collections.reverse, in place.
func reverseFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	items, ok := input.(*List)
	if !ok {
		return nil, castError(input, "java.util.List")
	}
	for i, j := 0, len(items.Items)-1; i < j; i, j = i+1, j-1 {
		items.Items[i], items.Items[j] = items.Items[j], items.Items[i]
	}
	return items, nil
}

// titleFilter is TitleFilter: every character that follows whitespace becomes title case.
func titleFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	value, err := asString(input)
	if err != nil {
		return nil, err
	}
	if value == "" {
		return value, nil
	}
	return jstr.Title(value, ctx.Locale()), nil
}

// trimFilter is TrimFilter: String.trim, which only drops characters up to U+0020.
func trimFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	value, err := asString(input)
	if err != nil {
		return nil, err
	}
	return jstr.Trim(value), nil
}

// urlEncodeFilter is UrlEncoderFilter: java.net.URLEncoder, so a space becomes a plus sign.
func urlEncodeFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	value, err := asString(input)
	if err != nil {
		return nil, err
	}
	return jstr.URLEncode(value), nil
}

// lengthFilter is LengthFilter, which reports an Integer and falls back to zero for anything that
// has no length at all.
func lengthFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	switch v := input.(type) {
	case nil:
		return int32(0), nil
	case string:
		return int32(jstr.Length(v)), nil
	case *List:
		return int32(len(v.Items)), nil
	case *Map:
		return int32(len(v.Keys)), nil
	default:
		return int32(0), nil
	}
}

// replaceFilter is ReplaceFilter: every entry of the map replaces a literal occurrence, in the
// map's iteration order.
func replaceFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	pairs, present := args["replace_pairs"]
	if !present || pairs == nil {
		return nil, NewError("The argument 'replace_pairs' is required.", ctx.Name(), line)
	}
	replacements, ok := pairs.(*Map)
	if !ok {
		return nil, castError(pairs, "java.util.Map")
	}
	data := valueOf(input)
	for _, key := range replacements.Keys {
		data = jstr.Replace(data, valueOf(key), valueOf(replacements.Vals[key]))
	}
	return data, nil
}

// mergeFilter is MergeFilter: the left hand side decides the type of the result.
func mergeFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	items := args["items"]
	if input == nil {
		if items == nil {
			return nil, NewError("The two arguments to be merged are null", ctx.Name(), line)
		}
		return items, nil
	}
	if items == nil {
		return input, nil
	}
	switch left := input.(type) {
	case *Map:
		return mergeAsMap(left, items)
	case *List:
		return mergeAsList(left, items, ctx, line)
	default:
		return nil, NewError("The object being filtered is not a Map/List/Array", ctx.Name(), line)
	}
}

// mergeAsMap builds the java.util.HashMap the filter creates, with the capacity it asks for, so
// that the result iterates in the same bucket order as the Java one.
func mergeAsMap(left *Map, right any) (any, error) {
	switch other := right.(type) {
	case *Map:
		out := newHashMap(len(left.Keys) + len(other.Keys) + 16)
		for _, key := range left.Keys {
			out.put(key, left.Vals[key])
		}
		for _, key := range other.Keys {
			out.put(key, other.Vals[key])
		}
		return out.build(), nil
	case *List:
		out := newHashMap(len(left.Keys) + len(other.Items) + 16)
		for _, key := range left.Keys {
			out.put(key, left.Vals[key])
		}
		for _, item := range other.Items {
			out.put(item, item)
		}
		return out.build(), nil
	default:
		return nil, &JavaError{Kind: "UnsupportedOperationException",
			Message: "Currently, only Maps and Lists can be merged with a Map. Arg2: " +
				javaClassName(right)}
	}
}

func mergeAsList(left *List, right any, ctx Ctx, line int) (any, error) {
	switch other := right.(type) {
	case *Map:
		items := append([]any{}, left.Items...)
		for _, key := range other.Keys {
			items = append(items, &mapEntry{Key: key, Value: other.Vals[key], hash: other.Hash})
		}
		return &List{Items: items}, nil
	case *List:
		items := append([]any{}, left.Items...)
		return &List{Items: append(items, other.Items...)}, nil
	default:
		return nil, NewError("Currently, only Maps and Lists can be merged with a List. Arg2: "+
			javaClassName(right), ctx.Name(), line)
	}
}

// splitFilter is SplitFilter. Java reports a String[]; this port reports a list, which behaves
// the same everywhere the array could be used but prints as "[a, b]" rather than as the identity
// hash code of an array.
func splitFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	delimiter, err := optionalString(args, "delimiter")
	if err != nil {
		return nil, err
	}
	if delimiter == nil {
		return nil, NewError("missing delimiter parameter in split filter", ctx.Name(), line)
	}
	limit := 0
	if value, present := args["limit"]; present && value != nil {
		if _, isNumber := javaDoubleValue(value); !isNumber {
			return nil, castError(value, "java.lang.Number")
		}
		limit = int(javaIntValue(value))
	}
	value, err := asString(input)
	if err != nil {
		return nil, err
	}
	parts, err := jstr.Split(value, *delimiter, limit)
	if err != nil {
		return nil, err
	}
	items := make([]any, len(parts))
	for i, part := range parts {
		items[i] = part
	}
	return &List{Items: items}, nil
}

// base64EncodeFilter is Base64EncoderFilter.
func base64EncodeFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	return jstr.Base64Encode(valueOf(input)), nil
}

// base64DecodeFilter is Base64DecoderFilter.
func base64DecodeFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	value, ok := input.(string)
	if !ok {
		return nil, NewError("This filter applies to String\n", ctx.Name(), line)
	}
	decoded, err := jstr.Base64Decode(value)
	if err != nil {
		return nil, &Error{Kind: "PebbleException",
			Message: "Please provide a correctly Base64 encoded string containing an UTF-8 string\n",
			File:    ctx.Name(), Line: line, Cause: err}
	}
	return decoded, nil
}

// sha256Filter is Sha256Filter.
func sha256Filter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	value, ok := input.(string)
	if !ok {
		return nil, NewError("Need a string to hash\n", ctx.Name(), line)
	}
	return jstr.Sha256Hex(value), nil
}

// nl2brFilter is Nl2brFilter: every CR, LF or CRLF becomes one "<br />".
func nl2brFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	value, ok := input.(string)
	if !ok {
		return nil, &JavaError{Kind: "IllegalArgumentException",
			Message: "nl2br filters only supports String input."}
	}
	if !strings.ContainsAny(value, "\n\r") {
		return value, nil
	}
	var b strings.Builder
	b.Grow(len(value) + 16)
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\r':
			b.WriteString("<br />")
			if i+1 < len(value) && value[i+1] == '\n' {
				i++
			}
		case '\n':
			b.WriteString("<br />")
		default:
			b.WriteByte(value[i])
		}
	}
	return b.String(), nil
}

// escapeFilter is io.pebbletemplates.pebble.extension.escaper.EscapeFilter. A SafeString input is
// already escaped, so it passes through; the filter expression is the one that keeps a SafeString
// from being flattened to a String before this filter sees it.
func escapeFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	if safe, ok := input.(SafeString); ok {
		return safe, nil
	}
	_, strategy := ctx.Escaping()
	if strategy == "" {
		strategy = htmlEscapeStrategy
	}
	if value, present := args["strategy"]; present && value != nil {
		text, ok := value.(string)
		if !ok {
			return nil, castError(value, "java.lang.String")
		}
		strategy = text
	}
	escaped, err := Escape(ToStr(input), strategy)
	if err != nil {
		return nil, NewError("Unknown escaping strategy ["+strategy+"]", ctx.Name(), line)
	}
	return SafeString(escaped), nil
}

// rawFilter is RawFilter, which simply declares its input safe.
func rawFilter(input any, args map[string]any, ctx Ctx, line int) (any, error) {
	if input == nil {
		return nil, nil
	}
	return SafeString(valueOf(input)), nil
}

// timSort sorts a slice in place the way java.util.Arrays.sort sorts an Object[]: it finds the
// initial run, reversing it when it is descending, and then runs a binary insertion sort. That is
// exactly java.util.ComparableTimSort for fewer than 32 elements, which is where the order of the
// comparisons - and so the ClassCastException a list of mixed types reports - is visible. Longer
// lists take the same path here, which sorts them identically but can report the cast failure of
// a different pair than the Java merge sort would.
func timSort(items []any, compare func(a, b any) (int, error)) error {
	if len(items) < 2 {
		return nil
	}
	runLen, err := countRunAndMakeAscending(items, compare)
	if err != nil {
		return err
	}
	return binarySort(items, runLen, compare)
}

func countRunAndMakeAscending(items []any, compare func(a, b any) (int, error)) (int, error) {
	runHi := 1
	if runHi == len(items) {
		return 1, nil
	}
	order, err := compare(items[runHi], items[runHi-1])
	if err != nil {
		return 0, err
	}
	runHi++
	if order < 0 {
		for runHi < len(items) {
			order, err = compare(items[runHi], items[runHi-1])
			if err != nil {
				return 0, err
			}
			if order >= 0 {
				break
			}
			runHi++
		}
		for i, j := 0, runHi-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	} else {
		for runHi < len(items) {
			order, err = compare(items[runHi], items[runHi-1])
			if err != nil {
				return 0, err
			}
			if order < 0 {
				break
			}
			runHi++
		}
	}
	return runHi, nil
}

func binarySort(items []any, start int, compare func(a, b any) (int, error)) error {
	if start == 0 {
		start = 1
	}
	for ; start < len(items); start++ {
		pivot := items[start]
		left, right := 0, start
		for left < right {
			mid := int(uint(left+right) >> 1)
			order, err := compare(pivot, items[mid])
			if err != nil {
				return err
			}
			if order < 0 {
				right = mid
			} else {
				left = mid + 1
			}
		}
		copy(items[left+1:start+1], items[left:start])
		items[left] = pivot
	}
	return nil
}
