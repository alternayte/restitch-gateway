package composition

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	upstreampkg "github.com/alternayte/restitch-gateway/internal/upstream"
)

func mustCompileTemplate(t *testing.T, raw string) *Template {
	t.Helper()
	env := BuildBaseEnvironment(nil)
	tmpl, err := CompileTemplate(raw, env)
	if err != nil {
		t.Fatalf("CompileTemplate(%q) failed: %v", raw, err)
	}
	return tmpl
}

func TestExecuteStep(t *testing.T) {
	t.Run("simple GET request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/users/123" {
				t.Errorf("Expected path /users/123, got %s", r.URL.Path)
			}
			if r.Method != "GET" {
				t.Errorf("Expected GET, got %s", r.Method)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   123,
				"name": "Alice",
			})
		}))
		defer server.Close()

		step := &CompiledStep{
			Step: &Step{
				Name:    "user",
				Method:  "GET",
				Headers: map[string]string{},
			},
			PathPart: mustCompileTemplate(t, "/users/123"),
			Headers:  map[string]*Template{},
		}

		upstream := &upstreampkg.Upstream{MaxResponseBytes: 10 * 1024 * 1024, Client: http.DefaultClient,
			BaseURL: server.URL,
		}

		env := buildRequestEnv(context.Background(), NewRequestData(httptest.NewRequest("GET", "/", nil), nil, nil), nil)

		result, err := ExecuteStep(context.Background(), step, upstream, env)
		if err != nil {
			t.Fatalf("ExecuteStep failed: %v", err)
		}

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
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/users/456" {
				t.Errorf("Expected path /users/456, got %s", r.URL.Path)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 456,
			})
		}))
		defer server.Close()

		incomingReq := httptest.NewRequest("GET", "/?user_id=456", nil)
		env := buildRequestEnv(context.Background(), NewRequestData(incomingReq, nil, nil), nil)

		pathTmpl, err := CompileTemplate("/users/{{ req.query.user_id }}", env)
		if err != nil {
			t.Fatalf("Failed to compile path template: %v", err)
		}

		step := &CompiledStep{
			Step: &Step{
				Name:    "user",
				Method:  "GET",
				Headers: map[string]string{},
			},
			PathPart: pathTmpl,
			Headers:  map[string]*Template{},
		}

		upstream := &upstreampkg.Upstream{MaxResponseBytes: 10 * 1024 * 1024, Client: http.DefaultClient,
			BaseURL: server.URL,
		}

		result, err := ExecuteStep(context.Background(), step, upstream, env)
		if err != nil {
			t.Fatalf("ExecuteStep failed: %v", err)
		}

		if result.Status != 200 {
			t.Errorf("Expected status 200, got %d", result.Status)
		}
	})

	t.Run("header propagation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-ID")
			if requestID != "test-request-123" {
				t.Errorf("Expected X-Request-ID test-request-123, got %s", requestID)
			}

			traceparent := r.Header.Get("traceparent")
			if traceparent != "00-trace-123" {
				t.Errorf("Expected traceparent 00-trace-123, got %s", traceparent)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		}))
		defer server.Close()

		incomingReq := httptest.NewRequest("GET", "/", nil)
		incomingReq.Header.Set("X-Request-ID", "test-request-123")
		incomingReq.Header.Set("X-Correlation-ID", "corr-456")
		incomingReq.Header.Set("traceparent", "00-trace-123")

		env := buildRequestEnv(context.Background(), NewRequestData(incomingReq, nil, nil), nil)

		step := &CompiledStep{
			Step: &Step{
				Name:    "test",
				Method:  "GET",
				Headers: map[string]string{},
			},
			PathPart: mustCompileTemplate(t, "/test"),
			Headers:  map[string]*Template{},
		}

		upstream := &upstreampkg.Upstream{MaxResponseBytes: 10 * 1024 * 1024, Client: http.DefaultClient,
			BaseURL: server.URL,
		}

		_, err := ExecuteStep(context.Background(), step, upstream, env)
		if err != nil {
			t.Fatalf("ExecuteStep failed: %v", err)
		}
	})

	t.Run("X-Request-ID generation when missing", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				t.Error("Expected X-Request-ID to be generated")
			}
			if len(requestID) < 36 {
				t.Errorf("Expected UUID format for X-Request-ID, got %s", requestID)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		}))
		defer server.Close()

		incomingReq := httptest.NewRequest("GET", "/", nil)
		env := buildRequestEnv(context.Background(), NewRequestData(incomingReq, nil, nil), nil)

		step := &CompiledStep{
			Step: &Step{
				Name:    "test",
				Method:  "GET",
				Headers: map[string]string{},
			},
			PathPart: mustCompileTemplate(t, "/test"),
			Headers:  map[string]*Template{},
		}

		upstream := &upstreampkg.Upstream{MaxResponseBytes: 10 * 1024 * 1024, Client: http.DefaultClient,
			BaseURL: server.URL,
		}

		_, err := ExecuteStep(context.Background(), step, upstream, env)
		if err != nil {
			t.Fatalf("ExecuteStep failed: %v", err)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		}))
		defer server.Close()

		step := &CompiledStep{
			Step: &Step{
				Name:    "test",
				Method:  "GET",
				Headers: map[string]string{},
			},
			PathPart: mustCompileTemplate(t, "/test"),
			Headers:  map[string]*Template{},
		}

		upstream := &upstreampkg.Upstream{MaxResponseBytes: 10 * 1024 * 1024, Client: http.DefaultClient,
			BaseURL: server.URL,
		}

		env := buildRequestEnv(context.Background(), NewRequestData(httptest.NewRequest("GET", "/", nil), nil, nil), nil)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := ExecuteStep(ctx, step, upstream, env)
		if err == nil {
			t.Error("Expected error from cancelled context")
		}
		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("Expected context canceled error, got: %v", err)
		}
	})

	t.Run("upstream error passthrough", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "user not found",
			})
		}))
		defer server.Close()

		step := &CompiledStep{
			Step: &Step{
				Name:    "user",
				Method:  "GET",
				Headers: map[string]string{},
			},
			PathPart: mustCompileTemplate(t, "/users/999"),
			Headers:  map[string]*Template{},
		}

		upstream := &upstreampkg.Upstream{MaxResponseBytes: 10 * 1024 * 1024, Client: http.DefaultClient,
			BaseURL: server.URL,
		}

		env := buildRequestEnv(context.Background(), NewRequestData(httptest.NewRequest("GET", "/", nil), nil, nil), nil)

		result, err := ExecuteStep(context.Background(), step, upstream, env)
		if err != nil {
			t.Fatalf("ExecuteStep should not return error for upstream 404: %v", err)
		}

		if result.Status != 404 {
			t.Errorf("Expected status 404, got %d", result.Status)
		}

		body, ok := result.Body.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected body to be map, got %T", result.Body)
		}
		if body["error"] != "user not found" {
			t.Errorf("Expected error message, got %v", body["error"])
		}
	})

	t.Run("response size cap", func(t *testing.T) {
		bigBody := strings.Repeat("x", 200)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":"` + bigBody + `"}`))
		}))
		defer server.Close()

		pathTmpl, _ := CompileTemplate("/test", map[string]any{})
		step := &CompiledStep{
			Step:     &Step{Name: "big", Method: "GET", Headers: map[string]string{}},
			PathPart: pathTmpl,
			Headers:  map[string]*Template{},
		}

		upstream := &upstreampkg.Upstream{
			BaseURL:          server.URL,
			MaxResponseBytes: 100,
			Client:           http.DefaultClient,
		}

		env := buildRequestEnv(context.Background(), NewRequestData(httptest.NewRequest("GET", "/", nil), nil, nil), nil)
		_, err := ExecuteStep(context.Background(), step, upstream, env)
		if err == nil {
			t.Fatal("expected error for oversized response")
		}
		if !strings.Contains(err.Error(), "exceeds 100 bytes") {
			t.Errorf("expected size cap error, got: %v", err)
		}
	})

	t.Run("response size cap optional step degrades", func(t *testing.T) {
		bigBody := strings.Repeat("x", 200)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":"` + bigBody + `"}`))
		}))
		defer server.Close()

		pathTmpl, _ := CompileTemplate("/test", map[string]any{})
		step := &CompiledStep{
			Step:     &Step{Name: "big", Method: "GET", Headers: map[string]string{}},
			PathPart: pathTmpl,
			Headers:  map[string]*Template{},
			Optional: true,
		}

		upstream := &upstreampkg.Upstream{
			BaseURL:          server.URL,
			MaxResponseBytes: 100,
			Client:           http.DefaultClient,
		}

		env := buildRequestEnv(context.Background(), NewRequestData(httptest.NewRequest("GET", "/", nil), nil, nil), nil)
		_, err := ExecuteStep(context.Background(), step, upstream, env)
		if err == nil {
			t.Fatal("expected error for oversized response")
		}
	})
}

