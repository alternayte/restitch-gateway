package composition

import (
	"fmt"
	"net/http"
)

// CompositionResponse is the final response to send to client.
type CompositionResponse struct {
	Status      int
	ContentType string
	Body        interface{}
}

// BuildResponse evaluates response template with step results.
func BuildResponse(template *CompiledResponse, results map[string]*StepResult, req *http.Request, stepErrors []StepErrorDetail) (*CompositionResponse, error) {
	env := buildRequestEnv(req, nil, nil, results)

	status := 200
	if template.StatusTmpl != nil {
		statusValue, err := template.StatusTmpl.EvalValue(env)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate status expression: %w", err)
		}

		switch v := statusValue.(type) {
		case int:
			status = v
		case float64:
			status = int(v)
		default:
			return nil, fmt.Errorf("status expression returned non-numeric value: %T", statusValue)
		}
	}

	body, err := evaluateBodyNode(template.Body, env)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate response body: %w", err)
	}

	if len(stepErrors) > 0 {
		if bodyMap, ok := body.(map[string]interface{}); ok {
			bodyMap["_errors"] = stepErrors
		}
	}

	return &CompositionResponse{
		Status:      status,
		ContentType: template.ContentType,
		Body:        body,
	}, nil
}

// evaluateBodyNode recursively evaluates a compiled body tree.
func evaluateBodyNode(node *CompiledBodyNode, env map[string]any) (any, error) {
	if node == nil {
		return nil, nil
	}

	if node.Tmpl != nil {
		return node.Tmpl.EvalValue(env)
	}

	if node.Map != nil {
		result := make(map[string]interface{}, len(node.Map))
		for key, child := range node.Map {
			val, err := evaluateBodyNode(child, env)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", key, err)
			}
			result[key] = val
		}
		return result, nil
	}

	if node.List != nil {
		result := make([]interface{}, len(node.List))
		for i, child := range node.List {
			val, err := evaluateBodyNode(child, env)
			if err != nil {
				return nil, fmt.Errorf("index %d: %w", i, err)
			}
			result[i] = val
		}
		return result, nil
	}

	return node.Literal, nil
}
