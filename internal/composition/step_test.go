package composition

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExecuteStep(t *testing.T) {
	t.Run("simple GET request", func(t *testing.T) {
		// Mock upstream server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/users/123" {
				t.Errorf("Expected path /users/123, got %s", r.URL.Path)
			}
			if r.Method != "GET" {
				t.Errorf("Expected GET, got %s", r.Method)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   123,
				"name": "Alice",
			})
		}))
		defer server.Close()

		// Create compiled step
		step := &CompiledStep{
			Step: &Step{
				Name:     "user",
				Method:   "GET",
				Headers:  map[string]string{},
			},
			PathExpr: &CompiledExpr{
				Raw: "/users/123",
			},
			HeaderExprs: map[string]*CompiledExpr{},
		}

		upstream := &CompiledUpstream{
			Upstream: &Upstream{URL: server.URL},
			Auth:     nil, // No auth for this test
		}

		env := buildRequestEnv(httptest.NewRequest("GET", "/", nil), nil)

		// Execute step
		result, err := ExecuteStep(context.Background(), step, upstream, env, http.DefaultClient)
		if err != nil {
			t.Fatalf("ExecuteStep failed: %v", err)
		}

		// Verify result
		if result.Status != 200 {
			t.Errorf("Expected status 200, got %d", result.Status)
		}

		body, ok := result.Body.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected body to be map[string]interface{}, got %T", result.Body)
		}

		if body["name"] != "Alice" {
			t.Errorf("Expected name Alice, got %v", body["name"])
		}
	})

	t.Run("path with expression evaluation", func(t *testing.T) {
		// Mock upstream server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/users/456" {
				t.Errorf("Expected path /users/456, got %s", r.URL.Path)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 456,
			})
		}))
		defer server.Close()

		// Create environment with query parameter
		incomingReq := httptest.NewRequest("GET", "/?user_id=456", nil)
		env := buildRequestEnv(incomingReq, nil)

		// Compile path expression
		pathExpr, err := CompileExpression("req.query.user_id", BuildBaseEnvironment(nil))
		if err != nil {
			t.Fatalf("Failed to compile path expression: %v", err)
		}
		pathExpr.Raw = "/users/{{ req.query.user_id }}"

		step := &CompiledStep{
			Step: &Step{
				Name:     "user",
				Method:   "GET",
				Headers:  map[string]string{},
			},
			PathExpr: pathExpr,
			HeaderExprs: map[string]*CompiledExpr{},
		}

		upstream := &CompiledUpstream{
			Upstream: &Upstream{URL: server.URL},
			Auth:     nil, // No auth for this test
		}

		// Execute step
		result, err := ExecuteStep(context.Background(), step, upstream, env, http.DefaultClient)
		if err != nil {
			t.Fatalf("ExecuteStep failed: %v", err)
		}

		if result.Status != 200 {
			t.Errorf("Expected status 200, got %d", result.Status)
		}
	})

	t.Run("header propagation", func(t *testing.T) {
		// Mock upstream server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify X-Request-ID was propagated
			requestID := r.Header.Get("X-Request-ID")
			if requestID != "test-request-123" {
				t.Errorf("Expected X-Request-ID test-request-123, got %s", requestID)
			}

			// Verify traceparent was propagated
			traceparent := r.Header.Get("traceparent")
			if traceparent != "00-trace-123" {
				t.Errorf("Expected traceparent 00-trace-123, got %s", traceparent)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}))
		defer server.Close()

		// Create incoming request with headers
		incomingReq := httptest.NewRequest("GET", "/", nil)
		incomingReq.Header.Set("X-Request-ID", "test-request-123")
		incomingReq.Header.Set("X-Correlation-ID", "corr-456")
		incomingReq.Header.Set("traceparent", "00-trace-123")

		env := buildRequestEnv(incomingReq, nil)

		step := &CompiledStep{
			Step: &Step{
				Name:     "test",
				Method:   "GET",
				Headers:  map[string]string{},
			},
			PathExpr: &CompiledExpr{
				Raw: "/test",
			},
			HeaderExprs: map[string]*CompiledExpr{},
		}

		upstream := &CompiledUpstream{
			Upstream: &Upstream{URL: server.URL},
			Auth:     nil, // No auth for this test
		}

		// Execute step
		_, err := ExecuteStep(context.Background(), step, upstream, env, http.DefaultClient)
		if err != nil {
			t.Fatalf("ExecuteStep failed: %v", err)
		}
	})

	t.Run("X-Request-ID generation when missing", func(t *testing.T) {
		// Mock upstream server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify X-Request-ID was generated (should be a UUID)
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				t.Error("Expected X-Request-ID to be generated")
			}
			if len(requestID) < 36 { // UUID length
				t.Errorf("Expected UUID format for X-Request-ID, got %s", requestID)
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}))
		defer server.Close()

		// Create incoming request WITHOUT X-Request-ID
		incomingReq := httptest.NewRequest("GET", "/", nil)
		env := buildRequestEnv(incomingReq, nil)

		step := &CompiledStep{
			Step: &Step{
				Name:     "test",
				Method:   "GET",
				Headers:  map[string]string{},
			},
			PathExpr: &CompiledExpr{
				Raw: "/test",
			},
			HeaderExprs: map[string]*CompiledExpr{},
		}

		upstream := &CompiledUpstream{
			Upstream: &Upstream{URL: server.URL},
			Auth:     nil, // No auth for this test
		}

		// Execute step
		_, err := ExecuteStep(context.Background(), step, upstream, env, http.DefaultClient)
		if err != nil {
			t.Fatalf("ExecuteStep failed: %v", err)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		// Mock upstream server with delay
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}))
		defer server.Close()

		step := &CompiledStep{
			Step: &Step{
				Name:     "test",
				Method:   "GET",
				Headers:  map[string]string{},
			},
			PathExpr: &CompiledExpr{
				Raw: "/test",
			},
			HeaderExprs: map[string]*CompiledExpr{},
		}

		upstream := &CompiledUpstream{
			Upstream: &Upstream{URL: server.URL},
			Auth:     nil, // No auth for this test
		}

		env := buildRequestEnv(httptest.NewRequest("GET", "/", nil), nil)

		// Create context that will be cancelled
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Execute step - should fail with context cancelled error
		_, err := ExecuteStep(ctx, step, upstream, env, http.DefaultClient)
		if err == nil {
			t.Error("Expected error from cancelled context")
		}
		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("Expected context canceled error, got: %v", err)
		}
	})

	t.Run("upstream error passthrough", func(t *testing.T) {
		// Mock upstream server returning error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "user not found",
			})
		}))
		defer server.Close()

		step := &CompiledStep{
			Step: &Step{
				Name:     "user",
				Method:   "GET",
				Headers:  map[string]string{},
			},
			PathExpr: &CompiledExpr{
				Raw: "/users/999",
			},
			HeaderExprs: map[string]*CompiledExpr{},
		}

		upstream := &CompiledUpstream{
			Upstream: &Upstream{URL: server.URL},
			Auth:     nil, // No auth for this test
		}

		env := buildRequestEnv(httptest.NewRequest("GET", "/", nil), nil)

		// Execute step - should NOT return error (passthrough upstream status)
		result, err := ExecuteStep(context.Background(), step, upstream, env, http.DefaultClient)
		if err != nil {
			t.Fatalf("ExecuteStep should not return error for upstream 404: %v", err)
		}

		// Verify status code is passed through
		if result.Status != 404 {
			t.Errorf("Expected status 404, got %d", result.Status)
		}

		// Verify error body is passed through
		body, ok := result.Body.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected body to be map, got %T", result.Body)
		}
		if body["error"] != "user not found" {
			t.Errorf("Expected error message, got %v", body["error"])
		}
	})
}

