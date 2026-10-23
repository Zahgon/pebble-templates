/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package jstr

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/PebbleTemplates/pebble/internal/jnum"
)

// localeSymbols are the java.text.DecimalFormatSymbols fields java.util.Formatter consults: the
// zero digit (every digit is written as zeroDigit+value, which shapes the Arabic and Persian
// locales), the decimal separator and the grouping separator. Formatter never uses the locale's
// minus sign, infinity or NaN symbol: it hard codes "-", "Infinity" and "NaN".
type localeSymbols struct {
	zero    rune
	decimal rune
	group   rune
}

// localeTable holds the symbols of the locales Pebble's reference command can be asked for,
// taken from the JDK with DecimalFormatSymbols.getInstance. The grouping size is 3 in all of
// them, hi-IN included: the JDK's hi-IN pattern is "#,##0.###", not lakh grouping.
var localeTable = map[string]localeSymbols{
	"en-US": {'0', '.', ','},
	"en":    {'0', '.', ','},
	"de-DE": {'0', ',', '.'},
	"de":    {'0', ',', '.'},
	"fr-FR": {'0', ',', ' '}, // a narrow no-break space, not U+00A0
	"fr":    {'0', ',', ' '},
	"tr-TR": {'0', ',', '.'},
	"tr":    {'0', ',', '.'},
	"ja-JP": {'0', '.', ','},
	"ja":    {'0', '.', ','},
	"hi-IN": {'0', '.', ','},
	"ar-EG": {'٠', '٫', '٬'},
	"fa-IR": {'۰', '٫', '٬'},
}

// rootSymbols is the fallback for a locale the table does not know, which is what the JDK's
// Locale.forLanguageTag gives such a tag: the root locale's symbols.
var rootSymbols = localeSymbols{'0', '.', ','}

// symbolsFor resolves a BCP 47 tag to its symbols, falling back from "de-AT" to "de" and then
// to the root locale.
func symbolsFor(locale string) localeSymbols {
	if s, ok := localeTable[locale]; ok {
		return s
	}
	if i := strings.IndexAny(locale, "-_"); i > 0 {
		if s, ok := localeTable[locale[:i]]; ok {
			return s
		}
	}
	return rootSymbols
}

// isTurkic reports whether the locale uses the dotted/dotless I case rules, the only case where
// the uppercase conversions can turn ASCII into non-ASCII.
func isTurkic(locale string) bool {
	l := strings.ToLower(locale)
	return strings.HasPrefix(l, "tr") || strings.HasPrefix(l, "az")
}

// Format is String.format(Locale, format, args...) with java.util.Formatter. It supports the
// conversions s S d o x X e E f g G a A b B c h H n and %%, the flags '-', '#', '+', ' ', '0',
// ',' and '(', a width, a precision, an explicit argument index ("%1$s") and the previous
// argument flag ("%<s"). Every failure is the JDK's exception as an error, with the JDK's
// message text.
//
// The arguments stand for the boxed Java types: string for String, bool for Boolean, int32 for
// Integer, int64 for Long, float64 for Double, *big.Int for BigInteger, *jnum.BigDecimal for
// BigDecimal and nil for null.
func Format(locale, format string, args []any) (string, error) {
	f := &formatter{locale: locale, sym: symbolsFor(locale)}
	var out strings.Builder
	i, last, lastOrdinary := 0, -1, -1
	for i < len(format) {
		n := strings.IndexByte(format[i:], '%')
		if n < 0 {
			out.WriteString(format[i:])
			break
		}
		out.WriteString(format[i : i+n])
		i += n
		sp, next, err := parseSpecifier(format, i)
		if err != nil {
			return "", err
		}
		i = next
		// Resolve the argument the way Formatter's index bookkeeping does: an explicit index
		// does not move the ordinary cursor, and '<' reuses the previous specifier's argument.
		var arg any
		var have bool
		switch {
		case sp.conv == 'n' || sp.conv == '%':
		case sp.has('<'):
			if last < 0 || last >= len(args) {
				return "", missingArg(sp)
			}
			arg, have = args[last], true
		case sp.index > 0:
			if sp.index-1 >= len(args) {
				return "", missingArg(sp)
			}
			last = sp.index - 1
			arg, have = args[last], true
		default:
			lastOrdinary++
			if lastOrdinary >= len(args) {
				return "", missingArg(sp)
			}
			last = lastOrdinary
			arg, have = args[last], true
		}
		s, err := f.print(sp, arg, have)
		if err != nil {
			return "", err
		}
		out.WriteString(s)
	}
	return out.String(), nil
}

