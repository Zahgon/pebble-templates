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
	"strings"
)

/*
 * tokenParser is io.pebbletemplates.pebble.tokenParser.TokenParser: it converts the tokens of one
 * tag into a node. token is the tag name token, which is not yet consumed. A token parser returns
 * nil when the tag renders nothing (the extends tag).
 *
 * It is the responsibility of a token parser to leave the stream pointing at the token after the
 * tag, which usually means that its last act is to consume an EXECUTE_END.
 */
type tokenParser func(token Token, p *parser) *Stmt

/*
 * tokenParserFor returns the parser of one tag: the tags of the core extension plus the autoescape tag of the escaper
 * extension. The verbatim tag is implemented directly in the lexer and deliberately has no entry
 * here, so "{% verbatim foo %}", which the lexer does not recognize, is an unexpected tag name.
 */
func tokenParserFor(tag string) tokenParser {
	switch tag {
	case "autoescape":
		return parseAutoEscapeTag
	case "block":
		return parseBlockTag
	case "cache":
		return parseCacheTag
	case "embed":
		return parseEmbedTag
	case "extends":
		return parseExtendsTag
	case "filter":
		return parseFilterTag
	case "flush":
		return parseFlushTag
	case "for":
		return parseForTag
	case "from":
		return parseFromTag
	case "if":
		return parseIfTag
	case "import":
		return parseImportTag
	case "include":
		return parseIncludeTag
	case "macro":
		return parseMacroTag
	case "parallel":
		return parseParallelTag
	case "set":
		return parseSetTag
	}
	return nil
}

// named builds the stopping condition that stops at a tag of the given name.
func named(names ...string) stoppingCondition {
	return func(token Token) bool { return test(token, TokenName, names...) }
}

// parseAutoEscapeTag parses {% autoescape [true|false] ["strategy"] %}.
func parseAutoEscapeTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line
	strategy := ""
	active := true

	// skip over the 'autoescape' token
	p.next()

	// did user specify active boolean?
	if p.currentIs(TokenName) {
		// Boolean.parseBoolean ignores case and treats anything but "true" as false
		active = strings.EqualFold(p.currentToken().Value, "true")
		p.next()
	}

	// did user specify a strategy?
	if p.currentIs(TokenString) {
		strategy = p.currentToken().Value
		p.next()
	}

	p.expect(TokenExecuteEnd)

	// now we parse the block body
	body := p.subparse(named("endautoescape"))

	// skip the 'endautoescape' token
	p.next()
	p.expect(TokenExecuteEnd)

	return &Stmt{Kind: StmtAutoEscape, Line: lineNumber, Body: body, Flag: active,
		Extra: strategy}
}

// parseBlockTag parses {% block name %}...{% endblock %}. The embed tag reuses it, and passes its
// own tag token in, which is where the line number of a block inside an embed comes from.
func parseBlockTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line

	// skip over the 'block' token to the name token
	blockName := p.next()

	// expect a name or string for the new block
	if !test(blockName, TokenName) && !test(blockName, TokenString) {
		/*
		 * We already know an error has occurred but let's just call the typical "expect" method
		 * so that a proper error message is given to the user.
		 */
		p.expect(TokenName)
	}

	// get the name of the new block
	name := blockName.Value

	// skip over name
	p.next()
	p.expect(TokenExecuteEnd)

	p.pushBlockStack(name)

	// now we parse the block body
	blockBody := p.subparse(named("endblock"))

	p.popBlockStack()

	// check whether the endblock exists
	if !p.currentIs(TokenName, "endblock") {
		p.fail("endblock tag should be present with block tag starting line number ", token.Line)
	}

	// skip the 'endblock' token
	p.next()

	// check if user included block name in endblock
	if p.currentIs(TokenName, name) || p.currentIs(TokenString, name) {
		p.next()
	}
	p.expect(TokenExecuteEnd)

	return &Stmt{Kind: StmtBlock, Line: lineNumber, Name: name, Body: blockBody}
}

/*
 * parseCacheTag parses {% cache key %}...{% endcache %}. The cache key lands in both Expr and the
 * single positional argument, as the caches themselves are out of the scope of this port.
 */
func parseCacheTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line

	// skip over the 'cache' token
	p.next()

	expression := p.parseExpression()

	// skip the execute end delimiter
	p.next()

	// now we parse the cache body
	cacheBody := p.subparse(named("endcache"))

	// skip the 'endcache' token
	p.next()
	p.expect(TokenExecuteEnd)

	return &Stmt{Kind: StmtCache, Line: lineNumber, Expr: expression, Body: cacheBody,
		Arguments: &Arguments{Positional: []*Expr{expression}, Line: expression.Line}}
}

