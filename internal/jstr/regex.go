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
	"regexp"
	"strings"
	"unicode/utf16"
)

// Java's java.util.regex differs from Go's regexp in the details the split and replace filters expose: the
// dot does not match any of the five line terminators, \s includes the vertical tab, and a malformed pattern
// raises a PatternSyntaxException whose multi-line message, with its "near index" and caret line, is part of the
// output. The translation normalizes the dialect differences those filters expose and reports the JDK's
// message for malformed forms.

// javaPattern is a compiled Java pattern.
type javaPattern struct {
	re    *regexp.Regexp
	empty bool // the pattern matches the empty string at every position
}

// patternError is a java.util.regex.PatternSyntaxException.
func patternError(description, pattern string, index int) error {
	message := description
	if index >= 0 {
		message += fmt.Sprintf(" near index %d", index)
	}
	message += "\n" + pattern
	if index >= 0 {
		message += "\n" + strings.Repeat(" ", index) + "^"
	}
	return &JavaError{Class: "PatternSyntaxException", Message: message}
}

// compileJava translates a Java pattern into a Go one.
func compileJava(pattern string) (*javaPattern, error) {
	var out strings.Builder
	inClass := false
	classStart := -1
	depth := 0
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case c == '\\':
			if i+1 >= len(runes) {
				return nil, patternError("Unexpected internal error", pattern, len(runes))
			}
			i++
			switch runes[i] {
			case 's':
				out.WriteString(classOrBare(inClass, ` \t\n\x0B\f\r`))
			case 'S':
				if inClass {
					out.WriteString(`\S`)
					break
				}
				out.WriteString(`[^ \t\n\x0B\f\r]`)
			case 'p', 'P':
				// Java spells its property names differently from Go; only the posix classes it shares are
				// translated, and the rest keeps Java's own error.
				if i+1 >= len(runes) || runes[i+1] != '{' {
					return nil, patternError("Illegal repetition", pattern, i+1)
				}
				end := indexRune(runes, '}', i+1)
				if end < 0 {
					return nil, patternError("Illegal repetition", pattern, len(runes))
				}
				name := string(runes[i+2 : end])
				translated, ok := javaProperties[name]
				if !ok {
					return nil, patternError("Illegal repetition", pattern, i+1)
				}
				out.WriteString(translated)
				i = end
			default:
				out.WriteRune('\\')
				out.WriteRune(runes[i])
			}
		case inClass:
			if c == ']' {
				inClass = false
			}
			if c == '-' && i+1 < len(runes) && runes[i+1] != ']' && i > classStart+1 {
				if runes[i-1] > runes[i+1] {
					return nil, patternError("Illegal character range", pattern, i+1)
				}
			}
			out.WriteRune(c)
		case c == '[':
			if i+1 < len(runes) && runes[i+1] == ']' {
				return nil, patternError("Unclosed character class", pattern, i+1)
			}
			if indexRune(runes, ']', i) < 0 {
				return nil, patternError("Unclosed character class", pattern, i)
			}
			inClass = true
			classStart = i
			out.WriteRune(c)
		case c == '.':
			// Java's dot excludes the five line terminators unless DOTALL is on, which a template cannot set.
			out.WriteString(`[^\n\r\x{0085}\x{2028}\x{2029}]`)
		case c == '(':
			depth++
			out.WriteRune(c)
			if i+2 < len(runes) && runes[i+1] == '?' && runes[i+2] == '<' {
				if indexRune(runes, '>', i) < 0 {
					return nil, patternError("named capturing group is missing trailing '>'", pattern,
						indexRune(runes, ')', i))
				}
				out.WriteString("?P<")
				i += 2
			}
		case c == ')':
			if depth == 0 {
				return nil, &JavaError{Class: "PatternSyntaxException",
					Message: "Unmatched closing ')'\n" + pattern}
			}
			depth--
			out.WriteRune(c)
		case c == '+' || c == '*' || c == '?':
			if out.Len() == 0 || endsWithQuantifier(out.String(), c) {
				return nil, patternError(fmt.Sprintf("Dangling meta character '%c'", c), pattern, i)
			}
			out.WriteRune(c)
		case c == '{':
			end := indexRune(runes, '}', i)
			if end < 0 {
				if i+1 < len(runes) {
					return nil, patternError("Unclosed counted closure", pattern, len(runes))
				}
				return nil, patternError("Illegal repetition", pattern, i+1)
			}
			if !countedClosure(runes[i : end+1]) {
				return nil, patternError("Illegal repetition", pattern, i+1)
			}
			out.WriteString(string(runes[i : end+1]))
			i = end
		default:
			out.WriteRune(c)
		}
	}
	if inClass {
		return nil, patternError("Unclosed character class", pattern, len(runes))
	}
	if depth > 0 {
		return nil, patternError("Unclosed group", pattern, len(runes))
	}
	re, err := regexp.Compile(out.String())
	if err != nil {
		return nil, patternError("Syntax error", pattern, 0)
	}
	return &javaPattern{re: re, empty: re.MatchString("")}, nil
}

