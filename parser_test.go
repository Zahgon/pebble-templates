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
	"testing"
)

// describe prints an error the way the reference command does, so that the expectations below can
// be copied straight out of its output.
func describe(err error) string {
	switch e := err.(type) {
	case nil:
		return ""
	case *Error:
		return e.Kind + ": " + e.Error()
	case *JavaError:
		return e.Kind + ": " + e.Message
	}
	return err.Error()
}

// TestParseErrors checks the parser errors against the reference command: every expectation here
// is the first line that `ref t i.peb` printed for the same template, and an empty one means that
// the reference command got past parsing.
func TestParseErrors(t *testing.T) {
	cases := []struct {
		source string
		want   string
	}{
		{"{% if %}", "ParserException: Unexpected token \"EXECUTE_END\" of value \"%}\" (i.peb:1)"},
		{"{{ }}", "ParserException: Unexpected token \"PRINT_END\" of value \"}}\" (i.peb:1)"},
		{"{{ foo", "ParserException: Unexpected token of value \"null\" and type EOF, expected token of type PRINT_END (i.peb:1)"},
		{"{% if x %}", "ParserException: Unexpected end of template. Pebble was looking for the \"endif\" tag (i.peb:1)"},
		{"{% if x %}a{% else %}b", "ParserException: Unexpected end of template. Pebble was looking for the \"endif\" tag (i.peb:1)"},
		{"{% if x %}a{% elseiff %}b{% endif %}", "ParserException: Unexpected tag name \"elseiff\" (i.peb:1)"},
		{"{% for %}", "ParserException: Unexpected token of value \"%}\" and type EXECUTE_END, expected token of type NAME (i.peb:1)"},
		{"{% for x in %}", "ParserException: Unexpected token \"EXECUTE_END\" of value \"%}\" (i.peb:1)"},
		{"{% for x in y %}", "ParserException: Unexpected end of template. Pebble was looking for the \"endfor\" tag (i.peb:1)"},
		{"{% block %}", "ParserException: Unexpected token of value \"%}\" and type EXECUTE_END, expected token of type NAME (i.peb:1)"},
		{"{% block a %}", "ParserException: endblock tag should be present with block tag starting line number  (i.peb:1)"},
		{"{% foo %}", "ParserException: Unexpected tag name \"foo\" (i.peb:1)"},
		{"{% %}", "ParserException: A block must start with a tag name. (i.peb:1)"},
		{"{{ 1 + }}", "ParserException: Unexpected token \"PRINT_END\" of value \"}}\" (i.peb:1)"},
		{"{{ foo( }}", "ParserException: Unclosed \")\" (i.peb:1)"},
		{"{{ foo(a=1, 2) }}", "ParserException: Positional arguments must be declared before any named arguments. (i.peb:1)"},
		{"{{ foo.bar(a=1) }}", "ParserException: Can not use named arguments when calling a bean method (i.peb:1)"},
		{"{% set 1 = 2 %}", "ParserException: Unexpected token of value \"1\" and type NUMBER, expected token of type NAME (i.peb:1)"},
		{"{% set true = 2 %}", "ParserException: Can not assign a value to true (i.peb:1)"},
		{"{# unclosed", "ParserException: Unclosed comment. (i.peb:1)"},
		{"{% verbatim %}x", "ParserException: Unclosed verbatim tag. (i.peb:1)"},
		{"{{ (1 }}", "ParserException: Unclosed \")\" (i.peb:1)"},
		{"{{ 1) }}", "ParserException: Unexpected \")\" (i.peb:1)"},
		{"{{ \"a }}", "ParserException: Unclosed \"\"\" (i.peb:1)"},
		{"{{ a|  }}", "ParserException: Unexpected token of value \"}}\" and type PRINT_END, expected token of type NAME (i.peb:1)"},
		{"{% import %}", "ParserException: Unexpected token \"EXECUTE_END\" of value \"%}\" (i.peb:1)"},
		{"{% from \"a\" %}", "ParserException: Unexpected token of value \"%}\" and type EXECUTE_END, expected token of type NAME (i.peb:1)"},
		{"{% from \"a\" import b c %}", "ParserException: Unexpected token of value \"c\" and type NAME, expected token of type EXECUTE_END or ','  (i.peb:1)"},
		{"{% include \"a\" with 1 %}", "ParserException: Unexpected expression 'io.pebbletemplates.pebble.node.expression.LiteralLongExpression'. (i.peb:1)"},
		{"{% include \"a\" with 'x' %}", "ParserException: Unexpected expression 'io.pebbletemplates.pebble.node.expression.LiteralStringExpression'. (i.peb:1)"},
		{"{% macro f(1) %}", "ParserException: Unexpected token of value \"1\" and type NUMBER, expected token of type NAME (i.peb:1)"},
		{"{{ [1, }}", "ParserException: Unclosed \"]\" (i.peb:1)"},
		{"{{ {1} }}", "ParserException: Unexpected token of value \"}\" and type PUNCTUATION, expected token of type PUNCTUATION (i.peb:1)"},
		{"{{ a ? b }}", "ParserException: Unexpected token of value \"}}\" and type PRINT_END, expected token of type PUNCTUATION (i.peb:1)"},
		{"{% autoescape %}", "IndexOutOfBoundsException: Index 4 out of bounds for length 4"},
		{"{{ @ }}", "ParserException: Unexpected character [@] (i.peb:1)"},
		{"{{ ", "ParserException: Unexpected character [\x00] (i.peb:1)"},
		{"\n\n{% if %}", "ParserException: Unexpected token \"EXECUTE_END\" of value \"%}\" (i.peb:3)"},
		{"{{ 1\n+ }}", "ParserException: Unexpected token \"PRINT_END\" of value \"}}\" (i.peb:2)"},
		{"{% embed \"a\" %}x{% endembed %}", "ParserException: A template that extends another one cannot include content outside blocks. Did you forget to put the content inside a {% block %} tag? (i.peb:1)"},
		{"{{ block() }}", "IndexOutOfBoundsException: Index 0 out of bounds for length 0"},
		{"{{ 99999999999999999999 }}", "NumberFormatException: For input string: \"99999999999999999999\""},
		{"{% endif %}", "ParserException: Unexpected tag name \"endif\" (i.peb:1)"},
		{"{{ foo bar }}", "ParserException: Unexpected token of value \"bar\" and type NAME, expected token of type PRINT_END (i.peb:1)"},
		{"{{ 'a' 'b' }}", "ParserException: Unexpected token of value \"b\" and type STRING, expected token of type PRINT_END (i.peb:1)"},
		{"{% cache %}", "ParserException: Unexpected token \"EXECUTE_END\" of value \"%}\" (i.peb:1)"},
		{"{% macro f() %}", "IndexOutOfBoundsException: Index 7 out of bounds for length 7"},
		{"{% filter %}", "ParserException: Unexpected token of value \"%}\" and type EXECUTE_END, expected token of type NAME (i.peb:1)"},
		{"{% filter upper %}", "IndexOutOfBoundsException: Index 5 out of bounds for length 5"},
		{"{% parallel %}", "IndexOutOfBoundsException: Index 4 out of bounds for length 4"},
		{"{% extends %}", "ParserException: Unexpected token \"EXECUTE_END\" of value \"%}\" (i.peb:1)"},
		{"{% embed %}", "ParserException: Unexpected token \"EXECUTE_END\" of value \"%}\" (i.peb:1)"},
		{"{% block a %}{% endblock b %}", "ParserException: Unexpected token of value \"b\" and type NAME, expected token of type EXECUTE_END (i.peb:1)"},
		{"{{ a is %}", "ParserException: Unexpected \"}\" (i.peb:1)"},
		{"{{ a is not }}", "ParserException: Unexpected token of value \"}}\" and type PRINT_END, expected token of type NAME (i.peb:1)"},
		{"{{ foo[1 }}", "ParserException: Unclosed \"]\" (i.peb:1)"},
		{"{{ foo[] }}", "ParserException: Unexpected token \"PUNCTUATION\" of value \"]\" (i.peb:1)"},
		{"{% set a = %}", "ParserException: Unexpected token \"EXECUTE_END\" of value \"%}\" (i.peb:1)"},
		{"{% if a %}{% endif %}{% endif %}", "ParserException: Unexpected tag name \"endif\" (i.peb:1)"},
		{"{{ 'a' ~ }}", "ParserException: Unexpected token \"PRINT_END\" of value \"}}\" (i.peb:1)"},
		{"{{ not }}", "ParserException: Unexpected token \"PRINT_END\" of value \"}}\" (i.peb:1)"},
		{"{{ 1..}}", "ParserException: Unexpected token \"PRINT_END\" of value \"}}\" (i.peb:1)"},
		{"{% for a, b in c %}{% endfor %}", "ParserException: Unexpected token of value \",\" and type PUNCTUATION, expected token of type NAME (i.peb:1)"},
		{"{{ \"a#{1}\" }}x{{", "ParserException: Unexpected token \"EOF\" of value \"null\" (i.peb:1)"},
		{"{% autoescape 'html' %}{% endautoescape %}", ""},
		{"{% verbatim %}{% endverbatim %}{% endverbatim %}", "ParserException: Unexpected tag name \"endverbatim\" (i.peb:1)"},
		{"{{ a.b. }}", "ParserException: Unexpected token of value \"}}\" and type PRINT_END, expected token of type NAME (i.peb:1)"},
		{"{{ a['x' }}", "ParserException: Unclosed \"]\" (i.peb:1)"},
		{"{% block 1 %}", "ParserException: Unexpected token of value \"1\" and type NUMBER, expected token of type NAME (i.peb:1)"},
		{"{% import 'a' as %}", "ParserException: Unexpected token of value \"%}\" and type EXECUTE_END, expected token of type NAME (i.peb:1)"},
		{"{% from 'a' import %}", ""},
		{"{% from 'a' import b as %}", "ParserException: Unexpected token of value \"%}\" and type EXECUTE_END, expected token of type NAME (i.peb:1)"},
		{"{{ a|b( }}", "ParserException: Unclosed \")\" (i.peb:1)"},
		{"{{ a|b(x=1,2) }}", "ParserException: Positional arguments must be declared before any named arguments. (i.peb:1)"},
		{"{% embed 'a' %}{% block b %}{% endblock %}", "ParserException: Unexpected token of value \"null\" and type EOF, expected token of type EXECUTE_START (i.peb:1)"},
		{"{% embed 'a' %}{% endembed %}{% endembed %}", "ParserException: Unexpected tag name \"endembed\" (i.peb:1)"},
		{"{{ foo }}\n{{ }}", "ParserException: Unexpected token \"PRINT_END\" of value \"}}\" (i.peb:2)"},
		{"a\r\n{% foo %}", "ParserException: Unexpected tag name \"foo\" (i.peb:2)"},
		{"a\n\n\n{% if %}", "ParserException: Unexpected token \"EXECUTE_END\" of value \"%}\" (i.peb:4)"},
		{"{{\n\n}}", "ParserException: Unexpected token \"PRINT_END\" of value \"}}\" (i.peb:1)"},
		{"{% if a %}\n{% endif %}\n{% endif %}", "ParserException: Unexpected tag name \"endif\" (i.peb:3)"},
		{"{{ 'a'\n|\n }}", "ParserException: Unexpected token of value \"}}\" and type PRINT_END, expected token of type NAME (i.peb:2)"},
		{"{# a\nb #}\n{{ }}", "ParserException: Unexpected token \"PRINT_END\" of value \"}}\" (i.peb:3)"},
		{"{% macro m(a,\nb) %}{% endmacro %}\n{{ }}", "ParserException: Unexpected token \"PRINT_END\" of value \"}}\" (i.peb:3)"},
		{"{% verbatim %}\n\n{% endverbatim %}{{ }}", "ParserException: Unexpected token \"PRINT_END\" of value \"}}\" (i.peb:3)"},
		{"{{ \"a#{\n}\" }}", "ParserException: Unexpected token \"STRING_INTERPOLATION_END\" of value \"null\" (i.peb:1)"},
		{"{{ 1 }}\n{{ 2 }}\n{{ @ }}", "ParserException: Unexpected character [@] (i.peb:3)"},
		{"{% block a %}\n{% endblock %}\n{% endblock %}", "ParserException: Unexpected tag name \"endblock\" (i.peb:3)"},
		{"{{ a[1\n] }}", ""},
		{"{{ a(\n1 2) }}", "ParserException: Unexpected token of value \"2\" and type NUMBER, expected token of type PUNCTUATION (i.peb:2)"},
		{"{% for x in y %}\n{% endfor %}\n{% else %}", "ParserException: Unexpected tag name \"else\" (i.peb:3)"},
		{"{% set a = b.c['d'](1, n=2) %}", "ClassCastException: class io.pebbletemplates.pebble.node.expression.GetAttributeExpression cannot be cast to class io.pebbletemplates.pebble.node.FunctionOrMacroNameNode (io.pebbletemplates.pebble.node.expression.GetAttributeExpression and io.pebbletemplates.pebble.node.FunctionOrMacroNameNode are in unnamed module of loader 'app')"},
		{"{{ f(1)(2) }}", "ClassCastException: class io.pebbletemplates.pebble.node.expression.FunctionOrMacroInvocationExpression cannot be cast to class io.pebbletemplates.pebble.node.FunctionOrMacroNameNode (io.pebbletemplates.pebble.node.expression.FunctionOrMacroInvocationExpression and io.pebbletemplates.pebble.node.FunctionOrMacroNameNode are in unnamed module of loader 'app')"},
		{"{{ 'x'() }}", "ClassCastException: class io.pebbletemplates.pebble.node.expression.LiteralStringExpression cannot be cast to class io.pebbletemplates.pebble.node.FunctionOrMacroNameNode (io.pebbletemplates.pebble.node.expression.LiteralStringExpression and io.pebbletemplates.pebble.node.FunctionOrMacroNameNode are in unnamed module of loader 'app')"},
		{"{{ (a)(1) }}", "ClassCastException: class io.pebbletemplates.pebble.node.expression.ContextVariableExpression cannot be cast to class io.pebbletemplates.pebble.node.FunctionOrMacroNameNode (io.pebbletemplates.pebble.node.expression.ContextVariableExpression and io.pebbletemplates.pebble.node.FunctionOrMacroNameNode are in unnamed module of loader 'app')"},
		{"{{ a[0](1) }}", "ClassCastException: class io.pebbletemplates.pebble.node.expression.GetAttributeExpression cannot be cast to class io.pebbletemplates.pebble.node.FunctionOrMacroNameNode (io.pebbletemplates.pebble.node.expression.GetAttributeExpression and io.pebbletemplates.pebble.node.FunctionOrMacroNameNode are in unnamed module of loader 'app')"},
		{"{{ 1() }}", "ClassCastException: class io.pebbletemplates.pebble.node.expression.LiteralLongExpression cannot be cast to class io.pebbletemplates.pebble.node.FunctionOrMacroNameNode (io.pebbletemplates.pebble.node.expression.LiteralLongExpression and io.pebbletemplates.pebble.node.FunctionOrMacroNameNode are in unnamed module of loader 'app')"},
		{"{{ a.b() }}", ""},
	}
	for _, c := range cases {
		_, err := Parse(c.source, "i.peb", &Options{NewLineTrimming: true})
		if got := describe(err); got != c.want {
			t.Errorf("Parse(%q):\n got %q\nwant %q", c.source, got, c.want)
		}
	}
}

