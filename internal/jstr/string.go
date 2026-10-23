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
	"fmt"
	"strings"
	"unicode"
	"unicode/utf16"
)

// Length is String.length: the number of UTF-16 code units.
func Length(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// Units returns the UTF-16 code units of a Java String.
func Units(s string) []uint16 { return utf16.Encode([]rune(s)) }

// FromUnits builds a string from Java UTF-16 code units, replacing unpaired surrogates in the
// same way as the rest of the port's UTF-8 boundary.
func FromUnits(units []uint16) string { return string(utf16.Decode(units)) }

// Substring is String.substring(begin, end) over UTF-16 code units. Out of range is a
// StringIndexOutOfBoundsException, whose JDK 21 text names both bounds and the length.
func Substring(s string, begin, end int) (string, error) {
	units := Units(s)
	if begin < 0 || end > len(units) || begin > end {
		return "", &JavaError{Class: "StringIndexOutOfBoundsException",
			Message: fmt.Sprintf("begin %d, end %d, length %d", begin, end, len(units))}
	}
	return FromUnits(units[begin:end]), nil
}

// CharAt is String.charAt as a UTF-16 code unit.
func CharAt(s string, index int) (uint16, error) {
	units := Units(s)
	if index < 0 || index >= len(units) {
		return 0, &JavaError{Class: "StringIndexOutOfBoundsException",
			Message: fmt.Sprintf("Index %d out of bounds for length %d", index, len(units))}
	}
	return units[index], nil
}

// JavaError is a Java exception this package reports: Class is the simple class name the command prints and
// Message the exception's own message.
type JavaError struct {
	Class   string
	Message string
}

func (e *JavaError) Error() string { return e.Message }

// ToUpper is String.toUpperCase(Locale) and ToLower String.toLowerCase(Locale): the full Unicode mappings, where
// one code point can become several (ß -> SS, ŉ -> ʼN, the final sigma, and the dotted-I rules of Turkish,
// Azeri and Lithuanian).
func ToUpper(s, locale string) string { return mapCase(s, locale, true) }

// ToLower is String.toLowerCase(Locale).
func ToLower(s, locale string) string { return mapCase(s, locale, false) }

func mapCase(s, locale string, up bool) string {
	language := locale
	if at := strings.IndexAny(locale, "-_"); at >= 0 {
		language = locale[:at]
	}
	var exceptions []caseException
	switch strings.ToLower(language) {
	case "tr":
		exceptions = pick(turkishUpper, turkishLower, up)
	case "az":
		exceptions = pick(azeriUpper, azeriLower, up)
	case "lt":
		exceptions = pick(lithuanianUpper, lithuanianLower, up)
	default:
		exceptions = pick(rootUpper, rootLower, up)
	}
	runes := []rune(s)
	var units []uint16
	for i, r := range runes {
		if !up && r == 'Σ' && finalSigma(runes, i) {
			// Java's one conditional rule that a table cannot hold: a capital sigma at the end of a word
			// becomes the final form.
			units = append(units, 'ς')
			continue
		}
		if mapped, ok := exception(exceptions, r); ok {
			units = append(units, mapped...)
			continue
		}
		if up {
			units = append(units, utf16.Encode([]rune{unicode.ToUpper(r)})...)
		} else {
			units = append(units, utf16.Encode([]rune{unicode.ToLower(r)})...)
		}
	}
	return string(utf16.Decode(units))
}

func pick(upper, lower []caseException, up bool) []caseException {
	if up {
		return upper
	}
	return lower
}

func exception(exceptions []caseException, r rune) ([]uint16, bool) {
	for _, each := range exceptions {
		if each.from == r {
			return each.to, true
		}
	}
	return nil, false
}

// finalSigma reports the Final_Sigma condition of the Unicode case rules: the sigma is preceded by a cased
// letter and not followed by one.
func finalSigma(runes []rune, at int) bool {
	before := false
	for i := at - 1; i >= 0; i-- {
		if isCaseIgnorable(runes[i]) {
			continue
		}
		before = isCased(runes[i])
		break
	}
	if !before {
		return false
	}
	for i := at + 1; i < len(runes); i++ {
		if isCaseIgnorable(runes[i]) {
			continue
		}
		return !isCased(runes[i])
	}
	return true
}

func isCased(r rune) bool {
	return unicode.IsUpper(r) || unicode.IsLower(r) || unicode.Is(unicode.Lt, r)
}

func isCaseIgnorable(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) ||
		unicode.Is(unicode.Lm, r) || unicode.Is(unicode.Sk, r)
}

