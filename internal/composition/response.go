package composition

import (
	"context"
	"fmt"
)

// CompositionResponse is the final response to send to client.
type CompositionResponse struct {
	Status      int
	ContentType string
	Body        interface{}
}

// BuildResponse evaluates response template with step results.
// failedSteps contains step names that failed or were skipped — used for
// nil-safe evaluation per K3: on eval error, if the template's deps
// intersect failedSteps, return nil instead of an error.
func BuildResponse(ctx context.Context, template *CompiledResponse, results map[string]*StepResult, rd *RequestData, stepErrors []StepErrorDetail, failedSteps map[string]bool) (*CompositionResponse, error) {
	env := buildRequestEnv(ctx, rd, results)

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

	body, err := evaluateBodyNode(template.Body, env, failedSteps)
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
// On eval error, if the template's deps intersect failedSteps, return nil
// (for IsSingle) or "" (for interpolation) instead of propagating the error.
func evaluateBodyNode(node *CompiledBodyNode, env map[string]any, failedSteps map[string]bool) (any, error) {
	if node == nil {
		return nil, nil
	}

	if node.Tmpl != nil {
		val, err := node.Tmpl.EvalValue(env)
		if err != nil {
			if hasFailedDep(node.Tmpl.Deps, failedSteps) {
				if node.Tmpl.IsSingle {
					return nil, nil
				}
				return "", nil
			}
			return nil, err
		}
		return val, nil
	}

	if node.Map != nil {
		result := make(map[string]interface{}, len(node.Map))
		for key, child := range node.Map {
			val, err := evaluateBodyNode(child, env, failedSteps)
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
			val, err := evaluateBodyNode(child, env, failedSteps)
			if err != nil {
				return nil, fmt.Errorf("index %d: %w", i, err)
			}
			result[i] = val
		}
		return result, nil
	}

	return node.Literal, nil
}

// hasFailedDep checks if any of the template's deps are in the failed set.
func hasFailedDep(deps []string, failedSteps map[string]bool) bool {
	for _, d := range deps {
		if failedSteps[d] {
			return true
		}
	}
	return false
}
