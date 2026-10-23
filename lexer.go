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
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The token types the Java lexer produces that the list in ast.go does not name; they continue
// its numbering.
const (
	TokenLong TokenType = TokenWhitespace + 1 + iota
	TokenInterpolationStart
	TokenInterpolationEnd
)

// javaTokenName is Token.Type.toString(), which the parser errors quote.
func javaTokenName(t TokenType) string {
	switch t {
	case TokenEOF:
		return "EOF"
	case TokenText:
		return "TEXT"
	case TokenExecuteStart:
		return "EXECUTE_START"
	case TokenExecuteEnd:
		return "EXECUTE_END"
	case TokenPrintStart:
		return "PRINT_START"
	case TokenPrintEnd:
		return "PRINT_END"
	case TokenString:
		return "STRING"
	case TokenNumber:
		return "NUMBER"
	case TokenOperator:
		return "OPERATOR"
	case TokenPunctuation:
		return "PUNCTUATION"
	case TokenName:
		return "NAME"
	case TokenLong:
		return "LONG"
	case TokenInterpolationStart:
		return "STRING_INTERPOLATION_START"
	case TokenInterpolationEnd:
		return "STRING_INTERPOLATION_END"
	}
	return "WHITESPACE"
}

/*
 * The Java lexer pushes the delimiter and interpolation tokens without a value, so their
 * getValue() is null; the parser both prints that null in its error messages and tests for it to
 * detect the end of the template. Token.Value cannot hold a null, but the types below are exactly
 * the ones that never carry a value, so nullness is a property of the type.
 */
func nullValued(t TokenType) bool {
	switch t {
	case TokenEOF, TokenPrintStart, TokenExecuteStart, TokenInterpolationStart,
		TokenInterpolationEnd:
		return true
	}
	return false
}

// javaTokenValue is Token.getValue() as String.format prints it.
func javaTokenValue(t Token) string {
	if nullValued(t.Type) {
		return "null"
	}
	return t.Value
}

