package composition

import (
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

	compiledCfg, err := CompileConfig(cfg)
	if err != nil {
		t.Fatalf("CompileConfig failed: %v", err)
	}

	// Create handler
	httpClient := &http.Client{}
	handler := NewHandler(compiledCfg, httpClient)

	// Create router and register routes
	router := server.NewRouter()
	handler.RegisterRoutes(router)

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