// formatter carries the per-call locale state.
type formatter struct {
	locale string
	sym    localeSymbols
}

// specifier is one parsed format specifier.
type specifier struct {
	index int    // the explicit argument index, 0 when absent
	flags string // the flags in the order they were written
	width int    // -1 when absent
	prec  int    // -1 when absent
	conv  byte
}

// has reports whether the flag was given.
func (s *specifier) has(flag byte) bool { return strings.IndexByte(s.flags, flag) >= 0 }

// flagOrder is the order Formatter's Flags.toString uses when it echoes the flags back in an
// exception message, which is not the order they were written in.
const flagOrder = "-#+ 0,(<"

// canonicalFlags spells the flags the way Flags.toString does.
func (s *specifier) canonicalFlags() string {
	var b strings.Builder
	for i := 0; i < len(flagOrder); i++ {
		if s.has(flagOrder[i]) {
			b.WriteByte(flagOrder[i])
		}
	}
	return b.String()
}

// String reconstructs the specifier the way FormatSpecifier.toString does: the flags come before
// the argument index, and the conversion keeps the case it was written in.
func (s *specifier) String() string {
	var b strings.Builder
	b.WriteByte('%')
	b.WriteString(strings.ReplaceAll(s.canonicalFlags(), "<", "<"))
	if s.index > 0 {
		b.WriteString(strconv.Itoa(s.index))
		b.WriteByte('$')
	}
	if s.width != -1 {
		b.WriteString(strconv.Itoa(s.width))
	}
	if s.prec != -1 {
		b.WriteByte('.')
		b.WriteString(strconv.Itoa(s.prec))
	}
	b.WriteByte(s.conv)
	return b.String()
}

// isFlag reports whether c is one of Formatter's flag characters.
func isFlag(c byte) bool { return strings.IndexByte("-#+ 0,(<", c) >= 0 }

// parseSpecifier parses one specifier starting at the '%' at position i, mirroring Formatter's
// "%(\d+\$)?([-#+ 0,(\<]*)?(\d+)?(\.\d+)?([tT])?([a-zA-Z%])" pattern. A specifier that does not
// match is an UnknownFormatConversionException naming the character right after the '%'.
func parseSpecifier(format string, i int) (*specifier, int, error) {
	p := i + 1
	if p >= len(format) {
		return nil, 0, unknownConversion("%")
	}
	bad := unknownConversion(string(format[p]))
	sp := &specifier{width: -1, prec: -1}
	// An explicit argument index is digits followed by '$'; without the '$' the digits are the
	// width, so the scan has to be able to back out.
	start := p
	for p < len(format) && format[p] >= '0' && format[p] <= '9' {
		p++
	}
	if p > start && p < len(format) && format[p] == '$' {
		n, ok := parseJavaInt(format[start:p])
		if !ok || n <= 0 {
			return nil, 0, &FormatError{"IllegalFormatArgumentIndexException",
				fmt.Sprintf("Illegal format argument index = %d", n)}
		}
		sp.index = n
		p++
	} else {
		p = start
	}
	fstart := p
	for p < len(format) && isFlag(format[p]) {
		if strings.IndexByte(format[fstart:p], format[p]) >= 0 {
			return nil, 0, &FormatError{"DuplicateFormatFlagsException",
				fmt.Sprintf("Flags = '%c'", format[p])}
		}
		p++
	}
	sp.flags = format[fstart:p]
	start = p
	for p < len(format) && format[p] >= '0' && format[p] <= '9' {
		p++
	}
	if p > start {
		n, _ := parseJavaInt(format[start:p])
		sp.width = n
	}
	if p < len(format) && format[p] == '.' {
		p++
		start = p
		for p < len(format) && format[p] >= '0' && format[p] <= '9' {
			p++
		}
		if p == start {
			return nil, 0, bad
		}
		n, _ := parseJavaInt(format[start:p])
		sp.prec = n
	}
	if p >= len(format) {
		return nil, 0, bad
	}
	c := format[p]
	if c != '%' && !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
		return nil, 0, bad
	}
	if !strings.ContainsRune("sSdoxXeEfgGaAbBcChHn%", rune(c)) {
		return nil, 0, unknownConversion(string(c))
	}
	sp.conv = c
	return sp, p + 1, nil
}

