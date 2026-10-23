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
	"io"
	"math/big"
	"reflect"
	"strings"

	"github.com/PebbleTemplates/pebble/internal/jnum"
	"github.com/PebbleTemplates/pebble/internal/jstr"
)

// maxRenderDepth guards against the endless recursion a self referencing block or macro causes.
// Java fails it with a StackOverflowError, whose depth depends on the thread's stack size.
const maxRenderDepth = 2000

// evalCtx is io.pebbletemplates.pebble.template.EvaluationContextImpl: everything the nodes and
// expressions of one evaluation need. The template being rendered is not part of it; like the
// Java code, every render and evaluate call carries its "self" template explicitly.
type evalCtx struct {
	engine *Engine
	loader *fileLoader

	strict  bool
	locale  string
	zone    string
	maxSize int

	// rendered counts the characters written through the size limited writers of this context.
	// A copied context starts counting again, as its AtomicInteger does.
	rendered int

	// imported is shared with the copies of this context, as the Java List is.
	imported      *[]*Template
	namedImported map[string]*Template

	scope *scopeChain
	hier  *hierarchy

	// depth is shared by every copy of this context, to catch runaway recursion.
	depth *int
}

// shallowCopyWithoutInheritanceChain is the context copy the include and embed tags make: it
// shares the scope chain and the imported templates but starts a fresh template hierarchy.
func (c *evalCtx) shallowCopyWithoutInheritanceChain(self *Template) *evalCtx {
	copied := *c
	copied.rendered = 0
	copied.hier = newHierarchy(self)
	return &copied
}

// threadSafeCopy is the context copy the parallel tag makes when it has an executor service.
func (c *evalCtx) threadSafeCopy() *evalCtx {
	copied := *c
	imported := append([]*Template{}, *c.imported...)
	copied.imported = &imported
	copied.namedImported = make(map[string]*Template, len(c.namedImported))
	for alias, template := range c.namedImported {
		copied.namedImported[alias] = template
	}
	copied.scope = c.scope.deepCopy()
	return &copied
}

// tw is a java.io.Writer as the nodes use it: they only ever write strings and flush.
type tw interface {
	write(s string) error
}

// ioWriter is the writer the caller of Render passed.
type ioWriter struct {
	w io.Writer
}

func (o *ioWriter) write(s string) error {
	if _, err := io.WriteString(o.w, s); err != nil {
		// Java lets the IOException of the output writer escape; the reference command reports it
		// as an IOException and exits with 74.
		return &JavaError{Kind: "IOException", Message: err.Error()}
	}
	return nil
}

// bufWriter is a java.io.StringWriter: the writer of a macro body, a block rendered by the
// parent() function, a cache tag and a filter tag.
type bufWriter struct {
	buf strings.Builder
}

func (b *bufWriter) write(s string) error {
	b.buf.WriteString(s)
	return nil
}

func (b *bufWriter) String() string { return b.buf.String() }

// limitWriter is io.pebbletemplates.pebble.utils.LimitedSizeWriter: it counts the characters
// written through it in the evaluation context and fails once they pass maxRenderedSize.
type limitWriter struct {
	inner tw
	ctx   *evalCtx
}

func (l *limitWriter) write(s string) error {
	// The count is incremented before the check, as the Java writer does for thread safety.
	l.ctx.rendered += utf16Len(s)
	if l.ctx.rendered > l.ctx.maxSize {
		err := NewError(fmt.Sprintf("Tried to write more than %d chars.", l.ctx.maxSize), "", 0)
		err.NoLine = true
		return err
	}
	return l.inner.write(s)
}

// limit is LimitedSizeWriter.from: an unlimited size leaves the writer alone.
func (c *evalCtx) limit(w tw) tw {
	if c.maxSize < 0 {
		return w
	}
	return &limitWriter{inner: w, ctx: c}
}

// utf16Len is String.length: Java counts the characters it writes in UTF-16 code units.
func utf16Len(s string) int {
	return jstr.Length(s)
}