/*
 * parseEmbedTag parses {% embed "template" [with {...}] %} followed by block tags and
 * {% endembed %}. The overriding blocks become the children of the body.
 */
func parseEmbedTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line

	// skip over the 'embed' token
	p.next()

	embedExpression := p.parseExpression()

	mapExpression := p.parseWithMap(token)

	p.expect(TokenExecuteEnd)

	blocks := p.parseEmbedBlocks(token)

	return &Stmt{Kind: StmtEmbed, Line: lineNumber, Expr: embedExpression,
		Arguments: mapExpression,
		Body:      &Stmt{Kind: StmtBody, Line: lineNumber, Children: blocks}}
}

// parseWithMap parses the optional "with {...}" of the include and embed tags, which has to be a
// map literal.
func (p *parser) parseWithMap(token Token) *Arguments {
	current := p.currentToken()
	if current.Type != TokenName || current.Value != "with" {
		return nil
	}

	// Skip over 'with'
	p.next()

	parsed := p.parseExpression()
	if parsed.Kind != ExprMap {
		p.fail(fmt.Sprintf("Unexpected expression '%s'.", javaExpressionClass(parsed)),
			token.Line)
	}
	return &Arguments{Positional: []*Expr{parsed}, Line: parsed.Line}
}

// parseEmbedBlocks parses the block tags between an embed tag and its endembed.
func (p *parser) parseEmbedBlocks(token Token) []*Stmt {
	var blocks []*Stmt
	for {
		if p.currentIs(TokenText) {
			textToken := p.expect(TokenText)
			if javaTrim(textToken.Value) != "" {
				p.fail("A template that extends another one cannot include content outside blocks."+
					" Did you forget to put the content inside a {% block %} tag?",
					textToken.Line)
			}
		}
		p.expect(TokenExecuteStart)

		// we're finished with blocks, expect {% endembed %}
		if p.currentIs(TokenName, "endembed") {
			p.expectValue(TokenName, "endembed")
			p.expect(TokenExecuteEnd)
			return blocks
		}

		/*
		 * Otherwise start parsing a block tag and let the block token parser do the rest, to make
		 * sure it parses the same as a top level block. It is handed the embed tag's token, just
		 * as the Java code hands it.
		 */
		blocks = append(blocks, parseBlockTag(token, p))
	}
}

// parseExtendsTag parses {% extends "template" %}; it renders nothing itself.
func parseExtendsTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line

	// skip the 'extends' token
	p.next()

	parentTemplateExpression := p.parseExpression()
	p.expect(TokenExecuteEnd)

	return &Stmt{Kind: StmtExtends, Line: lineNumber, Expr: parentTemplateExpression}
}

/*
 * parseFilterTag parses {% filter upper|trim %}...{% endfilter %}. Java builds a print node whose
 * expression is the filter chain applied to the rendered body, so the autoescaper sees a print
 * node and escapes the result; this port keeps that shape rather than using StmtFilter.
 */
func parseFilterTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line

	// skip the 'filter' token
	p.next()

	type invocation struct {
		name string
		args *Arguments
	}
	var invocations []invocation
	name, args := p.parseFilterInvocationExpression()
	invocations = append(invocations, invocation{name, args})
	for p.currentIs(TokenOperator, "|") {
		// skip the '|' token
		p.next()
		name, args = p.parseFilterInvocationExpression()
		invocations = append(invocations, invocation{name, args})
	}
	p.expect(TokenExecuteEnd)

	body := p.subparse(named("endfilter"))
	p.next()
	p.expect(TokenExecuteEnd)

	lastExpression := &Expr{Kind: ExprRenderableNode, Line: p.currentToken().Line, Body: body}
	for _, invocation := range invocations {
		/*
		 * The filter expressions of the filter tag never get a line number in Java, so errors
		 * raised inside them report no line at all.
		 */
		lastExpression = &Expr{Kind: ExprFilter, Name: invocation.name,
			Arguments: invocation.args, Args: []*Expr{lastExpression}}
	}

	return &Stmt{Kind: StmtPrint, Line: lineNumber, Expr: lastExpression}
}

// parseFlushTag parses {% flush %}.
func parseFlushTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line

	// skip over the 'flush' token
	p.next()
	p.expect(TokenExecuteEnd)

	return &Stmt{Kind: StmtFlush, Line: lineNumber}
}

