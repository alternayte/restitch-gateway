// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alternayte/restitch-gateway/internal/reqlog"
)

func testDeps() Deps {
	return Deps{
		Version:    "test",
		ConfigPath: "test.yaml",
		ConfigHash: func() string { return "abc" },
		Compositions: func() []CompositionInfo {
			return []CompositionInfo{
				{
					Name:   "user-dashboard",
					Path:   "/api/users/{id}/dashboard",
					Method: "GET",
					Public: false,
					Steps: []StepInfo{
						{Name: "user", Upstream: "users-api", Method: "GET", Optional: false, TimeoutMS: 5000, DependsOn: []string{}},
						{Name: "orders", Upstream: "orders-api", Method: "GET", Optional: true, TimeoutMS: 3000, DependsOn: []string{"user"}},
					},
					Waves: [][]string{{"user"}, {"orders"}},
				},
				{
					Name:   "health-check",
					Path:   "/api/health",
					Method: "GET",
					Public: true,
					Steps: []StepInfo{
						{Name: "ping", Upstream: "users-api", Method: "GET", Optional: false, TimeoutMS: 1000, DependsOn: []string{}},
					},
					Waves: [][]string{{"ping"}},
				},
			}
		},
		Upstreams: func(ctx context.Context) []UpstreamInfo {
			return []UpstreamInfo{
				{
					Name:      "users-api",
					URL:       "https://users.internal",
					AuthType:  "header",
					TimeoutMS: 10000,
					Health: UpstreamHealthInfo{
						Status:    "healthy",
						LatencyMS: 5.2,
						CheckedAt: "2026-07-09T00:00:00Z",
					},
				},
				{
					Name:      "orders-api",
					URL:       "https://orders.internal",
					AuthType:  "none",
					TimeoutMS: 5000,
					Health: UpstreamHealthInfo{
						Status:    "unhealthy",
						LatencyMS: 0,
						CheckedAt: "2026-07-09T00:00:00Z",
						Error:     "connection refused",
					},
				},
			}
		},
		Requests: NewRingBuffer(10),
		Stats:    NewStats(),
		Validate: func([]byte) []string { return nil },
		Reload:   func() (string, error) { return "abc", nil },
	}
}

// testServerConfig returns the config used by most handler tests: a port of
// 0 (never actually bound) and a non-empty API key, because the key is
// required by default since finding C3.
func testServerConfig() Config {
	return Config{Port: 0, APIKey: testAPIKey}
}

// testAPIKey is the shared admin key for handler tests.
const testAPIKey = "test-admin-key-123"

// keyedRequest builds a request with the admin key header set.
func keyedRequest(method, path string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("X-Admin-Key", testAPIKey)
	return req
}

func TestCompositionInfo_InferredDeps(t *testing.T) {
	si := StepInfo{
		Name:         "bonus",
		Upstream:     "api",
		Method:       "GET",
		DependsOn:    []string{},
		InferredDeps: []string{"user", "loyalty"},
	}
	data, _ := json.Marshal(si)
	var m map[string]any
	_ = json.Unmarshal(data, &m)

	inferred, ok := m["inferred_deps"].([]any)
	if !ok {
		t.Fatal("inferred_deps not present or not array")
	}
	if len(inferred) != 2 {
		t.Errorf("inferred_deps length = %d, want 2", len(inferred))
	}
}

func TestRingBuffer_Overflow(t *testing.T) {
	rb := NewRingBuffer(3)
	for i := 0; i < 5; i++ {
		rb.Record(reqlog.Record{ID: string(rune('a' + i))})
	}

	entries := rb.List(10)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].ID != "e" || entries[1].ID != "d" || entries[2].ID != "c" {
		t.Errorf("expected newest-first [e d c], got [%s %s %s]", entries[0].ID, entries[1].ID, entries[2].ID)
	}
}

func TestRingBuffer_LimitLessThanCount(t *testing.T) {
	rb := NewRingBuffer(10)
	for i := 0; i < 5; i++ {
		rb.Record(reqlog.Record{ID: string(rune('a' + i))})
	}

	entries := rb.List(2)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestStats_Snapshot(t *testing.T) {
	s := NewStats()
	s.Record("api-a", 10.0, false, false)
	s.Record("api-a", 20.0, true, false)
	s.Record("api-b", 5.0, false, true)

	snap := s.Snapshot()
	if snap.TotalRequests != 3 {
		t.Errorf("total = %d, want 3", snap.TotalRequests)
	}
	if snap.TotalErrors != 1 {
		t.Errorf("errors = %d, want 1", snap.TotalErrors)
	}
	if snap.PartialResponses != 1 {
		t.Errorf("partial = %d, want 1", snap.PartialResponses)
	}
	if snap.PerComposition["api-a"].Count != 2 {
		t.Errorf("api-a count = %d, want 2", snap.PerComposition["api-a"].Count)
	}
}

func TestServer_APIKeyAuth(t *testing.T) {
	srv := New(testServerConfig(), testDeps())

	handler := srv.httpServer.Handler

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/admin/api/info", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("without key: expected 401, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/api/info", nil)
	req.Header.Set("X-Admin-Key", "wrong-key")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("with wrong key: expected 401, got %d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/admin/api/info", nil)
	req.Header.Set("X-Admin-Key", testAPIKey)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("with key: expected 200, got %d", rec.Code)
	}
}

