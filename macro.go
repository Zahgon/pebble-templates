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

// macro is PebbleTemplateImpl.macro: it looks a macro up in the child template first, then in
// this template, then in the templates an import tag brought in, and finally in the parent. The
// result is a SafeString, so the output of a macro is never escaped again.
func (t *Template) macro(ctx *evalCtx, name string, args *Arguments, ignoreOverriden bool,
	line int) (any, error) {
	*ctx.depth++
	defer func() { *ctx.depth-- }()
	if *ctx.depth > maxRenderDepth {
		return nil, &JavaError{Kind: "StackOverflowError"}
	}

	var result any
	found := false

	child := ctx.hier.child()
	if !ignoreOverriden && child != nil {
		found = true
		ctx.hier.descend()
		value, err := child.macro(ctx, name, args, false, line)
		if err != nil {
			return nil, err
		}
		ctx.hier.ascend()
		result = value
	} else if macro, ok := t.macros[name]; ok {
		found = true
		named, err := ctx.argumentMap(args, macro.argumentNames(), line, t)
		if err != nil {
			return nil, err
		}
		body, err := macro.call(ctx, t, named)
		if err != nil {
			return nil, err
		}
		result = SafeString(body)
	}

	if !found {
		for _, imported := range *ctx.imported {
			if imported == nil {
				continue
			}
			if _, ok := imported.macros[name]; ok {
				found = true
				value, err := imported.macro(ctx, name, args, false, line)
				if err != nil {
					return nil, err
				}
				result = value
				break
			}
		}
	}

	if !found {
		parent := ctx.hier.parent()
		if parent == nil {
			return nil, &Error{Kind: "PebbleException", File: t.Name, Line: line,
				Message: fmt.Sprintf("Function or Macro [%s] does not exist.", name)}
		}
		ctx.hier.ascend()
		value, err := parent.macro(ctx, name, args, true, line)
		if err != nil {
			return nil, err
		}
		ctx.hier.descend()
		result = value
	}
	return result, nil
}

// argumentNames is Macro.getArgumentNames: the declared parameters of the macro, in order.
func (m *MacroDef) argumentNames() []string {
	if m.Params == nil {
		return nil
	}
	return m.Params.Names
}

// call is Macro.call: the declared parameters are bound to their defaults in a local scope, the
// arguments of the invocation are pushed on top of it and the body is rendered into a string.
// The self template is the one that invoked the macro, as in Java, which matters for a macro the
// from tag imported: its body renders with the name of the importing template.
func (m *MacroDef) call(ctx *evalCtx, self *Template, args map[string]any) (string, error) {
	buf := &bufWriter{}
	writer := ctx.limit(buf)

	// The scope of the default arguments is local, so a macro body cannot see the variables of
	// the template that called it.
	ctx.scope.pushLocalScope()
	if m.Params != nil {
		for index, name := range m.Params.Names {
			if index >= len(m.Params.Named) || m.Params.Named[index] == nil {
				ctx.scope.put(name, nil)
				continue
			}
			value, err := ctx.eval(m.Params.Named[index], self)
			if err != nil {
				return "", err
			}
			ctx.scope.put(name, value)
		}
	}
	ctx.scope.pushScope(args)
	if err := ctx.render(m.Body, self, writer); err != nil {
		return "", err
	}
	ctx.scope.popScope()
	ctx.scope.popScope()
	return buf.String(), nil
}

// macroProvider is io.pebbletemplates.pebble.template.MacroAttributeProvider: the value an
// "import ... as" alias holds. Every attribute of it is a macro invocation.
type macroProvider struct {
	template *Template
}

// macro invokes a macro of the imported template.
func (p *macroProvider) macro(ctx *evalCtx, name string, args *Arguments, line int) (any, error) {
	if p.template == nil {
		return nil, nil
	}
	return p.template.macro(ctx, name, args, false, line)
}
