// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package composition

import (
	"fmt"
	"sort"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/builtin"
)

// The compile environment holds untyped maps, so expr cannot see that
// req.path is a string or that req.nope does not exist. checkReferences
// walks the expression and checks every dotted name against the shape the
// executor builds (env.go), so a wrong reference fails validation instead
// of every request.

// scalarFields are the members that hold one value and take no member.
var scalarFields = map[string]map[string]string{
	"req":   {"method": "a string", "path": "a string"},
	"step":  {"status": "a number"},
	"steps": {},
}

// stepFields are the members of steps.<name>.
var stepFields = map[string]bool{"status": true, "headers": true, "body": true}

type refChecker struct {
	env      map[string]any
	declared map[string]bool
	// stepNames checks that steps.<name> names a step. Compilation leaves
	// that to the DAG check, which reports it in its own words; the lint
	// has no DAG and turns it on.
	stepNames bool
	err       error
}

func checkReferences(node ast.Node, env map[string]any, stepNames bool) error {
	if env == nil {
		return nil
	}
	c := &refChecker{env: env, declared: map[string]bool{}, stepNames: stepNames}
	ast.Walk(&node, c)
	return c.err
}

func (c *refChecker) fail(format string, args ...any) {
	if c.err == nil {
		c.err = fmt.Errorf(format, args...)
	}
}

func (c *refChecker) Visit(node *ast.Node) {
	switch n := (*node).(type) {
	case *ast.VariableDeclaratorNode:
		c.declared[n.Name] = true
	case *ast.IdentifierNode:
		if _, ok := c.env[n.Value]; !ok && !c.declared[n.Value] && !isFunction(n.Value) {
			c.fail("unknown name %q (use req, request or steps)", n.Value)
		}
	case *ast.MemberNode:
		c.checkChain(n)
	}
}

// chain returns the dotted names of a member expression rooted at an
// identifier, for example [req path id]; it stops at an index or a call.
func chain(n ast.Node) []string {
	switch m := n.(type) {
	case *ast.IdentifierNode:
		return []string{m.Value}
	case *ast.MemberNode:
		prop, ok := m.Property.(*ast.StringNode)
		if !ok {
			return nil
		}
		base := chain(m.Node)
		if base == nil {
			return nil
		}
		return append(base, prop.Value)
	}
	return nil
}

func (c *refChecker) checkChain(n *ast.MemberNode) {
	names := chain(n)
	if len(names) < 2 {
		return
	}
	root := names[0]
	switch root {
	case "req", "request":
		fields, _ := c.env[root].(map[string]any)
		if _, ok := fields[names[1]]; !ok {
			c.fail("%s has no field %q (it has %s)", root, names[1], keys(fields))
			return
		}
		if kind, ok := scalarFields["req"][names[1]]; ok && len(names) > 2 {
			c.fail("%s.%s is %s and has no member %q", root, names[1], kind, names[2])
		}
	case "steps":
		if known, ok := c.env["steps"].(map[string]any); ok && c.stepNames {
			if _, ok := known[names[1]]; !ok {
				c.fail("steps has no step %q (it has %s)", names[1], keys(known))
				return
			}
		}
		if len(names) < 3 {
			return
		}
		if !stepFields[names[2]] {
			c.fail("steps.%s has no field %q (it has body, headers and status)", names[1], names[2])
			return
		}
		if kind, ok := scalarFields["step"][names[2]]; ok && len(names) > 3 {
			c.fail("steps.%s.%s is %s and has no member %q", names[1], names[2], kind, names[3])
		}
	}
}

func keys(m map[string]any) string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return fmt.Sprint(out)
}

// isFunction reports whether name is an expr builtin used as a value.
func isFunction(name string) bool {
	_, ok := builtin.Index[name]
	return ok
}

// LintTemplate compiles one template the way publish does, and also checks
// that every steps.<name> names one of steps. Studio calls it as the user
// types.
func LintTemplate(raw string, steps []string) error {
	t, err := CompileTemplate(raw, BuildBaseEnvironment(steps))
	if err != nil {
		return err
	}
	env := BuildBaseEnvironment(steps)
	if len(steps) == 0 {
		env["steps"] = map[string]any{}
	}
	for _, seg := range t.Segments {
		if seg.Program == nil {
			continue
		}
		if err := checkReferences(seg.Program.Node(), env, true); err != nil {
			return fmt.Errorf("expression %q: %w", seg.Expr, err)
		}
	}
	return nil
}