// stmtNames and exprNames give the dump below readable node names.
var stmtNames = map[StmtKind]string{StmtRoot: "root", StmtBody: "body", StmtText: "text",
	StmtPrint: "print", StmtIf: "if", StmtFor: "for", StmtSet: "set", StmtBlock: "block",
	StmtExtends: "extends", StmtInclude: "include", StmtEmbed: "embed", StmtImport: "import",
	StmtFrom: "from", StmtMacro: "macro", StmtAutoEscape: "autoescape", StmtFilter: "filter",
	StmtCache: "cache", StmtParallel: "parallel", StmtFlush: "flush"}

var exprNames = map[ExprKind]string{ExprLiteralString: "str", ExprLiteralLong: "long",
	ExprLiteralInt: "int", ExprLiteralDouble: "double", ExprLiteralBigDecimal: "bigdec",
	ExprLiteralBool: "bool", ExprLiteralNull: "null", ExprContextVariable: "var",
	ExprGetAttribute: "attr", ExprFilter: "filter", ExprFunction: "func", ExprTest: "test",
	ExprBinary: "binary", ExprUnary: "unary", ExprTernary: "ternary", ExprArray: "array",
	ExprMap: "map", ExprRange: "range", ExprConcat: "concat",
	ExprParentFunction: "parent", ExprBlockFunction: "blockfn", ExprRenderableNode: "rendered"}

