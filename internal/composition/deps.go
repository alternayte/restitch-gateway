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
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// ExtractDependencies parses an expression and returns all step names referenced.
// It identifies step references by looking for patterns like:
//   - steps.user.body.id
//   - steps["user-service"].body
//
// Example: "steps.user.body.id + steps.orders.body[0]" returns ["user", "orders"]
//
// Returns an error if the expression has invalid syntax.
func ExtractDependencies(exprStr string) ([]string, error) {
	// Parse expression to AST
	tree, err := parser.Parse(exprStr)
	if err != nil {
		return nil, err
	}

	// Walk AST to find step references
	visitor := &dependencyVisitor{
		dependencies: make(map[string]struct{}),
	}
	ast.Walk(&tree.Node, visitor)

	// Convert to slice
	deps := make([]string, 0, len(visitor.dependencies))
	for dep := range visitor.dependencies {
		deps = append(deps, dep)
	}

	return deps, nil
}

// dependencyVisitor implements ast.Visitor to extract step dependencies.
type dependencyVisitor struct {
	dependencies map[string]struct{}
}

// Visit is called for each node in the AST.
func (v *dependencyVisitor) Visit(node *ast.Node) {
	// Look for MemberNode where the base is "steps" identifier
	// This matches: steps.user, steps["user"], steps.user.body, etc.
	if member, ok := (*node).(*ast.MemberNode); ok {
		if ident, ok := member.Node.(*ast.IdentifierNode); ok {
			if ident.Value == "steps" {
				// Extract step name from property
				// Handle both identifier property (steps.user) and string property (steps["user"])
				if prop, ok := member.Property.(*ast.IdentifierNode); ok {
					v.dependencies[prop.Value] = struct{}{}
				} else if prop, ok := member.Property.(*ast.StringNode); ok {
					v.dependencies[prop.Value] = struct{}{}
				}
			}
		}
	}
}

// ExtractAllDependencies extracts dependencies from multiple expression strings.
// It returns a deduplicated list of all step names referenced across all expressions.
//
// Returns an error if any expression has invalid syntax.
func ExtractAllDependencies(exprs ...string) ([]string, error) {
	allDeps := make(map[string]struct{})

	for _, expr := range exprs {
		deps, err := ExtractDependencies(expr)
		if err != nil {
			return nil, err
		}
		for _, dep := range deps {
			allDeps[dep] = struct{}{}
		}
	}

	// Convert to slice
	result := make([]string, 0, len(allDeps))
	for dep := range allDeps {
		result = append(result, dep)
	}

	return result, nil
}

// MergeDependencies combines explicit depends_on with inferred dependencies.
// Returns a deduplicated list of all dependencies.
func MergeDependencies(explicit []string, inferred []string) []string {
	merged := make(map[string]struct{})

	for _, dep := range explicit {
		merged[dep] = struct{}{}
	}
	for _, dep := range inferred {
		merged[dep] = struct{}{}
	}

	// Convert to slice
	result := make([]string, 0, len(merged))
	for dep := range merged {
		result = append(result, dep)
	}

	return result
}
