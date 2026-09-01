package composition

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/restitch/restitch-gateway/internal/server"
)

func TestHandler_ServeHTTP(t *testing.T) {
	// Create a mock upstream server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return different responses based on path
		switch r.URL.Path {
		case "/users/1":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   1,
				"name": "Alice",
			})
		case "/posts":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]interface{}{
				map[string]interface{}{"id": 101, "title": "Post 1"},
				map[string]interface{}{"id": 102, "title": "Post 2"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	// Create test configuration
	configYAML := `
upstreams:
  test:
    url: ` + mockServer.URL + `

compositions:
  user-posts:
    path: /api/user-posts
    method: GET
    steps:
      - name: user
        upstream: test
        path: /users/1
      - name: posts
        upstream: test
        path: /posts
    response:
      status: 200
      body:
        user: "{{ steps.user.body }}"
        posts: "{{ steps.posts.body }}"
`

	cfg, err := ParseConfig([]byte(configYAML))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	compiledCfg, err := CompileConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CompileConfig failed: %v", err)
	}

	// Create handler
	handler := NewHandler(compiledCfg, nil)

	// Create router and register routes
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   bool // Just verify body exists
	}{
		{
			name:       "valid composition request",
			method:     "GET",
			path:       "/api/user-posts",
			wantStatus: http.StatusOK,
			wantBody:   true,
		},
		{
			name:       "unknown path",
			method:     "GET",
			path:       "/api/unknown",
			wantStatus: http.StatusNotFound,
			wantBody:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("ServeHTTP() status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantBody && w.Body.Len() == 0 {
				t.Errorf("ServeHTTP() expected response body but got none")
			}
		})
	}
}

func TestHandler_PathParams(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   r.URL.Path[len("/users/"):],
			"name": "user-" + r.URL.Path[len("/users/"):],
		})
	}))
	defer mockServer.Close()

	configYAML := `
upstreams:
  mock:
    url: ` + mockServer.URL + `

compositions:
  user:
    path: "/api/users/{id}"
    method: GET
    steps:
      - name: u
        upstream: mock
        path: "/users/{{ req.params.id }}"
    response:
      status: 200
      body:
        user: "{{ steps.u.body }}"
`

	cfg, err := ParseConfig([]byte(configYAML))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	compiledCfg, err := CompileConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CompileConfig: %v", err)
	}

	handler := NewHandler(compiledCfg, nil)
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	req := httptest.NewRequest("GET", "/api/users/42", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	user, ok := body["user"].(map[string]any)
	if !ok {
		t.Fatalf("user field not a map: %v", body["user"])
	}
	if user["id"] != "42" {
		t.Errorf("user.id = %v, want 42", user["id"])
	}
}

func TestHandler_PassthroughAuthMissing(t *testing.T) {
	// Create a mock upstream server that should never be called
	// (request should fail before reaching upstream due to missing auth)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should not be called when passthrough auth is missing")
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	// Create test configuration with passthrough auth
	configYAML := `
upstreams:
  test:
    url: ` + mockServer.URL + `
    auth:
      passthrough: {}

compositions:
  protected:
    path: /api/protected
    method: GET
    steps:
      - name: data
        upstream: test
        path: /data
    response:
      status: 200
      body:
        data: "{{ steps.data.body }}"
`

	cfg, err := ParseConfig([]byte(configYAML))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	compiledCfg, err := CompileConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CompileConfig failed: %v", err)
	}

	// Create handler
	handler := NewHandler(compiledCfg, nil)

	// Create router and register routes
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	// Request without Authorization header
	req := httptest.NewRequest("GET", "/api/protected", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 401 Unauthorized
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	// Should have WWW-Authenticate header
	wwwAuth := w.Header().Get("WWW-Authenticate")
	if wwwAuth != "Bearer" {
		t.Errorf("expected WWW-Authenticate: Bearer, got %q", wwwAuth)
	}

	// Should have error body
	var errorResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errorResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errorResp["error"] != "authorization header required" {
		t.Errorf("unexpected error message: %s", errorResp["error"])
	}
}

