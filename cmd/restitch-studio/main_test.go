package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyRewrite(t *testing.T) {
	var gotPath, gotKey string
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-Admin-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"test"}`))
	}))
	defer admin.Close()

	adminKey := "test-key-123"
	mux := buildMux(admin.URL, adminKey, nil)

	req := httptest.NewRequest("GET", "/api/info", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if gotPath != "/admin/api/info" {
		t.Errorf("admin received path %q, want /admin/api/info", gotPath)
	}
	if gotKey != adminKey {
		t.Errorf("admin received key %q, want %q", gotKey, adminKey)
	}
}

func TestSPAFallback(t *testing.T) {
	mux := buildMux("http://localhost:9999", "", nil)

	t.Run("unknown path returns index.html", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/compositions", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		body, _ := io.ReadAll(rec.Result().Body)
		if !strings.Contains(string(body), "html") && !strings.Contains(string(body), "Restitch") {
			t.Errorf("expected index.html content for unknown path, got %q", string(body)[:100])
		}
	})

	t.Run("root returns index.html", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != 200 {
			t.Errorf("root status = %d, want 200", rec.Code)
		}
	})
}
