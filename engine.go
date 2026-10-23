/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package pebble

import "io"

// parseTemplate and visitEscaper are the parser and the escaper's node visitor, held in
// variables so that the tests of the evaluator can feed it an AST they built by hand.
var (
	parseTemplate = Parse
	visitEscaper  = VisitEscaper
)

// Engine is the PebbleEngine: it loads, parses and renders the templates of a directory. The
// reference command builds it with a FileLoader, no template cache and no executor service, so
// every lookup of a template re-reads and re-parses it.
type Engine struct {
	Opts *Options
}

// NewEngine builds an engine for the given options. A nil Options is the engine's own default
// configuration: new line trimming and auto escaping on, the html escaping strategy, an unlimited
// rendered size and the en-US locale in UTC.
func NewEngine(opts *Options) *Engine {
	copied := Options{
		NewLineTrimming:         true,
		AutoEscaping:            true,
		DefaultEscapingStrategy: "html",
		MaxRenderedSize:         -1,
		Locale:                  "en-US",
		TimeZone:                "UTC",
	}
	if opts != nil {
		copied = *opts
	}
	return &Engine{Opts: &copied}
}

// Template is a parsed template of the engine, the PebbleTemplateImpl of the Java code. Its
// blocks and macros are the ones a node visitor collected from the AST right after parsing.
type Template struct {
	// Name is the template name as the loader received it, used in error messages.
	Name string
	// Root is the root node of the template AST.
	Root *Stmt

	engine *Engine
	blocks map[string]*Stmt
	macros map[string]*MacroDef
}

// Render loads the named template, renders it with the context and writes the output. The output
// written before a failure stays written, as the reference command's does.
func (e *Engine) Render(w io.Writer, name string, context *Map) error {
	loader, err := newFileLoader(e.Opts.Prefix)
	if err != nil {
		return err
	}
	template, err := e.getTemplate(loader, name)
	if err != nil {
		return err
	}
	ctx := e.newContext(loader, template)

	// The user context is pushed as a scope of its own and then an empty scope on top of it, so
	// that the set tag can define variables even when the context map is immutable (issue #449).
	ctx.scope.pushScope(contextScope(context))
	ctx.scope.pushScope(nil)

	return template.evaluate(ctx, &ioWriter{w: w})
}

// getTemplate is PebbleEngine.getTemplate: it loads the source, parses it, runs the node visitors
// of the extensions over the AST and returns the template. A template name that evaluated to null
// yields no template at all, as the extends tag relies on.
func (e *Engine) getTemplate(loader *fileLoader, name string) (*Template, error) {
	if name == "" {
		return nil, nil
	}
	source, err := loader.read(name)
	if err != nil {
		return nil, err
	}
	root, err := parseTemplate(source, name, e.Opts)
	if err != nil {
		return nil, err
	}
	template := &Template{Name: name, Root: root, engine: e,
		blocks: map[string]*Stmt{}, macros: map[string]*MacroDef{}}

	// The core extension registers the blocks and macros of the AST, the escaper extension wraps
	// the print nodes in escape filters.
	if err := template.registerBlocksAndMacros(root); err != nil {
		return nil, err
	}
	visitEscaper(root, e.Opts)
	return template, nil
}

// newContext is PebbleTemplateImpl.initContext: it builds the evaluation context and pushes the
// scope that holds the global variables.
func (e *Engine) newContext(loader *fileLoader, self *Template) *evalCtx {
	imported := make([]*Template, 0, 1)
	ctx := &evalCtx{
		engine:        e,
		loader:        loader,
		strict:        e.Opts.StrictVariables,
		locale:        e.Opts.Locale,
		zone:          e.Opts.TimeZone,
		maxSize:       e.Opts.MaxRenderedSize,
		imported:      &imported,
		namedImported: map[string]*Template{},
		scope:         newScopeChain(),
		depth:         new(int),
	}
	ctx.hier = newHierarchy(self)

	globals := map[string]any{
		"locale":   Locale(e.Opts.LocaleDisplay),
		"template": self,
		"_context": &globalContext{chain: ctx.scope},
	}
	ctx.scope.pushScope(globals)

	// The scope of the global variables the extensions provide; the core extension has none.
	ctx.scope.pushScope(nil)
	return ctx
}

// contextScope turns the context map the caller passed into the backing map of a scope. Only
// string keys can reach a scope, as Java's Map<String, Object> signature demands.
func contextScope(context *Map) map[string]any {
	scope := map[string]any{}
	if context == nil {
		return scope
	}
	for _, key := range context.Keys {
		if name, ok := key.(string); ok {
			scope[name] = context.Vals[key]
		}
	}
	return scope
}

// registerBlocksAndMacros is the core extension's MacroAndBlockRegistrantNodeVisitor. It walks
// the AST the way AbstractNodeVisitor does, which notably never descends into an embed tag, a
// cache tag or the body of a filter tag: blocks and macros declared there stay unregistered.
func (t *Template) registerBlocksAndMacros(node *Stmt) error {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case StmtRoot:
		return t.registerBlocksAndMacros(node.Body)
	case StmtBody:
		for _, child := range node.Children {
			if err := t.registerBlocksAndMacros(child); err != nil {
				return err
			}
		}
	case StmtBlock:
		t.blocks[node.Name] = node
		return t.registerBlocksAndMacros(node.Body)
	case StmtMacro:
		if _, exists := t.macros[node.Name]; exists {
			// The visitor wraps the PebbleException of registerMacro in a RuntimeException, whose
			// message is the exception's toString.
			cause := NewError("More than one macro can not share the same name: "+node.Name, "", 0)
			cause.NoLine = true
			return &JavaError{Kind: "RuntimeException",
				Message: "io.pebbletemplates.pebble.error.PebbleException: " + cause.Error(),
				Cause:   cause}
		}
		t.macros[node.Name] = &MacroDef{Name: node.Name, Params: node.Arguments,
			Body: node.Body, Template: t}
		return t.registerBlocksAndMacros(node.Body)
	case StmtAutoEscape, StmtParallel:
		return t.registerBlocksAndMacros(node.Body)
	case StmtFor:
		if err := t.registerBlocksAndMacros(node.Body); err != nil {
			return err
		}
		return t.registerBlocksAndMacros(node.Else)
	case StmtIf:
		for _, body := range node.Bodies {
			if err := t.registerBlocksAndMacros(body); err != nil {
				return err
			}
		}
		return t.registerBlocksAndMacros(node.Else)
	}
	return nil
}

// shallowCopy is PebbleTemplateImpl.shallowCopy, which the embed tag uses so that it can add
// blocks to a template without disturbing the one the include and extends tags see.
func (t *Template) shallowCopy() *Template {
	copied := &Template{Name: t.Name, Root: t.Root, engine: t.engine,
		blocks: make(map[string]*Stmt, len(t.blocks)), macros: make(map[string]*MacroDef, len(t.macros))}
	for name, block := range t.blocks {
		copied.blocks[name] = block
	}
	for name, macro := range t.macros {
		copied.macros[name] = macro
	}
	return copied
}

// resolveRelativePath resolves a template name mentioned by an include, import or extends tag
// against the name of this template.
func (t *Template) resolveRelativePath(ctx *evalCtx, relativePath string) string {
	if resolved := ctx.loader.resolveRelativePath(relativePath, t.Name); resolved != "" {
		return resolved
	}
	return relativePath
}
