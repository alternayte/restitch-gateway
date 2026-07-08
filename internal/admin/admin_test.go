package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/restitch/restitch-gateway/internal/reqlog"
)

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
	srv := New(Config{
		Port:   0,
		APIKey: "secret123",
	}, Deps{
		Version:    "test",
		ConfigPath: "test.yaml",
		ConfigHash: func() string { return "abc" },
		Requests:   NewRingBuffer(10),
		Stats:      NewStats(),
		Validate:   func([]byte) []string { return nil },
		Reload:     func() (string, error) { return "abc", nil },
	})

	handler := srv.httpServer.Handler

	// Without key → 401
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/admin/api/info", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("without key: expected 401, got %d", rec.Code)
	}

	// With key → 200
	req := httptest.NewRequest("GET", "/admin/api/info", nil)
	req.Header.Set("X-Admin-Key", "secret123")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("with key: expected 200, got %d", rec.Code)
	}
}

func TestServer_Info(t *testing.T) {
	srv := New(Config{Port: 0}, Deps{
		Version:    "v2.0.0",
		ConfigPath: "/etc/restitch.yaml",
		ConfigHash: func() string { return "deadbeef" },
		Requests:   NewRingBuffer(10),
		Stats:      NewStats(),
		Validate:   func([]byte) []string { return nil },
		Reload:     func() (string, error) { return "", nil },
	})

	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/admin/api/info", nil))

	var info map[string]any
	json.NewDecoder(rec.Body).Decode(&info)

	if info["version"] != "v2.0.0" {
		t.Errorf("version = %v, want v2.0.0", info["version"])
	}
	if info["config_hash"] != "deadbeef" {
		t.Errorf("config_hash = %v, want deadbeef", info["config_hash"])
	}
}

func TestServer_Validate(t *testing.T) {
	srv := New(Config{Port: 0}, Deps{
		Version:    "test",
		ConfigPath: "test.yaml",
		ConfigHash: func() string { return "" },
		Requests:   NewRingBuffer(10),
		Stats:      NewStats(),
		Validate: func(b []byte) []string {
			if strings.Contains(string(b), "bad") {
				return []string{"invalid config"}
			}
			return nil
		},
		Reload: func() (string, error) { return "", nil },
	})

	// Valid
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/admin/api/validate", strings.NewReader("good config"))
	srv.httpServer.Handler.ServeHTTP(rec, req)

	var result map[string]any
	json.NewDecoder(rec.Body).Decode(&result)
	if result["valid"] != true {
		t.Errorf("expected valid=true, got %v", result["valid"])
	}

	// Invalid
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/admin/api/validate", strings.NewReader("bad config"))
	srv.httpServer.Handler.ServeHTTP(rec, req)

	result = map[string]any{}
	json.NewDecoder(rec.Body).Decode(&result)
	if result["valid"] != false {
		t.Errorf("expected valid=false, got %v", result["valid"])
	}
}
