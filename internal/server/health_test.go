package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	srv := New(Config{Port: 0, TLSPort: 0})
	handler := HealthHandler(srv)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
}

func TestReadyHandler(t *testing.T) {
	srv := New(Config{Port: 0, TLSPort: 0})
	handler := ReadyHandler(srv)

	// Not ready yet
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/ready", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("before ready: expected 503, got %d", rec.Code)
	}

	// Set ready
	srv.SetReady(true)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/ready", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("after ready: expected 200, got %d", rec.Code)
	}
}

func TestServer_ReadyAfterListen(t *testing.T) {
	srv := New(Config{Port: 0, TLSPort: 0})

	if srv.Ready() {
		t.Error("server should not be ready before Listen")
	}
}