// evaluate is the authoritative PebbleTemplateImpl.evaluate: it renders the root node and then,
// when the template turned out to have a parent, renders the parent, which pulls the blocks of
// this template back in. The first pass of a child template only renders the set and import tags.
func (t *Template) evaluate(ctx *evalCtx, w tw) error {
	w = ctx.limit(w)
	if err := ctx.render(t.Root, t, w); err != nil {
		return err
	}
	if parent := ctx.hier.parent(); parent != nil {
		ctx.hier.ascend()
		return parent.evaluate(ctx, w)
	}
	return nil
}

// render renders one node of the AST.
func (c *evalCtx) render(n *Stmt, self *Template, w tw) error {
	if n == nil {
		return nil
	}
	*c.depth++
	defer func() { *c.depth-- }()
	if *c.depth > maxRenderDepth {
		return &JavaError{Kind: "StackOverflowError"}
	}

	switch n.Kind {
	case StmtRoot:
		// RootNode asks its body to render only the inheritance safe nodes, which matters as soon
		// as an extends tag of the body has set a parent.
		return c.renderBody(n.Body, self, w, true)
	case StmtBody:
		return c.renderBody(n, self, w, false)
	case StmtText:
		return w.write(n.Text)
	case StmtPrint:
		return c.renderPrint(n, self, w)
	case StmtIf:
		return c.renderIf(n, self, w)
	case StmtFor:
		return c.renderFor(n, self, w)
	case StmtSet:
		value, err := c.eval(n.Expr, self)
		if err != nil {
			return err
		}
		c.scope.set(n.Name, value)
		return nil
	case StmtBlock:
		return self.block(c, w, n.Name, false)
	case StmtExtends:
		return c.renderExtends(n, self)
	case StmtInclude:
		return c.renderInclude(n, self, w)
	case StmtEmbed:
		return c.renderEmbed(n, self, w)
	case StmtImport:
		return c.renderImport(n, self)
	case StmtFrom:
		return c.renderFrom(n, self)
	case StmtMacro:
		// A macro declaration renders nothing; it was registered when the template was parsed.
		return nil
	case StmtAutoEscape:
		// The escaper's node visitor has already dealt with the strategy of this tag.
		return c.render(n.Body, self, w)
	case StmtFilter:
		return c.renderFilter(n, self, w)
	case StmtCache:
		return c.renderCache(n, self, w)
	case StmtParallel:
		// Without an executor service the parallel tag renders its body in sequence.
		return c.render(n.Body, self, w)
	case StmtFlush:
		// The output of this port is never buffered by the engine itself.
		return nil
	}
	return nil
}

// renderBody is BodyNode.render. While onlyInheritanceSafe is set and the template has a parent,
// only the set and import tags of the body render; everything else is left to the parent pass.
func (c *evalCtx) renderBody(body *Stmt, self *Template, w tw, onlyInheritanceSafe bool) error {
	if body == nil {
		return nil
	}
	for _, child := range body.Children {
		if onlyInheritanceSafe && c.hier.parent() != nil {
			if child.Kind != StmtSet && child.Kind != StmtImport {
				continue
			}
		}
		if err := c.render(child, self, w); err != nil {
			return err
		}
	}
	return nil
}

// renderPrint is PrintNode.render: null prints nothing, a BigDecimal prints with toPlainString
// and everything else with String.valueOf.
func (c *evalCtx) renderPrint(n *Stmt, self *Template, w tw) error {
	value, err := c.eval(n.Expr, self)
	if err != nil {
		return err
	}
	if value == nil {
		return nil
	}
	return w.write(printString(value))
}

// printString is io.pebbletemplates.pebble.utils.StringUtils.toString.
func printString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case SafeString:
		return string(typed)
	case *BigDecimal:
		return typed.PlainString()
	default:
		return ToStr(value)
	}
}

