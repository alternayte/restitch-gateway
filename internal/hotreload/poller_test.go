package hotreload

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func bundleHandler(yaml, etag string, count int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		json.NewEncoder(w).Encode(map[string]any{
			"yaml_content":      yaml,
			"etag":              etag,
			"composition_count": count,
			"composition_names": []string{},
		})
	}
}

func TestPoller_HappyPath(t *testing.T) {
	srv := httptest.NewServer(bundleHandler("upstreams: {}\n", "e1", 1))
	defer srv.Close()

	var reloadCalls atomic.Int32
	reloadFn := func(yaml []byte) (string, error) {
		reloadCalls.Add(1)
		return "hash1", nil
	}

	client := NewRegistryClient(srv.URL)
	p := NewPoller(client, 50*time.Millisecond, reloadFn, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	p.Run(ctx)

	if reloadCalls.Load() < 1 {
		t.Error("expected at least one reload call")
	}
	status := p.Status()
	if status.LastETag != "e1" {
		t.Errorf("etag = %q, want %q", status.LastETag, "e1")
	}
	if status.LastError != "" {
		t.Errorf("unexpected error: %s", status.LastError)
	}
}

func TestPoller_NotModified_NoReload(t *testing.T) {
	srv := httptest.NewServer(bundleHandler("x: y\n", "e1", 1))
	defer srv.Close()

	var reloadCalls atomic.Int32
	reloadFn := func(yaml []byte) (string, error) {
		reloadCalls.Add(1)
		return "hash1", nil
	}

	client := NewRegistryClient(srv.URL)
	p := NewPoller(client, 50*time.Millisecond, reloadFn, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	p.Run(ctx)

	if reloadCalls.Load() != 1 {
		t.Errorf("expected exactly 1 reload (initial), got %d", reloadCalls.Load())
	}
}

func TestPoller_FetchError_Backoff(t *testing.T) {
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	p := NewPoller(client, 50*time.Millisecond, func([]byte) (string, error) { return "", nil }, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	p.Run(ctx)

	status := p.Status()
	if status.ErrorType != "fetch" {
		t.Errorf("error_type = %q, want %q", status.ErrorType, "fetch")
	}
	if status.ConsecutiveErrors < 2 {
		t.Errorf("expected at least 2 consecutive errors, got %d", status.ConsecutiveErrors)
	}
}

func TestPoller_Trigger_ImmediatePoll(t *testing.T) {
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		w.Header().Set("ETag", "e1")
		json.NewEncoder(w).Encode(map[string]any{
			"yaml_content": "x: y\n", "etag": "e1",
			"composition_count": 0, "composition_names": []string{},
		})
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	p := NewPoller(client, 10*time.Second, func([]byte) (string, error) { return "", nil }, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		p.Trigger()
	}()

	p.Run(ctx)

	if reqCount.Load() < 2 {
		t.Errorf("expected >=2 requests (initial + triggered), got %d", reqCount.Load())
	}
}

func TestPoller_ContextCancel_CleansUp(t *testing.T) {
	srv := httptest.NewServer(bundleHandler("x: y\n", "e1", 0))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	p := NewPoller(client, time.Hour, func([]byte) (string, error) { return "", nil }, nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
}
