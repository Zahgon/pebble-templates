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
	"math/big"

	"github.com/PebbleTemplates/pebble/internal/jnum"
)

// A template value is one of: nil (Java null), bool (Boolean), string (String), SafeString,
// int32 (Integer), int64 (Long), float32 (Float), float64 (Double), *big.Int (BigInteger),
// *jnum.BigDecimal (BigDecimal), *List (java.util.List), *Map (java.util.Map), *MacroDef (a
// macro), *Template (the "template" global) or Locale (the "locale" global).

// BigDecimal is java.math.BigDecimal.
type BigDecimal = jnum.BigDecimal

// SafeString is io.pebbletemplates.pebble.extension.escaper.SafeString: a string the escaper
// leaves alone.
type SafeString string

// Locale is the value of the "locale" global variable; it prints like java.util.Locale.toString.
type Locale string

// List is a java.util.List, usually an ArrayList.
type List struct {
	Items []any
}

// Map is a java.util.Map. Hash reports a java.util.HashMap, whose iteration order is the JDK's
// bucket order rather than insertion order; LinkedHashMap keeps insertion order.
type Map struct {
	Keys []any
	Vals map[any]any
	Hash bool
}

// MacroDef is a macro of a template, callable as a function.
type MacroDef struct {
	Name     string
	Params   *Arguments
	Body     *Stmt
	Template *Template
}

// Ctx is what the evaluator gives to filters, functions and tests. It mirrors the parts of
// io.pebbletemplates.pebble.template.EvaluationContext they use.
type Ctx interface {
	// Locale is the locale of the evaluation, as a language tag ("en-US").
	Locale() string
	// Zone is the default time zone id of the command.
	Zone() string
	// Strict reports the strictVariables setting.
	Strict() bool
	// Name is the name of the template being rendered, for error messages.
	Name() string
	// Escaping reports whether auto escaping is on, and the default strategy.
	Escaping() (bool, string)
	// Render renders a body into a string (the filter tag and macro invocations need it).
	Render(body *Stmt) (string, error)
	// Lookup resolves a variable the way a context variable expression does.
	Lookup(name string) (any, bool, error)
	// Eval evaluates an expression in the current scope.
	Eval(expr *Expr) (any, error)
	// GetAttribute resolves obj.name the way GetAttributeExpression does.
	GetAttribute(obj any, name any, args *Arguments, line int) (any, error)
}

// Filter is one entry of the filter registry (io.pebbletemplates.pebble.extension.Filter).
// ArgNames are the filter's argument names in order, as getArgumentNames returns them; Apply
// receives the arguments resolved by name, with absent ones missing from the map.
type Filter struct {
	ArgNames []string
	Apply    func(input any, args map[string]any, ctx Ctx, line int) (any, error)
}

// Test is one entry of the test registry, Function one of the function registry.
type Test struct {
	ArgNames []string
	Apply    func(input any, args map[string]any, ctx Ctx, line int) (bool, error)
}

// Function is a template function; Args are its declared argument names.
type Function struct {
	ArgNames []string
	Apply    func(args map[string]any, ctx Ctx, line int) (any, error)
}

// Filters, Tests and Functions are the registries of the core extension, filled by the files
// that implement them.
var (
	Filters   = map[string]Filter{}
	Tests     = map[string]Test{}
	Functions = map[string]Function{}
)

// Big is a convenience constructor used by the parser for big integer literals.
func Big(text string) *big.Int {
	v, _ := new(big.Int).SetString(text, 10)
	return v
}
