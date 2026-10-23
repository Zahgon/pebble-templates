/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package cli

import (
	"fmt"
	"math/big"
	"strings"
	"unicode/utf16"

	"github.com/PebbleTemplates/pebble"
	"github.com/PebbleTemplates/pebble/internal/jnum"
)

// jsonError is a malformed context; the command prints it as "context error: <message>".
type jsonError struct {
	message string
}

func (e *jsonError) Error() string { return e.message }

// jsonReader reads a context document. It follows the reference command's reader, so the values are the Java ones:
// objects become a LinkedHashMap (a java.util.HashMap with hashMaps), arrays an ArrayList, strings String,
// true/false Boolean, null null, whole numbers Long (BigInteger when they do not fit a long) and numbers with a
// fraction or an exponent Double, or BigDecimal with bigDecimals.
type jsonReader struct {
	text        []uint16 // the document as UTF-16 code units, because the offsets of its errors count those
	at          int
	bigDecimals bool
	hashMaps    bool
}

func newJSONReader(text string, bigDecimals, hashMaps bool) *jsonReader {
	return &jsonReader{text: utf16.Encode([]rune(text)), bigDecimals: bigDecimals, hashMaps: hashMaps}
}

// parse reads one value and reports trailing text, as the reference command's reader does.
func (r *jsonReader) parse() (value any, err error) {
	defer func() {
		if raised := recover(); raised != nil {
			fault, ok := raised.(*jsonError)
			if !ok {
				panic(raised)
			}
			value, err = nil, fault
		}
	}()
	value = r.value()
	r.space()
	if r.at < len(r.text) {
		r.fail("trailing text at offset %d", r.at)
	}
	return value, nil
}

func (r *jsonReader) fail(format string, args ...any) {
	panic(&jsonError{message: fmt.Sprintf(format, args...)})
}

func (r *jsonReader) space() {
	for r.at < len(r.text) {
		switch r.text[r.at] {
		case ' ', '\t', '\n', '\r':
			r.at++
		default:
			return
		}
	}
}

func (r *jsonReader) peek() uint16 {
	if r.at >= len(r.text) {
		r.fail("unexpected end of input")
	}
	return r.text[r.at]
}

func (r *jsonReader) expect(c uint16) {
	if r.peek() != c {
		r.fail("expected '%s' at offset %d", unit(c), r.at)
	}
	r.at++
}

func (r *jsonReader) value() any {
	r.space()
	switch c := r.peek(); c {
	case '{':
		return r.object()
	case '[':
		return r.array()
	case '"':
		return r.str()
	case 't':
		r.word("true")
		return true
	case 'f':
		r.word("false")
		return false
	case 'n':
		r.word("null")
		return nil
	default:
		return r.number()
	}
}

func (r *jsonReader) word(word string) {
	if !hasPrefixAt(r.text, r.at, word) {
		r.fail("unexpected character '%s' at offset %d", unit(r.peek()), r.at)
	}
	r.at += len(word)
}

func (r *jsonReader) object() *pebble.Map {
	m := &pebble.Map{Vals: map[any]any{}, Hash: r.hashMaps}
	r.expect('{')
	r.space()
	if r.peek() == '}' {
		r.at++
		return m
	}
	for {
		r.space()
		key := r.str()
		r.space()
		r.expect(':')
		// java.util.Map.put keeps the position a repeated key already had.
		if _, seen := m.Vals[any(key)]; !seen {
			m.Keys = append(m.Keys, any(key))
		}
		m.Vals[any(key)] = r.value()
		r.space()
		c := r.peek()
		r.at++
		if c == '}' {
			return m
		}
		if c != ',' {
			r.fail("expected ',' or '}' at offset %d", r.at-1)
		}
	}
}

func (r *jsonReader) array() *pebble.List {
	list := &pebble.List{}
	r.expect('[')
	r.space()
	if r.peek() == ']' {
		r.at++
		return list
	}
	for {
		list.Items = append(list.Items, r.value())
		r.space()
		c := r.peek()
		r.at++
		if c == ']' {
			return list
		}
		if c != ',' {
			r.fail("expected ',' or ']' at offset %d", r.at-1)
		}
	}
}

func (r *jsonReader) str() string {
	r.expect('"')
	var out []uint16
	for {
		c := r.peek()
		r.at++
		if c == '"' {
			return string(utf16.Decode(out))
		}
		if c != '\\' {
			out = append(out, c)
			continue
		}
		escape := r.peek()
		r.at++
		switch escape {
		case '"', '\\', '/':
			out = append(out, escape)
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			if r.at+4 > len(r.text) {
				r.fail("unexpected end of input")
			}
			hex := string(utf16.Decode(r.text[r.at : r.at+4]))
			value, err := parseHex16(hex)
			if err != nil {
				r.fail("bad \\u escape '%s' at offset %d", hex, r.at)
			}
			out = append(out, value)
			r.at += 4
		default:
			r.fail("bad escape '\\%s' at offset %d", unit(escape), r.at-1)
		}
	}
}

func (r *jsonReader) number() any {
	start := r.at
	if r.at < len(r.text) && r.text[r.at] == '-' {
		r.at++
	}
	fraction := false
	for r.at < len(r.text) {
		c := r.text[r.at]
		if c >= '0' && c <= '9' {
			r.at++
			continue
		}
		if c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
			fraction = true
			r.at++
			continue
		}
		break
	}
	literal := string(utf16.Decode(r.text[start:r.at]))
	if literal == "" || literal == "-" {
		r.fail("unexpected character '%s' at offset %d", unit(r.text[start]), start)
	}
	if fraction {
		if r.bigDecimals {
			value, err := jnum.ParseBigDecimal(literal)
			if err != nil {
				r.fail("bad number '%s' at offset %d", literal, start)
			}
			return value
		}
		value, err := jnum.ParseDouble(literal)
		if err != nil {
			r.fail("bad number '%s' at offset %d", literal, start)
		}
		return value
	}
	whole, ok := new(big.Int).SetString(literal, 10)
	if !ok {
		r.fail("bad number '%s' at offset %d", literal, start)
	}
	if whole.BitLen() < 64 {
		return whole.Int64()
	}
	return whole
}

// unit spells one UTF-16 code unit the way Java's string concatenation of a char does.
func unit(c uint16) string {
	return string(utf16.Decode([]uint16{c}))
}

func hasPrefixAt(text []uint16, at int, word string) bool {
	if at+len(word) > len(text) {
		return false
	}
	return string(utf16.Decode(text[at:at+len(word)])) == word
}

func parseHex16(hex string) (uint16, error) {
	var value uint16
	if len(hex) != 4 {
		return 0, fmt.Errorf("bad length")
	}
	for _, c := range hex {
		digit := strings.IndexRune("0123456789abcdef", unicode(c))
		if digit < 0 {
			return 0, fmt.Errorf("bad digit")
		}
		value = value<<4 | uint16(digit)
	}
	return value, nil
}

// unicode lowercases an ASCII hexadecimal digit; Integer.parseInt(hex, 16) accepts either case.
func unicode(c rune) rune {
	if c >= 'A' && c <= 'F' {
		return c + ('a' - 'A')
	}
	return c
}