// renderIf is IfNode.render. Every failure of a condition, including the one it raises itself for
// an unsupported value type, is wrapped in the same "wrong operand(s)" exception, because the
// Java catch clause encloses its own throw.
func (c *evalCtx) renderIf(n *Stmt, self *Template, w tw) error {
	satisfied := false
	for index, condition := range n.Conditions {
		if err := c.evalCondition(condition, self, &satisfied); err != nil {
			if !isRuntimeException(err) {
				return err
			}
			return &Error{Kind: "PebbleException",
				Message: "Wrong operand(s) type in conditional expression",
				File:    self.Name, Line: n.Line, Cause: err}
		}
		if satisfied {
			return c.render(n.Bodies[index], self, w)
		}
	}
	if n.Else != nil {
		return c.render(n.Else, self, w)
	}
	return nil
}

// evalCondition evaluates one condition of an if tag. Only a Boolean, a String and a Number are
// accepted; null is an error when strict variables are on and false otherwise.
func (c *evalCtx) evalCondition(condition *Expr, self *Template, satisfied *bool) error {
	result, err := c.eval(condition, self)
	if err != nil {
		return err
	}
	if result != nil {
		switch result.(type) {
		case bool, string, int32, int64, float32, float64, *big.Int, *BigDecimal:
			*satisfied = Truthy(result)
		default:
			return &Error{Kind: "PebbleException", File: self.Name, Line: condition.Line,
				Message: fmt.Sprintf(
					"Unsupported value type %s. Expected Boolean, String, Number in \"if\" statement",
					javaSimpleName(result))}
		}
	} else if c.strict {
		return &Error{Kind: "PebbleException", File: self.Name, Line: condition.Line,
			Message: "null value given to if statement and strict variables is set to true"}
	}
	return nil
}

// isRuntimeException reports whether Java would have let a catch of RuntimeException see this
// failure. An IOException of the output writer and a StackOverflowError are not caught.
func isRuntimeException(err error) bool {
	if javaErr, ok := err.(*JavaError); ok {
		return javaErr.Kind != "IOException" && javaErr.Kind != "StackOverflowError"
	}
	return true
}

// renderFor is ForNode.render: it iterates lists and the entry set of maps, pushes a scope with
// the iteration variable and the loop variable and renders the else body for an empty iterable.
func (c *evalCtx) renderFor(n *Stmt, self *Template, w tw) error {
	iterable, err := c.eval(n.Expr, self)
	if err != nil {
		return err
	}
	if iterable == nil {
		return nil
	}
	items, ok := toIterable(iterable)
	if !ok {
		return &Error{Kind: "PebbleException", File: self.Name, Line: n.Line,
			Message: "Not an iterable object. Value = [" + printString(iterable) + "]"}
	}
	if len(items) == 0 {
		if n.Else != nil {
			return c.render(n.Else, self, w)
		}
		return nil
	}

	c.scope.pushScope(nil)
	length := int32(len(items))
	loop := &loopVars{length: length}
	for index, item := range items {
		if index == 0 {
			loop.first = true
			loop.last = len(items) == 1
		} else if index == 1 {
			loop.first = false
		}
		loop.revindex = length - int32(index) - 1
		loop.index = int32(index)
		c.scope.put("loop", loop)
		c.scope.put(n.Name, item)
		if index == len(items)-1 {
			loop.last = true
		}
		if err := c.render(n.Body, self, w); err != nil {
			return err
		}
	}
	c.scope.popScope()
	return nil
}

// toIterable is ForNode.toIterable: a list iterates its elements and a map its entries. A string
// is not iterable in Pebble, nor is anything else.
func toIterable(value any) ([]any, bool) {
	switch typed := value.(type) {
	case *List:
		return typed.Items, true
	case *Map:
		entries := make([]any, 0, len(typed.Keys))
		for _, key := range typed.Keys {
			entries = append(entries, &mapEntry{Key: key, Value: typed.Vals[key], hash: typed.Hash})
		}
		return entries, true
	default:
		return nil, false
	}
}

// loopVars is ForNode.LoopVariables, the "loop" variable of a for tag. Java computes the length
// and the reverse index lazily, which only shows in the type of the value (a Number subclass);
// this port computes them eagerly as Integers.
type loopVars struct {
	index    int32
	length   int32
	revindex int32
	first    bool
	last     bool
}

