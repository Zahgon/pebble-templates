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
	"regexp"
	"unicode"
	"unicode/utf8"
)

/*
 * javaSpace is Java's regex "\s" class. Java does not make \s Unicode aware, it is exactly
 * [ \t\n\x0B\f\r]; Go's \s leaves the vertical tab out, so every \s of the Java patterns is
 * spelled out here instead.
 */
const javaSpace = `[ \t\n\x0B\f\r]`

// possibleNewline is Syntax.POSSIBLE_NEW_LINE, the optional line break that a tag swallows when
// newline trimming is enabled.
const possibleNewline = `(\r\n|\n\r|\r|\n|\x{0085}|\x{2028}|\x{2029})?`

// syntax is io.pebbletemplates.pebble.lexer.Syntax: the delimiters of the Pebble language and the
// regular expressions the lexer builds out of them. The port only ever uses the default
// delimiters, which the reference command does not let the user change.
type syntax struct {
	commentOpen        string
	commentClose       string
	executeOpen        string
	executeClose       string
	printOpen          string
	printClose         string
	interpolationOpen  string
	interpolationClose string
	whitespaceTrim     string

	regexPrintClose             *regexp.Regexp
	regexExecuteClose           *regexp.Regexp
	regexCommentClose           *regexp.Regexp
	regexStartDelimiters        *regexp.Regexp
	regexLeadingWhitespaceTrim  *regexp.Regexp
	regexTrailingWhitespaceTrim *regexp.Regexp
	regexInterpolationOpen      *regexp.Regexp
	regexInterpolationClose     *regexp.Regexp
	regexVerbatimStart          *regexp.Regexp
	regexVerbatimEnd            *regexp.Regexp
}

// newSyntax builds the default syntax: {{ }} for print, {% %} for execute, {# #} for comments,
// #{ } for string interpolation and "-" as the whitespace trim marker.
func newSyntax(newLineTrimming bool) *syntax {
	s := &syntax{
		commentOpen:        "{#",
		commentClose:       "#}",
		executeOpen:        "{%",
		executeClose:       "%}",
		printOpen:          "{{",
		printClose:         "}}",
		interpolationOpen:  "#{",
		interpolationClose: "}",
		whitespaceTrim:     "-",
	}
	q := regexp.QuoteMeta

	// Do we trim the newline following a tag?
	newline := ""
	if newLineTrimming {
		newline = possibleNewline
	}

	// regexes used to find the individual delimiters
	s.regexPrintClose = regexp.MustCompile("^" + javaSpace + "*" + q(s.whitespaceTrim) + "?" +
		q(s.printClose) + newline)
	s.regexExecuteClose = regexp.MustCompile("^" + javaSpace + "*" + q(s.whitespaceTrim) + "?" +
		q(s.executeClose) + newline)
	s.regexCommentClose = regexp.MustCompile(q(s.commentClose) + newline)

	// combination regex used to find the next START delimiter of any kind
	s.regexStartDelimiters = regexp.MustCompile(q(s.printOpen) + "|" + q(s.executeOpen) + "|" +
		q(s.commentOpen))

	// regex to find the verbatim tag
	s.regexVerbatimStart = regexp.MustCompile("^" + javaSpace + "*verbatim" + javaSpace + "*(" +
		q(s.whitespaceTrim) + ")?" + q(s.executeClose) + newline)
	s.regexVerbatimEnd = regexp.MustCompile(q(s.executeOpen) + "(" + q(s.whitespaceTrim) + ")?" +
		javaSpace + "*endverbatim" + javaSpace + "*(" + q(s.whitespaceTrim) + ")?" +
		q(s.executeClose) + newline)

	// regex for the whitespace trim character
	s.regexLeadingWhitespaceTrim = regexp.MustCompile(q(s.whitespaceTrim) + javaSpace + "+")
	s.regexTrailingWhitespaceTrim = regexp.MustCompile("^" + javaSpace + "*" + q(s.whitespaceTrim) +
		"(" + q(s.printClose) + "|" + q(s.executeClose) + "|" + q(s.commentClose) + ")")

	s.regexInterpolationOpen = regexp.MustCompile("^" + q(s.interpolationOpen))
	s.regexInterpolationClose = regexp.MustCompile("^" + javaSpace + "*" + q(s.interpolationClose))

	return s
}

// lookingAt is java.util.regex.Matcher.lookingAt: it reports whether re matches a prefix of s and
// returns the end of that match. Go's regexp is leftmost-first like Java's, so a match that does
// not start at the beginning means lookingAt would have failed.
func lookingAt(re *regexp.Regexp, s string) (int, bool) {
	loc := re.FindStringIndex(s)
	if loc == nil || loc[0] != 0 {
		return 0, false
	}
	return loc[1], true
}

// javaIsWhitespace is java.lang.Character.isWhitespace: the Unicode space separators that are not
// non-breaking, the line and paragraph separators, and the control characters \t to \r plus
// \u001C to \u001F. Note that it is false for \u0085, which the newline regex does match.
func javaIsWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', 0x1C, 0x1D, 0x1E, 0x1F:
		return true
	case 0x00A0, 0x2007, 0x202F: // non-breaking spaces
		return false
	}
	return unicode.Is(unicode.Zs, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r)
}

// javaIsLineTerminator reports whether r is one of the characters TemplateSource counts as a line
// break while it advances through the source.
func javaIsLineTerminator(r rune) bool {
	switch r {
	case '\n', '\r', 0x0085, 0x2028, 0x2029:
		return true
	}
	return false
}

// ltrim is StringUtils.ltrim, rtrim is StringUtils.rtrim; both use Character.isWhitespace.
func ltrim(s string) string {
	for i := 0; i < len(s); {
		r, w := utf8.DecodeRuneInString(s[i:])
		if !javaIsWhitespace(r) {
			return s[i:]
		}
		i += w
	}
	return ""
}

func rtrim(s string) string {
	for len(s) > 0 {
		r, w := utf8.DecodeLastRuneInString(s)
		if !javaIsWhitespace(r) {
			return s
		}
		s = s[:len(s)-w]
	}
	return ""
}