// The static regular expressions of LexerImpl. The ones Java writes with a negative lookahead
// (the operator, string and interpolated string patterns) are matched by hand below, as Go's
// RE2 has no lookahead.
var (
	regexIdentifier = regexp.MustCompile(`^[\p{L}_][\p{L}\p{Nd}_]*`)
	regexLong       = regexp.MustCompile(`^[0-9]+L`)
	regexNumber     = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?`)
)

// punctuation is LexerImpl.PUNCTUATION.
const punctuation = "()[]{}?:.,|="

// lexState is LexerImpl.State.
type lexState int

const (
	stateData lexState = iota
	stateExecute
	statePrint
	stateComment
	stateString
	stateStringInterpolation
)

// bracket is one entry of the lexer's bracket stack: the bracket itself (a double quote counts as
// one) and the line it was opened on.
type bracket struct {
	text string
	line int
}

// templateSource is io.pebbletemplates.pebble.lexer.TemplateSource: the template text plus a
// cursor that counts the lines it passes. Offsets are byte offsets into the UTF-8 text rather
// than Java's UTF-16 indexes; only the lexer uses them, and it only ever slices with them.
type templateSource struct {
	text   string
	offset int
	line   int
	name   string
}

func newTemplateSource(text, name string) *templateSource {
	return &templateSource{text: text, name: name, line: 1}
}

// rest is the part of the template that has yet to be tokenized; every regex match of the lexer
// runs against it.
func (s *templateSource) rest() string { return s.text[s.offset:] }

func (s *templateSource) length() int { return len(s.text) - s.offset }

/*
 * charAt returns the character at the given offset of the remaining source, or 0 when that is
 * past the end. Java reads the character array straight out, where the characters past the end of
 * the template are the zeroes the array was allocated with, and it relies on that: the lexer
 * reports "Unexpected character [\0]" for a template that ends inside a tag.
 */
func (s *templateSource) charAt(index int) rune {
	if s.offset+index >= len(s.text) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(s.text[s.offset+index:])
	return r
}

/*
 * advance moves the cursor forward by the given number of bytes, counting the newlines it passes.
 * A \r\n pair counts as one line break even when the \n lies past the end of the range, exactly
 * as TemplateSource does; the \n is then counted a second time by the next advance, which is how
 * Java counts a line break that a tag is split across.
 */
func (s *templateSource) advance(amount int) {
	end := s.offset + amount
	for i := s.offset; i < end; {
		r, w := utf8.DecodeRuneInString(s.text[i:])
		if r == '\r' && i+w < len(s.text) && s.text[i+w] == '\n' {
			s.line++
			i += w + 1
			continue
		}
		if javaIsLineTerminator(r) {
			s.line++
		}
		i += w
	}
	s.offset = end
}

// advanceThroughWhitespace skips the whitespace in front of the cursor, counting newlines.
func (s *templateSource) advanceThroughWhitespace() {
	i := s.offset
	for i < len(s.text) {
		r, w := utf8.DecodeRuneInString(s.text[i:])
		if !javaIsWhitespace(r) {
			break
		}
		if r == '\r' && i+w < len(s.text) && s.text[i+w] == '\n' {
			s.line++
			i += w + 1
			continue
		}
		if javaIsLineTerminator(r) {
			s.line++
		}
		i += w
	}
	s.offset = i
}

// substring returns the first end bytes of the remaining source.
func (s *templateSource) substring(end int) string { return s.text[s.offset : s.offset+end] }

// substringRange returns the given range of the remaining source.
func (s *templateSource) substringRange(start, end int) string {
	return s.text[s.offset+start : s.offset+end]
}

// lexer is io.pebbletemplates.pebble.lexer.LexerImpl.
type lexer struct {
	syn    *syntax
	source *templateSource
	tokens []Token

	// brackets are the brackets we are currently inside, innermost last.
	brackets []bracket

	// states is the lexer state stack, current state last.
	states []lexState

	// trimLeadingWhitespaceFromNextData records that we passed an end delimiter with a whitespace
	// trim marker ("{{ foo -}}"), so the next text token loses its leading whitespace.
	trimLeadingWhitespaceFromNextData bool

	// operators are every unary and binary operator symbol, longest first.
	operators []string
}

// tokenize turns the template text into the token list the parser consumes. It is
// LexerImpl.tokenize, and like it, it reports a ParserException for an unclosed comment, an
// unclosed verbatim tag, a mismatched bracket or a character it cannot read.
func tokenize(source, name string, newLineTrimming bool) ([]Token, error) {
	l := &lexer{
		syn:    newSyntax(newLineTrimming),
		source: newTemplateSource(source, name),
	}
	l.buildOperatorList()

	/*
	 * Start in a DATA state, which means that we are NOT in between a pair of meaningful
	 * delimiters, and keep lexing with the method the current state calls for.
	 */
	l.states = append(l.states, stateData)
	for l.source.length() > 0 {
		var err error
		switch l.states[len(l.states)-1] {
		case stateData:
			err = l.tokenizeData()
		case stateExecute:
			err = l.tokenizeBetweenExecuteDelimiters()
		case statePrint:
			err = l.tokenizeBetweenPrintDelimiters()
		case stateComment:
			err = l.tokenizeComment()
		case stateString:
			err = l.tokenizeString()
		case stateStringInterpolation:
			err = l.tokenizeStringInterpolation()
		}
		if err != nil {
			return nil, err
		}
	}

	// end of file token
	l.pushToken(TokenEOF, "")
	l.popState()

	// make sure that all brackets have been closed, else throw an error
	if len(l.brackets) > 0 {
		expected := l.brackets[len(l.brackets)-1].text
		return nil, l.fail(`Unclosed "` + expected + `"`)
	}

	return l.tokens, nil
}

// fail builds the ParserException of the lexer, which always reports the line the cursor is on.
func (l *lexer) fail(message string) error {
	return NewParserError(message, l.source.name, l.source.line)
}

/*
 * The DATA state assumes that we are currently NOT in between any pair of meaningful delimiters.
 * We are looking for the next "open" or "start" delimiter.
 */
func (l *lexer) tokenizeData() error {
	// find the next start delimiter
	loc := l.syn.regexStartDelimiters.FindStringIndex(l.source.rest())

	var text, startDelimiter string

	// if we didn't find another start delimiter, the text token goes all the way to the end of
	// the template.
	if loc == nil {
		text = l.source.rest()
		l.source.advance(l.source.length())
	} else {
		text = l.source.substring(loc[0])
		startDelimiter = l.source.substringRange(loc[0], loc[1])
		// advance to after the start delimiter
		l.source.advance(loc[1])
	}

	// trim leading whitespace from this text if we previously encountered the appropriate
	// whitespace trim character
	if l.trimLeadingWhitespaceFromNextData {
		text = ltrim(text)
		l.trimLeadingWhitespaceFromNextData = false
	}
	textToken := l.pushToken(TokenText, text)

	if loc == nil {
		return nil
	}
	l.checkForLeadingWhitespaceTrim(textToken)
	switch startDelimiter {
	case l.syn.commentOpen:
		// we don't actually push any tokens for comments
		l.states = append(l.states, stateComment)
	case l.syn.printOpen:
		l.pushToken(TokenPrintStart, "")
		l.states = append(l.states, statePrint)
	case l.syn.executeOpen:
		// check for the verbatim tag, which the lexer handles itself
		if m := l.syn.regexVerbatimStart.FindStringSubmatchIndex(l.source.rest()); m != nil && m[0] == 0 {
			if err := l.lexVerbatimData(m); err != nil {
				return err
			}
			l.states = append(l.states, stateData)
		} else {
			l.pushToken(TokenExecuteStart, "")
			l.states = append(l.states, stateExecute)
		}
	}
	return nil
}

// tokenizeBetweenExecuteDelimiters tokenizes between execute delimiters.
func (l *lexer) tokenizeBetweenExecuteDelimiters() error {
	// check for the trailing whitespace trim character
	l.checkForTrailingWhitespaceTrim()

	end, ok := lookingAt(l.syn.regexExecuteClose, l.source.rest())

	// check if we are at the execute closing delimiter
	if len(l.brackets) == 0 && ok {
		l.pushToken(TokenExecuteEnd, l.syn.executeClose)
		l.source.advance(end)
		l.popState()
		return nil
	}
	return l.tokenizeExpression()
}

// tokenizeBetweenPrintDelimiters tokenizes between print delimiters.
func (l *lexer) tokenizeBetweenPrintDelimiters() error {
	// check for the trailing whitespace trim character
	l.checkForTrailingWhitespaceTrim()

	end, ok := lookingAt(l.syn.regexPrintClose, l.source.rest())

	// check if we are at the print closing delimiter
	if len(l.brackets) == 0 && ok {
		l.pushToken(TokenPrintEnd, l.syn.printClose)
		l.source.advance(end)
		l.popState()
		return nil
	}
	return l.tokenizeExpression()
}

/*
 * tokenizeComment finds the closing delimiter of a comment and moves the cursor to that point. No
 * tokens are pushed for a comment.
 */
func (l *lexer) tokenizeComment() error {
	// all we need to do is find the end of the comment.
	loc := l.syn.regexCommentClose.FindStringIndex(l.source.rest())
	if loc == nil {
		return l.fail("Unclosed comment.")
	}

	/*
	 * Check if the comment ended with the whitespace trim character by reversing the comment and
	 * performing a regular forward regex search.
	 */
	comment := l.source.substring(loc[0])
	if _, ok := lookingAt(l.syn.regexLeadingWhitespaceTrim, reverseString(comment)); ok {
		l.trimLeadingWhitespaceFromNextData = true
	}

	// move cursor to end of comment (and closing delimiter)
	l.source.advance(loc[1])
	l.popState()
	return nil
}

// tokenizeString tokenizes the inside of a double quoted string, which may hold #{} interpolation.
func (l *lexer) tokenizeString() error {
	// interpolation
	if end, ok := lookingAt(l.syn.regexInterpolationOpen, l.source.rest()); ok {
		l.brackets = append(l.brackets, bracket{l.syn.interpolationOpen, l.source.line})
		l.pushToken(TokenInterpolationStart, "")
		l.source.advance(end)
		l.states = append(l.states, stateStringInterpolation)
		return nil
	}

	// regular string start (always full string if single quotes)
	if end := matchStringNonInterpolatedPart(l.source.rest()); end > 0 {
		token := l.source.substring(end)
		l.source.advance(end)
		l.pushToken(TokenString, token)
		return nil
	}

	// end of string (which may have contained interpolation)
	if strings.HasPrefix(l.source.rest(), `"`) {
		l.brackets = l.brackets[:len(l.brackets)-1]
		l.popState()
		l.source.advance(1)
	}
	return nil
}

