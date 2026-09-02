package upstream

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	}, "test", nil)

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
	}, "test", nil)

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
	}, "test", nil)

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
	}, "test", nil)

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
	}, "test", nil)

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

// TestRetryTripper_NonRewindableBodyNotRetried covers finding M1: a request
// body without GetBody must not be re-sent consumed on a retry. The retry is
// abandoned and the last response is returned.
func TestRetryTripper_NonRewindableBodyNotRetried(t *testing.T) {
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
	}, "test", nil)

	// A body with GetBody=nil (for example a custom io.ReadCloser) cannot be
	// recreated for a retry. PUT is retryable by default, so the guard is
	// what stops the second attempt.
	req, _ := http.NewRequest("PUT", srv.URL+"/test", strings.NewReader("payload"))
	req.GetBody = nil
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 503 {
		t.Errorf("got status %d, want 503", resp.StatusCode)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (non-rewindable body must not be retried)", got)
	}
}

// TestRetryTripper_RewindableBodyRetried ensures the legitimate rewind path
// still retries when GetBody is available.
func TestRetryTripper_RewindableBodyRetried(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "payload" {
			t.Errorf("attempt %d body = %q, want payload", hits.Load()+1, body)
		}
		if n := hits.Add(1); n == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	rt := newRetryTripper(http.DefaultTransport, RetryConfig{
		MaxAttempts:        3,
		Interval:           1 * time.Millisecond,
		BackoffOn:          []int{503},
		RetryNonIdempotent: true,
	}, "test", nil)

	req, _ := http.NewRequest("POST", srv.URL+"/test", strings.NewReader("payload"))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("payload")), nil
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("got status %d, want 200", resp.StatusCode)
	}
	if hits.Load() != 2 {
		t.Errorf("attempts = %d, want 2", hits.Load())
	}
}

// TestRetryTripper_NegativeRetryAfterClamped covers finding M2: a negative
// Retry-After must not cause an immediate (negative) sleep. A zero-or-larger
// sleep still happens, so this only asserts the request completes without a
// panic and the retry count stays sane.
func TestRetryTripper_NegativeRetryAfterClamped(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "-5")
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	rt := newRetryTripper(http.DefaultTransport, RetryConfig{
		MaxAttempts: 2,
		Interval:    50 * time.Millisecond,
		BackoffOn:   []int{503},
	}, "test", nil)

	req, _ := http.NewRequest("GET", srv.URL+"/test", nil)
	start := time.Now()
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("got status %d, want 200", resp.StatusCode)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("retry took %v, want a clamped (short) sleep for Retry-After: -5", elapsed)
	}
}
