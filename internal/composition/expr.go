package composition

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// CompiledExpr represents a pre-compiled expression that can be evaluated
// at request time with runtime data.
type CompiledExpr struct {
	Raw     string      // Original expression string
	Program *vm.Program // Compiled bytecode
}

// CompileExpression compiles an expression string for later evaluation.
// The env parameter defines the expected variable types for compile-time checking.
//
// Per RESEARCH.md Pitfall 3: Functions must be registered at BOTH compile time
// (via expr.Function) and runtime (in environment map). Use consistent environment
// builders to avoid runtime failures.
//
// Returns an error if the expression has invalid syntax or references undefined variables.
func CompileExpression(exprStr string, env map[string]interface{}) (*CompiledExpr, error) {
	// Compile with type checking
	program, err := expr.Compile(exprStr,
		expr.Env(env),
		expr.AllowUndefinedVariables(), // Allow partial env for validation
	)
	if err != nil {
		return nil, fmt.Errorf("expression compilation failed: %w", err)
	}

	return &CompiledExpr{
		Raw:     exprStr,
		Program: program,
	}, nil
}

// EvaluateExpression runs a compiled expression with runtime environment data.
// The environment should contain actual values for variables referenced in the expression.
//
// Returns the result of the expression evaluation or an error if evaluation fails.
func EvaluateExpression(compiled *CompiledExpr, env map[string]interface{}) (interface{}, error) {
	result, err := expr.Run(compiled.Program, env)
	if err != nil {
		return nil, fmt.Errorf("expression evaluation failed: %w", err)
	}
	return result, nil
}

// IsExpression checks if a string contains expression syntax using {{ }} delimiters.
// This follows the template style per CONTEXT.md decisions.
//
// Examples:
//   - "{{ req.query.user_id }}" -> true
//   - "/users/{{ req.query.id }}" -> true
//   - "/static/path" -> false
func IsExpression(s string) bool {
	return strings.Contains(s, "{{") && strings.Contains(s, "}}")
}

// exprPattern matches {{ expr }} patterns for extraction
var exprPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// ExtractExpressions finds all {{ expr }} patterns in a string and returns
// the expression content (without the delimiters).
//
// Example:
//   - "/users/{{ req.query.id }}/orders/{{ req.query.order }}"
//     returns: ["req.query.id", "req.query.order"]
func ExtractExpressions(s string) []string {
	matches := exprPattern.FindAllStringSubmatch(s, -1)
	if matches == nil {
		return nil
	}

	var exprs []string
	for _, match := range matches {
		if len(match) > 1 {
			// Trim whitespace from expression content
			exprs = append(exprs, strings.TrimSpace(match[1]))
		}
	}
	return exprs
}

