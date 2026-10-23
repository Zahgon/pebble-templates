/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package jtime

import "strings"

// patternChars is java.text.DateFormatSymbols.patternChars, the letters SimpleDateFormat accepts.
// Anything else in the ASCII letter range is a pattern error, not a literal.
const patternChars = "GyMdkHmsSEDFwWahKzZYuXL"

// patternItem is one run of a compiled pattern: either a run of count equal pattern letters or a
// stretch of literal text (letter 0).
type patternItem struct {
	letter byte
	count  int
	text   string
}

// IllegalArgumentError is a java.lang.IllegalArgumentException thrown by the SimpleDateFormat
// constructor over a malformed pattern. Its message is the Java one, verbatim.
type IllegalArgumentError struct{ Message string }

func (e *IllegalArgumentError) Error() string { return e.Message }

// ParseError is a java.text.ParseException: the date filter wraps it, but the message is the one
// java.text.DateFormat.parse produces.
type ParseError struct {
	Message string
	// Offset is ParseException.getErrorOffset.
	Offset int
}

func (e *ParseError) Error() string { return e.Message }

// compilePattern is SimpleDateFormat.compile: it splits a pattern into literal runs and field
// runs, resolving the quoting rules (” is a literal quote whether or not it is inside a quoted
// section) and rejecting unknown pattern letters.
func compilePattern(pattern string) ([]patternItem, error) {
	var items []patternItem
	var lit strings.Builder
	var lastLetter byte
	count := 0
	inQuote := false

	flushField := func() {
		if count != 0 {
			items = append(items, patternItem{letter: lastLetter, count: count})
			lastLetter, count = 0, 0
		}
	}
	flushLit := func() {
		if lit.Len() > 0 {
			items = append(items, patternItem{text: lit.String()})
			lit.Reset()
		}
	}

	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		if c == '\'' {
			// '' is a single quote regardless of being inside a quoted section.
			if i+1 < len(runes) && runes[i+1] == '\'' {
				i++
				flushField()
				lit.WriteByte('\'')
				continue
			}
			if !inQuote {
				flushField()
			}
			inQuote = !inQuote
			continue
		}
		if inQuote {
			lit.WriteRune(c)
			continue
		}
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			flushField()
			lit.WriteRune(c)
			continue
		}
		if strings.IndexRune(patternChars, c) < 0 {
			return nil, &IllegalArgumentError{Message: "Illegal pattern character '" + string(c) + "'"}
		}
		b := byte(c)
		if count != 0 && b != lastLetter {
			flushField()
		}
		if count == 0 {
			flushLit()
		}
		lastLetter, count = b, count+1
	}
	if inQuote {
		return nil, &IllegalArgumentError{Message: "Unterminated quote"}
	}
	flushField()
	flushLit()
	return items, nil
}

// flushField and flushLit above append in the order the runs appear, except that a literal run
// which precedes a field run is emitted when the field starts; the loop keeps at most one of the
// two pending at a time, so the order of items matches the pattern.