func TestHandler_PassthroughAuthPresent(t *testing.T) {
	// Create a mock upstream server that verifies Authorization header
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-token-123" {
			t.Errorf("expected Authorization: Bearer test-token-123, got %q", authHeader)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	defer mockServer.Close()

	// Create test configuration with passthrough auth
	configYAML := `
upstreams:
  test:
    url: ` + mockServer.URL + `
    auth:
      passthrough: {}

compositions:
  protected:
    path: /api/protected
    method: GET
    steps:
      - name: data
        upstream: test
        path: /data
    response:
      status: 200
      body:
        data: "{{ steps.data.body }}"
`

	cfg, err := ParseConfig([]byte(configYAML))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	compiledCfg, err := CompileConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CompileConfig failed: %v", err)
	}

	// Create handler
	handler := NewHandler(compiledCfg, nil)

	// Create router and register routes
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	// Request WITH Authorization header
	req := httptest.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer test-token-123")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 200 OK
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHandler_HeaderAuth(t *testing.T) {
	// Create a mock upstream server that verifies custom header
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey != "test-api-key-456" {
			t.Errorf("expected X-API-Key: test-api-key-456, got %q", apiKey)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	defer mockServer.Close()

	// Set env var for auth
	t.Setenv("TEST_API_KEY", "test-api-key-456")

	// Create test configuration with header auth
	configYAML := `
upstreams:
  test:
    url: ` + mockServer.URL + `
    auth:
      header:
        name: "X-API-Key"
        value: "test-api-key-456"

compositions:
  api:
    path: /api/data
    method: GET
    steps:
      - name: data
        upstream: test
        path: /data
    response:
      status: 200
      body:
        data: "{{ steps.data.body }}"
`

	cfg, err := ParseConfig([]byte(configYAML))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	compiledCfg, err := CompileConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CompileConfig failed: %v", err)
	}

	// Create handler
	handler := NewHandler(compiledCfg, nil)

	// Create router and register routes
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	// Request (no client auth needed for header auth)
	req := httptest.NewRequest("GET", "/api/data", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 200 OK
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestHandler_PartialResponse_OptionalStepInTemplate is the permanent D1
// regression test. An optional step to a dead upstream fails; the response
// template references {{ steps.loyalty.body.points }} — must return 200
// with null for that field, X-Restitch-Complete: false, and _errors.
func TestHandler_PartialResponse_OptionalStepInTemplate(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   1,
			"name": "Alice",
		})
	}))
	defer mockServer.Close()

	configYAML := `
upstreams:
  mock:
    url: ` + mockServer.URL + `
  dead:
    url: "http://127.0.0.1:1"

compositions:
  partial:
    path: /p
    steps:
      - name: user
        upstream: mock
        path: /users/1
      - name: loyalty
        upstream: dead
        path: /x
        optional: true
    response:
      status: 200
      body:
        user: "{{ steps.user.body }}"
        points: "{{ steps.loyalty.body.points }}"
`

	cfg, err := ParseConfig([]byte(configYAML))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	compiledCfg, err := CompileConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CompileConfig: %v", err)
	}

	handler := NewHandler(compiledCfg, nil)
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	req := httptest.NewRequest("GET", "/p", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if w.Header().Get("X-Restitch-Complete") != "false" {
		t.Errorf("X-Restitch-Complete = %q, want false", w.Header().Get("X-Restitch-Complete"))
	}

	if w.Header().Get("X-Partial-Response") != "true" {
		t.Errorf("X-Partial-Response = %q, want true", w.Header().Get("X-Partial-Response"))
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body["user"] == nil {
		t.Error("user field should be populated")
	}

	if body["points"] != nil {
		t.Errorf("points should be null for failed optional step, got %v", body["points"])
	}

	errors, ok := body["_errors"].([]any)
	if !ok || len(errors) == 0 {
		t.Fatalf("expected _errors array, got %v", body["_errors"])
	}

	foundLoyalty := false
	for _, e := range errors {
		eMap, ok := e.(map[string]any)
		if ok && eMap["step"] == "loyalty" {
			foundLoyalty = true
		}
	}
	if !foundLoyalty {
		t.Errorf("_errors should contain loyalty step, got %v", errors)
	}
}

func TestHandler_ErrorTaxonomy_DeadUpstream502(t *testing.T) {
	configYAML := `
upstreams:
  dead:
    url: "http://127.0.0.1:1"
compositions:
  fail:
    path: /fail
    steps:
      - name: s
        upstream: dead
        path: /x
    response:
      status: 200
      body: {}
`
	cfg, _ := ParseConfig([]byte(configYAML))
	compiled, _ := CompileConfig(context.Background(), cfg)
	handler := NewHandler(compiled, nil)
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/fail", nil))

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}

	var body map[string]any
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "upstream error" {
		t.Errorf("error = %v, want 'upstream error'", body["error"])
	}
	if body["step"] != "s" {
		t.Errorf("step = %v, want 's'", body["step"])
	}
}

