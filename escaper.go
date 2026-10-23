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
	"unicode/utf16"

	"github.com/PebbleTemplates/pebble/internal/jstr"
)

// The escaping strategies io.pebbletemplates.pebble.extension.escaper.EscapeFilter registers.
const (
	htmlEscapeStrategy       = "html"
	javaScriptEscapeStrategy = "js"
	cssEscapeStrategy        = "css"
	urlParamEscapeStrategy   = "url_param"
	jsonEscapeStrategy       = "json"
)

// Escape applies an escaping strategy by name, as the escape filter and the escaper's node
// visitor do. An unknown name is an error; the caller turns it into the filter's
// "Unknown escaping strategy [...]" message.
func Escape(value string, strategy string) (string, error) {
	switch strategy {
	case htmlEscapeStrategy:
		return jstr.EscapeHTML(value), nil
	case javaScriptEscapeStrategy:
		return jstr.EscapeJS(value), nil
	case cssEscapeStrategy:
		return jstr.EscapeCSS(value), nil
	case urlParamEscapeStrategy:
		return jstr.EscapeURLParam(value), nil
	case jsonEscapeStrategy:
		return escapeJSON(value), nil
	default:
		return "", fmt.Errorf("unknown escaping strategy %q", strategy)
	}
}

// escapeJSON is org.unbescape.json.JsonEscape.escapeJson at its default level, which is
// SINGLE_ESCAPE_CHARS_DEFAULT_TO_UHEXA over LEVEL_2_ALL_NON_ASCII_PLUS_BASIC_ESCAPE_SET: the five
// single character escapes of JSON, a quote, a backslash and an ampersand, every other control
// character and every non-ASCII code unit as \uXXXX with upper case hexadecimal digits.
//
// The other four strategies live in the jstr package, which holds the unbescape tables; this one
// is small enough to spell out and unbescape offers no JSON entry there.
func escapeJSON(value string) string {
	var b strings.Builder
	for _, unit := range utf16.Encode([]rune(value)) {
		switch unit {
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '&':
			b.WriteString(`\u0026`)
		default:
			if unit < 0x20 || unit > 0x7e {
				fmt.Fprintf(&b, `\u%04X`, unit)
			} else {
				b.WriteByte(byte(unit))
			}
		}
	}
	return b.String()
}

// VisitEscaper is io.pebbletemplates.pebble.extension.escaper.EscaperNodeVisitor: it walks the
// tree and wraps the expression of every print node that is not already known to be safe in a
// call of the escape filter, with the strategy of the innermost autoescape tag as its argument.
//
// The walk is the one java.util.AbstractNodeVisitor defines, which has a telling gap: the visitor
// interface declares no overload for the cache, embed and from nodes, so visitor.visit resolves
// to the do-nothing visit(Node) for them and the bodies of a cache and of an embed tag are never
// escaped at all. This port keeps that.
func VisitEscaper(root *Stmt, opts *Options) {
	auto := true
	if opts != nil {
		auto = opts.AutoEscaping
	}
	v := &escaperVisitor{active: []bool{auto}, strategies: []string{""}}
	v.visit(root)
}

// escaperVisitor holds the two stacks of the Java visitor: whether escaping is on, and the
// strategy of the enclosing autoescape tags. An empty strategy is the Java null, which means the
// escape filter is called with no argument at all and so uses the engine's default.
type escaperVisitor struct {
	active     []bool
	strategies []string
}

func (v *escaperVisitor) visit(node *Stmt) {
	if node == nil {
		return
	}
	switch node.Kind {
	case StmtRoot, StmtBlock, StmtParallel:
		v.visit(node.Body)
	case StmtBody:
		for _, child := range node.Children {
			v.visit(child)
		}
	case StmtPrint, StmtFilter:
		// The filter tag is a print node in the Java parser, so it is escaped the same way; its
		// body is not walked, exactly as a print node's expression is not.
		node.Expr = v.escapeIfUnsafe(node.Expr)
	case StmtIf:
		for _, body := range node.Bodies {
			v.visit(body)
		}
		v.visit(node.Else)
	case StmtFor:
		v.visit(node.Body)
		v.visit(node.Else)
	case StmtMacro:
		v.visit(node.Body)
	case StmtAutoEscape:
		v.active = append(v.active, node.Flag)
		v.strategies = append(v.strategies, node.Extra)
		v.visit(node.Body)
		v.active = v.active[:len(v.active)-1]
		v.strategies = v.strategies[:len(v.strategies)-1]
	}
	// Text, set, extends, include, import, flush carry no print node, and cache, embed and from
	// are the nodes the Java visitor cannot see into.
}

// escapeIfUnsafe wraps an expression in the escape filter when it is not already safe. A ternary
// operator is treated branch by branch, as the Java visitor does, because its condition never
// reaches the output.
func (v *escaperVisitor) escapeIfUnsafe(expr *Expr) *Expr {
	if expr == nil {
		return nil
	}
	if expr.Kind == ExprTernary && len(expr.Args) == 3 {
		if v.isUnsafe(expr.Args[1]) {
			expr.Args[1] = v.escape(expr.Args[1])
		}
		if v.isUnsafe(expr.Args[2]) {
			expr.Args[2] = v.escape(expr.Args[2])
		}
		return expr
	}
	if v.isUnsafe(expr) {
		return v.escape(expr)
	}
	return expr
}

// isUnsafe is the safe-output analysis of the Java visitor. It is deliberately narrow: only a
// string literal, the parent and block functions and a concatenation of two string literals are
// known to be safe. Everything else is wrapped, and the escape filter itself is what lets a
// SafeString - the result of raw, of date or of an earlier escape - through untouched.
func (v *escaperVisitor) isUnsafe(expr *Expr) bool {
	if len(v.active) > 0 && !v.active[len(v.active)-1] {
		return false
	}
	switch expr.Kind {
	case ExprLiteralString:
		// An interpolated string is a chain of concatenations in the Java parser, not a literal.
		return expr.Extra == "interpolated"
	case ExprParentFunction, ExprBlockFunction:
		return false
	case ExprConcat:
		return !(len(expr.Args) == 2 &&
			isPlainStringLiteral(expr.Args[0]) && isPlainStringLiteral(expr.Args[1]))
	default:
		return true
	}
}

func isPlainStringLiteral(expr *Expr) bool {
	return expr != nil && expr.Kind == ExprLiteralString && expr.Extra != "interpolated"
}

// escape builds the "expression | escape(strategy='...')" filter expression the Java visitor
// splices into the tree.
func (v *escaperVisitor) escape(expr *Expr) *Expr {
	args := &Arguments{}
	if strategy := v.strategies[len(v.strategies)-1]; strategy != "" {
		args.Names = append(args.Names, "strategy")
		args.Named = append(args.Named, &Expr{
			Kind: ExprLiteralString, Line: expr.Line, Value: strategy})
	}
	return &Expr{Kind: ExprFilter, Line: expr.Line, Name: "escape",
		Args: []*Expr{expr}, Arguments: args}
}
