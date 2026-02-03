package composition

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/restitch/restitch-gateway/internal/client"
)

// StepResult holds the response from an upstream service.
type StepResult struct {
	Status  int                    // HTTP status code
	Headers http.Header            // Response headers
	Body    interface{}            // Parsed JSON body as map[string]interface{} or []interface{}
	RawBody []byte                 // Original response body
}

// ExecuteStep executes a single step by making HTTP request to upstream.
// It evaluates expressions for the request, builds the HTTP request with proper
// context propagation, executes it, and returns the parsed result.
//
// Per CONTEXT.md:
//   - Auto-propagates headers: X-Request-ID, X-Correlation-ID, traceparent, Accept, Accept-Language
//   - Generates UUID for X-Request-ID if not present
//   - Non-2xx status is NOT an error (upstream error passthrough)
//   - Only returns error for network failure or context cancellation
//
// Per RESEARCH.md Pitfall 7: ALWAYS use http.NewRequestWithContext for cancellation.
func ExecuteStep(ctx context.Context, step *CompiledStep, upstream *Upstream, env map[string]interface{}, httpClient *http.Client) (*StepResult, error) {
	// Evaluate path expression
	path, err := evaluatePath(step.PathExpr, env)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate path: %w", err)
	}

	// Build full URL
	url := strings.TrimRight(upstream.URL, "/") + "/" + strings.TrimLeft(path, "/")

	// Evaluate body expression if present (for POST/PUT)
	var body io.Reader
	if step.BodyExpr != nil && step.BodyExpr.Raw != "" {
		bodyContent, err := evaluateBody(step.BodyExpr, env)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate body: %w", err)
		}
		body = bytes.NewReader(bodyContent)
	}

	// Create HTTP request with context (CRITICAL for cancellation)
	req, err := http.NewRequestWithContext(ctx, step.Step.Method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Propagate headers per CONTEXT.md
	propagateHeaders(req, env)

	// Evaluate and set custom headers from step configuration
	for key, value := range step.Step.Headers {
		if expr, exists := step.HeaderExprs[key]; exists {
			// Header has expression - evaluate it
			evaluatedValue, err := evaluateHeader(expr, env)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate header %s: %w", key, err)
			}
			req.Header.Set(key, evaluatedValue)
		} else {
			// Static header value
			req.Header.Set(key, value)
		}
	}

	// Execute request using the provided HTTP client
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer client.DrainAndClose(resp.Body) // CRITICAL: Always drain for connection reuse

	// Read response body
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse JSON body if content type is JSON
	parsedBody, err := parseJSONBody(rawBody, resp.Header.Get("Content-Type"))
	if err != nil {
		// Non-JSON or invalid JSON - store as string
		parsedBody = string(rawBody)
	}

	return &StepResult{
		Status:  resp.StatusCode,
		Headers: resp.Header,
		Body:    parsedBody,
		RawBody: rawBody,
	}, nil
}

// buildRequestEnv creates the environment for expression evaluation.
// It includes the incoming request data and results from completed steps.
func buildRequestEnv(req *http.Request, stepResults map[string]*StepResult) map[string]interface{} {
	// Parse query parameters
	query := make(map[string]string)
	for key, values := range req.URL.Query() {
		if len(values) > 0 {
			query[key] = values[0] // Take first value for simplicity
		}
	}

	// Parse headers
	headers := make(map[string]string)
	for key, values := range req.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	env := map[string]interface{}{
		"req": map[string]interface{}{
			"path":    req.URL.Path,
			"query":   query,
			"headers": headers,
			"body":    map[string]interface{}{}, // TODO: Parse request body in future phases
		},
	}

	// Add step results if any
	if len(stepResults) > 0 {
		steps := make(map[string]interface{})
		for name, result := range stepResults {
			steps[name] = map[string]interface{}{
				"status":  result.Status,
				"headers": convertHeaders(result.Headers),
				"body":    result.Body,
			}
		}
		env["steps"] = steps
	}

	return env
}