// String is LoopVariables.toString, which prints the fields in the order Java's HashMap of fields
// happens to produce.
func (l *loopVars) String() string {
	return fmt.Sprintf("{last=%t, length=%d, index=%d, revindex=%d, first=%t}",
		l.last, l.length, l.index, l.revindex, l.first)
}

// mapEntry is the java.util.Map.Entry a for tag over a map yields.
type mapEntry struct {
	Key   any
	Value any
	hash  bool
}

// String is Map.Entry.toString.
func (e *mapEntry) String() string {
	return printString(e.Key) + "=" + printString(e.Value)
}

// renderExtends is ExtendsNode.render: it only records the parent of this template.
func (c *evalCtx) renderExtends(n *Stmt, self *Template) error {
	name, err := c.evalTemplateName(n.Expr, self)
	if err != nil {
		return err
	}
	parent, err := c.engine.getTemplate(c.loader, self.resolveRelativePath(c, name))
	if err != nil {
		return err
	}
	// A null template name pushes a null ancestor, which the hierarchy then reports as no parent.
	c.hier.pushAncestor(parent)
	return nil
}

// renderInclude is IncludeNode.render.
func (c *evalCtx) renderInclude(n *Stmt, self *Template, w tw) error {
	name, err := c.evalTemplateName(n.Expr, self)
	if err != nil {
		return err
	}
	vars, err := c.evalWithMap(n.Arguments, self)
	if err != nil {
		return err
	}
	if name == "" {
		return &Error{Kind: "PebbleException", File: self.Name, Line: n.Line,
			Message: "The template name in an include tag evaluated to NULL. If the template name is static, make sure to wrap it in quotes."}
	}
	return self.includeTemplate(c, w, name, vars)
}

// renderEmbed is EmbedNode.render.
func (c *evalCtx) renderEmbed(n *Stmt, self *Template, w tw) error {
	name, err := c.evalTemplateName(n.Expr, self)
	if err != nil {
		return err
	}
	vars, err := c.evalWithMap(n.Arguments, self)
	if err != nil {
		return err
	}
	if name == "" {
		return &Error{Kind: "PebbleException", File: self.Name, Line: n.Line,
			Message: "The template name in an embed tag evaluated to NULL. If the template name is static, make sure to wrap it in quotes."}
	}
	var overridden []*Stmt
	if n.Body != nil {
		for _, child := range n.Body.Children {
			if child.Kind == StmtBlock {
				overridden = append(overridden, child)
			}
		}
	}
	return self.embedTemplate(c, n.Line, w, name, vars, overridden)
}

// renderImport is ImportNode.render.
func (c *evalCtx) renderImport(n *Stmt, self *Template) error {
	name, err := c.evalTemplateName(n.Expr, self)
	if err != nil {
		return err
	}
	if n.Name != "" {
		if err := self.importNamedTemplate(c, name, n.Name); err != nil {
			return err
		}
		// The alias is put into scope so that its macros can be reached as its attributes.
		c.scope.put(n.Name, &macroProvider{template: c.namedImported[n.Name]})
		return nil
	}
	return self.importTemplate(c, name)
}

// renderFrom is FromNode.render.
func (c *evalCtx) renderFrom(n *Stmt, self *Template) error {
	name, err := c.evalTemplateName(n.Expr, self)
	if err != nil {
		return err
	}
	return self.importNamedMacros(c, name, n.Names, n.Aliases)
}

// renderFilter renders the body of a filter tag and sends it through the filter chain, which is
// what the print node of a FilterExpression over a RenderableNodeExpression does in Java.
func (c *evalCtx) renderFilter(n *Stmt, self *Template, w tw) error {
	body, err := c.renderToString(n.Body, self)
	if err != nil {
		return err
	}
	value, err := c.evalFilterChain(n.Expr, self, body)
	if err != nil {
		return err
	}
	if value == nil {
		return nil
	}
	return w.write(printString(value))
}