func TestBuildRequestEnv(t *testing.T) {
	t.Run("parses request data", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/test?user_id=123&status=active", nil)
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("X-Request-ID", "req-123")

		env := buildRequestEnv(context.Background(), NewRequestData(req, nil, nil), nil)

		reqData, ok := env["req"].(map[string]any)
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
					"id":   float64(1),
					"name": "Alice",
				},
			},
			"orders": {
				Status:  200,
				Headers: http.Header{},
				Body: []interface{}{
					map[string]interface{}{"id": float64(1)},
					map[string]interface{}{"id": float64(2)},
				},
			},
		}

		env := buildRequestEnv(context.Background(), NewRequestData(req, nil, nil), stepResults)

		steps, ok := env["steps"].(map[string]any)
		if !ok {
			t.Fatalf("Expected steps to be map, got %T", env["steps"])
		}

		user, ok := steps["user"].(map[string]any)
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

func TestTemplate_Interpolation(t *testing.T) {
	t.Run("simple interpolation", func(t *testing.T) {
		env := map[string]any{
			"req": map[string]any{
				"query": map[string]string{
					"id": "123",
				},
			},
		}

		tmpl, err := CompileTemplate("/users/{{ req.query.id }}", env)
		if err != nil {
			t.Fatalf("CompileTemplate failed: %v", err)
		}

		result, err := tmpl.EvalString(env, EscapeNone)
		if err != nil {
			t.Fatalf("EvalString failed: %v", err)
		}

		if result != "/users/123" {
			t.Errorf("Expected /users/123, got %s", result)
		}
	})

	t.Run("multiple interpolations", func(t *testing.T) {
		env := map[string]any{
			"req": map[string]any{
				"query": map[string]string{
					"user_id":  "123",
					"order_id": "456",
				},
			},
		}

		tmpl, err := CompileTemplate("/users/{{ req.query.user_id }}/orders/{{ req.query.order_id }}", env)
		if err != nil {
			t.Fatalf("CompileTemplate failed: %v", err)
		}

		result, err := tmpl.EvalString(env, EscapeNone)
		if err != nil {
			t.Fatalf("EvalString failed: %v", err)
		}

		if result != "/users/123/orders/456" {
			t.Errorf("Expected /users/123/orders/456, got %s", result)
		}
	})

	t.Run("interpolate with step results", func(t *testing.T) {
		env := map[string]any{
			"steps": map[string]any{
				"user": map[string]any{
					"status": 200,
					"body": map[string]any{
						"id":   float64(123),
						"name": "Alice",
					},
				},
			},
		}

		tmpl, err := CompileTemplate("/orders?user_id={{ steps.user.body.id }}", env)
		if err != nil {
			t.Fatalf("CompileTemplate failed: %v", err)
		}

		result, err := tmpl.EvalString(env, EscapeNone)
		if err != nil {
			t.Fatalf("EvalString failed: %v", err)
		}

		if result != "/orders?user_id=123" {
			t.Errorf("Expected /orders?user_id=123, got %s", result)
		}
	})
}
