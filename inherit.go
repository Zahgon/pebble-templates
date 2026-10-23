/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package pebble

// hierarchy is io.pebbletemplates.pebble.template.Hierarchy: the chain of templates of one
// evaluation, from the child that was asked for up to the last ancestor, with a cursor that the
// block lookup moves up and down. An ancestor may be nil, which happens when an extends tag
// evaluated its template name to null.
type hierarchy struct {
	templates []*Template
	current   int
}

func newHierarchy(current *Template) *hierarchy {
	return &hierarchy{templates: []*Template{current}}
}

func (h *hierarchy) pushAncestor(ancestor *Template) {
	h.templates = append(h.templates, ancestor)
}

func (h *hierarchy) ascend() { h.current++ }

func (h *hierarchy) descend() { h.current-- }

// child is the template below the cursor, which overrides the blocks of the current one.
func (h *hierarchy) child() *Template {
	if h.current == 0 {
		return nil
	}
	return h.templates[h.current-1]
}

// parent is the template above the cursor.
func (h *hierarchy) parent() *Template {
	if h.current == len(h.templates)-1 {
		return nil
	}
	return h.templates[h.current+1]
}

// block is PebbleTemplateImpl.block: a block declaration renders the override of the child
// template when there is one, its own body when it has one, and otherwise asks the parent.
func (t *Template) block(ctx *evalCtx, w tw, name string, ignoreOverriden bool) error {
	*ctx.depth++
	defer func() { *ctx.depth-- }()
	if *ctx.depth > maxRenderDepth {
		return &JavaError{Kind: "StackOverflowError"}
	}

	child := ctx.hier.child()
	if !ignoreOverriden && child != nil {
		ctx.hier.descend()
		if err := child.block(ctx, w, name, false); err != nil {
			return err
		}
		ctx.hier.ascend()
		return nil
	}
	if block, ok := t.blocks[name]; ok {
		// The body of a block renders with the template that declared it as its self.
		return ctx.render(block.Body, t, w)
	}
	if parent := ctx.hier.parent(); parent != nil {
		ctx.hier.ascend()
		if err := parent.block(ctx, w, name, true); err != nil {
			return err
		}
		ctx.hier.descend()
	}
	return nil
}

// includeTemplate is PebbleTemplateImpl.includeTemplate: the included template is evaluated with
// a copy of the context that has its own template hierarchy and an extra scope for the variables
// of the with clause.
func (t *Template) includeTemplate(ctx *evalCtx, w tw, name string, vars map[string]any) error {
	template, err := t.engine.getTemplate(ctx.loader, t.resolveRelativePath(ctx, name))
	if err != nil {
		return err
	}
	newCtx := ctx.shallowCopyWithoutInheritanceChain(template)
	newCtx.scope.pushScope(nil)
	for key, value := range vars {
		newCtx.scope.put(key, value)
	}
	if err := template.evaluate(newCtx, w); err != nil {
		return err
	}
	newCtx.scope.popScope()
	return nil
}

// embedTemplate is PebbleTemplateImpl.embedTemplate: the embedded template is evaluated as if it
// extended a template that holds its own blocks, so that the blocks of the embed tag override
// them exactly as the blocks of a child template do.
func (t *Template) embedTemplate(ctx *evalCtx, line int, w tw, name string, vars map[string]any,
	overridden []*Stmt) error {
	embeddedName := t.resolveRelativePath(ctx, name)
	loaded, err := t.engine.getTemplate(ctx.loader, embeddedName)
	if err != nil {
		return err
	}

	// A shallow copy keeps the blocks of the embed tag out of the template the include and
	// extends tags see.
	embedded := loaded.shallowCopy()

	newCtx := ctx.shallowCopyWithoutInheritanceChain(embedded)
	newCtx.scope.pushScope(nil)
	for key, value := range vars {
		newCtx.scope.put(key, value)
	}

	// The fake root template renders the body of the embedded template and owns its blocks, which
	// makes it the parent of the embedded template in the hierarchy.
	var body *Stmt
	if embedded.Root != nil && embedded.Root.Kind == StmtRoot {
		body = embedded.Root.Body
	} else {
		body = embedded.Root
	}
	fakeRoot := &Template{
		Name:   embeddedName,
		Root:   &Stmt{Kind: StmtBody, Line: line, Children: []*Stmt{body}},
		engine: t.engine,
		blocks: map[string]*Stmt{},
		macros: map[string]*MacroDef{},
	}
	for blockName, block := range embedded.blocks {
		fakeRoot.blocks[blockName] = block
	}
	// The overriding blocks were parsed into the host template, so they have to be moved into the
	// embedded template, which the fake root now treats as its child.
	for _, block := range overridden {
		embedded.blocks[block.Name] = block
	}
	newCtx.hier.pushAncestor(fakeRoot)

	if err := embedded.evaluate(newCtx, w); err != nil {
		return err
	}
	newCtx.scope.popScope()
	return nil
}

// importTemplate is PebbleTemplateImpl.importTemplate: the macros of the imported template become
// reachable by their bare names.
func (t *Template) importTemplate(ctx *evalCtx, name string) error {
	template, err := t.engine.getTemplate(ctx.loader, t.resolveRelativePath(ctx, name))
	if err != nil {
		return err
	}
	*ctx.imported = append(*ctx.imported, template)
	return nil
}

// importNamedTemplate is PebbleTemplateImpl.importNamedTemplate, the "import ... as" form.
func (t *Template) importNamedTemplate(ctx *evalCtx, name, alias string) error {
	template, err := t.engine.getTemplate(ctx.loader, t.resolveRelativePath(ctx, name))
	if err != nil {
		return err
	}
	if _, exists := ctx.namedImported[alias]; exists {
		err := NewError("More than one named template can not share the same name: "+alias, "", 0)
		err.NoLine = true
		return err
	}
	ctx.namedImported[alias] = template
	return nil
}

// importNamedMacros is PebbleTemplateImpl.importNamedMacrosFromTemplate, the from tag: the named
// macros of another template are registered in this one under their aliases.
func (t *Template) importNamedMacros(ctx *evalCtx, name string, names, aliases []string) error {
	template, err := t.engine.getTemplate(ctx.loader, t.resolveRelativePath(ctx, name))
	if err != nil {
		return err
	}
	for index, macroName := range names {
		alias := macroName
		if index < len(aliases) && aliases[index] != "" {
			alias = aliases[index]
		}
		macro, ok := template.macros[macroName]
		if !ok {
			err := NewError("Function or Macro ["+macroName+"] referenced by alias ["+alias+
				"] does not exist.", "", 0)
			err.NoLine = true
			return err
		}
		if _, exists := t.macros[alias]; exists {
			err := NewError("More than one macro can not share the same name: "+alias, "", 0)
			err.NoLine = true
			return err
		}
		t.macros[alias] = macro
	}
	return nil
}