// parseForTag parses {% for item in items %}...[{% else %}...]{% endfor %}.
func parseForTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line

	// skip the 'for' token
	p.next()

	// get the iteration variable
	iterationVariable := p.parseNewVariableName()

	p.expectValue(TokenName, "in")

	// get the iterable variable
	iterable := p.parseExpression()
	p.expect(TokenExecuteEnd)

	body := p.subparse(named("else", "endfor"))

	var elseBody *Stmt
	if p.currentIs(TokenName, "else") {
		// skip the 'else' token
		p.next()
		p.expect(TokenExecuteEnd)
		elseBody = p.subparse(named("endfor"))
	}

	if nullValued(p.currentToken().Type) {
		p.fail(`Unexpected end of template. Pebble was looking for the "endfor" tag`,
			p.currentToken().Line)
	}

	// skip the 'endfor' token
	p.next()
	p.expect(TokenExecuteEnd)

	return &Stmt{Kind: StmtFor, Line: lineNumber, Name: iterationVariable, Expr: iterable,
		Body: body, Else: elseBody}
}

// parseFromTag parses {% from "template" import macro [as alias], ... %}.
func parseFromTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line

	// skip over the 'from' token
	p.next()

	// parse the template name expression
	fromExpression := p.parseExpression()

	// parse the import
	names, aliases := p.parseNamedMacros()
	p.expect(TokenExecuteEnd)

	return &Stmt{Kind: StmtFrom, Line: lineNumber, Expr: fromExpression, Names: names,
		Aliases: aliases}
}

// parseNamedMacros parses the macro list of a from tag: the names in the other template and the
// local names they are registered under.
func (p *parser) parseNamedMacros() (names, aliases []string) {
	p.expectValue(TokenName, "import")

	for p.currentToken().Type != TokenExecuteEnd {
		pre := p.expect(TokenName)
		if p.currentIs(TokenName, "as") {
			// Skips over 'as'
			p.next()
			post := p.expect(TokenName)
			names = append(names, pre.Value)
			aliases = append(aliases, post.Value)
		} else {
			names = append(names, pre.Value)
			aliases = append(aliases, pre.Value)
		}

		token := p.currentToken()
		if test(token, TokenPunctuation, ",") {
			// Skips over ','
			p.next()
		} else if token.Type == TokenExecuteEnd {
			break
		} else {
			p.fail(fmt.Sprintf(
				"Unexpected token of value \"%s\" and type %s, expected token of type %s or ',' ",
				javaTokenValue(token), javaTokenName(token.Type),
				javaTokenName(TokenExecuteEnd)), token.Line)
		}
	}
	return names, aliases
}

// parseIfTag parses {% if %}...{% elseif %}...{% else %}...{% endif %}.
func parseIfTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line

	// skip the 'if' token
	p.next()

	node := &Stmt{Kind: StmtIf, Line: lineNumber}

	expression := p.parseExpression()
	p.expect(TokenExecuteEnd)
	body := p.subparse(decideIfFork)
	node.Conditions = append(node.Conditions, expression)
	node.Bodies = append(node.Bodies, body)

	for end := false; !end; {
		if nullValued(p.currentToken().Type) {
			p.fail(`Unexpected end of template. Pebble was looking for the "endif" tag`,
				p.currentToken().Line)
		}
		switch p.currentToken().Value {
		case "else":
			p.next()
			p.expect(TokenExecuteEnd)
			node.Else = p.subparse(named("endif"))
		case "elseif":
			p.next()
			expression = p.parseExpression()
			p.expect(TokenExecuteEnd)
			body = p.subparse(decideIfFork)
			node.Conditions = append(node.Conditions, expression)
			node.Bodies = append(node.Bodies, body)
		case "endif":
			p.next()
			end = true
		default:
			p.fail("Unexpected end of template. Pebble was looking for the following tags "+
				`"else", "elseif", or "endif"`, p.currentToken().Line)
		}
	}
	p.expect(TokenExecuteEnd)

	return node
}

// decideIfFork stops the body of an if branch at any of the tags that can follow it.
var decideIfFork = named("elseif", "else", "endif")

// parseImportTag parses {% import "template" [as alias] %}.
func parseImportTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line

	// skip over the 'import' token
	p.next()

	importExpression := p.parseExpression()
	current := p.currentToken()
	alias := ""

	// We check if there is an optional 'as' keyword on the import tag.
	if current.Type == TokenName && current.Value == "as" {
		// Skip over 'as'
		p.next()
		alias = p.expect(TokenName).Value
	}
	p.expect(TokenExecuteEnd)

	return &Stmt{Kind: StmtImport, Line: lineNumber, Expr: importExpression, Name: alias}
}

// parseIncludeTag parses {% include "template" [with {...}] %}.
func parseIncludeTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line

	// skip over the 'include' token
	p.next()

	includeExpression := p.parseExpression()

	mapExpression := p.parseWithMap(token)

	p.expect(TokenExecuteEnd)

	return &Stmt{Kind: StmtInclude, Line: lineNumber, Expr: includeExpression,
		Arguments: mapExpression}
}