// evalFilterChain evaluates a chain of filter expressions whose innermost input is the already
// rendered body of a filter tag.
func (c *evalCtx) evalFilterChain(e *Expr, self *Template, body string) (any, error) {
	if e == nil {
		return body, nil
	}
	switch e.Kind {
	case ExprFilter:
		return c.applyFilter(e.Name, e.Arguments, e.Line, self, func() (any, error) {
			return c.evalFilterChain(e.Args[0], self, body)
		})
	case ExprRenderableNode:
		if e.Body == nil {
			return body, nil
		}
	}
	return c.eval(e, self)
}

// renderCache is CacheNode.render. The reference command has no tag cache, so the body is
// rendered on every pass.
func (c *evalCtx) renderCache(n *Stmt, self *Template, w tw) error {
	if n.Arguments != nil {
		// The cache key is evaluated even though nothing caches.
		for _, arg := range n.Arguments.Positional {
			if _, err := c.eval(arg, self); err != nil {
				return err
			}
		}
	}
	body, err := c.renderToString(n.Body, self)
	if err != nil {
		return err
	}
	return w.write(body)
}

// renderToString renders a node into a java.io.StringWriter, which the size limit still watches.
func (c *evalCtx) renderToString(n *Stmt, self *Template) (string, error) {
	buf := &bufWriter{}
	if err := c.render(n, self, c.limit(buf)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// evalTemplateName evaluates the template name of an extends, include, embed, import or from tag.
// Java casts the result to String, so a non string is a ClassCastException.
func (c *evalCtx) evalTemplateName(e *Expr, self *Template) (string, error) {
	value, err := c.eval(e, self)
	if err != nil {
		return "", err
	}
	return castString(value)
}

// evalWithMap evaluates the map of a "with" clause of an include or embed tag.
func (c *evalCtx) evalWithMap(args *Arguments, self *Template) (map[string]any, error) {
	vars := map[string]any{}
	if args == nil {
		return vars, nil
	}
	for _, arg := range args.Positional {
		value, err := c.eval(arg, self)
		if err != nil {
			return nil, err
		}
		entries, ok := value.(*Map)
		if !ok {
			continue
		}
		for _, key := range entries.Keys {
			name, err := castString(key)
			if err != nil {
				return nil, err
			}
			vars[name] = entries.Vals[key]
		}
	}
	for index, name := range args.Names {
		value, err := c.eval(args.Named[index], self)
		if err != nil {
			return nil, err
		}
		vars[name] = value
	}
	return vars, nil
}

// eval evaluates one expression of the AST.
func (c *evalCtx) eval(e *Expr, self *Template) (any, error) {
	if e == nil {
		return nil, nil
	}
	switch e.Kind {
	case ExprLiteralString, ExprLiteralLong, ExprLiteralInt, ExprLiteralDouble, ExprLiteralBool:
		return e.Value, nil
	case ExprLiteralBigDecimal:
		text, _ := e.Value.(string)
		value, err := jnum.ParseBigDecimal(text)
		if err != nil {
			return nil, &JavaError{Kind: "NumberFormatException", Message: err.Error()}
		}
		return value, nil
	case ExprLiteralNull:
		return nil, nil
	case ExprContextVariable:
		return c.lookup(e.Name, e.Line, self)
	case ExprGetAttribute:
		return c.getAttributeExpr(e, self)
	case ExprFilter:
		return c.applyFilter(e.Name, e.Arguments, e.Line, self, func() (any, error) {
			return c.eval(e.Args[0], self)
		})
	case ExprTest:
		return c.applyTest(e, self)
	case ExprFunction:
		return c.invokeFunctionOrMacro(e.Name, e.Arguments, e.Line, self)
	case ExprBinary:
		return c.evalBinary(e, self)
	case ExprUnary:
		value, err := c.eval(e.Args[0], self)
		if err != nil {
			return nil, err
		}
		return ApplyUnary(e.Op, value, c.view(self), e.Line)
	case ExprTernary:
		condition, err := c.eval(e.Args[0], self)
		if err != nil {
			return nil, err
		}
		if condition != nil && Truthy(condition) {
			return c.eval(e.Args[1], self)
		}
		return c.eval(e.Args[2], self)
	case ExprArray:
		items := make([]any, 0, len(e.Args))
		for _, arg := range e.Args {
			value, err := c.eval(arg, self)
			if err != nil {
				return nil, err
			}
			items = append(items, value)
		}
		return &List{Items: items}, nil
	case ExprMap:
		return c.evalMap(e, self)
	case ExprRange:
		// RangeExpression delegates to the range function with two positional arguments.
		return c.invokeFunctionOrMacro("range",
			&Arguments{Positional: []*Expr{e.Args[0], e.Args[1]}}, e.Line, self)
	case ExprConcat:
		return c.evalConcat(e, self)
	case ExprParentFunction:
		return c.evalParentFunction(e, self)
	case ExprBlockFunction:
		return c.evalBlockFunction(e, self)
	case ExprRenderableNode:
		return c.renderToString(e.Body, self)
	}
	return nil, nil
}

// lookup is ContextVariableExpression.evaluate.
func (c *evalCtx) lookup(name string, line int, self *Template) (any, error) {
	result := c.scope.get(name)
	if result == nil && c.strict && !c.scope.containsKey(name) {
		return nil, NewRootAttributeError(fmt.Sprintf(
			"Root attribute [%s] does not exist or can not be accessed and strict variables is set to true.",
			name), name, self.Name, line)
	}
	return result, nil
}

// evalBinary applies a binary operator. The and and or operators evaluate their right hand side
// only when they have to, so the evaluator keeps them for itself.
func (c *evalCtx) evalBinary(e *Expr, self *Template) (any, error) {
	switch e.Op {
	case "and", "or":
		left, err := c.eval(e.Args[0], self)
		if err != nil {
			return nil, err
		}
		if Truthy(left) == (e.Op == "or") {
			return e.Op == "or", nil
		}
		right, err := c.eval(e.Args[1], self)
		if err != nil {
			return nil, err
		}
		return Truthy(right), nil
	}
	left, err := c.eval(e.Args[0], self)
	if err != nil {
		return nil, err
	}
	right, err := c.eval(e.Args[1], self)
	if err != nil {
		return nil, err
	}
	return ApplyBinary(e.Op, left, right, c.view(self), e.Line)
}

// evalMap is MapExpression.evaluate, which builds a java.util.HashMap: the iteration order of a
// map literal is the hash order of its keys, not the order they were written in.
func (c *evalCtx) evalMap(e *Expr, self *Template) (any, error) {
	result := &Map{Vals: map[any]any{}, Hash: true}
	for index := 0; index+1 < len(e.Args); index += 2 {
		key, err := c.eval(e.Args[index], self)
		if err != nil {
			return nil, err
		}
		value, err := c.eval(e.Args[index+1], self)
		if err != nil {
			return nil, err
		}
		// Every value produced by the evaluator has a Java-object representation and is normally
		// comparable in Go (lists and maps are pointers). Keep malformed values supplied by a
		// custom caller from reaching a map assignment, which would otherwise panic.
		if key != nil && !reflect.TypeOf(key).Comparable() {
			return nil, &JavaError{Kind: "IllegalArgumentException",
				Message: "map key has no Java object representation: " + fmt.Sprintf("%T", key)}
		}
		if _, exists := result.Vals[key]; !exists {
			result.Keys = append(result.Keys, key)
		}
		result.Vals[key] = value
	}
	return result, nil
}

// evalConcat is ConcatenateExpression.evaluate: null operands contribute nothing.
func (c *evalCtx) evalConcat(e *Expr, self *Template) (any, error) {
	var result strings.Builder
	for _, arg := range e.Args {
		value, err := c.eval(arg, self)
		if err != nil {
			return nil, err
		}
		if value != nil {
			result.WriteString(concatString(value))
		}
	}
	return result.String(), nil
}

// concatString is Object.toString, which a BigDecimal answers with its scientific notation,
// unlike the print node.
func concatString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case SafeString:
		return string(typed)
	default:
		return ToStr(value)
	}
}