// dumpStmt and dumpExpr print the AST as an s-expression, with the line number of every node.
func dumpStmt(s *Stmt) string {
	if s == nil {
		return "-"
	}
	out := "(" + stmtNames[s.Kind] + fmt.Sprintf("@%d", s.Line)
	for _, part := range []struct{ name, value string }{{"name", s.Name}, {"name2", s.Name2},
		{"extra", s.Extra}} {
		if part.value != "" {
			out += " " + part.name + "=" + part.value
		}
	}
	if s.Kind == StmtAutoEscape {
		out += fmt.Sprintf(" flag=%v", s.Flag)
	}
	if s.Kind == StmtText {
		out += " " + fmt.Sprintf("%q", s.Text)
	}
	if len(s.Names) > 0 {
		out += fmt.Sprintf(" names=%v aliases=%v", s.Names, s.Aliases)
	}
	if s.Expr != nil {
		out += " " + dumpExpr(s.Expr)
	}
	if s.Arguments != nil {
		out += " " + dumpArgs(s.Arguments)
	}
	for _, c := range s.Conditions {
		out += " cond:" + dumpExpr(c)
	}
	for _, b := range s.Bodies {
		out += " then:" + dumpStmt(b)
	}
	if s.Else != nil {
		out += " else:" + dumpStmt(s.Else)
	}
	if s.Body != nil {
		out += " " + dumpStmt(s.Body)
	}
	for _, c := range s.Children {
		out += " " + dumpStmt(c)
	}
	return out + ")"
}

