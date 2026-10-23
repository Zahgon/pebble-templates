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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The parser is written by another part of the port, so these tests hand the evaluator the ASTs
// they want to render. Every expected value in this file was taken from the reference command.

func body(children ...*Stmt) *Stmt { return &Stmt{Kind: StmtBody, Line: 1, Children: children} }
func root(children ...*Stmt) *Stmt { return &Stmt{Kind: StmtRoot, Body: body(children...)} }
func text(value string) *Stmt      { return &Stmt{Kind: StmtText, Line: 1, Text: value} }
func print(e *Expr) *Stmt          { return &Stmt{Kind: StmtPrint, Line: 1, Expr: e} }
func name(n string) *Expr          { return &Expr{Kind: ExprContextVariable, Line: 1, Name: n} }
func str(value string) *Expr       { return &Expr{Kind: ExprLiteralString, Line: 1, Value: value} }
func attr(obj *Expr, a *Expr) *Expr {
	return &Expr{Kind: ExprGetAttribute, Line: 1, Args: []*Expr{obj, a}}
}

// show prints a value through a test filter, which spares these tests the ToStr of the part of
// the port that owns the value model.
func show(e *Expr) *Stmt {
	return print(&Expr{Kind: ExprFilter, Line: 1, Name: "show", Args: []*Expr{e}})
}

func init() {
	Filters["show"] = Filter{Apply: func(input any, args map[string]any, ctx Ctx, line int) (any, error) {
		if input == nil {
			return "null", nil
		}
		if text, ok := input.(fmt.Stringer); ok {
			return text.String(), nil
		}
		return fmt.Sprint(input), nil
	}}
	// A filter of this test's own, so that the tests here do not depend on the core extension.
	Filters["shout"] = Filter{Apply: func(input any, args map[string]any, ctx Ctx, line int) (any, error) {
		return strings.ToUpper(fmt.Sprint(input)), nil
	}}
}