// evalParentFunction is ParentFunctionExpression.evaluate: it renders the block of the parent
// template, ignoring the override in this one.
func (c *evalCtx) evalParentFunction(e *Expr, self *Template) (any, error) {
	buf := &bufWriter{}
	parent := c.hier.parent()
	if parent == nil {
		return nil, &Error{Kind: "PebbleException", File: self.Name, Line: e.Line,
			Message: "Can not use parent function if template does not extend another template."}
	}
	c.hier.ascend()
	if err := parent.block(c, buf, e.Name, true); err != nil {
		return nil, err
	}
	c.hier.descend()
	return buf.String(), nil
}

// evalBlockFunction is BlockFunctionExpression.evaluate: the block("name") call.
func (c *evalCtx) evalBlockFunction(e *Expr, self *Template) (any, error) {
	var nameExpr *Expr
	if e.Arguments != nil && len(e.Arguments.Positional) > 0 {
		nameExpr = e.Arguments.Positional[0]
	}
	value, err := c.eval(nameExpr, self)
	if err != nil {
		return nil, err
	}
	name, err := castString(value)
	if err != nil {
		return nil, err
	}
	buf := &bufWriter{}
	if err := self.block(c, buf, name, false); err != nil {
		return nil, err
	}
	return buf.String(), nil
}