// TestServer_KeyRequiredByDefault covers finding C3: with no configured key,
// every admin API request is rejected because no request key can match.
func TestServer_KeyRequiredByDefault(t *testing.T) {
	srv := New(Config{Port: 0}, testDeps())

	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/admin/api/info", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no configured key: expected 401, got %d", rec.Code)
	}
}

// TestServer_BindDefaultsToLoopback covers finding C3: the admin server must
// bind loopback by default and honor a configured bind address.
func TestServer_BindDefaultsToLoopback(t *testing.T) {
	srv := New(Config{Port: 0}, testDeps())
	if want := "127.0.0.1:9090"; srv.httpServer.Addr != want {
		t.Errorf("default addr = %q, want %q", srv.httpServer.Addr, want)
	}

	srv = New(Config{Port: 9999, Bind: "0.0.0.0"}, testDeps())
	if want := "0.0.0.0:9999"; srv.httpServer.Addr != want {
		t.Errorf("configured addr = %q, want %q", srv.httpServer.Addr, want)
	}
}

// TestServer_OptionsRequiresKey covers finding C4: the CORS preflight must
// not be answered before the key check.
func TestServer_OptionsRequiresKey(t *testing.T) {
	srv := New(testServerConfig(), testDeps())
	handler := srv.httpServer.Handler

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/admin/api/reload", nil)
	req.Header.Set("Origin", "https://evil.example")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("OPTIONS without key: expected 401, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodOptions, "/admin/api/reload", nil)
	req.Header.Set("Origin", "https://good.example")
	req.Header.Set("X-Admin-Key", testAPIKey)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS with key: expected 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://good.example" {
		t.Errorf("allow-origin = %q, want the request origin", got)
	}
}

func TestServer_Info(t *testing.T) {
	deps := testDeps()
	deps.Version = "v2.0.0"
	deps.ConfigPath = "/etc/restitch.yaml"
	deps.ConfigHash = func() string { return "deadbeef" }
	srv := New(testServerConfig(), deps)

	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, keyedRequest("GET", "/admin/api/info", nil))

	var info map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&info)

	if info["version"] != "v2.0.0" {
		t.Errorf("version = %v, want v2.0.0", info["version"])
	}
	if info["config_hash"] != "deadbeef" {
		t.Errorf("config_hash = %v, want deadbeef", info["config_hash"])
	}
	if info["compositions"] != float64(2) {
		t.Errorf("compositions = %v, want 2", info["compositions"])
	}
	if info["upstreams"] != float64(2) {
		t.Errorf("upstreams = %v, want 2", info["upstreams"])
	}
}

func TestServer_Validate(t *testing.T) {
	deps := testDeps()
	deps.Validate = func(b []byte) []string {
		if strings.Contains(string(b), "bad") {
			return []string{"invalid config"}
		}
		return nil
	}
	srv := New(testServerConfig(), deps)

	rec := httptest.NewRecorder()
	req := keyedRequest("POST", "/admin/api/validate", strings.NewReader("good config"))
	srv.httpServer.Handler.ServeHTTP(rec, req)

	var result map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&result)
	if result["valid"] != true {
		t.Errorf("expected valid=true, got %v", result["valid"])
	}

	rec = httptest.NewRecorder()
	req = keyedRequest("POST", "/admin/api/validate", strings.NewReader("bad config"))
	srv.httpServer.Handler.ServeHTTP(rec, req)

	result = map[string]any{}
	_ = json.NewDecoder(rec.Body).Decode(&result)
	if result["valid"] != false {
		t.Errorf("expected valid=false, got %v", result["valid"])
	}
}