func TestBuildRequestEnv(t *testing.T) {
	t.Run("parses request data", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test?user_id=123&status=active", nil)
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("X-Request-ID", "req-123")

		env := buildRequestEnv(req, nil)

		// Verify request data
		reqData, ok := env["req"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected req to be map, got %T", env["req"])
		}

		if reqData["path"] != "/api/test" {
			t.Errorf("Expected path /api/test, got %v", reqData["path"])
		}

		query, ok := reqData["query"].(map[string]string)
		if !ok {
			t.Fatalf("Expected query to be map[string]string, got %T", reqData["query"])
		}
		if query["user_id"] != "123" {
			t.Errorf("Expected user_id 123, got %v", query["user_id"])
		}
		if query["status"] != "active" {
			t.Errorf("Expected status active, got %v", query["status"])
		}

		headers, ok := reqData["headers"].(map[string]string)
		if !ok {
			t.Fatalf("Expected headers to be map[string]string, got %T", reqData["headers"])
		}
		if headers["Authorization"] != "Bearer token" {
			t.Errorf("Expected Authorization header, got %v", headers["Authorization"])
		}
	})

	t.Run("includes step results", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)

		stepResults := map[string]*StepResult{
			"user": {
				Status: 200,
				Headers: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: map[string]interface{}{
					"id":   123,
					"name": "Alice",
				},
			},
			"orders": {
				Status: 200,
				Headers: http.Header{},
				Body: []interface{}{
					map[string]interface{}{"id": 1},
					map[string]interface{}{"id": 2},
				},
			},
		}

		env := buildRequestEnv(req, stepResults)

		// Verify steps are included
		steps, ok := env["steps"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected steps to be map, got %T", env["steps"])
		}

		user, ok := steps["user"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected user to be map, got %T", steps["user"])
		}
		if user["status"] != 200 {
			t.Errorf("Expected user status 200, got %v", user["status"])
		}

		userBody, ok := user["body"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected user body to be map, got %T", user["body"])
		}
		if userBody["name"] != "Alice" {
			t.Errorf("Expected user name Alice, got %v", userBody["name"])
		}
	})
}