// applyFilter is FilterExpression.evaluate. The arguments are bound before the input is
// evaluated, the default filter swallows an AttributeNotFoundException of its input and every
// filter but escape receives a SafeString as a plain String.
func (c *evalCtx) applyFilter(name string, args *Arguments, line int, self *Template,
	input func() (any, error)) (any, error) {
	filter, ok := Filters[name]
	if !ok {
		return nil, &Error{Kind: "PebbleException", File: self.Name, Line: line,
			Message: fmt.Sprintf("Filter [%s] does not exist.", name)}
	}
	named, err := c.argumentMap(args, filter.ArgNames, line, self)
	if err != nil {
		return nil, err
	}
	value, err := input()
	if err != nil {
		if name != "default" || !isAttributeNotFound(err) {
			return nil, err
		}
		value = nil
	}
	if safe, ok := value.(SafeString); ok && name != "escape" {
		value = string(safe)
	}
	return filter.Apply(value, named, c.view(self), line)
}

// applyTest is PositiveTestExpression.evaluate, and its negation for "is not".
func (c *evalCtx) applyTest(e *Expr, self *Template) (any, error) {
	test, ok := Tests[e.Name]
	if !ok {
		return nil, &Error{Kind: "PebbleException", File: self.Name, Line: e.Line,
			Message: fmt.Sprintf("Test [%s] does not exist.", e.Name)}
	}
	named, err := c.argumentMap(e.Arguments, test.ArgNames, e.Line, self)
	if err != nil {
		return nil, err
	}
	input, err := c.eval(e.Args[0], self)
	if err != nil {
		// The defined test is the one test that must not fail on a missing attribute.
		if e.Name != "defined" || !isAttributeNotFound(err) {
			return nil, err
		}
		input = nil
	}
	result, err := test.Apply(input, named, c.view(self), e.Line)
	if err != nil {
		return nil, err
	}
	if e.Negated {
		return !result, nil
	}
	return result, nil
}

// isAttributeNotFound reports whether the failure is an AttributeNotFoundException, which the
// default filter and the defined test turn into a null input.
func isAttributeNotFound(err error) bool {
	pebbleErr, ok := err.(*Error)
	return ok && (pebbleErr.Kind == "AttributeNotFoundException" ||
		pebbleErr.Kind == "RootAttributeNotFoundException")
}

// invokeFunctionOrMacro is FunctionOrMacroInvocationExpression.evaluate: a function of the
// extensions wins over a macro of the template.
func (c *evalCtx) invokeFunctionOrMacro(name string, args *Arguments, line int,
	self *Template) (any, error) {
	if function, ok := Functions[name]; ok {
		named, err := c.argumentMap(args, function.ArgNames, line, self)
		if err != nil {
			return nil, err
		}
		return function.Apply(named, c.view(self), line)
	}
	return self.macro(c, name, args, false, line)
}

