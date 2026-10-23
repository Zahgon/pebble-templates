/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package pebble

import "fmt"

/*
 * Parse turns a template source into its AST. name is the template name used in error messages
 * (ParserException), opts the engine settings that change lexing and parsing (newLineTrimming,
 * literalDecimalTreatedAsInteger, literalNumbersAsBigDecimals). A nil opts means the engine
 * defaults, which have newline trimming on.
 */
func Parse(source, name string, opts *Options) (*Stmt, error) {
	if opts == nil {
		opts = &Options{NewLineTrimming: true}
	}
	tokens, err := tokenize(source, name, opts.NewLineTrimming)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens, file: name, opts: opts}
	return p.parse()
}

/*
 * bailout carries the error of a failed parse out of the recursive descent, the way the Java
 * parser throws a ParserException out of it. Parse recovers it and returns it; no other panic is
 * caught. This is the go/parser bailout idiom: it keeps the port's control flow identical to the
 * Java original, where every one of these methods can throw.
 */
type bailout struct{ err error }

// stoppingCondition is io.pebbletemplates.pebble.parser.StoppingCondition: a token parser hands
// one to subparse, which stops when it evaluates to true on the tag name of a block.
type stoppingCondition func(Token) bool

// parser is ParserImpl together with the TokenStream it walks and the ExpressionParser; the Java
// code splits them over three classes that all point at each other.
type parser struct {
	tokens []Token
	// current is the index of the token the stream points at.
	current int
	file    string
	opts    *Options

	// blockStack keeps the name of the block we are inside of, for the parent() function.
	blockStack []string
}

func (p *parser) parse() (root *Stmt, err error) {
	defer func() {
		if r := recover(); r != nil {
			b, ok := r.(bailout)
			if !ok {
				panic(r)
			}
			root, err = nil, b.err
		}
	}()
	body := p.subparse(nil)
	return &Stmt{Kind: StmtRoot, Line: body.Line, Body: body}, nil
}

// fail aborts the parse with a ParserException, as every parser error of Pebble is.
func (p *parser) fail(message string, line int) {
	panic(bailout{NewParserError(message, p.file, line)})
}

/*
 * subparse is the main method of the parser: it converts the token stream into a body node and
 * stops either at the end of the template or when the stopping condition of the token parser that
 * called it matches the tag name of a block.
 */
func (p *parser) subparse(stop stoppingCondition) *Stmt {
	// these nodes will be the children of the body node
	var nodes []*Stmt

	for !p.isEOF() {
		switch p.currentToken().Type {

		case TokenText:
			/*
			 * The current token is a text token. Not much to do here other than convert it to a
			 * text node.
			 */
			token := p.currentToken()
			nodes = append(nodes, &Stmt{Kind: StmtText, Line: token.Line, Text: token.Value})
			p.next()

		case TokenPrintStart:
			/*
			 * We are entering a print delimited region, which contains an expression, so let's
			 * pass control to the expression parser. We go to the next token first because the
			 * current one is just the opening delimiter.
			 */
			token := p.next()
			expression := p.parseExpression()
			nodes = append(nodes, &Stmt{Kind: StmtPrint, Line: token.Line, Expr: expression})

			// we expect to see a print closing delimiter
			p.expect(TokenPrintEnd)

		case TokenExecuteStart:
			// go to the next token because the current one is just the opening delimiter
			p.next()

			token := p.currentToken()

			/*
			 * We expect a name token at the beginning of every block. We do not consume it,
			 * because the current token may be needed by a token parser which has provided a
			 * stopping condition: the 'if' token parser needs to check whether the current token
			 * is 'endif' or 'else' and act accordingly.
			 */
			if token.Type != TokenName {
				p.fail("A block must start with a tag name.", token.Line)
			}

			// check the stopping condition of the token parser that called us, if any
			if stop != nil && stop(token) {
				return &Stmt{Kind: StmtBody, Line: token.Line, Children: nodes}
			}

			// find an appropriate parser for this name
			tokenParser := tokenParserFor(token.Value)
			if tokenParser == nil {
				p.fail(`Unexpected tag name "`+token.Value+`"`, token.Line)
			}

			node := tokenParser(token, p)

			// node might be nil (ex. the "extends" token parser)
			if node != nil {
				nodes = append(nodes, node)
			}

		default:
			p.fail("Parser ended in undefined state.", p.currentToken().Line)
		}
	}

	// create the body node with the children that we have found
	return &Stmt{Kind: StmtBody, Line: p.currentToken().Line, Children: nodes}
}

/*
 * The token stream of the parser. The Java TokenStream indexes an ArrayList, and a parse that
 * walks past the end of it fails with the IndexOutOfBoundsException of the list rather than with a
 * ParserException; the reference command prints that exception, so get reproduces it.
 */
func (p *parser) get(index int) Token {
	if index < 0 || index >= len(p.tokens) {
		panic(bailout{&JavaError{
			Kind: "IndexOutOfBoundsException",
			Message: fmt.Sprintf("Index %d out of bounds for length %d", index,
				len(p.tokens)),
		}})
	}
	return p.tokens[index]
}

// currentToken looks at the current token without consuming it.
func (p *parser) currentToken() Token { return p.get(p.current) }

// next consumes and returns the next token in the stream.
func (p *parser) next() Token {
	p.current++
	return p.get(p.current)
}

// peek returns the next token in the stream without consuming it.
func (p *parser) peek() Token { return p.get(p.current + 1) }

func (p *parser) isEOF() bool { return p.get(p.current).Type == TokenEOF }

// test is Token.test: the type has to match, and the value too when one is given.
func test(token Token, t TokenType, values ...string) bool {
	if token.Type != t {
		return false
	}
	if len(values) == 0 {
		return true
	}
	for _, v := range values {
		if !nullValued(token.Type) && token.Value == v {
			return true
		}
	}
	return false
}

// currentIs tests the current token.
func (p *parser) currentIs(t TokenType, values ...string) bool {
	return test(p.currentToken(), t, values...)
}

/*
 * expect checks the current token against the given type, fails the parse when it does not match
 * and consumes it. It is TokenStream.expect, whose message every mistyped template runs into.
 */
func (p *parser) expect(t TokenType) Token {
	return p.expectValues(t)
}

// expectValue is expect with an expected token value.
func (p *parser) expectValue(t TokenType, value string) Token {
	return p.expectValues(t, value)
}

func (p *parser) expectValues(t TokenType, values ...string) Token {
	token := p.get(p.current)
	if !test(token, t, values...) {
		p.fail(fmt.Sprintf("Unexpected token of value \"%s\" and type %s, expected token of type %s",
			javaTokenValue(token), javaTokenName(token.Type), javaTokenName(t)), token.Line)
	}
	p.next()
	return token
}

// pushBlockStack, popBlockStack and peekBlockStack track the block we are inside of.
func (p *parser) pushBlockStack(name string) { p.blockStack = append(p.blockStack, name) }

func (p *parser) popBlockStack() { p.blockStack = p.blockStack[:len(p.blockStack)-1] }

func (p *parser) peekBlockStack() string {
	if len(p.blockStack) == 0 {
		return ""
	}
	return p.blockStack[len(p.blockStack)-1]
}
