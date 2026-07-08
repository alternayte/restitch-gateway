package composition

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   1,
				"name": "Alice",
			})
		case "/posts":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]interface{}{
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
	httpClient := &http.Client{}
	handler := NewHandler(compiledCfg, httpClient)

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

func TestHandler_matchComposition(t *testing.T) {
	handler := &Handler{
		routes: map[string]string{
			"GET:/api/users":  "get-users",
			"POST:/api/users": "create-user",
			"GET:/api/posts":  "get-posts",
		},
	}

	tests := []struct {
		name   string
		path   string
		method string
		want   string
	}{
		{
			name:   "exact match",
			path:   "/api/users",
			method: "GET",
			want:   "get-users",
		},
		{
			name:   "different method",
			path:   "/api/users",
			method: "POST",
			want:   "create-user",
		},
		{
			name:   "no match",
			path:   "/api/unknown",
			method: "GET",
			want:   "",
		},
		{
			name:   "trailing slash removed",
			path:   "/api/users/",
			method: "GET",
			want:   "get-users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handler.matchComposition(tt.path, tt.method)
			if got != tt.want {
				t.Errorf("matchComposition() = %v, want %v", got, tt.want)
			}
		})
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
	handler := NewHandler(compiledCfg, &http.Client{})

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
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
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
	handler := NewHandler(compiledCfg, &http.Client{})

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
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
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
        value: "${TEST_API_KEY}"

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
	handler := NewHandler(compiledCfg, &http.Client{})

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
		json.NewEncoder(w).Encode(map[string]any{
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

	handler := NewHandler(compiledCfg, &http.Client{})
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