func dumpExpr(e *Expr) string {
	if e == nil {
		return "-"
	}
	out := "(" + exprNames[e.Kind] + fmt.Sprintf("@%d", e.Line)
	if e.Name != "" {
		out += " " + e.Name
	}
	if e.Op != "" {
		out += " " + e.Op
	}
	if e.Negated {
		out += " not"
	}
	if e.Value != nil {
		out += fmt.Sprintf(" %#v", e.Value)
	}
	if e.Arguments != nil {
		out += " " + dumpArgs(e.Arguments)
	}
	for _, a := range e.Args {
		out += " " + dumpExpr(a)
	}
	if e.Body != nil {
		out += " " + dumpStmt(e.Body)
	}
	return out + ")"
}

func dumpArgs(a *Arguments) string {
	out := fmt.Sprintf("args@%d[", a.Line)
	for i, p := range a.Positional {
		if i > 0 {
			out += " "
		}
		out += dumpExpr(p)
	}
	out += "]{"
	for i, n := range a.Names {
		if i > 0 {
			out += " "
		}
		out += n + "=" + dumpExpr(a.Named[i])
	}
	return out + "}"
}

// TestParseShapes pins the shape of the tree for one template per tag and per expression kind.
// Every line number and every nesting here is the one the Java parser produces: the filter chain
// of a filter tag carries no line number at all, a block inside an embed reports the line of the
// embed tag, and a function name node takes the line of its opening bracket.
func TestParseShapes(t *testing.T) {
	cases := []struct {
		source string
		opts   *Options
		want   string
	}{
		{"{% if a %}1{% elseif b %}2{% else %}3{% endif %}", nil, "(root@1 (body@1 (if@1 cond:(var@1 a) cond:(var@1 b) then:(body@1 (text@1 \"1\")) then:(body@1 (text@1 \"2\")) else:(body@1 (text@1 \"3\")))))"},
		{"{% for x in 1..3 %}{{ loop.index }}{% else %}none{% endfor %}", nil, "(root@1 (body@1 (for@1 name=x (range@1 (long@1 1) (long@1 3)) else:(body@1 (text@1 \"none\")) (body@1 (print@1 (attr@1 (var@1 loop) (str@1 \"index\")))))))"},
		{"{% set a = b.c['d'] %}", nil, "(root@1 (body@1 (set@1 name=a (attr@1 (attr@1 (var@1 b) (str@1 \"c\")) (str@1 \"d\")))))"},
		{"{% block head %}{{ parent() }}{{ block('x') }}{% endblock head %}", nil, "(root@1 (body@1 (block@1 name=head (body@1 (print@1 (parent@1 head)) (print@1 (blockfn@1 args@1[(str@1 \"x\")]{}))))))"},
		{"{% extends \"base\" %}", nil, "(root@1 (body@1 (extends@1 (str@1 \"base\"))))"},
		{"{% include \"a\" with {'x':1} %}", nil, "(root@1 (body@1 (include@1 (str@1 \"a\") args@1[(map@1 (str@1 \"x\") (long@1 1))]{})))"},
		{"{% import 'm' as mm %}{% from 'm' import a as b, c %}", nil, "(root@1 (body@1 (import@1 name=mm (str@1 \"m\")) (from@1 names=[a c] aliases=[b c] (str@1 \"m\"))))"},
		{"{% macro m(a, b=2) %}{{ a }}{% endmacro %}", nil, "(root@1 (body@1 (macro@1 name=m args@1[]{a=- b=(long@1 2)} (body@1 (print@1 (var@1 a))))))"},
		{"{% filter upper|trim('x') %}x{% endfilter %}", nil, "(root@1 (body@1 (print@1 (filter@0 trim args@1[(str@1 \"x\")]{} (filter@0 upper args@1[]{} (rendered@1 (body@1 (text@1 \"x\"))))))))"},
		{"{% autoescape false %}{{ a }}{% endautoescape %}", nil, "(root@1 (body@1 (autoescape@1 flag=false (body@1 (print@1 (var@1 a))))))"},
		{"{% autoescape 'js' %}{{ a }}{% endautoescape %}", nil, "(root@1 (body@1 (autoescape@1 extra=js flag=true (body@1 (print@1 (var@1 a))))))"},
		{"{{ a is not empty }}{{ a is defined }}", nil, "(root@1 (body@1 (print@1 (test@1 empty not args@1[]{} (var@1 a))) (print@1 (test@1 defined args@1[]{} (var@1 a)))))"},
		{"{{ a ? b : c }}", nil, "(root@1 (body@1 (print@1 (ternary@1 (var@1 a) (var@1 b) (var@1 c)))))"},
		{"{{ -a + 2 * 3 }}", nil, "(root@1 (body@1 (print@1 (binary@1 + (unary@1 - (var@1 a)) (binary@1 * (long@1 2) (long@1 3))))))"},
		{"{{ 1 + 2 * 3 - 4 }}", nil, "(root@1 (body@1 (print@1 (binary@1 - (binary@1 + (long@1 1) (binary@1 * (long@1 2) (long@1 3))) (long@1 4)))))"},
		{"{{ a or b and c }}", nil, "(root@1 (body@1 (print@1 (binary@1 or (var@1 a) (binary@1 and (var@1 b) (var@1 c))))))"},
		{"{{ not a == b }}", nil, "(root@1 (body@1 (print@1 (binary@1 == (unary@1 not (var@1 a)) (var@1 b)))))"},
		{"{{ [1,2] }}{{ [] }}", nil, "(root@1 (body@1 (print@1 (array@1 (long@1 1) (long@1 2))) (print@1 (array@1))))"},
		{"{{ {'a':1} }}{{ {} }}", nil, "(root@1 (body@1 (print@1 (map@1 (str@1 \"a\") (long@1 1))) (print@1 (map@1))))"},
		{"{{ a|date('Y') }}", nil, "(root@1 (body@1 (print@1 (filter@1 date args@1[(str@1 \"Y\")]{} (var@1 a)))))"},
		{"{{ a ~ b ~ 'c' }}", nil, "(root@1 (body@1 (print@1 (concat@1 (concat@1 (var@1 a) (var@1 b)) (str@1 \"c\")))))"},
		{"{{ a.b[c] }}", nil, "(root@1 (body@1 (print@1 (attr@1 (attr@1 (var@1 a) (str@1 \"b\")) (var@1 c)))))"},
		{"{% cache 'k' %}x{% endcache %}", nil, "(root@1 (body@1 (cache@1 (str@1 \"k\") args@1[(str@1 \"k\")]{} (body@1 (text@1 \"x\")))))"},
		{"{% parallel %}x{% endparallel %}{% flush %}", nil, "(root@1 (body@1 (parallel@1 (body@1 (text@1 \"x\"))) (flush@1)))"},
		{"{% embed 'a' with {'k':1} %}{% block b %}y{% endblock %}{% endembed %}", nil, "(root@1 (body@1 (embed@1 (str@1 \"a\") args@1[(map@1 (str@1 \"k\") (long@1 1))]{} (body@1 (block@1 name=b (body@1 (text@1 \"y\")))))))"},
		{"{{ 1 }}{{ 1.5 }}{{ 2L }}{{ true }}{{ null }}{{ 'x' }}", nil, "(root@1 (body@1 (print@1 (long@1 1)) (print@1 (double@1 1.5)) (print@1 (long@1 2)) (print@1 (bool@1 true)) (print@1 (null@1)) (print@1 (str@1 \"x\"))))"},
		{"{{ 1 }}{{ 1.5 }}", &Options{NewLineTrimming: true, LiteralDecimalTreatedAsInt: true}, "(root@1 (body@1 (print@1 (int@1 1)) (print@1 (double@1 1.5))))"},
		{"{{ 1 }}{{ 1.5 }}", &Options{NewLineTrimming: true, LiteralNumbersAsBigDecimals: true}, "(root@1 (body@1 (print@1 (bigdec@1 \"1\")) (print@1 (bigdec@1 \"1.5\"))))"},
		{"{{ \"a#{b}c#{d}\" }}", nil, "(root@1 (body@1 (print@1 (concat@1 (str@1 \"a\") (concat@1 (var@1 b) (concat@1 (str@1 \"c\") (var@1 d)))))))"},
		{"{{ a.b(1) }}", nil, "(root@1 (body@1 (print@1 (attr@1 args@1[(long@1 1)]{} (var@1 a) (str@1 \"b\")))))"},
		{"{% verbatim %}{{ x }}{% endverbatim %}", nil, "(root@1 (body@1 (text@1 \"{{ x }}\")))"},
	}
	for _, c := range cases {
		root, err := Parse(c.source, "i.peb", c.opts)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.source, err)
			continue
		}
		if got := dumpStmt(root); got != c.want {
			t.Errorf("Parse(%q):\n got %s\nwant %s", c.source, got, c.want)
		}
	}
}