// parseJavaInt parses a run of digits with Formatter's own wrapping arithmetic: an overflowing
// width wraps to a negative int rather than failing, which is how "%2147483648s" ends up
// reporting a width of -2147483648.
func parseJavaInt(digits string) (int, bool) {
	var n int32
	for i := 0; i < len(digits); i++ {
		n = n*10 + int32(digits[i]-'0')
	}
	return int(n), len(digits) > 0
}

// FormatError is a java.util.IllegalFormatException: the simple name of the JDK exception class
// and its message.
type FormatError struct {
	Class   string
	Message string
}

// Error renders the error the way the JDK's toString would, as "Class: message".
func (e *FormatError) Error() string { return e.Class + ": " + e.Message }

func unknownConversion(c string) error {
	return &FormatError{"UnknownFormatConversionException", "Conversion = '" + c + "'"}
}

func missingArg(sp *specifier) error {
	return &FormatError{"MissingFormatArgumentException", "Format specifier '" + sp.String() + "'"}
}

func mismatch(conv byte, flag byte) error {
	return &FormatError{"FormatFlagsConversionMismatchException",
		fmt.Sprintf("Conversion = %c, Flags = %c", conv, flag)}
}

func badFlags(sp *specifier) error {
	return &FormatError{"IllegalFormatFlagsException", "Flags = '" + sp.canonicalFlags() + "'"}
}

func missingWidth(sp *specifier) error {
	return &FormatError{"MissingFormatWidthException", sp.String()}
}

func badPrecision(p int) error {
	return &FormatError{"IllegalFormatPrecisionException", strconv.Itoa(p)}
}

func badWidth(w int) error {
	return &FormatError{"IllegalFormatWidthException", strconv.Itoa(w)}
}

func badConversion(conv byte, arg any) error {
	return &FormatError{"IllegalFormatConversionException",
		fmt.Sprintf("%c != %s", lowerConv(conv), javaClassName(arg))}
}

// lowerConv is the lowercase conversion character, which is what
// IllegalFormatConversionException always reports even for "%X" or "%E".
func lowerConv(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}

// javaClassName is the fully qualified name of the Java class an argument stands for.
func javaClassName(arg any) string {
	switch arg.(type) {
	case string:
		return "java.lang.String"
	case bool:
		return "java.lang.Boolean"
	case int32:
		return "java.lang.Integer"
	case int64:
		return "java.lang.Long"
	case float64:
		return "java.lang.Double"
	case *big.Int:
		return "java.math.BigInteger"
	case *jnum.BigDecimal:
		return "java.math.BigDecimal"
	}
	return fmt.Sprintf("%T", arg)
}

// print renders one specifier.
func (f *formatter) print(sp *specifier, arg any, have bool) (string, error) {
	switch sp.conv {
	case 'n':
		if sp.prec != -1 {
			return "", badPrecision(sp.prec)
		}
		if sp.width != -1 {
			return "", badWidth(sp.width)
		}
		if sp.flags != "" {
			return "", badFlags(sp)
		}
		return "\n", nil
	case '%':
		if sp.flags != "" && sp.flags != "-" {
			return "", badFlags(sp)
		}
		if sp.prec != -1 {
			return "", badPrecision(sp.prec)
		}
		if sp.width == -1 && sp.has('-') {
			return "", missingWidth(sp)
		}
		return f.pad(sp, "%"), nil
	case 's', 'S', 'b', 'B', 'h', 'H':
		return f.printGeneral(sp, arg)
	case 'c', 'C':
		return f.printCharacter(sp, arg)
	case 'd', 'o', 'x', 'X':
		return f.printIntegral(sp, arg)
	default:
		return f.printFloat(sp, arg)
	}
}

