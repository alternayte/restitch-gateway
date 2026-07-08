package composition

import (
	"net/http"

	"github.com/restitch/restitch-gateway/internal/inbound"
)

// buildRequestEnv creates the environment for expression evaluation.
// Populates per §2.4: req.method/path/params/query/query_all/headers/body,
// sets env["request"] = env["req"] (alias), and steps.
func buildRequestEnv(req *http.Request, params map[string]string, body any, stepResults map[string]*StepResult) map[string]any {
	query := make(map[string]string)
	queryAll := make(map[string][]string)
	for key, values := range req.URL.Query() {
		queryAll[key] = values
		if len(values) > 0 {
			query[key] = values[0]
		}
	}

	headers := make(map[string]string)
	for key, values := range req.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	if params == nil {
		params = map[string]string{}
	}

	var authClaims any
	if claims := inbound.GetClaims(req.Context()); claims != nil {
		authClaims = map[string]any(claims)
	}

	reqData := map[string]any{
		"method":    req.Method,
		"path":      req.URL.Path,
		"params":    params,
		"query":     query,
		"query_all": queryAll,
		"headers":   headers,
		"body":      body,
		"auth":      authClaims,
	}

	env := map[string]any{
		"req":     reqData,
		"request": reqData,
	}

	if len(stepResults) > 0 {
		steps := make(map[string]any)
		for name, result := range stepResults {
			if result == nil {
				steps[name] = nil
			} else {
				steps[name] = map[string]any{
					"status":  result.Status,
					"headers": convertHeaders(result.Headers),
					"body":    result.Body,
				}
			}
		}
		env["steps"] = steps
	}

	return env
}

// BuildBaseEnvironment creates a compile-time environment for expression validation.
func BuildBaseEnvironment(stepNames []string) map[string]any {
	reqData := map[string]any{
		"method":    "",
		"path":      "",
		"params":    map[string]string{},
		"query":     map[string]string{},
		"query_all": map[string][]string{},
		"headers":   map[string]string{},
		"body":      nil,
		"auth":      nil,
	}

	env := map[string]any{
		"req":     reqData,
		"request": reqData,
	}

	if len(stepNames) > 0 {
		steps := make(map[string]any)
		for _, name := range stepNames {
			steps[name] = map[string]any{
				"status":  0,
				"headers": map[string]string{},
				"body":    map[string]any{},
			}
		}
		env["steps"] = steps
	}

	return env
}
