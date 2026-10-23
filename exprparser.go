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
	"strconv"
	"strings"
)

// reservedKeywords are the names ExpressionParser refuses to assign to.
var reservedKeywords = map[string]bool{"true": true, "false": true, "null": true, "none": true}

// The operator tables of optable.go, keyed by symbol as the extension registry keys them.
var (
	binaryOperators = func() map[string]BinaryOp {
		m := make(map[string]BinaryOp, len(BinaryOps))
		for _, op := range BinaryOps {
			m[op.Symbol] = op
		}
		return m
	}()
	unaryOperators = func() map[string]UnaryOp {
		m := make(map[string]UnaryOp, len(UnaryOps))
		for _, op := range UnaryOps {
			m[op.Symbol] = op
		}
		return m
	}()
)

// unaryOperator and binaryOperator report whether the token is an operator of that kind.
func unaryOperator(token Token) (UnaryOp, bool) {
	if token.Type != TokenOperator {
		return UnaryOp{}, false
	}
	op, ok := unaryOperators[token.Value]
	return op, ok
}

func binaryOperator(token Token) (BinaryOp, bool) {
	if token.Type != TokenOperator {
		return BinaryOp{}, false
	}
	op, ok := binaryOperators[token.Value]
	return op, ok
}

// parseExpression is the public entry point of the expression parser.
func (p *parser) parseExpression() *Expr { return p.parseExpressionPrec(0) }

/*
 * parseExpressionPrec takes the precedence required to operate a "precedence climbing" parsing
 * algorithm; it is recursive.
 *
 * See http://en.wikipedia.org/wiki/Operator-precedence_parser
 */
func (p *parser) parseExpressionPrec(minPrecedence int) *Expr {
	token := p.currentToken()
	var expression *Expr

	/*
	 * The first check is to see if the expression begins with a unary operator, or an opening
	 * bracket, or neither.
	 */
	if operator, ok := unaryOperator(token); ok {
		p.next()
		child := p.parseExpressionPrec(operator.Precedence)
		expression = &Expr{Kind: ExprUnary, Line: p.currentToken().Line, Op: operator.Symbol,
			Args: []*Expr{child}}
	} else {
		switch {
		case test(token, TokenPunctuation, "("):
			p.next()
			expression = p.parseExpression()
			p.expectValue(TokenPunctuation, ")")
			expression = p.parsePostfixExpression(expression)

		case test(token, TokenPunctuation, "["):
			// array definition syntax; the [ token is preserved for the array parser, and the
			// closing ] is expected there, so no postfix expression follows
			expression = p.parseArrayDefinitionExpression()

		case test(token, TokenPunctuation, "{"):
			// map definition syntax, same as above
			expression = p.parseMapDefinitionExpression()

		default:
			/*
			 * Starts with neither. Let's parse out the first expression that we can find; there
			 * may be more of them, separated by binary operators.
			 */
			expression = p.subparseExpression()
		}
	}

	/*
	 * If, after parsing the first expression, we encounter a binary operator then we know we have
	 * another expression on the other side of the operator that requires parsing.
	 */
	token = p.currentToken()
	for {
		operator, ok := binaryOperator(token)
		if !ok || operator.Precedence < minPrecedence {
			break
		}

		// find out which operator we are dealing with and then skip over it
		p.next()

		final := &Expr{Args: []*Expr{expression, nil}}
		switch operator.Type {
		case OpFilter:
			// the right hand expression of the FILTER operator is handled in a unique way
			final.Kind = ExprFilter
			final.Name, final.Arguments = p.parseFilterInvocationExpression()
			final.Args = final.Args[:1]
		case OpTest:
			// the right hand expression of TEST operators is handled in a unique way
			final.Kind = ExprTest
			final.Negated = operator.Symbol == "is not"
			final.Name, final.Arguments = p.parseTestInvocationExpression()
			final.Args = final.Args[:1]
		default:
			/*
			 * Parse the expression on the right hand side of the operator while maintaining
			 * proper associativity and precedence. Every operator of the core extension is left
			 * associative.
			 */
			right := operator.Precedence
			if !operator.RightAssoc {
				right++
			}
			final.Args[1] = p.parseExpressionPrec(right)
			switch operator.Symbol {
			case "~":
				final.Kind = ExprConcat
			case "..":
				final.Kind = ExprRange
			default:
				final.Kind = ExprBinary
				final.Op = operator.Symbol
				// "equals" is a second spelling of "==": both build an EqualsExpression
				if final.Op == "equals" {
					final.Op = "=="
				}
			}
		}

		final.Line = p.currentToken().Line
		expression = final
		token = p.currentToken()
	}

	if minPrecedence == 0 {
		return p.parseTernaryExpression(expression)
	}
	return expression
}