func TestParseJSONBody(t *testing.T) {
	t.Run("parses JSON object", func(t *testing.T) {
		body := []byte(`{"id": 123, "name": "Alice"}`)
		result, err := parseJSONBody(body, "application/json")
		if err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		obj, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected map, got %T", result)
		}
		if obj["name"] != "Alice" {
			t.Errorf("Expected name Alice, got %v", obj["name"])
		}
	})

	t.Run("parses JSON array", func(t *testing.T) {
		body := []byte(`[{"id": 1}, {"id": 2}]`)
		result, err := parseJSONBody(body, "application/json")
		if err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		arr, ok := result.([]interface{})
		if !ok {
			t.Fatalf("Expected array, got %T", result)
		}
		if len(arr) != 2 {
			t.Errorf("Expected 2 items, got %d", len(arr))
		}
	})

	t.Run("handles non-JSON content type", func(t *testing.T) {
		body := []byte(`{"id": 123}`)
		_, err := parseJSONBody(body, "text/html")
		if err == nil {
			t.Error("Expected error for non-JSON content type")
		}
	})

	t.Run("handles invalid JSON", func(t *testing.T) {
		body := []byte(`{invalid json}`)
		_, err := parseJSONBody(body, "application/json")
		if err == nil {
			t.Error("Expected error for invalid JSON")
		}
	})
}

func TestInterpolateTemplate(t *testing.T) {
	t.Run("simple interpolation", func(t *testing.T) {
		env := map[string]interface{}{
			"req": map[string]interface{}{
				"query": map[string]string{
					"id": "123",
				},
			},
		}

		result, err := interpolateTemplate("/users/{{ req.query.id }}", env)
		if err != nil {
			t.Fatalf("Interpolation failed: %v", err)
		}

		if result != "/users/123" {
			t.Errorf("Expected /users/123, got %s", result)
		}
	})

	t.Run("multiple interpolations", func(t *testing.T) {
		env := map[string]interface{}{
			"req": map[string]interface{}{
				"query": map[string]string{
					"user_id":  "123",
					"order_id": "456",
				},
			},
		}

		result, err := interpolateTemplate("/users/{{ req.query.user_id }}/orders/{{ req.query.order_id }}", env)
		if err != nil {
			t.Fatalf("Interpolation failed: %v", err)
		}

		if result != "/users/123/orders/456" {
			t.Errorf("Expected /users/123/orders/456, got %s", result)
		}
	})

	t.Run("interpolate with step results", func(t *testing.T) {
		env := map[string]interface{}{
			"steps": map[string]interface{}{
				"user": map[string]interface{}{
					"status": 200,
					"body": map[string]interface{}{
						"id":   float64(123), // JSON unmarshals numbers as float64
						"name": "Alice",
					},
				},
			},
		}

		result, err := interpolateTemplate("/orders?user_id={{ steps.user.body.id }}", env)
		if err != nil {
			t.Fatalf("Interpolation failed: %v", err)
		}

		if result != "/orders?user_id=123" {
			t.Errorf("Expected /orders?user_id=123, got %s", result)
		}
	})
}
