package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryTripper_SuccessAfterRetries(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n <= 2 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	rt := newRetryTripper(http.DefaultTransport, RetryConfig{
		MaxAttempts: 3,
		Interval:    1 * time.Millisecond,
		MaxBackoff:  10 * time.Millisecond,
		BackoffOn:   []int{503},
	}, "test")

	req, _ := http.NewRequest("GET", srv.URL+"/test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("got status %d, want 200", resp.StatusCode)
	}
	if hits.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", hits.Load())
	}
}

func TestRetryTripper_NoRetryOnNonBackoffStatus(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(400)
	}))
	defer srv.Close()

	rt := newRetryTripper(http.DefaultTransport, RetryConfig{
		MaxAttempts: 3,
		Interval:    1 * time.Millisecond,
		BackoffOn:   []int{503},
	}, "test")

	req, _ := http.NewRequest("GET", srv.URL+"/test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("got status %d, want 400", resp.StatusCode)
	}
	if hits.Load() != 1 {
		t.Errorf("expected 1 attempt for non-backoff status, got %d", hits.Load())
	}
}

func TestRetryTripper_PostNotRetriedByDefault(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(503)
	}))
	defer srv.Close()

	rt := newRetryTripper(http.DefaultTransport, RetryConfig{
		MaxAttempts: 3,
		Interval:    1 * time.Millisecond,
		BackoffOn:   []int{503},
	}, "test")

	req, _ := http.NewRequest("POST", srv.URL+"/test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 503 {
		t.Errorf("got status %d, want 503", resp.StatusCode)
	}
	if hits.Load() != 1 {
		t.Errorf("POST should not be retried, got %d attempts", hits.Load())
	}
}

func TestRetryTripper_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	rt := newRetryTripper(http.DefaultTransport, RetryConfig{
		MaxAttempts: 5,
		Interval:    100 * time.Millisecond,
		BackoffOn:   []int{503},
	}, "test")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/test", nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRetryTripper_DropOn(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(503)
	}))
	defer srv.Close()

	rt := newRetryTripper(http.DefaultTransport, RetryConfig{
		MaxAttempts: 3,
		Interval:    1 * time.Millisecond,
		BackoffOn:   []int{502},
		DropOn:      []int{503},
	}, "test")

	req, _ := http.NewRequest("GET", srv.URL+"/test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 503 {
		t.Errorf("got status %d, want 503", resp.StatusCode)
	}
	if hits.Load() != 1 {
		t.Errorf("DropOn should prevent retry, got %d attempts", hits.Load())
	}
}
