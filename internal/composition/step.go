package composition

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/restitch/restitch-gateway/internal/client"
)

// StepResult holds the response from an upstream service.
type StepResult struct {
	Status           int                    // HTTP status code
	Headers          http.Header            // Response headers
	Body             interface{}            // Parsed JSON body as map[string]interface{} or []interface{}
	RawBody          []byte                 // Original response body
	ErrorRuleMatched bool                   // True if error rule replaced the body
}

// ExecuteStep executes a single step with timeout hierarchy resolution.
// This is a wrapper that resolves timeout (step > upstream > default) and
// calls ExecuteStepWithTimeout.
func ExecuteStep(ctx context.Context, step *CompiledStep, upstream *CompiledUpstream, env map[string]interface{}, baseClient *http.Client) (*StepResult, error) {
	// Resolve timeout: step > upstream > default
	timeout := resolveTimeout(step, upstream)
	return ExecuteStepWithTimeout(ctx, step, upstream, env, baseClient, timeout)
}

// resolveTimeout resolves the timeout for a step using the hierarchy:
// step timeout > upstream default timeout > global default (30s)
func resolveTimeout(step *CompiledStep, upstream *CompiledUpstream) time.Duration {
	if step.Step.Timeout != nil && *step.Step.Timeout > 0 {
		return *step.Step.Timeout
	}
	if upstream.Timeout > 0 {
		return upstream.Timeout
	}
	return DefaultStepTimeout
}

// ExecuteStepWithTimeout executes a single step by making HTTP request to upstream
// with the specified timeout.
// It evaluates expressions for the request, builds the HTTP request with proper
// context propagation, executes it, and returns the parsed result.
//
// Per CONTEXT.md:
//   - Auto-propagates headers: X-Request-ID, X-Correlation-ID, traceparent, Accept, Accept-Language
//   - Generates UUID for X-Request-ID if not present
//   - Non-2xx status is NOT an error (upstream error passthrough)
//   - Only returns error for network failure or context cancellation
//
// Auth handling:
//   - If upstream has auth strategy, wraps transport with auth RoundTripper
//   - Creates new http.Client per request (reuses underlying transport connection pool)
//   - Auth RoundTripper injects credentials (header, basic, bearer, or passthrough)
//
// Timeout handling:
//   - Creates step context with timeout derived from parent context
//   - Timeout errors are distinguished via context.DeadlineExceeded
//
// Per RESEARCH.md Pitfall 7: ALWAYS use http.NewRequestWithContext for cancellation.
func ExecuteStepWithTimeout(
	parentCtx context.Context,
	step *CompiledStep,
	upstream *CompiledUpstream,
	env map[string]interface{},
	baseClient *http.Client,
	timeout time.Duration,
) (*StepResult, error) {
	// Create step context with timeout derived from parent
	stepCtx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel() // CRITICAL: Always release resources per RESEARCH.md
	// Evaluate path expression
	path, err := evaluatePath(step.PathExpr, env)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate path: %w", err)
	}

	// Build full URL
	url := strings.TrimRight(upstream.Upstream.URL, "/") + "/" + strings.TrimLeft(path, "/")

	// Evaluate body expression if present (for POST/PUT)
	var body io.Reader
	if step.BodyExpr != nil && step.BodyExpr.Raw != "" {
		bodyContent, err := evaluateBody(step.BodyExpr, env)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate body: %w", err)
		}
		body = bytes.NewReader(bodyContent)
	}

	// Create HTTP request with step context (CRITICAL for timeout and cancellation)
	req, err := http.NewRequestWithContext(stepCtx, step.Step.Method, url, body)
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

	// Build HTTP client with auth transport if configured
	// This creates a new http.Client per request but reuses the underlying transport
	// connection pool. The auth RoundTripper is stateless (strategy state is in the Strategy).
	httpClient := baseClient
	if upstream.Auth != nil {
		transport := baseClient.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		// Wrap transport with auth RoundTripper
		authTransport := upstream.Auth.RoundTripper(transport)
		httpClient = &http.Client{
			Transport: authTransport,
			Timeout:   baseClient.Timeout,
		}
	}

	// Execute request using the HTTP client (potentially with auth)
	resp, err := httpClient.Do(req)
	if err != nil {
		// Check for timeout specifically per RESEARCH.md Pitfall 6
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("timeout after %v: %w", timeout, err)
		}
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

	// Check error rules against response status
	// Per CONTEXT.md: "Error matching rules replace the failed step's slot with the configured body value"
	// Per RESEARCH.md Pitfall 5: "Always add matched error rule to `_errors` array"
	if replacementBody, matched := matchErrorRule(resp.StatusCode, step.Step.ErrorRules); matched {
		slog.Debug("error rule matched",
			"status", resp.StatusCode,
			"step", step.Step.Name)

		return &StepResult{
			Status:           resp.StatusCode,
			Headers:          resp.Header,
			Body:             replacementBody,
			RawBody:          rawBody,
			ErrorRuleMatched: true,
		}, nil
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
// Nil results (from failed optional steps) are included as nil values.
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
	// Nil results from failed optional steps are included as nil
	// Per CONTEXT.md: "Failed optional steps return `null` for `steps.X` in expressions"
	if len(stepResults) > 0 {
		steps := make(map[string]interface{})
		for name, result := range stepResults {
			if result == nil {
				// Failed optional step - null in expressions
				steps[name] = nil
			} else {
				steps[name] = map[string]interface{}{
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
	// Authorization is included for passthrough auth - other auth strategies
	// (header, basic, oauth2) will override it via their RoundTripper
	propagatedHeaders := []string{
		"X-Request-ID",
		"X-Correlation-ID",
		"traceparent",
		"Accept",
		"Accept-Language",
		"Authorization", // For passthrough auth
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

// matchErrorRule checks if a status code matches any error rule.
// Returns the replacement body and true if matched, nil and false otherwise.
// Per CONTEXT.md: "Error matching rules replace the failed step's slot with the configured body value"
func matchErrorRule(statusCode int, rules []ErrorRule) (interface{}, bool) {
	if rules == nil {
		return nil, false
	}
	for _, rule := range rules {
		for _, status := range rule.Statuses {
			if status == statusCode {
				return rule.Body, true
			}
		}
	}
	return nil, false
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