// argumentMap is ArgumentsNode.getArgumentMap: it binds the positional arguments to the declared
// names of the invocable, and checks the named ones against them. An invocable that declares no
// names at all takes its positional arguments under the names "0", "1" and so on.
func (c *evalCtx) argumentMap(args *Arguments, names []string, line int,
	self *Template) (map[string]any, error) {
	result := map[string]any{}
	if args == nil {
		return result, nil
	}
	if len(names) == 0 {
		for index, arg := range args.Positional {
			value, err := c.eval(arg, self)
			if err != nil {
				return nil, err
			}
			result[fmt.Sprint(index)] = value
		}
		for index, name := range args.Names {
			value, err := c.eval(args.Named[index], self)
			if err != nil {
				return nil, err
			}
			result[name] = value
		}
		return result, nil
	}
	for index, arg := range args.Positional {
		if index >= len(names) {
			return nil, &Error{Kind: "PebbleException", File: self.Name, Line: line,
				Message: fmt.Sprintf("The argument at position %d is not allowed. Only %d argument(s) are allowed.",
					index+1, len(names))}
		}
		value, err := c.eval(arg, self)
		if err != nil {
			return nil, err
		}
		result[names[index]] = value
	}
	for index, name := range args.Names {
		if !containsString(names, name) {
			return nil, &Error{Kind: "PebbleException", File: self.Name, Line: line,
				Message: "The following named argument does not exist: " + name}
		}
		value, err := c.eval(args.Named[index], self)
		if err != nil {
			return nil, err
		}
		result[name] = value
	}
	return result, nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// castString is a Java cast to String: null stays null, a String passes and anything else is a
// ClassCastException.
func castString(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	default:
		return "", classCast(value, "java.lang.String")
	}
}

// classCast builds the ClassCastException of a failed Java cast.
func classCast(value any, target string) error {
	from := javaClassName(value)
	return &JavaError{Kind: "ClassCastException", Message: fmt.Sprintf(
		"class %s cannot be cast to class %s (%s and %s are in module java.base of loader 'bootstrap')",
		from, target, from, target)}
}

// view is the Ctx the filters, functions and tests receive.
func (c *evalCtx) view(self *Template) Ctx {
	return &ctxView{ctx: c, self: self}
}

// ctxView is the part of the evaluation context the extensions see, bound to the template whose
// node is being evaluated.
type ctxView struct {
	ctx  *evalCtx
	self *Template
}

// Locale is the locale of the evaluation.
func (v *ctxView) Locale() string { return v.ctx.locale }

// Zone is the default time zone of the command.
func (v *ctxView) Zone() string { return v.ctx.zone }

// Strict reports the strictVariables setting.
func (v *ctxView) Strict() bool { return v.ctx.strict }

// Name is the name of the template being rendered.
func (v *ctxView) Name() string { return v.self.Name }

// Escaping reports whether auto escaping is on, and the default strategy.
func (v *ctxView) Escaping() (bool, string) {
	return v.ctx.engine.Opts.AutoEscaping, v.ctx.engine.Opts.DefaultEscapingStrategy
}

// Render renders a body into a string.
func (v *ctxView) Render(body *Stmt) (string, error) {
	return v.ctx.renderToString(body, v.self)
}

// Lookup resolves a variable the way a context variable expression does.
func (v *ctxView) Lookup(name string) (any, bool, error) {
	value, err := v.ctx.lookup(name, 0, v.self)
	if err != nil {
		return nil, false, err
	}
	return value, v.ctx.scope.containsKey(name), nil
}

// Eval evaluates an expression in the current scope.
func (v *ctxView) Eval(expr *Expr) (any, error) {
	return v.ctx.eval(expr, v.self)
}

// GetAttribute resolves obj.name the way GetAttributeExpression does.
func (v *ctxView) GetAttribute(obj any, name any, args *Arguments, line int) (any, error) {
	return v.ctx.getAttribute(obj, name, args, line, v.self, nil)
}

// utf16Runes is the UTF-16 form of a string, which the attribute methods of java.lang.String
// index into.
func utf16Runes(s string) []uint16 {
	return jstr.Units(s)
}

// utf16String turns UTF-16 code units back into a Go string.
func utf16String(units []uint16) string {
	return jstr.FromUnits(units)
}
