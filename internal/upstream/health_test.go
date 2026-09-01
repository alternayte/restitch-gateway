package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestChecker_CachedResult(t *testing.T) {
	var hitCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	up := &Upstream{
		Name:             "test",
		BaseURL:          srv.URL,
		Client:           srv.Client(),
		HealthPath:       "/health",
		MaxResponseBytes: 10 * 1024 * 1024,
	}

	checker := NewChecker(map[string]*Upstream{"test": up}, 5*time.Second)

	ctx := context.Background()
	result1 := checker.Check(ctx)
	if result1["test"].Status != "healthy" {
		t.Errorf("first check: got %q, want healthy", result1["test"].Status)
	}

	result2 := checker.Check(ctx)
	if result2["test"].Status != "healthy" {
		t.Errorf("second check: got %q, want healthy", result2["test"].Status)
	}

	if hitCount.Load() != 1 {
		t.Errorf("expected 1 upstream hit (cached), got %d", hitCount.Load())
	}
}

func TestChecker_UnhealthyUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	up := &Upstream{
		Name:             "bad",
		BaseURL:          srv.URL,
		Client:           srv.Client(),
		MaxResponseBytes: 10 * 1024 * 1024,
	}

	checker := NewChecker(map[string]*Upstream{"bad": up}, 1*time.Second)
	result := checker.Check(context.Background())

	if result["bad"].Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %q", result["bad"].Status)
	}
}

// TestChecker_NilUpstream covers finding H3: a checker whose upstream map
// lacks a name (for example during the window before a reload rebuilds it)
// must report "unknown" instead of panicking.
func TestChecker_NilUpstream(t *testing.T) {
	checker := NewChecker(map[string]*Upstream{}, 1*time.Second)

	got := checker.checkOne(context.Background(), "ghost")
	if got.Status != "unknown" {
		t.Errorf("status = %q, want unknown", got.Status)
	}
}
