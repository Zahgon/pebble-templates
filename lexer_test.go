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
	"testing"
)

// dumpTokens prints a token list as "TYPE:value@line" entries.
func dumpTokens(tokens []Token) string {
	var out []string
	for _, t := range tokens {
		out = append(out, fmt.Sprintf("%s:%s@%d", javaTokenName(t.Type), javaTokenValue(t), t.Line))
	}
	return strings.Join(out, " ")
}

func TestTokenize(t *testing.T) {
	cases := []struct {
		source string
		want   string
	}{
		// the line of a token is the line the cursor has reached, which is why the closing
		// delimiter of a print block reports the line it started on even when the newline that
		// follows it has already been matched
		{"hi {{ 1 }}\nbye", "TEXT:hi @1 PRINT_START:null@1 NUMBER:1@1 PRINT_END:}}@1 " +
			"TEXT:bye@2 EOF:null@2"},
		{"{{ a|upper('x', y=1) }}", "PRINT_START:null@1 NAME:a@1 OPERATOR:|@1 NAME:upper@1 " +
			"PUNCTUATION:(@1 STRING:x@1 PUNCTUATION:,@1 NAME:y@1 PUNCTUATION:=@1 NUMBER:1@1 " +
			"PUNCTUATION:)@1 PRINT_END:}}@1 EOF:null@1"},
		// "is not" has to win over "is", and a name that starts with an operator stays a name
		{"{{ a is not empty }}", "PRINT_START:null@1 NAME:a@1 OPERATOR:is not@1 NAME:empty@1 " +
			"PRINT_END:}}@1 EOF:null@1"},
		{"{{ organization }}", "PRINT_START:null@1 NAME:organization@1 PRINT_END:}}@1 EOF:null@1"},
		// a long literal drops its L, a decimal keeps its dot
		{"{{ 3L }}{{ 2.5 }}", "PRINT_START:null@1 LONG:3@1 PRINT_END:}}@1 PRINT_START:null@1 " +
			"NUMBER:2.5@1 PRINT_END:}}@1 EOF:null@1"},
		// an interpolated string is a token sequence of its own
		{`{{ "a#{1}b" }}`, "PRINT_START:null@1 STRING:a@1 STRING_INTERPOLATION_START:null@1 " +
			"NUMBER:1@1 STRING_INTERPOLATION_END:null@1 STRING:b@1 PRINT_END:}}@1 EOF:null@1"},
		// comments produce no tokens at all
		{"a{# c #}b", "TEXT:a@1 TEXT:b@1 EOF:null@1"},
	}
	for _, c := range cases {
		tokens, err := tokenize(c.source, "i.peb", true)
		if err != nil {
			t.Errorf("tokenize(%q): %v", c.source, err)
			continue
		}
		if got := dumpTokens(tokens); got != c.want {
			t.Errorf("tokenize(%q):\n got %s\nwant %s", c.source, got, c.want)
		}
	}
}

/*
 * renderText renders the text nodes and the literal print nodes of a template, which is all the
 * whitespace control cases below need. It is enough to compare the port against the output of the
 * reference command for them.
 */
func renderText(t *testing.T, node *Stmt) string {
	switch node.Kind {
	case StmtRoot:
		return renderText(t, node.Body)
	case StmtBody:
		var out strings.Builder
		for _, child := range node.Children {
			out.WriteString(renderText(t, child))
		}
		return out.String()
	case StmtText:
		return node.Text
	case StmtPrint:
		return renderLiteral(t, node.Expr)
	}
	t.Fatalf("unexpected node kind %d", node.Kind)
	return ""
}

func renderLiteral(t *testing.T, e *Expr) string {
	switch e.Kind {
	case ExprLiteralString, ExprLiteralLong, ExprLiteralDouble:
		return fmt.Sprint(e.Value)
	case ExprConcat:
		return renderLiteral(t, e.Args[0]) + renderLiteral(t, e.Args[1])
	}
	t.Fatalf("unexpected expression kind %d", e.Kind)
	return ""
}

// TestWhitespaceControl checks the whitespace trim markers, the newline trimming option, comments
// and the verbatim tag against the output of the reference command.
func TestWhitespaceControl(t *testing.T) {
	cases := []struct {
		source          string
		newLineTrimming bool
		want            string
	}{
		{"a\n{{ 1 }}\nb", true, "a\n1b"},
		{"a\n{{- 1 -}}\nb", true, "a1b"},
		{"a  {{- 'x' }}  b", true, "ax  b"},
		{"a{# c #}b", true, "ab"},
		{"a{# c -#}  b", true, "ab"},
		{"a  {#- c #}b", true, "ab"},
		{"a  {#- c -#}  b", true, "ab"},
		{"a{# c #}\nb", true, "ab"},
		// the text of a verbatim tag is always left trimmed, because the Java lexer tests the
		// whole match of its start pattern instead of the trim marker group
		{"{% verbatim %} {{ raw }} {% endverbatim %}", true, "{{ raw }} "},
		{"x{% verbatim %}\n  a\n{% endverbatim %}y", true, "xa\ny"},
		{"x  {%- verbatim %}  a  {%- endverbatim -%}  y", true, "xay"},
		{"x{% verbatim -%}  a  {% endverbatim %}  y", true, "xa    y"},
		{`{{ "a#{1}b" }}`, true, "a1b"},
		{`{{ "#{1}#{2}" }}`, true, "12"},
		{`{{ 'it\'s' }}`, true, "it's"},
		{`{{ "a\"b" }}`, true, `a"b`},
		// only the quote that wraps a literal is unescaped, so \n stays a backslash and an n
		{`{{ "x\ny" }}`, true, `x\ny`},
		{"{{ 'a' ~ 'b' }}", true, "ab"},
		{"{{ 1 }}\r\n{{ 2 }}", true, "12"},
		{"{{ 10 }}{{ 2.5 }}{{ 3L }}", true, "102.53"},
		{"{{ 'a' }}\t{{ 'b' }}", true, "a\tb"},
		{"  {{- 'x' -}}  ", true, "x"},
		{`{{ "a" }}{{ "#{'q'}" }}`, true, "aq"},
		// with newline trimming off the line break after a tag survives
		{"a\n{{ 1 }}\nb", false, "a\n1\nb"},
		{"a\n{{- 1 -}}\nb", false, "a1b"},
		{"x{% verbatim %}\n  a\n{% endverbatim %}y", false, "xa\ny"},
		{"a{# c #}\nb", false, "a\nb"},
	}
	for _, c := range cases {
		root, err := Parse(c.source, "i.peb", &Options{NewLineTrimming: c.newLineTrimming})
		if err != nil {
			t.Errorf("Parse(%q): %v", c.source, err)
			continue
		}
		if got := renderText(t, root); got != c.want {
			t.Errorf("Parse(%q) (newLineTrimming=%v):\n got %q\nwant %q", c.source,
				c.newLineTrimming, got, c.want)
		}
	}
}