// tokenizeStringInterpolation tokenizes the expression inside a #{} interpolation.
func (l *lexer) tokenizeStringInterpolation() error {
	lastBracket := l.brackets[len(l.brackets)-1].text
	end, ok := lookingAt(l.syn.regexInterpolationClose, l.source.rest())
	if lastBracket == l.syn.interpolationOpen && ok {
		l.brackets = l.brackets[:len(l.brackets)-1]
		l.pushToken(TokenInterpolationEnd, "")
		l.source.advance(end)
		l.popState()
		return nil
	}
	return l.tokenizeExpression()
}

// tokenizeExpression tokenizes an expression, which can be found within both execute and print
// regions as well as inside a string interpolation.
func (l *lexer) tokenizeExpression() error {
	l.source.advanceThroughWhitespace()

	// operators
	if op, ok := l.matchOperator(l.source.rest()); ok {
		l.pushToken(TokenOperator, op)
		l.source.advance(len(op))
		return nil
	}

	// names
	if loc := regexIdentifier.FindStringIndex(l.source.rest()); loc != nil {
		token := l.source.substring(loc[1])
		l.pushToken(TokenName, token)
		l.source.advance(loc[1])
		return nil
	}

	// long
	if loc := regexLong.FindStringIndex(l.source.rest()); loc != nil {
		// the value drops the trailing "L"
		token := l.source.substring(loc[1] - 1)
		l.pushToken(TokenLong, token)
		l.source.advance(loc[1])
		return nil
	}

	// numbers
	if loc := regexNumber.FindStringIndex(l.source.rest()); loc != nil {
		token := l.source.substring(loc[1])
		l.pushToken(TokenNumber, token)
		l.source.advance(loc[1])
		return nil
	}

	// punctuation
	if c := l.source.charAt(0); c < utf8.RuneSelf && strings.IndexByte(punctuation, byte(c)) >= 0 {
		character := string(c)

		// opening bracket
		if strings.Contains("([{", character) {
			l.brackets = append(l.brackets, bracket{character, l.source.line})
		} else if strings.Contains(")]}", character) {
			// closing bracket
			if len(l.brackets) == 0 {
				return l.fail(`Unexpected "` + character + `"`)
			}
			validPairs := map[string]string{"(": ")", "[": "]", "{": "}"}
			lastBracket := l.brackets[len(l.brackets)-1].text
			l.brackets = l.brackets[:len(l.brackets)-1]
			if expected := validPairs[lastBracket]; expected != character {
				return l.fail(`Unclosed "` + expected + `"`)
			}
		}

		l.pushToken(TokenPunctuation, character)
		l.source.advance(1)
		return nil
	}

	// Plain (non-interpolated) string
	if end, ok := matchStringPlain(l.source.rest()); ok {
		token := l.source.substring(end)
		l.source.advance(end)
		l.pushToken(TokenString, unquoteAndUnescape(token))
		return nil
	}

	// Interpolated strings
	if strings.HasPrefix(l.source.rest(), `"`) {
		l.brackets = append(l.brackets, bracket{`"`, l.source.line})
		l.states = append(l.states, stateString)
		l.source.advance(1)
		return nil
	}

	// we should have found something and returned by this point
	return l.fail(fmt.Sprintf("Unexpected character [%c]", l.source.charAt(0)))
}

