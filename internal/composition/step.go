package composition

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/restitch/restitch-gateway/internal/client"
)

// StepResult holds the response from an upstream service.
type StepResult struct {
	Status           int
	Headers          http.Header
	Body             interface{}
	RawBody          []byte
	ErrorRuleMatched bool
}

// ExecuteStep executes a single step with timeout hierarchy resolution.
func ExecuteStep(ctx context.Context, step *CompiledStep, upstream *CompiledUpstream, env map[string]interface{}, baseClient *http.Client) (*StepResult, error) {
	timeout := resolveTimeout(step, upstream)
	return ExecuteStepWithTimeout(ctx, step, upstream, env, baseClient, timeout)
}

func resolveTimeout(step *CompiledStep, upstream *CompiledUpstream) time.Duration {
	if step.Step.Timeout != nil && *step.Step.Timeout > 0 {
		return *step.Step.Timeout
	}
	if upstream.Timeout > 0 {
		return upstream.Timeout
	}
	return DefaultStepTimeout
}

// ExecuteStepWithTimeout executes a single step by making HTTP request to upstream.
func ExecuteStepWithTimeout(
	parentCtx context.Context,
	step *CompiledStep,
	upstream *CompiledUpstream,
	env map[string]interface{},
	baseClient *http.Client,
	timeout time.Duration,
) (*StepResult, error) {
	stepCtx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	// Evaluate path using compiled templates
	path, err := step.PathPart.EvalString(env, EscapePath)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate path: %w", err)
	}
	if step.QueryPart != nil {
		queryStr, err := step.QueryPart.EvalString(env, EscapeQuery)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate query: %w", err)
		}
		path += "?" + queryStr
	}

	fullURL := strings.TrimRight(upstream.Upstream.URL, "/") + "/" + strings.TrimLeft(path, "/")

	// Evaluate body
	var body io.Reader
	if step.BodyTmpl != nil {
		var bodyBytes []byte
		if step.BodyTmpl.IsSingle {
			val, err := step.BodyTmpl.EvalValue(env)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate body: %w", err)
			}
			bodyBytes, err = json.Marshal(val)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal body: %w", err)
			}
		} else {
			s, err := step.BodyTmpl.EvalString(env, EscapeJSON)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate body: %w", err)
			}
			bodyBytes = []byte(s)
		}
		body = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(stepCtx, step.Step.Method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Content-Type for requests with a body
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	propagateHeaders(req, env)

	// Evaluate and set custom headers from compiled templates
	for key, tmpl := range step.Headers {
		val, err := tmpl.EvalString(env, EscapeNone)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate header %s: %w", key, err)
		}
		req.Header.Set(key, val)
	}

	httpClient := baseClient
	if upstream.Auth != nil {
		transport := baseClient.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		authTransport := upstream.Auth.RoundTripper(transport)
		httpClient = &http.Client{
			Transport: authTransport,
			Timeout:   baseClient.Timeout,
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("timeout after %v: %w", timeout, err)
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer client.DrainAndClose(resp.Body)

	maxBytes := upstream.MaxResponseBytes
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if int64(len(rawBody)) > maxBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxBytes)
	}

	parsedBody, err := parseJSONBody(rawBody, resp.Header.Get("Content-Type"))
	if err != nil {
		parsedBody = string(rawBody)
	}

	if replacementBody, matched := matchErrorRule(resp.StatusCode, step.Step.ErrorRules); matched {
		slog.DebugContext(stepCtx, "error rule matched",
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

// parseJSONBody parses response body as JSON.
func parseJSONBody(body []byte, contentType string) (interface{}, error) {
	if !strings.Contains(strings.ToLower(contentType), "json") {
		return nil, fmt.Errorf("not JSON content type: %s", contentType)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("empty body")
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	return result, nil
}

// propagateHeaders auto-propagates tracing and accept headers.
func propagateHeaders(req *http.Request, env map[string]interface{}) {
	var incomingHeaders map[string]string
	if reqData, ok := env["req"].(map[string]any); ok {
		if headers, ok := reqData["headers"].(map[string]string); ok {
			incomingHeaders = headers
		}
	}

	propagatedHeaders := []string{
		"X-Request-ID",
		"X-Correlation-ID",
		"traceparent",
		"Accept",
		"Accept-Language",
		"Authorization",
	}

	for _, headerName := range propagatedHeaders {
		value := incomingHeaders[headerName]
		if value == "" {
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

	if req.Header.Get("X-Request-ID") == "" {
		req.Header.Set("X-Request-ID", uuid.New().String())
	}
}

// matchErrorRule checks if a status code matches any error rule.
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