// upper applies the uppercase conversions' toUpperCase(locale). An ASCII string in a
// non-Turkic locale can be mapped inline; anything else needs the full Java mapping.
func (f *formatter) upper(s string) string {
	if isTurkic(f.locale) {
		return ToUpper(s, f.locale)
	}
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return ToUpper(s, f.locale)
		}
	}
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 32
		}
	}
	return string(b)
}

// printGeneral handles s, S, b, B, h and H.
func (f *formatter) printGeneral(sp *specifier, arg any) (string, error) {
	conv := lowerConv(sp.conv)
	if (conv == 'b' || conv == 'h') && sp.has('#') {
		return "", mismatch(conv, '#')
	}
	if sp.width == -1 && sp.has('-') {
		return "", missingWidth(sp)
	}
	for _, bad := range []byte{'+', ' ', '0', ',', '('} {
		if sp.has(bad) {
			return "", mismatch(conv, bad)
		}
	}
	var s string
	switch conv {
	case 'b':
		// Only null and Boolean.FALSE are false; every other object is "true".
		if arg == nil {
			s = "false"
		} else if b, ok := arg.(bool); ok {
			s = strconv.FormatBool(b)
		} else {
			s = "true"
		}
	case 'h':
		if arg == nil {
			s = "null"
		} else {
			s = strconv.FormatUint(uint64(uint32(javaHashCode(arg))), 16)
		}
	default:
		if sp.has('#') {
			return "", mismatch(conv, '#')
		}
		s = javaToString(arg)
	}
	if sp.prec != -1 {
		s = truncateUTF16(s, sp.prec)
	}
	if sp.conv >= 'A' && sp.conv <= 'Z' {
		s = f.upper(s)
	}
	return f.pad(sp, s), nil
}

// javaToString is the Java toString of a boxed value, which is what "%s" prints.
func javaToString(arg any) string {
	switch v := arg.(type) {
	case nil:
		return "null"
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return jnum.DoubleString(v)
	case *big.Int:
		return v.String()
	case *jnum.BigDecimal:
		return v.String()
	}
	return fmt.Sprint(arg)
}

// javaHashCode is the hashCode of a boxed value, which "%h" prints in hexadecimal.
func javaHashCode(arg any) int32 {
	switch v := arg.(type) {
	case string:
		var h int32
		for _, u := range utf16.Encode([]rune(v)) {
			h = 31*h + int32(u)
		}
		return h
	case bool:
		if v {
			return 1231
		}
		return 1237
	case int32:
		return v
	case int64:
		return int32(v ^ (v >> 32))
	case float64:
		bits := math.Float64bits(v)
		if math.IsNaN(v) {
			bits = 0x7ff8000000000000
		}
		return int32(bits ^ (bits >> 32))
	case *big.Int:
		// BigInteger.hashCode walks the magnitude as unsigned 32 bit words, then applies the
		// sign.
		var h int32
		mag := new(big.Int).Abs(v).Bytes()
		for len(mag)%4 != 0 && len(mag) > 0 {
			mag = append([]byte{0}, mag...)
		}
		for i := 0; i+3 < len(mag); i += 4 {
			w := uint32(mag[i])<<24 | uint32(mag[i+1])<<16 | uint32(mag[i+2])<<8 | uint32(mag[i+3])
			h = int32(31*int64(h) + int64(w))
		}
		return h * int32(v.Sign())
	case *jnum.BigDecimal:
		return v.HashCode()
	}
	return 0
}

// printCharacter handles c and C. Only Integer (and null) reach it from Pebble; a Long or a
// String is a conversion error, as in Java.
func (f *formatter) printCharacter(sp *specifier, arg any) (string, error) {
	if sp.prec != -1 {
		return "", badPrecision(sp.prec)
	}
	if sp.width == -1 && sp.has('-') {
		return "", missingWidth(sp)
	}
	for _, bad := range []byte{'#', '+', ' ', '0', ',', '('} {
		if sp.has(bad) {
			return "", mismatch(lowerConv(sp.conv), bad)
		}
	}
	var s string
	switch v := arg.(type) {
	case nil:
		s = "null"
	case int32:
		if v < 0 || v > 0x10FFFF || (v >= 0xD800 && v <= 0xDFFF) {
			return "", &FormatError{"IllegalFormatCodePointException",
				fmt.Sprintf("Code point = 0x%x", uint32(v))}
		}
		s = string(rune(v))
	default:
		return "", badConversion(sp.conv, arg)
	}
	if sp.conv == 'C' {
		s = f.upper(s)
	}
	return f.pad(sp, s), nil
}

