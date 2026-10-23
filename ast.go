/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

// Package pebble is a Go port of the Pebble template engine (release 4.1.2).
package pebble

import "io"

// TokenType is the kind of a token produced by the lexer.
type TokenType int

// The token types of the Java lexer, in the same order.
const (
	TokenEOF TokenType = iota
	TokenText
	TokenExecuteStart
	TokenExecuteEnd
	TokenPrintStart
	TokenPrintEnd
	TokenString
	TokenNumber
	TokenOperator
	TokenPunctuation
	TokenName
	TokenWhitespace
)

// Token is one lexer token: its type, its value and the 1-based line it starts on.
type Token struct {
	Type  TokenType
	Value string
	Line  int
}

// ExprKind tags the Expr union below.
type ExprKind int

// The expression kinds. The parser builds them; the evaluator renders them.
const (
	ExprLiteralString     ExprKind = iota // Value: string, Extra: "interpolated" when it came from "" with #{}
	ExprLiteralLong                       // Value: int64
	ExprLiteralInt                        // Value: int32 (literalDecimalTreatedAsInteger)
	ExprLiteralDouble                     // Value: float64
	ExprLiteralBigDecimal                 // Value: the literal text, converted by the evaluator
	ExprLiteralBool                       // Value: bool
	ExprLiteralNull
	ExprContextVariable // Name: variable name
	ExprGetAttribute    // Args[0]: object, Args[1]: attribute expression; Arguments: call arguments or nil
	ExprFilter          // Args[0]: input, Name: filter name, Arguments: filter arguments
	ExprFunction        // Name: function or macro name, Arguments: call arguments
	ExprTest            // Args[0]: subject, Name: test name, Arguments: test arguments, Negated
	ExprBinary          // Op: operator symbol, Args[0], Args[1]
	ExprUnary           // Op: operator symbol ("not", "+", "-"), Args[0]
	ExprTernary         // Args[0]: condition, Args[1]: then, Args[2]: else
	ExprArray           // Args: elements
	ExprMap             // Args: alternating key, value pairs
	ExprRange           // Args[0]: from, Args[1]: to
	ExprConcat          // Args[0], Args[1]
	ExprParentFunction  // the parent() call inside a block
	ExprBlockFunction   // the block("name") call; Arguments holds the name expression
	ExprRenderableNode  // Body: a node rendered into a string (a filter or macro body)
)

// Expr is one expression node of the template AST.
type Expr struct {
	Kind      ExprKind
	Line      int
	Name      string
	Op        string
	Value     any
	Extra     string
	Negated   bool
	Args      []*Expr
	Arguments *Arguments
	Body      *Stmt
}

// Arguments is an argument list of a filter, function, macro or test invocation: the positional
// arguments first, then the named ones in source order.
type Arguments struct {
	// Line is the line ArgumentsNode reports, which the argument errors of a filter, function,
	// macro or test invocation carry.
	Line       int
	Positional []*Expr
	Names      []string
	Named      []*Expr
}

// StmtKind tags the Stmt union below.
type StmtKind int

// The renderable node kinds.
const (
	StmtRoot       StmtKind = iota // Body: the body; the root of a template
	StmtBody                       // Children: the nodes of a body
	StmtText                       // Text: literal template text
	StmtPrint                      // Expr: the expression to print
	StmtIf                         // Conditions/Bodies pairwise, Else may be nil
	StmtFor                        // Name, Name2 (key, value or just value), Expr: the iterable, Body, Else
	StmtSet                        // Name: variable, Expr: value
	StmtBlock                      // Name: block name, Body
	StmtExtends                    // Expr: the parent template name
	StmtInclude                    // Expr: template name, Arguments: with/map
	StmtEmbed                      // Expr: template name, Arguments: with/map, Body: the overriding blocks
	StmtImport                     // Expr: template name, Name: alias ("" when none)
	StmtFrom                       // Expr: template name, Names/Aliases: the imported macro names
	StmtMacro                      // Name: macro name, Arguments: the declared parameters, Body
	StmtAutoEscape                 // Extra: strategy ("" means the default), Flag: on/off, Body
	StmtFilter                     // Expr: the filter chain applied to Body
	StmtCache                      // Arguments: the cache key arguments, Body
	StmtParallel                   // Body
	StmtFlush
)

// Stmt is one renderable node of the template AST.
type Stmt struct {
	Kind       StmtKind
	Line       int
	Name       string
	Name2      string
	Text       string
	Extra      string
	Flag       bool
	Expr       *Expr
	Arguments  *Arguments
	Body       *Stmt
	Else       *Stmt
	Children   []*Stmt
	Conditions []*Expr
	Bodies     []*Stmt
	Names      []string
	Aliases    []string
}

// Options are the engine settings the reference command exposes.
type Options struct {
	Prefix                      string // the absolute template directory
	StrictVariables             bool
	NewLineTrimming             bool // default true
	AutoEscaping                bool // default true
	DefaultEscapingStrategy     string
	LiteralDecimalTreatedAsInt  bool
	LiteralNumbersAsBigDecimals bool
	MaxRenderedSize             int    // -1 is unlimited
	Locale                      string // the canonical language tag, "en-US"
	LocaleDisplay               string // java.util.Locale.toString of it, "en_US", which the locale global prints
	TimeZone                    string
}

// Writer is what a template renders into.
type Writer interface {
	io.Writer
}