/*
 * subparseExpression finds the next "simple" expression: one that can be found on either side of a
 * binary operator but does not itself contain one, such as "var.field", "true" or "12".
 */
func (p *parser) subparseExpression() *Expr {
	token := p.currentToken()
	var node *Expr

	switch token.Type {

	case TokenName:
		switch token.Value {

		// a constant?
		case "true", "TRUE":
			node = &Expr{Kind: ExprLiteralBool, Line: token.Line, Value: true}
			p.next()
		case "false", "FALSE":
			node = &Expr{Kind: ExprLiteralBool, Line: token.Line, Value: false}
			p.next()
		case "none", "NONE", "null", "NULL":
			node = &Expr{Kind: ExprLiteralNull, Line: token.Line}
			p.next()

		default:
			if test(p.peek(), TokenPunctuation, "(") {
				// name of a function: the node carries the line of the opening bracket, as the
				// FunctionOrMacroNameNode of the Java parser does
				node = &Expr{Kind: ExprFunction, Line: p.peek().Line, Name: token.Value}
			} else {
				// variable name
				node = &Expr{Kind: ExprContextVariable, Line: token.Line, Name: token.Value}
			}
			p.next()
		}

	case TokenLong:
		node = &Expr{Kind: ExprLiteralLong, Line: token.Line, Value: p.parseLong(token.Value)}
		p.next()

	case TokenNumber:
		node = p.parseNumber(token)
		p.next()

	case TokenString, TokenInterpolationStart:
		node = p.parseStringExpression()

	default:
		// not found, syntax error
		p.fail(fmt.Sprintf("Unexpected token \"%s\" of value \"%s\"", javaTokenName(token.Type),
			javaTokenValue(token)), token.Line)
	}

	// there may or may not be more to this expression - let's keep looking
	return p.parsePostfixExpression(node)
}

// parseNumber turns a NUMBER token into a literal, which the parser options decide the type of.
func (p *parser) parseNumber(token Token) *Expr {
	switch {
	case p.opts.LiteralNumbersAsBigDecimals:
		// the evaluator builds the BigDecimal out of the literal text
		return &Expr{Kind: ExprLiteralBigDecimal, Line: token.Line, Value: token.Value}
	case strings.Contains(token.Value, "."):
		value, err := strconv.ParseFloat(token.Value, 64)
		if err != nil {
			p.failNumberFormat(token.Value)
		}
		return &Expr{Kind: ExprLiteralDouble, Line: token.Line, Value: value}
	case p.opts.LiteralDecimalTreatedAsInt:
		value, err := strconv.ParseInt(token.Value, 10, 32)
		if err != nil {
			p.failNumberFormat(token.Value)
		}
		return &Expr{Kind: ExprLiteralInt, Line: token.Line, Value: int32(value)}
	default:
		return &Expr{Kind: ExprLiteralLong, Line: token.Line, Value: p.parseLong(token.Value)}
	}
}

func (p *parser) parseLong(text string) int64 {
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		p.failNumberFormat(text)
	}
	return value
}

/*
 * failNumberFormat aborts with the NumberFormatException of Long.valueOf and friends: a literal
 * that does not fit its type is not a ParserException but the raw Java exception, which the
 * reference command prints as such.
 */
func (p *parser) failNumberFormat(text string) {
	panic(bailout{&JavaError{Kind: "NumberFormatException",
		Message: `For input string: "` + text + `"`}})
}

/*
 * parseStringExpression parses a string literal, which may be several tokens when it contains
 * #{} interpolation: the parts become a right leaning chain of concatenations.
 */
func (p *parser) parseStringExpression() *Expr {
	var nodes []*Expr
	previousWasString := false

	// Sequential strings are not OK, but strings can follow interpolation
	for {
		if p.currentIs(TokenString) && !previousWasString {
			token := p.expect(TokenString)
			nodes = append(nodes, &Expr{Kind: ExprLiteralString, Line: token.Line,
				Value: token.Value})
			previousWasString = true
		} else if p.currentIs(TokenInterpolationStart) {
			p.expect(TokenInterpolationStart)
			nodes = append(nodes, p.parseExpression())
			p.expect(TokenInterpolationEnd)
			previousWasString = false
		} else {
			break
		}
	}

	first := nodes[0]
	nodes = nodes[1:]
	if len(nodes) == 0 {
		return first
	}

	// a concatenation takes its line number from its left hand side
	firstExpr := &Expr{Kind: ExprConcat, Line: first.Line, Args: []*Expr{first, nil}}
	expr := firstExpr
	for i, node := range nodes {
		if i == len(nodes)-1 {
			expr.Args[1] = node
		} else {
			newExpr := &Expr{Kind: ExprConcat, Line: node.Line, Args: []*Expr{node, nil}}
			expr.Args[1] = newExpr
			expr = newExpr
		}
	}
	return firstExpr
}