// convertHeaders converts http.Header to map[string]string (first value only).
func convertHeaders(headers http.Header) map[string]string {
	result := make(map[string]string)
	for key, values := range headers {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

// parseJSONBody parses response body as JSON, returns raw bytes if not JSON.
// Per RESEARCH.md Pitfall 2: expr creates maps with string keys only.
func parseJSONBody(body []byte, contentType string) (interface{}, error) {
	// Check if content type indicates JSON
	if !strings.Contains(strings.ToLower(contentType), "json") {
		return nil, fmt.Errorf("not JSON content type: %s", contentType)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("empty body")
	}

	// Try to parse as JSON
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	return result, nil
}

// propagateHeaders auto-propagates tracing and accept headers per CONTEXT.md.
// Generates UUID for X-Request-ID if not present.
func propagateHeaders(req *http.Request, env map[string]interface{}) {
	// Extract headers from environment
	var incomingHeaders map[string]string
	if reqData, ok := env["req"].(map[string]interface{}); ok {
		if headers, ok := reqData["headers"].(map[string]string); ok {
			incomingHeaders = headers
		}
	}

	// List of headers to auto-propagate per CONTEXT.md
	propagatedHeaders := []string{
		"X-Request-ID",
		"X-Correlation-ID",
		"traceparent",
		"Accept",
		"Accept-Language",
	}

	// Propagate headers from incoming request
	// Note: HTTP headers are case-insensitive, so we need case-insensitive lookup
	for _, headerName := range propagatedHeaders {
		// Try exact match first
		value := incomingHeaders[headerName]
		if value == "" {
			// Try case-insensitive lookup (headers may be stored with canonical casing)
			for k, v := range incomingHeaders {
				if strings.EqualFold(k, headerName) {
					value = v
					break
				}
			}
		}
		if value != "" {
			req.Header.Set(headerName, value)
		}
	}

	// Generate X-Request-ID if not present
	if req.Header.Get("X-Request-ID") == "" {
		req.Header.Set("X-Request-ID", uuid.New().String())
	}
}

// evaluatePath evaluates the path expression and returns the path string.
func evaluatePath(pathExpr *CompiledExpr, env map[string]interface{}) (string, error) {
	if pathExpr == nil {
		return "", fmt.Errorf("path expression is nil")
	}

	// If template string with embedded expressions, interpolate them
	// Check this FIRST before checking Program, because templates have Program==nil
	if strings.Contains(pathExpr.Raw, "{{") {
		return interpolateTemplate(pathExpr.Raw, env)
	}

	// If no program (literal string), return raw value
	if pathExpr.Program == nil {
		return pathExpr.Raw, nil
	}

	// Single expression - evaluate it
	result, err := EvaluateExpression(pathExpr, env)
	if err != nil {
		return "", err
	}

	// Convert result to string
	return fmt.Sprintf("%v", result), nil
}

// evaluateBody evaluates the body expression and returns JSON bytes.
func evaluateBody(bodyExpr *CompiledExpr, env map[string]interface{}) ([]byte, error) {
	if bodyExpr == nil {
		return nil, fmt.Errorf("body expression is nil")
	}

	// If no program (literal string), return as JSON
	if bodyExpr.Program == nil {
		return []byte(bodyExpr.Raw), nil
	}

	// If template string with embedded expressions, interpolate them
	if strings.Contains(bodyExpr.Raw, "{{") {
		interpolated, err := interpolateTemplate(bodyExpr.Raw, env)
		if err != nil {
			return nil, err
		}
		return []byte(interpolated), nil
	}

	// Single expression - evaluate and marshal to JSON
	result, err := EvaluateExpression(bodyExpr, env)
	if err != nil {
		return nil, err
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body to JSON: %w", err)
	}

	return jsonBytes, nil
}

// evaluateHeader evaluates a header expression and returns the string value.
func evaluateHeader(headerExpr *CompiledExpr, env map[string]interface{}) (string, error) {
	if headerExpr == nil {
		return "", fmt.Errorf("header expression is nil")
	}

	// If no program (literal string), return raw value
	if headerExpr.Program == nil {
		return headerExpr.Raw, nil
	}

	// If template string with embedded expressions, interpolate them
	if strings.Contains(headerExpr.Raw, "{{") {
		return interpolateTemplate(headerExpr.Raw, env)
	}

	// Single expression - evaluate it
	result, err := EvaluateExpression(headerExpr, env)
	if err != nil {
		return "", err
	}

	// Convert result to string
	return fmt.Sprintf("%v", result), nil
}

// interpolateTemplate replaces {{ expr }} patterns with evaluated values.
// Example: "/users/{{ req.query.id }}" with id=123 -> "/users/123"
func interpolateTemplate(template string, env map[string]interface{}) (string, error) {
	result := template
	exprs := ExtractExpressions(template)

	for _, exprStr := range exprs {
		// Compile and evaluate the expression
		// Note: We compile at runtime here because env structure varies per request
		// BuildBaseEnvironment is used at config parse time for validation only
		compiled, err := CompileExpression(exprStr, env)
		if err != nil {
			return "", fmt.Errorf("failed to compile expression %q: %w", exprStr, err)
		}

		value, err := EvaluateExpression(compiled, env)
		if err != nil {
			return "", fmt.Errorf("failed to evaluate expression %q: %w", exprStr, err)
		}

		// Replace the {{ expr }} pattern with the value
		// Try both with and without spaces
		placeholder := "{{ " + exprStr + " }}"
		placeholderNoSpace := "{{" + exprStr + "}}"

		valueStr := fmt.Sprintf("%v", value)
		result = strings.ReplaceAll(result, placeholder, valueStr)
		result = strings.ReplaceAll(result, placeholderNoSpace, valueStr)
	}

	return result, nil
}