// checkNumeric runs Formatter's checkNumeric in its order: an overflowed width, a negative
// precision, then the flags that need a width, then the illegal flag combinations.
func checkNumeric(sp *specifier) error {
	if sp.width != -1 && sp.width < 0 {
		return badWidth(sp.width)
	}
	if sp.prec != -1 && sp.prec < 0 {
		return badPrecision(sp.prec)
	}
	if sp.width == -1 && (sp.has('-') || sp.has('0')) {
		return missingWidth(sp)
	}
	if (sp.has('+') && sp.has(' ')) || (sp.has('-') && sp.has('0')) {
		return badFlags(sp)
	}
	return nil
}

// printIntegral handles d, o, x and X.
func (f *formatter) printIntegral(sp *specifier, arg any) (string, error) {
	if err := checkNumeric(sp); err != nil {
		return "", err
	}
	if sp.conv == 'd' && sp.has('#') {
		return "", mismatch('d', '#')
	}
	if sp.conv != 'd' && sp.has(',') {
		return "", mismatch(lowerConv(sp.conv), ',')
	}
	if sp.prec != -1 {
		return "", badPrecision(sp.prec)
	}
	if arg == nil {
		return f.padNull(sp), nil
	}
	switch v := arg.(type) {
	case int32:
		return f.printLong(sp, int64(v), 32)
	case int64:
		return f.printLong(sp, v, 64)
	case *big.Int:
		return f.printBigInt(sp, v)
	}
	return "", badConversion(sp.conv, arg)
}

// printLong renders an Integer or a Long. For the hexadecimal and octal conversions Java prints
// the two's complement unsigned value, whose width depends on the Java type, and rejects the
// sign flags.
func (f *formatter) printLong(sp *specifier, v int64, bits int) (string, error) {
	var sign, digits string
	switch sp.conv {
	case 'd':
		if v < 0 {
			if sp.has('(') {
				sign = "("
			} else {
				sign = "-"
			}
			digits = strconv.FormatUint(uint64(-v), 10)
			if v == math.MinInt64 {
				digits = strings.TrimPrefix(strconv.FormatInt(v, 10), "-")
			}
		} else {
			digits = strconv.FormatInt(v, 10)
			if sp.has('+') {
				sign = "+"
			} else if sp.has(' ') {
				sign = " "
			}
		}
		if sp.has(',') {
			digits = f.group(digits)
		}
	default:
		for _, bad := range []byte{'(', ' ', '+'} {
			if sp.has(bad) {
				return "", mismatch(lowerConv(sp.conv), bad)
			}
		}
		u := uint64(v)
		if bits == 32 {
			u = uint64(uint32(int32(v)))
		}
		base := 16
		if sp.conv == 'o' {
			base = 8
		}
		digits = strconv.FormatUint(u, base)
		if sp.has('#') {
			if sp.conv == 'o' {
				digits = "0" + digits
			} else {
				digits = "0x" + digits
			}
		}
	}
	return f.finishNumeric(sp, sign, digits, sp.conv == 'd'), nil
}

// printBigInt renders a BigInteger. Unlike a Long, a negative BigInteger keeps a minus sign in
// the hexadecimal and octal conversions, and the sign flags are allowed there.
func (f *formatter) printBigInt(sp *specifier, v *big.Int) (string, error) {
	neg := v.Sign() < 0
	abs := new(big.Int).Abs(v)
	var sign, digits string
	if neg {
		if sp.has('(') {
			sign = "("
		} else {
			sign = "-"
		}
	} else if sp.has('+') {
		sign = "+"
	} else if sp.has(' ') {
		sign = " "
	}
	switch sp.conv {
	case 'd':
		digits = abs.String()
		if sp.has(',') {
			digits = f.group(digits)
		}
	case 'o':
		digits = abs.Text(8)
		if sp.has('#') {
			digits = "0" + digits
		}
	default:
		digits = abs.Text(16)
		if sp.has('#') {
			digits = "0x" + digits
		}
	}
	return f.finishNumeric(sp, sign, digits, sp.conv == 'd'), nil
}