/*
 * unquoteAndUnescape removes the wrapping quotes of a string literal and un-escapes the quotes
 * within it. Only the quotation mark that wraps the string is un-escaped: "a\nb" keeps its
 * backslash and prints as a backslash and an n.
 */
func unquoteAndUnescape(str string) string {
	quotationType := str[0]
	str = str[1 : len(str)-1]
	switch quotationType {
	case '\'':
		return strings.ReplaceAll(str, `\'`, `'`)
	case '"':
		return strings.ReplaceAll(str, `\"`, `"`)
	}
	return str
}

// checkForLeadingWhitespaceTrim right-trims the text in front of a start delimiter that carries
// the trim marker ("foo {{- bar }}"), and moves past the marker.
func (l *lexer) checkForLeadingWhitespaceTrim(leadingToken int) {
	end, ok := lookingAt(l.syn.regexLeadingWhitespaceTrim, l.source.rest())
	if !ok {
		return
	}
	if leadingToken >= 0 {
		l.tokens[leadingToken].Value = rtrim(l.tokens[leadingToken].Value)
	}
	l.source.advance(end)
}

// checkForTrailingWhitespaceTrim notes an end delimiter that carries the trim marker, so that the
// text that follows it loses its leading whitespace.
func (l *lexer) checkForTrailingWhitespaceTrim() {
	if _, ok := lookingAt(l.syn.regexTrailingWhitespaceTrim, l.source.rest()); ok {
		l.trimLeadingWhitespaceFromNextData = true
	}
}

// lexVerbatimData implements the "verbatim" tag, whose body becomes one text token.
func (l *lexer) lexVerbatimData(verbatimStart []int) error {
	// move cursor past the opening verbatim tag
	l.source.advance(verbatimStart[1])

	// look for the "endverbatim" tag, storing everything between now and then into a TEXT node
	end := l.syn.regexVerbatimEnd.FindStringSubmatchIndex(l.source.rest())

	// check for EOF
	if end == nil {
		return l.fail("Unclosed verbatim tag.")
	}
	verbatimText := l.source.substring(end[0])

	/*
	 * Java checks group(0) of the start matcher here, which is the whole match and therefore
	 * never null, so the text of a verbatim tag is always left trimmed whether or not the opening
	 * tag carried the whitespace trim marker.
	 */
	verbatimText = ltrim(verbatimText)

	// check if the verbatim end tag had a leading whitespace trim
	if end[2] >= 0 {
		verbatimText = rtrim(verbatimText)
	}

	// check if the verbatim end tag had a trailing whitespace trim
	if end[4] >= 0 {
		l.trimLeadingWhitespaceFromNextData = true
	}

	// move cursor past the verbatim text and end delimiter
	l.source.advance(end[1])

	l.pushToken(TokenText, verbatimText)
	return nil
}