func TestHandler_ErrorTaxonomy_Timeout504(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-make(chan struct{}):
		}
	}))
	defer slow.Close()

	configYAML := `
upstreams:
  slow:
    url: ` + slow.URL + `
compositions:
  timeout:
    path: /timeout
    steps:
      - name: s
        upstream: slow
        path: /x
        timeout: 50ms
    response:
      status: 200
      body: {}
`
	cfg, _ := ParseConfig([]byte(configYAML))
	compiled, _ := CompileConfig(context.Background(), cfg)
	handler := NewHandler(compiled, nil)
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/timeout", nil))

	if w.Code != http.StatusGatewayTimeout {
		t.Errorf("expected 504, got %d", w.Code)
	}

	var body map[string]any
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "upstream timeout" {
		t.Errorf("error = %v, want 'upstream timeout'", body["error"])
	}
	if body["step"] != "s" {
		t.Errorf("step = %v, want 's'", body["step"])
	}
}

func TestHandler_ErrorTaxonomy_TemplateError500(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("not json"))
	}))
	defer mock.Close()

	// Template tries to access .body.id on a non-JSON response, which
	// is a string — accessing .id on a string causes a template eval error
	// with no failed dependency, so it's a real template bug → 500.
	configYAML := `
upstreams:
  mock:
    url: ` + mock.URL + `
compositions:
  buggy:
    path: /buggy
    steps:
      - name: s
        upstream: mock
        path: /x
    response:
      status: 200
      body:
        val: "{{ steps.s.body.id }}"
`
	cfg, _ := ParseConfig([]byte(configYAML))
	compiled, _ := CompileConfig(context.Background(), cfg)
	handler := NewHandler(compiled, nil)
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/buggy", nil))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}

	var body map[string]any
	_ = json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "internal error" {
		t.Errorf("error = %v, want 'internal error' (must not leak internals)", body["error"])
	}
}

func TestHandler_RateLimit_PerComposition(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer mockServer.Close()

	configYAML := `
upstreams:
  mock:
    url: ` + mockServer.URL + `

compositions:
  limited:
    path: /limited
    method: GET
    rate_limit:
      requests_per_second: 1
      burst: 2
      key: ip
    steps:
      - name: s
        upstream: mock
        path: /ok
    response:
      status: 200
      body:
        data: "{{ steps.s.body }}"
`

	cfg, err := ParseConfig([]byte(configYAML))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	compiled, err := CompileConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CompileConfig: %v", err)
	}

	handler := NewHandler(compiled, nil)
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	// First two requests should succeed (burst=2)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/limited", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// Third request should be rate limited
	req := httptest.NewRequest("GET", "/limited", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") != "1" {
		t.Errorf("Retry-After = %q, want \"1\"", w.Header().Get("Retry-After"))
	}
}