// group inserts the locale's grouping separator every three digits.
func (f *formatter) group(digits string) string {
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	head := len(digits) % 3
	if head == 0 {
		head = 3
	}
	b.WriteString(digits[:head])
	for i := head; i < len(digits); i += 3 {
		b.WriteRune(f.group0())
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

func (f *formatter) group0() rune { return f.sym.group }

// finishNumeric applies localisation, the zero padding (which goes between the sign and the
// digits and is never grouped), the closing parenthesis and the width.
func (f *formatter) finishNumeric(sp *specifier, sign, digits string, localize bool) string {
	if localize {
		digits = f.localizeDigits(digits)
	} else {
		digits = f.localizeDigits(digits)
	}
	tail := ""
	if sign == "(" {
		tail = ")"
	}
	if sp.has('0') && sp.width != -1 {
		n := sp.width - utf16Len(sign) - utf16Len(digits) - utf16Len(tail)
		for ; n > 0; n-- {
			digits = string(f.sym.zero) + digits
		}
	}
	s := sign + digits + tail
	if sp.conv >= 'A' && sp.conv <= 'Z' {
		s = f.upper(s)
	}
	return f.pad(sp, s)
}

// localizeDigits maps the ASCII digits and the '.' and ',' placeholders to the locale's symbols.
func (f *formatter) localizeDigits(s string) string {
	if f.sym.zero == '0' && f.sym.decimal == '.' && f.sym.group == ',' {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(f.sym.zero + (r - '0'))
		case r == '.':
			b.WriteRune(f.sym.decimal)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// padNull renders a null argument, which every conversion accepts: the word "null", with the
// precision and width of a string.
func (f *formatter) padNull(sp *specifier) string {
	s := "null"
	if sp.prec != -1 {
		s = truncateUTF16(s, sp.prec)
	}
	if sp.conv >= 'A' && sp.conv <= 'Z' {
		s = f.upper(s)
	}
	return f.pad(sp, s)
}

// pad applies the width, counting UTF-16 code units as Java does.
func (f *formatter) pad(sp *specifier, s string) string {
	if sp.width == -1 {
		return s
	}
	n := sp.width - utf16Len(s)
	if n <= 0 {
		return s
	}
	if sp.has('-') {
		return s + strings.Repeat(" ", n)
	}
	return strings.Repeat(" ", n) + s
}

// utf16Len is the Java String.length of s: the number of UTF-16 code units.
func utf16Len(s string) int {
	return Length(s)
}

// truncateUTF16 is s.substring(0, n) over UTF-16 code units.
func truncateUTF16(s string, n int) string {
	if Length(s) <= n {
		return s
	}
	truncated, _ := Substring(s, 0, n)
	return truncated
}

// printFloat handles e, E, f, g, G, a and A.
func (f *formatter) printFloat(sp *specifier, arg any) (string, error) {
	if err := checkNumeric(sp); err != nil {
		return "", err
	}
	conv := lowerConv(sp.conv)
	switch conv {
	case 'e':
		if sp.has(',') {
			return "", mismatch('e', ',')
		}
	case 'g':
		if sp.has('#') {
			return "", mismatch('g', '#')
		}
	case 'a':
		if sp.has(',') {
			return "", mismatch('a', ',')
		}
		if sp.has('(') {
			return "", mismatch('a', '(')
		}
	}
	if arg == nil {
		return f.padNull(sp), nil
	}
	if conv == 'a' {
		v, ok := arg.(float64)
		if !ok {
			return "", badConversion(sp.conv, arg)
		}
		return f.printHexFloat(sp, v)
	}
	switch v := arg.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return f.printSpecialFloat(sp, v), nil
		}
		digits, decExp := shortestDigits(math.Abs(v))
		return f.printDecimal(sp, digits, decExp, math.Signbit(v)), nil
	case *jnum.BigDecimal:
		// A BigDecimal carries its own exact digits, so no shortest representation is involved.
		digits, decExp := bigDecimalDigits(v)
		return f.printDecimal(sp, digits, decExp, v.Signum() < 0), nil
	}
	return "", badConversion(sp.conv, arg)
}

// printSpecialFloat renders an infinity or a NaN. Formatter hard codes these words and ignores
// the locale's symbols; the uppercase conversions uppercase them.
func (f *formatter) printSpecialFloat(sp *specifier, v float64) string {
	var sign, s string
	if math.IsNaN(v) {
		s = "NaN"
	} else {
		s = "Infinity"
		if v < 0 {
			if sp.has('(') {
				sign, s = "(", "Infinity)"
			} else {
				sign = "-"
			}
		} else if sp.has('+') {
			sign = "+"
		} else if sp.has(' ') {
			sign = " "
		}
	}
	out := sign + s
	if sp.conv >= 'A' && sp.conv <= 'Z' {
		out = f.upper(out)
	}
	return f.pad(sp, out)
}

// shortestDigits returns the decimal digits of the shortest representation of a non-negative
// finite double (the digits of Double.toString) and the exponent such that the value is
// 0.<digits> * 10^exp.
//
// Java's Formatter rounds this shortest representation, not the exact binary value, which is why
// "%.2f" of 2.675 is "2.68" (the exact double is 2.67499999...) and why "%.20f" of 0.1 is
// 0.10000000000000000000 rather than the exact binary expansion.
func shortestDigits(v float64) (string, int) {
	s := strconv.FormatFloat(v, 'e', -1, 64)
	i := strings.IndexByte(s, 'e')
	mant, exp := s[:i], s[i+1:]
	e, _ := strconv.Atoi(exp)
	mant = strings.Replace(mant, ".", "", 1)
	return mant, e + 1
}

// bigDecimalDigits returns the exact digits of a BigDecimal's magnitude and the exponent such
// that the value is 0.<digits> * 10^exp.
func bigDecimalDigits(b *jnum.BigDecimal) (string, int) {
	digits := new(big.Int).Abs(b.Unscaled()).String()
	if digits == "0" {
		return "0", 1
	}
	return digits, len(digits) - int(b.Scale())
}

// roundHalfUp rounds a digit string to n significant digits with RoundingMode.HALF_UP, which is
// what Formatter's applyPrecision does. It returns the rounded digits and 1 when the rounding
// carried into a new leading digit (so the caller must raise its exponent).
func roundHalfUp(d string, n int) (string, int) {
	if n <= 0 {
		if n == 0 && len(d) > 0 && d[0] >= '5' {
			return "1", 1
		}
		return "", 0
	}
	if n >= len(d) {
		return d + strings.Repeat("0", n-len(d)), 0
	}
	keep := []byte(d[:n])
	if d[n] >= '5' {
		i := n - 1
		for ; i >= 0; i-- {
			if keep[i] == '9' {
				keep[i] = '0'
			} else {
				keep[i]++
				break
			}
		}
		if i < 0 {
			return "1" + string(keep[:n-1]), 1
		}
	}
	return string(keep), 0
}

// printDecimal renders the f, e, E, g and G conversions from a digit string.
func (f *formatter) printDecimal(sp *specifier, digits string, decExp int, neg bool) string {
	conv := lowerConv(sp.conv)
	prec := sp.prec
	var body string
	switch conv {
	case 'f':
		if prec == -1 {
			prec = 6
		}
		body = f.fixed(sp, digits, decExp, prec, sp.has(','))
	case 'e':
		if prec == -1 {
			prec = 6
		}
		body = f.scientific(sp, digits, decExp, prec)
	default: // g and G
		if prec == -1 {
			prec = 6
		} else if prec == 0 {
			// Formatter silently treats a precision of 0 as 1 for %g.
			prec = 1
		}
		rd, carry := roundHalfUp(digits, prec)
		exp := decExp + carry
		if rd == "" {
			exp = 1
		}
		if exp <= -4 || exp > prec {
			body = f.scientific(sp, digits, decExp, prec-1)
		} else {
			// Grouping is honoured in the plain form and silently dropped in the other.
			body = f.fixed(sp, digits, decExp, prec-exp, sp.has(','))
		}
	}
	var sign, tail string
	if neg {
		if sp.has('(') {
			sign, tail = "(", ")"
		} else {
			sign = "-"
		}
	} else if sp.has('+') {
		sign = "+"
	} else if sp.has(' ') {
		sign = " "
	}
	if sp.has('0') && sp.width != -1 {
		n := sp.width - utf16Len(sign) - utf16Len(body) - utf16Len(tail)
		for ; n > 0; n-- {
			body = string(f.sym.zero) + body
		}
	}
	out := sign + body + tail
	if sp.conv >= 'A' && sp.conv <= 'Z' {
		out = f.upper(out)
	}
	return f.pad(sp, out)
}

// fixed renders the plain form with prec digits after the decimal separator.
func (f *formatter) fixed(sp *specifier, digits string, decExp, prec int, grouped bool) string {
	n := decExp + prec
	rd, carry := roundHalfUp(digits, n)
	exp := decExp + carry
	if rd == "" {
		rd, exp = "0", 1
	}
	var intPart, frac string
	if exp <= 0 {
		intPart = "0"
		frac = strings.Repeat("0", -exp) + rd
	} else {
		if len(rd) < exp {
			rd += strings.Repeat("0", exp-len(rd))
		}
		intPart, frac = rd[:exp], rd[exp:]
	}
	if len(frac) < prec {
		frac += strings.Repeat("0", prec-len(frac))
	}
	frac = frac[:prec]
	if grouped {
		intPart = f.group(intPart)
	}
	s := intPart
	if prec > 0 || sp.has('#') {
		s += "." + frac
	}
	return f.localizeDigits(s)
}

// scientific renders the computerised scientific form with prec digits after the decimal
// separator and a two digit exponent, as Formatter does.
func (f *formatter) scientific(sp *specifier, digits string, decExp, prec int) string {
	rd, carry := roundHalfUp(digits, prec+1)
	exp := decExp + carry - 1
	if strings.Trim(rd, "0") == "" {
		exp = 0
	}
	s := rd[:1]
	if prec > 0 {
		s += "." + rd[1:]
	} else if sp.has('#') {
		s += "."
	}
	s = f.localizeDigits(s)
	e := "e"
	sign := "+"
	if exp < 0 {
		sign = "-"
		exp = -exp
	}
	es := strconv.Itoa(exp)
	if len(es) < 2 {
		es = "0" + es
	}
	return s + e + sign + f.localizeDigits(es)
}

// printHexFloat is the %a conversion: Java's Double.toHexString layout, which keeps a "0x1."
// prefix for a normal value and "0x0." with an exponent of -1022 for a subnormal, writes the
// exponent in decimal with no sign for a positive exponent, and is never localised.
func (f *formatter) printHexFloat(sp *specifier, v float64) (string, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return f.printSpecialFloat(sp, v), nil
	}
	var sign string
	if math.Signbit(v) {
		sign = "-"
	} else if sp.has('+') {
		sign = "+"
	} else if sp.has(' ') {
		sign = " "
	}
	body := hexFloatBody(math.Abs(v))
	if sp.has('0') && sp.width != -1 {
		n := sp.width - utf16Len(sign) - utf16Len(body)
		for ; n > 0; n-- {
			body = "0" + body
		}
	}
	out := sign + body
	if sp.conv == 'A' {
		out = strings.ToUpper(out)
	}
	return f.pad(sp, out), nil
}

// hexFloatBody spells the magnitude of a finite double as Double.toHexString does.
func hexFloatBody(v float64) string {
	bits := math.Float64bits(v)
	expBits := int((bits >> 52) & 0x7FF)
	mant := bits & 0xFFFFFFFFFFFFF
	lead, exp := "1", expBits-1023
	if expBits == 0 {
		lead, exp = "0", -1022
	}
	hex := strings.TrimRight(fmt.Sprintf("%013x", mant), "0")
	if hex == "" {
		hex = "0"
	}
	return "0x" + lead + "." + hex + "p" + strconv.Itoa(exp)
}

// ensure errors is used for the package's sentinel-free error construction.
var _ = errors.New
