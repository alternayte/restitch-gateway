package composition

import (
	"fmt"
	"net/http"
)

// CompositionResponse is the final response to send to client.
type CompositionResponse struct {
	Status      int
	ContentType string
	Body        interface{} // JSON-serializable body
}

// BuildResponse evaluates response template with step results.
// It evaluates the status expression (if present) and recursively evaluates
// the body template to produce the final response.
//
// Per CONTEXT.md decisions:
//   - Status defaults to 200 if not specified
//   - Status can be static int or expression string
//   - Body template is recursively evaluated preserving structure
//   - Expression strings like "{{ steps.user.body }}" are evaluated and replaced
func BuildResponse(template *CompiledResponse, results map[string]*StepResult, req *http.Request) (*CompositionResponse, error) {
	// Build environment for expression evaluation
	env := buildRequestEnv(req, results)

	// Evaluate status
	status := 200 // Default status
	if template.StatusExpr != nil {
		statusValue, err := EvaluateExpression(template.StatusExpr, env)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate status expression: %w", err)
		}

		// Convert to int
		switch v := statusValue.(type) {
		case int:
			status = v
		case float64:
			status = int(v)
		default:
			return nil, fmt.Errorf("status expression returned non-numeric value: %T", statusValue)
		}
	}

	// Evaluate body template recursively
	body, err := evaluateTemplate(template.BodyTemplate, env)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate response body: %w", err)
	}

	return &CompositionResponse{
		Status:      status,
		ContentType: template.ContentType,
		Body:        body,
	}, nil
}

// evaluateTemplate recursively evaluates a template structure.
// It walks through maps, arrays, and strings, evaluating expression templates
// and preserving the overall structure.
//
// For strings containing {{ expr }}, it evaluates the expression and replaces it.
// For nested structures, it recurses and evaluates each part.
func evaluateTemplate(template interface{}, env map[string]interface{}) (interface{}, error) {
	switch v := template.(type) {
	case string:
		// Check if it's an expression template
		if IsExpression(v) {
			return evaluateExpressionString(v, env)
		}
		return v, nil

	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, value := range v {
			evaluated, err := evaluateTemplate(value, env)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", key, err)
			}
			result[key] = evaluated
		}
		return result, nil

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, value := range v {
			evaluated, err := evaluateTemplate(value, env)
			if err != nil {
				return nil, fmt.Errorf("index %d: %w", i, err)
			}
			result[i] = evaluated
		}
		return result, nil

	default:
		// Pass through numbers, bools, nil, etc.
		return v, nil
	}
}

// evaluateExpressionString evaluates a string that contains expression templates.
// If it's a single expression like "{{ expr }}", it returns the evaluated value.
// If it's a template like "Hello {{ name }}", it interpolates and returns a string.
func evaluateExpressionString(s string, env map[string]interface{}) (interface{}, error) {
	// Extract expressions from the string
	exprs := ExtractExpressions(s)
	if len(exprs) == 0 {
		return s, nil
	}

	// Check if the entire string is a single expression
	// If so, return the evaluated value directly (preserve type)
	trimmed := trimSpaces(s)
	if len(exprs) == 1 && len(trimmed) > 4 && trimmed[:2] == "{{" && trimmed[len(trimmed)-2:] == "}}" {
		// Single expression - evaluate and return as-is
		compiled, err := CompileExpression(exprs[0], env)
		if err != nil {
			return nil, fmt.Errorf("failed to compile expression: %w", err)
		}

		value, err := EvaluateExpression(compiled, env)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate expression: %w", err)
		}

		return value, nil
	}

	// Template string with embedded expressions - interpolate as string
	result, err := interpolateTemplate(s, env)
	if err != nil {
		return nil, err
	}

	return result, nil
}