func TestHandler_MaxRequestBytes(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer mockServer.Close()

	configYAML := `
upstreams:
  mock:
    url: ` + mockServer.URL + `

compositions:
  tiny:
    path: /tiny
    method: POST
    max_request_bytes: 16
    steps:
      - name: s
        upstream: mock
        path: /ok
    response:
      status: 200
      body:
        data: "{{ steps.s.body }}"
`

	cfg, err := ParseConfig([]byte(configYAML))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	compiled, err := CompileConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CompileConfig: %v", err)
	}

	handler := NewHandler(compiled, nil)
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	// Small body should succeed
	small := strings.NewReader(`{"a":1}`)
	req := httptest.NewRequest("POST", "/tiny", small)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("small body: expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	// Large body should get 413
	large := strings.NewReader(`{"data":"` + strings.Repeat("x", 100) + `"}`)
	req2 := httptest.NewRequest("POST", "/tiny", large)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large body: expected 413, got %d; body: %s", w2.Code, w2.Body.String())
	}

	var errBody map[string]string
	_ = json.NewDecoder(w2.Body).Decode(&errBody)
	if errBody["error"] != "request body too large" {
		t.Errorf("error = %q, want \"request body too large\"", errBody["error"])
	}
}

func TestHandler_RequestSchemaValidation(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer mockServer.Close()

	configYAML := `
upstreams:
  mock:
    url: ` + mockServer.URL + `

compositions:
  validated:
    path: /validated
    method: POST
    request_schema:
      type: object
      required:
        - name
      properties:
        name:
          type: string
        age:
          type: integer
    steps:
      - name: s
        upstream: mock
        path: /ok
    response:
      status: 200
      body:
        data: "{{ steps.s.body }}"
`

	cfg, err := ParseConfig([]byte(configYAML))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	compiled, err := CompileConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CompileConfig: %v", err)
	}

	handler := NewHandler(compiled, nil)
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	// Valid body
	validBody := strings.NewReader(`{"name":"Alice","age":30}`)
	req := httptest.NewRequest("POST", "/validated", validBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("valid body: expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	// Invalid body (missing required field)
	invalidBody := strings.NewReader(`{"age":30}`)
	req2 := httptest.NewRequest("POST", "/validated", invalidBody)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("invalid body: expected 400, got %d; body: %s", w2.Code, w2.Body.String())
	}

	var errResp map[string]any
	_ = json.NewDecoder(w2.Body).Decode(&errResp)
	if errResp["error"] != "request validation failed" {
		t.Errorf("error = %v, want \"request validation failed\"", errResp["error"])
	}
	details, ok := errResp["details"].([]any)
	if !ok || len(details) == 0 {
		t.Errorf("expected details array with entries, got %v", errResp["details"])
	}

	// Wrong type body (age is string instead of integer)
	wrongType := strings.NewReader(`{"name":"Alice","age":"not a number"}`)
	req3 := httptest.NewRequest("POST", "/validated", wrongType)
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("wrong type: expected 400, got %d; body: %s", w3.Code, w3.Body.String())
	}

	// Malformed JSON must be rejected with 400, not skipped past schema
	// validation (finding H11).
	malformed := strings.NewReader(`{"name": not-json}`)
	req4 := httptest.NewRequest("POST", "/validated", malformed)
	req4.Header.Set("Content-Type", "application/json")
	w4 := httptest.NewRecorder()
	router.ServeHTTP(w4, req4)
	if w4.Code != http.StatusBadRequest {
		t.Fatalf("malformed JSON: expected 400, got %d; body: %s", w4.Code, w4.Body.String())
	}
	var malformedResp map[string]string
	_ = json.NewDecoder(w4.Body).Decode(&malformedResp)
	if malformedResp["error"] != "invalid JSON body" {
		t.Errorf("error = %q, want \"invalid JSON body\"", malformedResp["error"])
	}
}

func TestHandler_ErrorTaxonomy_NoInternalLeak(t *testing.T) {
	configYAML := `
upstreams:
  dead:
    url: "http://127.0.0.1:1"
compositions:
  fail:
    path: /fail
    steps:
      - name: s
        upstream: dead
        path: /x
    response:
      status: 200
      body: {}
`
	cfg, _ := ParseConfig([]byte(configYAML))
	compiled, _ := CompileConfig(context.Background(), cfg)
	handler := NewHandler(compiled, nil)
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/fail", nil))

	bodyStr := w.Body.String()
	if strings.Contains(bodyStr, "127.0.0.1:1") {
		t.Errorf("response body leaks internal URL: %s", bodyStr)
	}
	if strings.Contains(bodyStr, "connection refused") {
		t.Errorf("response body leaks internal error: %s", bodyStr)
	}
}
