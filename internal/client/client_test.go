package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew_TransportConfig(t *testing.T) {
	c := New()

	transport := c.Transport()

	// CRITICAL: Verify MaxIdleConnsPerHost is 100, not the default 2
	if transport.MaxIdleConnsPerHost != 100 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 100", transport.MaxIdleConnsPerHost)
	}

	if transport.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100", transport.MaxIdleConns)
	}

	if transport.TLSClientConfig == nil {
		t.Error("TLSClientConfig is nil, want configured")
	} else if transport.TLSClientConfig.MinVersion != 0x0303 { // TLS 1.2
		t.Errorf("TLS MinVersion = %x, want TLS 1.2 (0x0303)", transport.TLSClientConfig.MinVersion)
	}
}

func TestClient_Get(t *testing.T) {
	// Create a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Method = %s, want GET", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer ts.Close()

	c := New()
	ctx := context.Background()

	resp, err := c.Get(ctx, ts.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer DrainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestClient_Do(t *testing.T) {
	// Create a test server that echoes the method
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(r.Method))
	}))
	defer ts.Close()

	c := New()
	ctx := context.Background()

	req, err := http.NewRequest(http.MethodPost, ts.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	resp, err := c.Do(ctx, req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer DrainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	// Create a test server that delays response
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if context is cancelled
		select {
		case <-r.Context().Done():
			return
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	c := New()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := c.Get(ctx, ts.URL)
	if err == nil {
		t.Error("Get() expected error with cancelled context, got nil")
	}
}

func TestDrainAndClose_NilBody(t *testing.T) {
	// Should not panic with nil body
	DrainAndClose(nil)
}

func TestDrainAndClose_ValidBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("some data that needs to be drained"))
	}))
	defer ts.Close()

	c := New()
	ctx := context.Background()

	resp, err := c.Get(ctx, ts.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Should not panic
	DrainAndClose(resp.Body)
}