func TestServer_Compositions(t *testing.T) {
	srv := New(testServerConfig(), testDeps())

	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, keyedRequest("GET", "/admin/api/compositions", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var comps []CompositionInfo
	_ = json.NewDecoder(rec.Body).Decode(&comps)
	if len(comps) != 2 {
		t.Fatalf("expected 2 compositions, got %d", len(comps))
	}

	found := false
	for _, c := range comps {
		if c.Name == "user-dashboard" {
			found = true
			if c.Method != "GET" {
				t.Errorf("method = %s, want GET", c.Method)
			}
			if len(c.Steps) != 2 {
				t.Errorf("steps = %d, want 2", len(c.Steps))
			}
			if len(c.Waves) != 2 {
				t.Errorf("waves = %d, want 2", len(c.Waves))
			}
		}
	}
	if !found {
		t.Error("user-dashboard composition not found")
	}
}

func TestServer_CompositionByName(t *testing.T) {
	srv := New(testServerConfig(), testDeps())
	h := srv.httpServer.Handler

	// Found
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, keyedRequest("GET", "/admin/api/compositions/user-dashboard", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var comp CompositionInfo
	_ = json.NewDecoder(rec.Body).Decode(&comp)
	if comp.Name != "user-dashboard" {
		t.Errorf("name = %s, want user-dashboard", comp.Name)
	}

	// Not found
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, keyedRequest("GET", "/admin/api/compositions/nonexistent", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestServer_Upstreams(t *testing.T) {
	srv := New(testServerConfig(), testDeps())

	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, keyedRequest("GET", "/admin/api/upstreams", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var ups []UpstreamInfo
	_ = json.NewDecoder(rec.Body).Decode(&ups)
	if len(ups) != 2 {
		t.Fatalf("expected 2 upstreams, got %d", len(ups))
	}

	foundHealthy := false
	foundUnhealthy := false
	for _, u := range ups {
		if u.Name == "users-api" {
			foundHealthy = true
			if u.Health.Status != "healthy" {
				t.Errorf("users-api health = %s, want healthy", u.Health.Status)
			}
			if u.AuthType != "header" {
				t.Errorf("users-api auth = %s, want header", u.AuthType)
			}
		}
		if u.Name == "orders-api" {
			foundUnhealthy = true
			if u.Health.Status != "unhealthy" {
				t.Errorf("orders-api health = %s, want unhealthy", u.Health.Status)
			}
			if u.Health.Error != "connection refused" {
				t.Errorf("orders-api error = %q, want 'connection refused'", u.Health.Error)
			}
		}
	}
	if !foundHealthy {
		t.Error("users-api upstream not found")
	}
	if !foundUnhealthy {
		t.Error("orders-api upstream not found")
	}
}

func TestServer_Compositions_NilDeps(t *testing.T) {
	deps := testDeps()
	deps.Compositions = nil
	deps.Upstreams = nil
	srv := New(testServerConfig(), deps)
	h := srv.httpServer.Handler

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, keyedRequest("GET", "/admin/api/compositions", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, keyedRequest("GET", "/admin/api/upstreams", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRegistryStatusEndpoint(t *testing.T) {
	now := time.Now()
	deps := testDeps()
	deps.RegistryStatus = func() any {
		return map[string]any{
			"mode":                  "registry",
			"registry_url":          "http://studio:8090",
			"poll_interval_seconds": 10,
			"last_poll":             now.Format(time.RFC3339),
			"last_success":          now.Format(time.RFC3339),
			"etag":                  "abc123",
			"composition_count":     5,
			"error":                 nil,
			"error_type":            nil,
			"consecutive_errors":    0,
		}
	}
	srv := New(testServerConfig(), deps)

	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, keyedRequest("GET", "/admin/api/registry/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var status map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if status["mode"] != "registry" {
		t.Errorf("mode = %v, want registry", status["mode"])
	}
	if status["etag"] != "abc123" {
		t.Errorf("etag = %v, want abc123", status["etag"])
	}
	if status["composition_count"] != float64(5) {
		t.Errorf("composition_count = %v, want 5", status["composition_count"])
	}
}

func TestRegistryStatusEndpoint_NotRegistered(t *testing.T) {
	deps := testDeps()
	deps.RegistryStatus = nil
	srv := New(testServerConfig(), deps)

	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, keyedRequest("GET", "/admin/api/registry/status", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestServer_MutationRateLimit(t *testing.T) {
	srv := New(testServerConfig(), testDeps())
	h := srv.httpServer.Handler

	// The mutation limiter is configured with burst=5, so the first 5
	// requests should succeed. The 6th should be rate-limited.
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := keyedRequest("POST", "/admin/api/reload", strings.NewReader(""))
		req.RemoteAddr = "10.0.0.99:1234"
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}

	// 6th request should be rate limited
	rec := httptest.NewRecorder()
	req := keyedRequest("POST", "/admin/api/reload", strings.NewReader(""))
	req.RemoteAddr = "10.0.0.99:1234"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "rate limit exceeded" {
		t.Errorf("error = %q, want \"rate limit exceeded\"", body["error"])
	}

	// A different IP should still be allowed
	rec2 := httptest.NewRecorder()
	req2 := keyedRequest("POST", "/admin/api/reload", strings.NewReader(""))
	req2.RemoteAddr = "10.0.0.100:1234"
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("different IP: expected 200, got %d", rec2.Code)
	}

	// GET endpoints should not be rate limited even from the exhausted IP
	rec3 := httptest.NewRecorder()
	req3 := keyedRequest("GET", "/admin/api/info", nil)
	req3.RemoteAddr = "10.0.0.99:1234"
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("GET endpoint: expected 200, got %d", rec3.Code)
	}
}