func (p *parser) parseTernaryExpression(expression *Expr) *Expr {
	// if the next token isn't a ?, we're not dealing with a ternary expression
	if !p.currentIs(TokenPunctuation, "?") {
		return expression
	}

	p.next()
	expression2 := p.parseExpression()
	p.expectValue(TokenPunctuation, ":")
	expression3 := p.parseExpression()

	return &Expr{Kind: ExprTernary, Line: p.currentToken().Line,
		Args: []*Expr{expression, expression2, expression3}}
}

/*
 * parsePostfixExpression determines whether there is more to the provided expression than we
 * originally thought: an attribute of a variable (var.attribute, var['attribute'],
 * var.attribute(bar)) or a function call.
 */
func (p *parser) parsePostfixExpression(node *Expr) *Expr {
	for {
		current := p.currentToken()
		switch {
		case test(current, TokenPunctuation, "."), test(current, TokenPunctuation, "["):
			// a period represents getting an attribute from a variable or calling a method
			node = p.parseBeanAttributeExpression(node)
		case test(current, TokenPunctuation, "("):
			// function call
			node = p.parseFunctionOrMacroInvocation(node)
		default:
			return node
		}
	}
}

func (p *parser) parseFunctionOrMacroInvocation(node *Expr) *Expr {
	/*
	 * Java casts the expression to a FunctionOrMacroNameNode here, which only the bare name of a
	 * function is; anything else, as in "a[0](1)" or "f(1)(2)", fails the cast. A function name
	 * node is an ExprFunction that does not have its arguments yet.
	 */
	if node.Kind != ExprFunction || node.Arguments != nil {
		const target = "io.pebbletemplates.pebble.node.FunctionOrMacroNameNode"
		class := javaExpressionClass(node)
		panic(bailout{&JavaError{Kind: "ClassCastException", Message: fmt.Sprintf(
			"class %s cannot be cast to class %s (%s and %s are in unnamed module of loader 'app')",
			class, target, class, target)}})
	}
	functionName := node.Name
	args := p.parseArguments(false)

	/*
	 * The following core functions have their own nodes and are rendered in unique ways for the
	 * sake of performance.
	 */
	switch functionName {
	case "parent":
		return &Expr{Kind: ExprParentFunction, Line: p.currentToken().Line,
			Name: p.peekBlockStack()}
	case "block":
		if len(args.Positional) == 0 {
			// BlockFunctionExpression reads the first positional argument in its constructor
			panic(bailout{&JavaError{Kind: "IndexOutOfBoundsException",
				Message: "Index 0 out of bounds for length 0"}})
		}
		return &Expr{Kind: ExprBlockFunction, Line: node.Line, Arguments: args}
	}

	return &Expr{Kind: ExprFunction, Line: node.Line, Name: functionName, Arguments: args}
}

// parseFilterInvocationExpression parses the right hand side of the filter operator: the filter
// name and its arguments.
func (p *parser) parseFilterInvocationExpression() (string, *Arguments) {
	filterToken := p.expect(TokenName)

	var args *Arguments
	if p.currentIs(TokenPunctuation, "(") {
		args = p.parseArguments(false)
	} else {
		args = &Arguments{Line: filterToken.Line}
	}
	return filterToken.Value, args
}

// parseTestInvocationExpression parses the right hand side of the "is" and "is not" operators.
func (p *parser) parseTestInvocationExpression() (string, *Arguments) {
	testToken := p.expect(TokenName)

	var args *Arguments
	if p.currentIs(TokenPunctuation, "(") {
		args = p.parseArguments(false)
	} else {
		args = &Arguments{Line: testToken.Line}
	}
	return testToken.Value, args
}

/*
 * parseBeanAttributeExpression parses an expression getting an attribute from a variable in the
 * context, or calling a method on it: foo.bar, foo['bar'] or foo.bar('baz').
 */