// javaProperties maps the few \p{...} names Java and Go spell the same way onto Go classes.
var javaProperties = map[string]string{
	"Alpha": "[[:alpha:]]", "Digit": "[[:digit:]]", "Alnum": "[[:alnum:]]", "Upper": "[[:upper:]]",
	"Lower": "[[:lower:]]", "Space": "[[:space:]]", "Punct": "[[:punct:]]", "L": `\pL`, "N": `\pN`,
	"Lu": `\p{Lu}`, "Ll": `\p{Ll}`,
}

func classOrBare(inClass bool, body string) string {
	if inClass {
		return body
	}
	return "[" + body + "]"
}

func indexRune(runes []rune, want rune, from int) int {
	for i := from + 1; i < len(runes); i++ {
		if runes[i] == '\\' {
			i++
			continue
		}
		if runes[i] == want {
			return i
		}
	}
	return -1
}

// endsWithQuantifier reports whether a quantifier would follow another one, which Java rejects except for the
// lazy and possessive forms ("a*?" and "a*+" are legal, "a**" is not).
func endsWithQuantifier(sofar string, next rune) bool {
	if sofar == "" {
		return true
	}
	last := sofar[len(sofar)-1]
	if last != '*' && last != '+' && last != '?' {
		return false
	}
	return next != '?' && next != '+'
}

func countedClosure(runes []rune) bool {
	body := string(runes[1 : len(runes)-1])
	if body == "" {
		return false
	}
	for _, c := range body {
		if (c < '0' || c > '9') && c != ',' {
			return false
		}
	}
	return body[0] != ','
}

// Split is String.split(regex, limit). Java drops a zero-width match at the start of the input, keeps at most
// limit parts when limit is positive, removes the trailing empty parts when limit is zero, and keeps them when
// limit is negative.
func Split(s, regex string, limit int) ([]string, error) {
	pattern, err := compileJava(regex)
	if err != nil {
		return nil, err
	}
	units := utf16.Encode([]rune(s))
	var parts []string
	at := 0
	for offset := 0; offset <= len(units); {
		if limit > 0 && len(parts) == limit-1 {
			break
		}
		loc := pattern.re.FindStringIndex(string(utf16.Decode(units[offset:])))
		if loc == nil {
			break
		}
		start, end := offset+unitsIn(string(utf16.Decode(units[offset:])), loc[0]),
			offset+unitsIn(string(utf16.Decode(units[offset:])), loc[1])
		if start == end {
			// A zero-width match at the very beginning never produces a leading empty part, and elsewhere it
			// advances by one code unit.
			if start == 0 {
				offset = 1
				continue
			}
			if start >= len(units) {
				break
			}
			parts = append(parts, string(utf16.Decode(units[at:start])))
			at, offset = start, start+1
			continue
		}
		parts = append(parts, string(utf16.Decode(units[at:start])))
		at, offset = end, end
	}
	if at == 0 && len(parts) == 0 {
		return []string{s}, nil
	}
	parts = append(parts, string(utf16.Decode(units[at:])))
	if limit == 0 {
		for len(parts) > 0 && parts[len(parts)-1] == "" {
			parts = parts[:len(parts)-1]
		}
	}
	return parts, nil
}

// unitsIn is the number of UTF-16 code units in the first byteLen bytes of s.
func unitsIn(s string, byteLen int) int {
	return Length(s[:byteLen])
}

// ReplaceAll is String.replaceAll(regex, replacement), with Java's $group and backslash escapes in the
// replacement.
func ReplaceAll(s, regex, replacement string) (string, error) {
	pattern, err := compileJava(regex)
	if err != nil {
		return "", err
	}
	var fault error
	out := pattern.re.ReplaceAllStringFunc(s, func(match string) string {
		groups := pattern.re.FindStringSubmatch(match)
		expanded, err := expandReplacement(replacement, groups)
		if err != nil && fault == nil {
			fault = err
		}
		return expanded
	})
	if fault != nil {
		return "", fault
	}
	return out, nil
}

func expandReplacement(replacement string, groups []string) (string, error) {
	var out strings.Builder
	runes := []rune(replacement)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\\':
			if i+1 >= len(runes) {
				return "", &JavaError{Class: "IllegalArgumentException",
					Message: "character to be escaped is missing"}
			}
			i++
			out.WriteRune(runes[i])
		case '$':
			number := 0
			digits := 0
			for i+1 < len(runes) && runes[i+1] >= '0' && runes[i+1] <= '9' {
				next := number*10 + int(runes[i+1]-'0')
				if next >= len(groups) && digits > 0 {
					break
				}
				number, digits = next, digits+1
				i++
			}
			if digits == 0 {
				return "", &JavaError{Class: "IllegalArgumentException",
					Message: "Illegal group reference"}
			}
			if number >= len(groups) {
				return "", &JavaError{Class: "IndexOutOfBoundsException",
					Message: fmt.Sprintf("No group %d", number)}
			}
			out.WriteString(groups[number])
		default:
			out.WriteRune(runes[i])
		}
	}
	return out.String(), nil
}

// Replace is String.replace(target, replacement): a literal replacement, with no regular expression.
func Replace(s, target, replacement string) string {
	return strings.ReplaceAll(s, target, replacement)
}