// UpperCodePoint and LowerCodePoint are Character.toUpperCase(int) and toLowerCase(int), the one-to-one mappings,
// which differ from the string ones.
func UpperCodePoint(r rune) rune { return unicode.ToUpper(r) }

// LowerCodePoint is Character.toLowerCase(int).
func LowerCodePoint(r rune) rune { return unicode.ToLower(r) }

// titleCodePoint is Character.toTitleCase(int), which the capitalize and title filters use: the upper case
// mapping, except for the digraphs that have a title case form of their own.
func titleCodePoint(r rune) rune {
	if title, ok := javaTitleCase[r]; ok {
		return title
	}
	return unicode.ToUpper(r)
}

// Trim is String.trim, which strips every code unit up to and including the space, and Strip is String.strip,
// which strips Unicode whitespace.
func Trim(s string) string {
	units := utf16.Encode([]rune(s))
	start, end := 0, len(units)
	for start < end && units[start] <= ' ' {
		start++
	}
	for end > start && units[end-1] <= ' ' {
		end--
	}
	return string(utf16.Decode(units[start:end]))
}

// Strip is String.strip: it removes the characters Character.isWhitespace accepts.
func Strip(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return javaWhitespace[r] })
}

// CompareTo is String.compareTo: lexicographic over UTF-16 code units, which orders an astral character before
// U+E000..U+FFFF, unlike a comparison of the UTF-8 bytes.
func CompareTo(a, b string) int {
	left, right := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] != right[i] {
			return int(left[i]) - int(right[i])
		}
	}
	return len(left) - len(right)
}

// HashCode is String.hashCode, the 31-based hash over UTF-16 code units; java.util.HashMap's iteration order
// depends on it.
func HashCode(s string) int32 {
	var hash int32
	for _, unit := range utf16.Encode([]rune(s)) {
		hash = 31*hash + int32(unit)
	}
	return hash
}

// Abbreviate is the abbreviate filter's algorithm (AbbreviateFilter), which counts UTF-16 units and may cut a
// surrogate pair in half.
func Abbreviate(s string, maxWidth int) (string, error) {
	length := Length(s)
	if length < maxWidth || length <= 3 {
		return s, nil
	}
	if maxWidth <= 3 {
		return Substring(s, 0, maxWidth)
	}
	head, err := Substring(s, 0, max(0, maxWidth-3))
	if err != nil {
		return "", err
	}
	return head + "...", nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Capitalize is the capitalize filter (CapitalizeFilter): the first character that is not whitespace becomes its
// title case form and the rest of the string is kept as it is. It works on UTF-16 units, so the title case of an
// astral character's leading surrogate is the surrogate itself.
func Capitalize(s, locale string) string {
	units := utf16.Encode([]rune(s))
	for i, unit := range units {
		if javaWhitespace[rune(unit)] {
			continue
		}
		out := append([]uint16{}, units[:i]...)
		out = append(out, utf16.Encode([]rune{titleCodePoint(rune(unit))})...)
		out = append(out, units[i+1:]...)
		return string(utf16.Decode(out))
	}
	return s
}

// Title is the title filter (TitleFilter): every character after whitespace becomes its title case form.
func Title(s, locale string) string {
	units := utf16.Encode([]rune(s))
	out := make([]uint16, 0, len(units))
	next := true
	for _, unit := range units {
		switch {
		case javaWhitespace[rune(unit)]:
			next = true
		case next:
			out = append(out, utf16.Encode([]rune{titleCodePoint(rune(unit))})...)
			next = false
			continue
		}
		out = append(out, unit)
	}
	return string(utf16.Decode(out))
}