// parseMacroTag parses {% macro name(a, b=1) %}...{% endmacro %}.
func parseMacroTag(token Token, p *parser) *Stmt {
	// skip over the 'macro' token
	p.next()

	macroName := p.expect(TokenName).Value
	args := p.parseArguments(true)
	p.expect(TokenExecuteEnd)

	// parse the body
	body := p.subparse(named("endmacro"))

	// skip the 'endmacro' token
	p.next()
	p.expect(TokenExecuteEnd)

	return &Stmt{Kind: StmtMacro, Line: token.Line, Name: macroName, Arguments: args, Body: body}
}

// parseParallelTag parses {% parallel %}...{% endparallel %}; this port renders the body
// sequentially, as the reference command has no executor service.
func parseParallelTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line

	// skip the 'parallel' token
	p.next()
	p.expect(TokenExecuteEnd)

	body := p.subparse(named("endparallel"))

	// skip the 'endparallel' token
	p.next()
	p.expect(TokenExecuteEnd)

	return &Stmt{Kind: StmtParallel, Line: lineNumber, Body: body}
}

// parseSetTag parses {% set name = value %}.
func parseSetTag(token Token, p *parser) *Stmt {
	lineNumber := token.Line

	// skip the 'set' token
	p.next()

	name := p.parseNewVariableName()
	p.expectValue(TokenPunctuation, "=")
	value := p.parseExpression()
	p.expect(TokenExecuteEnd)

	return &Stmt{Kind: StmtSet, Line: lineNumber, Name: name, Expr: value}
}

/*
 * javaExpressionClass is the canonical class name of the expression node Java would have built,
 * which the include and embed tags quote when the "with" argument is not a map literal.
 */
func javaExpressionClass(e *Expr) string {
	const pkg = "io.pebbletemplates.pebble.node.expression."
	switch e.Kind {
	case ExprLiteralString:
		return pkg + "LiteralStringExpression"
	case ExprLiteralLong:
		return pkg + "LiteralLongExpression"
	case ExprLiteralInt:
		return pkg + "LiteralIntegerExpression"
	case ExprLiteralDouble:
		return pkg + "LiteralDoubleExpression"
	case ExprLiteralBigDecimal:
		return pkg + "LiteralBigDecimalExpression"
	case ExprLiteralBool:
		return pkg + "LiteralBooleanExpression"
	case ExprLiteralNull:
		return pkg + "LiteralNullExpression"
	case ExprContextVariable:
		return pkg + "ContextVariableExpression"
	case ExprGetAttribute:
		return pkg + "GetAttributeExpression"
	case ExprFilter:
		return pkg + "FilterExpression"
	case ExprFunction:
		return pkg + "FunctionOrMacroInvocationExpression"
	case ExprTest:
		if e.Negated {
			return pkg + "NegativeTestExpression"
		}
		return pkg + "PositiveTestExpression"
	case ExprUnary:
		switch e.Op {
		case "not":
			return pkg + "UnaryNotExpression"
		case "+":
			return pkg + "UnaryPlusExpression"
		}
		return pkg + "UnaryMinusExpression"
	case ExprTernary:
		return pkg + "TernaryExpression"
	case ExprArray:
		return pkg + "ArrayExpression"
	case ExprMap:
		return pkg + "MapExpression"
	case ExprRange:
		return pkg + "RangeExpression"
	case ExprConcat:
		return pkg + "ConcatenateExpression"
	case ExprParentFunction:
		return pkg + "ParentFunctionExpression"
	case ExprBlockFunction:
		return pkg + "BlockFunctionExpression"
	case ExprRenderableNode:
		return pkg + "RenderableNodeExpression"
	}
	return pkg + binaryExpressionClass(e.Op)
}

// binaryExpressionClass is the class name of the node of one binary operator.
func binaryExpressionClass(op string) string {
	switch op {
	case "or":
		return "OrExpression"
	case "and":
		return "AndExpression"
	case "contains":
		return "ContainsExpression"
	case "==":
		return "EqualsExpression"
	case "!=":
		return "NotEqualsExpression"
	case ">":
		return "GreaterThanExpression"
	case "<":
		return "LessThanExpression"
	case ">=":
		return "GreaterThanEqualsExpression"
	case "<=":
		return "LessThanEqualsExpression"
	case "+":
		return "AddExpression"
	case "-":
		return "SubtractExpression"
	case "*":
		return "MultiplyExpression"
	case "/":
		return "DivideExpression"
	}
	return "ModulusExpression"
}