// templates sets up an engine over a directory of templates whose ASTs are given directly.
func templates(t *testing.T, opts *Options, trees map[string]*Stmt) *Engine {
	t.Helper()
	dir := t.TempDir()
	for name := range trees {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("ignored"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if opts == nil {
		opts = &Options{MaxRenderedSize: -1}
	}
	opts.Prefix = dir
	engine := NewEngine(opts)

	previousParse, previousVisitor := parseTemplate, visitEscaper
	parseTemplate = func(source, templateName string, options *Options) (*Stmt, error) {
		tree, ok := trees[templateName]
		if !ok {
			t.Fatalf("no AST for template %q", templateName)
		}
		return tree, nil
	}
	visitEscaper = func(*Stmt, *Options) {}
	t.Cleanup(func() { parseTemplate, visitEscaper = previousParse, previousVisitor })
	return engine
}

func render(t *testing.T, engine *Engine, name string, context *Map) (string, error) {
	t.Helper()
	var out strings.Builder
	err := engine.Render(&out, name, context)
	return out.String(), err
}

func ctxMap(pairs ...any) *Map {
	m := &Map{Vals: map[any]any{}}
	for index := 0; index+1 < len(pairs); index += 2 {
		m.Keys = append(m.Keys, pairs[index])
		m.Vals[pairs[index]] = pairs[index+1]
	}
	return m
}

func TestRenderTextAndPrint(t *testing.T) {
	engine := templates(t, nil, map[string]*Stmt{
		"i.peb": root(text("a="), print(name("a")), text("|"), print(name("missing"))),
	})
	out, err := render(t, engine, "i.peb", ctxMap("a", "x"))
	if err != nil {
		t.Fatal(err)
	}
	if out != "a=x|" {
		t.Fatalf("got %q", out)
	}
}

func TestForLoopVariables(t *testing.T) {
	loop := &Stmt{Kind: StmtFor, Line: 1, Name: "x", Expr: name("l"),
		Body: body(show(attr(name("loop"), str("index"))),
			show(attr(name("loop"), str("first"))),
			show(attr(name("loop"), str("last"))),
			show(attr(name("loop"), str("length"))),
			show(attr(name("loop"), str("revindex"))),
			text(";")),
		Else: body(text("none"))}
	engine := templates(t, nil, map[string]*Stmt{"i.peb": root(loop)})

	list := &List{Items: []any{int64(1), int64(2), int64(3)}}
	out, err := render(t, engine, "i.peb", ctxMap("l", list))
	if err != nil {
		t.Fatal(err)
	}
	if out != "0truefalse32;1falsefalse31;2falsetrue30;" {
		t.Fatalf("got %q", out)
	}

	out, err = render(t, engine, "i.peb", ctxMap("l", &List{}))
	if err != nil || out != "none" {
		t.Fatalf("got %q, %v", out, err)
	}

	// A map iterates its entries, whose key and value are attributes; a string is not iterable.
	out, err = render(t, engine, "i.peb", ctxMap("l", "x"))
	if err == nil || err.Error() != "Not an iterable object. Value = [x] (i.peb:1)" {
		t.Fatalf("got %q, %v", out, err)
	}
}

func TestForOverMapEntries(t *testing.T) {
	loop := &Stmt{Kind: StmtFor, Line: 1, Name: "e", Expr: name("m"),
		Body: body(show(attr(name("e"), str("key"))), text("="),
			show(attr(name("e"), str("value"))), text(";"), show(name("e")), text(" "))}
	engine := templates(t, nil, map[string]*Stmt{"i.peb": root(loop)})
	out, err := render(t, engine, "i.peb", ctxMap("m", ctxMap("k", "1")))
	if err != nil {
		t.Fatal(err)
	}
	if out != "k=1;k=1 " {
		t.Fatalf("got %q", out)
	}
}

func TestSetDoesNotEscapeTheLoopScope(t *testing.T) {
	loop := &Stmt{Kind: StmtFor, Line: 1, Name: "x", Expr: name("l"),
		Body: body(&Stmt{Kind: StmtSet, Line: 1, Name: "y", Expr: name("x")})}
	engine := templates(t, nil, map[string]*Stmt{
		"i.peb": root(loop, show(name("y"))),
	})
	out, err := render(t, engine, "i.peb", ctxMap("l", &List{Items: []any{int64(1)}}))
	if err != nil {
		t.Fatal(err)
	}
	if out != "null" {
		t.Fatalf("got %q", out)
	}
}

func TestAttributeErrorsWithStrictVariables(t *testing.T) {
	cases := []struct {
		label   string
		node    *Stmt
		kind    string
		message string
	}{
		{"missing root", print(name("nope")), "RootAttributeNotFoundException",
			"Root attribute [nope] does not exist or can not be accessed and strict variables is set to true. (i.peb:1)"},
		{"missing root attribute", print(attr(name("nope"), str("foo"))), "RootAttributeNotFoundException",
			"Root attribute [nope] does not exist or can not be accessed and strict variables is set to true. (i.peb:1)"},
		{"missing map key", print(attr(name("m"), str("z"))), "AttributeNotFoundException",
			"Attribute [z] of [java.util.LinkedHashMap] does not exist or can not be accessed and strict variables is set to true. (i.peb:1)"},
		{"missing string member", print(attr(name("a"), str("foo"))), "AttributeNotFoundException",
			"Attribute [foo] of [java.lang.String] does not exist or can not be accessed and strict variables is set to true. (i.peb:1)"},
		{"index out of bounds", print(attr(name("l"), str("3"))), "AttributeNotFoundException",
			"Index out of bounds while accessing array with strict variables on. (i.peb:1)"},
		{"unsupported if value", &Stmt{Kind: StmtIf, Line: 1, Conditions: []*Expr{name("l")},
			Bodies: []*Stmt{body(text("y"))}}, "PebbleException",
			"Wrong operand(s) type in conditional expression (i.peb:1)"},
	}
	for _, testCase := range cases {
		t.Run(testCase.label, func(t *testing.T) {
			engine := templates(t, &Options{StrictVariables: true, MaxRenderedSize: -1},
				map[string]*Stmt{"i.peb": root(testCase.node)})
			_, err := render(t, engine, "i.peb", ctxMap("a", "x", "m", ctxMap("k", int64(1)),
				"l", &List{Items: []any{int64(1)}}))
			pebbleErr, ok := err.(*Error)
			if !ok {
				t.Fatalf("got %v", err)
			}
			if pebbleErr.Kind != testCase.kind || pebbleErr.Error() != testCase.message {
				t.Fatalf("got %s: %s", pebbleErr.Kind, pebbleErr.Error())
			}
		})
	}
}

func TestIfUnsupportedValueCause(t *testing.T) {
	engine := templates(t, nil, map[string]*Stmt{
		"i.peb": root(&Stmt{Kind: StmtIf, Line: 1, Conditions: []*Expr{name("l")},
			Bodies: []*Stmt{body(text("y"))}}),
	})
	_, err := render(t, engine, "i.peb", ctxMap("l", &List{}))
	pebbleErr, ok := err.(*Error)
	if !ok || pebbleErr.Cause == nil {
		t.Fatalf("got %v", err)
	}
	want := "Unsupported value type ArrayList. Expected Boolean, String, Number in \"if\" statement (i.peb:1)"
	if pebbleErr.Cause.Error() != want {
		t.Fatalf("got %q", pebbleErr.Cause.Error())
	}
}

func TestInheritanceAndBlocks(t *testing.T) {
	trees := map[string]*Stmt{
		"base.peb": root(text("P["), &Stmt{Kind: StmtBlock, Line: 1, Name: "b",
			Body: body(text("base"))}, text("]["), &Stmt{Kind: StmtBlock, Line: 1, Name: "c",
			Body: body(text("cc"))}, text("]")),
		"i.peb": root(&Stmt{Kind: StmtExtends, Line: 1, Expr: str("base.peb")},
			&Stmt{Kind: StmtBlock, Line: 1, Name: "b", Body: body(text("child"),
				print(&Expr{Kind: ExprParentFunction, Line: 1, Name: "b"}),
				print(&Expr{Kind: ExprBlockFunction, Line: 1,
					Arguments: &Arguments{Positional: []*Expr{str("c")}}}))}),
	}
	engine := templates(t, nil, trees)
	out, err := render(t, engine, "i.peb", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "P[childbasecc][cc]" {
		t.Fatalf("got %q", out)
	}
}

func TestIncludeAndEmbed(t *testing.T) {
	trees := map[string]*Stmt{
		"inc.peb": root(text("INC a="), print(name("a")), text(" w="), print(name("w"))),
		"emb.peb": root(text("EMB["), &Stmt{Kind: StmtBlock, Line: 1, Name: "b",
			Body: body(text("eb"))}, text("]")),
		"i.peb": root(
			&Stmt{Kind: StmtInclude, Line: 1, Expr: str("inc.peb")},
			text("|"),
			&Stmt{Kind: StmtInclude, Line: 1, Expr: str("inc.peb"),
				Arguments: &Arguments{Names: []string{"w"}, Named: []*Expr{str("9")}}},
			text("|"),
			&Stmt{Kind: StmtEmbed, Line: 1, Expr: str("emb.peb"),
				Arguments: &Arguments{Names: []string{"a"}, Named: []*Expr{str("Z")}},
				Body: body(&Stmt{Kind: StmtBlock, Line: 1, Name: "b",
					Body: body(text("over"), print(name("a")))})}),
	}
	engine := templates(t, nil, trees)
	out, err := render(t, engine, "i.peb", ctxMap("a", "x"))
	if err != nil {
		t.Fatal(err)
	}
	if out != "INC a=x w=|INC a=x w=9|EMB[overZ]" {
		t.Fatalf("got %q", out)
	}
}

func TestMacrosAndImports(t *testing.T) {
	macros := root(&Stmt{Kind: StmtMacro, Line: 1, Name: "hi",
		Arguments: &Arguments{Names: []string{"n"}, Named: []*Expr{nil}},
		Body:      body(text("hi "), print(name("n")))})
	trees := map[string]*Stmt{
		"macros.peb": macros,
		"i.peb": root(&Stmt{Kind: StmtImport, Line: 1, Expr: str("macros.peb")},
			print(&Expr{Kind: ExprFunction, Line: 1, Name: "hi",
				Arguments: &Arguments{Positional: []*Expr{str("1")}}}),
			text("|"),
			&Stmt{Kind: StmtImport, Line: 1, Expr: str("macros.peb"), Name: "m"},
			print(attr(name("m"), str("hi")))),
		"from.peb": root(&Stmt{Kind: StmtFrom, Line: 1, Expr: str("macros.peb"),
			Names: []string{"hi"}, Aliases: []string{"greet"}},
			print(&Expr{Kind: ExprFunction, Line: 1, Name: "greet",
				Arguments: &Arguments{Positional: []*Expr{str("2")}}})),
		"bad.peb": root(&Stmt{Kind: StmtFrom, Line: 1, Expr: str("macros.peb"),
			Names: []string{"nope"}, Aliases: []string{""}}),
		"args.peb": root(print(&Expr{Kind: ExprFunction, Line: 1, Name: "hi",
			Arguments: &Arguments{Positional: []*Expr{str("1"), str("2")}}})),
	}
	engine := templates(t, nil, trees)

	// An attribute of an import alias invokes the macro, with no arguments here.
	out, err := render(t, engine, "i.peb", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hi 1|hi " {
		t.Fatalf("got %q", out)
	}

	if out, err = render(t, engine, "from.peb", nil); err != nil || out != "hi 2" {
		t.Fatalf("got %q, %v", out, err)
	}

	_, err = render(t, engine, "bad.peb", nil)
	if err == nil ||
		err.Error() != "Function or Macro [nope] referenced by alias [nope] does not exist. (?:?)" {
		t.Fatalf("got %v", err)
	}

	// The macro of args.peb is only reachable after an import, so the name does not exist.
	_, err = render(t, engine, "args.peb", nil)
	if err == nil || err.Error() != "Function or Macro [hi] does not exist. (args.peb:1)" {
		t.Fatalf("got %v", err)
	}
}

func TestMacroArgumentErrors(t *testing.T) {
	macro := &Stmt{Kind: StmtMacro, Line: 1, Name: "foo",
		Arguments: &Arguments{Names: []string{"x"}, Named: []*Expr{nil}},
		Body:      body(text("["), print(name("x")), text("]"))}
	cases := []struct {
		label   string
		args    *Arguments
		out     string
		message string
	}{
		{"positional", &Arguments{Positional: []*Expr{str("1")}}, "[1]", ""},
		{"absent", nil, "[]", ""},
		{"too many", &Arguments{Positional: []*Expr{str("1"), str("2")}}, "",
			"The argument at position 2 is not allowed. Only 1 argument(s) are allowed. (i.peb:1)"},
		{"unknown name", &Arguments{Names: []string{"y"}, Named: []*Expr{str("2")}}, "",
			"The following named argument does not exist: y (i.peb:1)"},
	}
	for _, testCase := range cases {
		t.Run(testCase.label, func(t *testing.T) {
			engine := templates(t, nil, map[string]*Stmt{
				"i.peb": root(macro, print(&Expr{Kind: ExprFunction, Line: 1, Name: "foo",
					Arguments: testCase.args})),
			})
			out, err := render(t, engine, "i.peb", nil)
			if testCase.message == "" {
				if err != nil || out != testCase.out {
					t.Fatalf("got %q, %v", out, err)
				}
				return
			}
			if err == nil || err.Error() != testCase.message {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestDuplicateMacroName(t *testing.T) {
	macro := func() *Stmt {
		return &Stmt{Kind: StmtMacro, Line: 1, Name: "f", Arguments: &Arguments{},
			Body: body(text("a"))}
	}
	engine := templates(t, nil, map[string]*Stmt{"i.peb": root(macro(), macro())})
	_, err := render(t, engine, "i.peb", nil)
	javaErr, ok := err.(*JavaError)
	if !ok || javaErr.Kind != "RuntimeException" {
		t.Fatalf("got %v", err)
	}
	want := "io.pebbletemplates.pebble.error.PebbleException: More than one macro can not share the same name: f (?:?)"
	if javaErr.Message != want {
		t.Fatalf("got %q", javaErr.Message)
	}
}

func TestMaxRenderedSize(t *testing.T) {
	engine := templates(t, &Options{MaxRenderedSize: 3},
		map[string]*Stmt{"i.peb": root(text("abcdefgh"))})
	out, err := render(t, engine, "i.peb", nil)
	if err == nil || err.Error() != "Tried to write more than 3 chars. (?:?)" {
		t.Fatalf("got %v", err)
	}
	if out != "" {
		t.Fatalf("the failing write must not reach the output, got %q", out)
	}

	// A macro body counts towards the same limit even though it renders into a string.
	engine = templates(t, &Options{MaxRenderedSize: 3}, map[string]*Stmt{
		"i.peb": root(&Stmt{Kind: StmtMacro, Line: 1, Name: "f", Arguments: &Arguments{},
			Body: body(text("abcdef"))},
			print(&Expr{Kind: ExprFunction, Line: 1, Name: "f", Arguments: &Arguments{}})),
	})
	if _, err = render(t, engine, "i.peb", nil); err == nil ||
		err.Error() != "Tried to write more than 3 chars. (?:?)" {
		t.Fatalf("got %v", err)
	}
}

func TestFilterTagAndCacheAndParallel(t *testing.T) {
	engine := templates(t, nil, map[string]*Stmt{
		"i.peb": root(
			&Stmt{Kind: StmtFilter, Line: 1, Body: body(text("abc")),
				Expr: &Expr{Kind: ExprFilter, Line: 1, Name: "shout",
					Args: []*Expr{{Kind: ExprRenderableNode, Line: 1}}}},
			&Stmt{Kind: StmtCache, Line: 1, Arguments: &Arguments{Positional: []*Expr{str("k")}},
				Body: body(text("C"))},
			&Stmt{Kind: StmtParallel, Line: 1, Body: body(text("P"))},
			&Stmt{Kind: StmtFlush, Line: 1},
			&Stmt{Kind: StmtAutoEscape, Line: 1, Body: body(text("A"))}),
	})
	out, err := render(t, engine, "i.peb", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "ABCCPA" {
		t.Fatalf("got %q", out)
	}
}

func TestOutputBeforeFailureStaysWritten(t *testing.T) {
	engine := templates(t, &Options{StrictVariables: true, MaxRenderedSize: -1},
		map[string]*Stmt{"i.peb": root(text("before"), print(name("nope")))})
	out, err := render(t, engine, "i.peb", nil)
	if err == nil {
		t.Fatal("expected a failure")
	}
	if out != "before" {
		t.Fatalf("got %q", out)
	}
}