func (p *parser) parseBeanAttributeExpression(node *Expr) *Expr {
	if p.currentIs(TokenPunctuation, ".") {
		// skip over the '.' token
		p.next()

		token := p.expect(TokenName)

		var args *Arguments
		if p.currentIs(TokenPunctuation, "(") {
			args = p.parseArguments(false)
			if len(args.Named) > 0 {
				p.fail("Can not use named arguments when calling a bean method",
					p.currentToken().Line)
			}
		}

		return &Expr{Kind: ExprGetAttribute, Line: token.Line, Arguments: args,
			Args: []*Expr{node, {Kind: ExprLiteralString, Line: token.Line, Value: token.Value}}}
	}

	if p.currentIs(TokenPunctuation, "[") {
		// skip over opening '[' bracket
		p.next()

		attribute := p.parseExpression()
		result := &Expr{Kind: ExprGetAttribute, Line: p.currentToken().Line,
			Args: []*Expr{node, attribute}}

		// move past the closing ']' bracket
		p.expectValue(TokenPunctuation, "]")
		return result
	}

	return node
}

/*
 * parseArguments parses an argument list. Most arguments consist of values with optional names,
 * but in the case of a macro definition the user is specifying names with optional values, so the
 * logic changes slightly.
 */
func (p *parser) parseArguments(isMacroDefinition bool) *Arguments {
	args := &Arguments{}

	p.expectValue(TokenPunctuation, "(")

	for !p.currentIs(TokenPunctuation, ")") {

		argumentName := ""
		named := false
		var argumentValue *Expr

		if len(args.Named) > 0 || len(args.Positional) > 0 {
			p.expectValue(TokenPunctuation, ",")
		}

		if isMacroDefinition {
			argumentName = p.parseNewVariableName()
			named = true
			if p.currentIs(TokenPunctuation, "=") {
				p.expectValue(TokenPunctuation, "=")
				argumentValue = p.parseExpression()
			}
		} else {
			if test(p.peek(), TokenPunctuation, "=") {
				argumentName = p.parseNewVariableName()
				named = true
				p.expectValue(TokenPunctuation, "=")
			}
			argumentValue = p.parseExpression()
		}

		if !named {
			if len(args.Named) > 0 {
				p.fail("Positional arguments must be declared before any named arguments.",
					p.currentToken().Line)
			}
			args.Positional = append(args.Positional, argumentValue)
		} else {
			args.Names = append(args.Names, argumentName)
			args.Named = append(args.Named, argumentValue)
		}
	}

	p.expectValue(TokenPunctuation, ")")

	args.Line = p.currentToken().Line
	return args
}

/*
 * parseNewVariableName parses the name of a new variable, which is used by the set tag, the for
 * loop and named arguments.
 */
func (p *parser) parseNewVariableName() string {
	token := p.expect(TokenName)

	if reservedKeywords[token.Value] {
		p.fail("Can not assign a value to "+token.Value, token.Line)
	}

	return token.Value
}

func (p *parser) parseArrayDefinitionExpression() *Expr {
	// expect the opening bracket and check for an empty array
	p.expectValue(TokenPunctuation, "[")
	if p.currentIs(TokenPunctuation, "]") {
		p.next()
		return &Expr{Kind: ExprArray, Line: p.currentToken().Line}
	}

	// there's at least one expression in the array
	var elements []*Expr
	for {
		elements = append(elements, p.parseExpression())
		if p.currentIs(TokenPunctuation, "]") {
			// this seems to be the end of the array
			break
		}
		// expect the comma separator, until we either find a closing bracket or fail the expect
		p.expectValue(TokenPunctuation, ",")
	}

	// expect the closing bracket
	p.expectValue(TokenPunctuation, "]")

	return &Expr{Kind: ExprArray, Line: p.currentToken().Line, Args: elements}
}

/*
 * parseMapDefinitionExpression parses a map literal. The entries are kept in source order, as
 * alternating key and value expressions; Java keeps them in a HashMap of expressions, whose order
 * the evaluation does not depend on, because it builds a HashMap of the evaluated keys.
 */
func (p *parser) parseMapDefinitionExpression() *Expr {
	// expect the opening brace and check for an empty map
	p.expectValue(TokenPunctuation, "{")
	if p.currentIs(TokenPunctuation, "}") {
		p.next()
		return &Expr{Kind: ExprMap, Line: p.currentToken().Line}
	}

	// there's at least one expression in the map
	var elements []*Expr
	for {
		// key : value
		key := p.parseExpression()
		p.expectValue(TokenPunctuation, ":")
		value := p.parseExpression()
		elements = append(elements, key, value)
		if p.currentIs(TokenPunctuation, "}") {
			// this seems to be the end of the map
			break
		}
		// expect the comma separator, until we either find a closing brace or fail the expect
		p.expectValue(TokenPunctuation, ",")
	}

	// expect the closing brace
	p.expectValue(TokenPunctuation, "}")

	return &Expr{Kind: ExprMap, Line: p.currentToken().Line, Args: elements}
}
