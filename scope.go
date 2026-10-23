/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package pebble

// scope is io.pebbletemplates.pebble.template.Scope: one map of variables, which is "local" when
// a lookup must not fall through it. Only a macro invocation pushes a local scope.
type scope struct {
	vars  map[string]any
	local bool
}

func newScope(vars map[string]any, local bool) *scope {
	if vars == nil {
		vars = map[string]any{}
	}
	return &scope{vars: vars, local: local}
}

func (s *scope) shallowCopy() *scope {
	copied := make(map[string]any, len(s.vars))
	for key, value := range s.vars {
		copied[key] = value
	}
	return &scope{vars: copied, local: s.local}
}

// scopeChain is io.pebbletemplates.pebble.template.ScopeChain: the stack of scopes of one
// evaluation. Index 0 of stack is the top of the Java LinkedList, i.e. the innermost scope.
type scopeChain struct {
	stack []*scope
}

func newScopeChain() *scopeChain {
	return &scopeChain{}
}

// deepCopy copies every scope of the chain; the parallel tag needs it when it has an executor.
func (c *scopeChain) deepCopy() *scopeChain {
	copied := &scopeChain{stack: make([]*scope, 0, len(c.stack))}
	for _, s := range c.stack {
		copied.stack = append(copied.stack, s.shallowCopy())
	}
	return copied
}

// pushScope pushes a non local scope backed by the given map, which may be nil for a fresh one.
func (c *scopeChain) pushScope(vars map[string]any) {
	c.stack = append([]*scope{newScope(vars, false)}, c.stack...)
}

// pushLocalScope pushes a scope a lookup does not fall through.
func (c *scopeChain) pushLocalScope() {
	c.stack = append([]*scope{newScope(nil, true)}, c.stack...)
}

func (c *scopeChain) popScope() {
	c.stack = c.stack[1:]
}

// put writes into the innermost scope.
func (c *scopeChain) put(key string, value any) {
	c.stack[0].vars[key] = value
}

// get looks a variable up from the innermost scope outwards. A scope that holds the key with a
// null value, and a local scope, both stop the search.
func (c *scopeChain) get(key string) any {
	first := c.stack[0]
	if result := first.vars[key]; result != nil {
		return result
	}
	if len(c.stack) > 1 {
		if first.local || contains(first, key) {
			// the key could be defined with null and override another value below in the stack
			return nil
		}
		for _, s := range c.stack[1:] {
			if result := s.vars[key]; result != nil {
				return result
			} else if s.local || contains(s, key) {
				return nil
			}
		}
	}
	return nil
}

// containsKey reports whether any reachable scope declares the key.
func (c *scopeChain) containsKey(key string) bool {
	first := c.stack[0]
	if contains(first, key) {
		return true
	}
	if first.local {
		return false
	}
	for _, s := range c.stack[1:] {
		if contains(s, key) {
			return true
		}
		if s.local {
			return false
		}
	}
	return false
}

// set writes a variable into the scope that already declares it, or into the innermost one. It is
// what the set tag uses, which is why a set inside a for loop does not outlive the loop.
func (c *scopeChain) set(key string, value any) {
	for _, s := range c.stack {
		if s.local || contains(s, key) {
			s.vars[key] = value
			return
		}
	}
	c.put(key, value)
}

// globalScopes is ScopeChain.getGlobalScopes: the scopes below the innermost local scope, in the
// order a lookup would visit them.
func (c *scopeChain) globalScopes() []*scope {
	var scopes []*scope
	for _, s := range c.stack {
		if s.local {
			scopes = scopes[:0]
		} else {
			scopes = append(scopes, s)
		}
	}
	return scopes
}

func contains(s *scope, key string) bool {
	_, ok := s.vars[key]
	return ok
}

// globalContext is io.pebbletemplates.pebble.template.GlobalContext, the value of the "_context"
// global variable. It pretends to be a java.util.Map but implements only get: every other method
// throws an UnsupportedOperationException, which is why "_context" cannot be indexed with a
// number and fails outright when strict variables are on.
type globalContext struct {
	chain *scopeChain
}

// get is GlobalContext.get, which searches the global scopes only.
func (g *globalContext) get(key string) any {
	for _, s := range g.chain.globalScopes() {
		if result := s.vars[key]; result != nil {
			return result
		} else if contains(s, key) {
			return nil
		}
	}
	return nil
}

// unsupported is the UnsupportedOperationException every other Map method of GlobalContext
// throws. Its message is null, which the reference command prints as "null".
func unsupported() error {
	return &JavaError{Kind: "UnsupportedOperationException"}
}