/*
 * pushToken appends a token of the given type and value and returns its index, or -1 for the
 * empty text tokens the lexer drops. The line number is the one the cursor has reached, which is
 * past the token itself: that is where Pebble's line numbers come from.
 */
func (l *lexer) pushToken(t TokenType, value string) int {
	// ignore empty text tokens
	if t == TokenText && value == "" {
		return -1
	}
	l.tokens = append(l.tokens, Token{Type: t, Value: value, Line: l.source.line})
	return len(l.tokens) - 1
}

// popState pops the state stack.
func (l *lexer) popState() { l.states = l.states[:len(l.states)-1] }

/*
 * buildOperatorList collects every unary and binary operator symbol and sorts it by descending
 * length with a stable sort, as LexerImpl does before it builds one big alternation out of them:
 * the longest alternative has to come first for "is not" to win over "is".
 */
func (l *lexer) buildOperatorList() {
	for _, op := range UnaryOps {
		l.operators = append(l.operators, op.Symbol)
	}
	for _, op := range BinaryOps {
		l.operators = append(l.operators, op.Symbol)
	}
	sort.SliceStable(l.operators, func(i, j int) bool {
		return len(l.operators[i]) > len(l.operators[j])
	})
}

/*
 * matchOperator returns the operator at the start of s, trying the symbols in the order of the
 * alternation Java builds. An operator that ends in a letter only matches when the next character
 * is not a word character, so that "organization" does not start with the "or" operator.
 */
func (l *lexer) matchOperator(s string) (string, bool) {
	for _, op := range l.operators {
		if !strings.HasPrefix(s, op) {
			continue
		}
		last, _ := utf8.DecodeLastRuneInString(op)
		if isJavaLetterish(last) {
			if rest := s[len(op):]; rest != "" && isWordByte(rest[0]) {
				continue
			}
		}
		return op, true
	}
	return "", false
}

// isJavaLetterish is the Character.isLetter() or LETTER_NUMBER test LexerImpl uses to decide
// whether an operator needs the word boundary.
func isJavaLetterish(r rune) bool {
	return unicode.IsLetter(r) || unicode.Is(unicode.Nl, r)
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

/*
 * matchStringNonInterpolatedPart is REGEX_STRING_NON_INTERPOLATED_PART,
 * ^[^#"\\]*(?:(?:\\.|#(?!\{))[^#"\\]*)* with DOTALL: everything up to the first interpolation or
 * the closing quote of a double quoted string. It returns how many bytes it matched.
 */
func matchStringNonInterpolatedPart(s string) int {
	i := 0
	for i < len(s) {
		switch s[i] {
		case '"':
			return i
		case '\\':
			if i+1 >= len(s) {
				return i
			}
			_, w := utf8.DecodeRuneInString(s[i+1:])
			i += 1 + w
		case '#':
			if i+1 < len(s) && s[i+1] == '{' {
				return i
			}
			i++
		default:
			_, w := utf8.DecodeRuneInString(s[i:])
			i += w
		}
	}
	return i
}

/*
 * matchStringPlain is REGEX_STRING_PLAIN, a single quoted string or a double quoted string
 * without interpolation, both allowing escaped quotes. It returns the end of the literal,
 * including the quotes.
 */
func matchStringPlain(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	var quote, forbidden byte
	switch s[0] {
	case '"':
		// the double quoted alternative rejects '#', which sends the literal to the
		// interpolating path instead
		quote, forbidden = '"', '#'
	case '\'':
		quote, forbidden = '\'', 0
	default:
		return 0, false
	}
	for i := 1; i < len(s); {
		switch {
		case s[i] == quote:
			return i + 1, true
		case forbidden != 0 && s[i] == forbidden:
			return 0, false
		case s[i] == '\\':
			if i+1 >= len(s) {
				return 0, false
			}
			_, w := utf8.DecodeRuneInString(s[i+1:])
			i += 1 + w
		default:
			_, w := utf8.DecodeRuneInString(s[i:])
			i += w
		}
	}
	return 0, false
}

// reverseString reverses the characters of s, as the comment lexer does to look for a trailing
// whitespace trim marker with a forward regex.
func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
