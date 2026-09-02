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

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tokenResponse represents the OAuth2 token endpoint response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// mockTokenServer creates a test server that returns OAuth2 tokens.
// It returns the server and a function to get the request count.
func mockTokenServer(t *testing.T, accessToken string, expiresIn int) (*httptest.Server, *int32) {
	t.Helper()
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)

		// Verify it's a POST to token endpoint
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Return token response
		resp := tokenResponse{
			AccessToken: accessToken,
			TokenType:   "Bearer",
			ExpiresIn:   expiresIn,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))

	return server, &requestCount
}

// mockFailingTokenServer returns errors for all requests.
func mockFailingTokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
	}))
}

func TestNewOAuth2Strategy(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	t.Run("valid config fetches initial token", func(t *testing.T) {
		os.Clearenv()

		server, requestCount := mockTokenServer(t, "test-token-123", 3600)
		defer server.Close()

		cfg := &OAuth2Config{
			TokenURL:     server.URL,
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			Scopes:       []string{"read", "write"},
		}

		strategy, err := NewOAuth2Strategy(context.Background(), cfg)
		if err != nil {
			t.Fatalf("NewOAuth2Strategy() error: %v", err)
		}

		if strategy == nil {
			t.Fatal("NewOAuth2Strategy() returned nil strategy")
		}

		// Verify initial token was fetched
		if atomic.LoadInt32(requestCount) != 1 {
			t.Errorf("Expected 1 token request, got %d", *requestCount)
		}
	})

	t.Run("initial token fetch failure fails startup", func(t *testing.T) {
		server := mockFailingTokenServer(t)
		defer server.Close()

		cfg := &OAuth2Config{
			TokenURL:     server.URL,
			ClientID:     "client",
			ClientSecret: "secret",
		}

		_, err := NewOAuth2Strategy(context.Background(), cfg)
		if err == nil {
			t.Fatal("Expected error for failed token fetch")
		}
		if !strings.Contains(err.Error(), "initial oauth2 token fetch failed") {
			t.Errorf("Error should mention initial fetch failed: %v", err)
		}
	})
}

func TestNewOAuth2StrategyValidateOnly(t *testing.T) {
	t.Run("valid config passes", func(t *testing.T) {
		cfg := &OAuth2Config{
			TokenURL:     "https://example.com/token",
			ClientID:     "client",
			ClientSecret: "secret",
		}
		strategy, err := NewOAuth2StrategyValidateOnly(cfg)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Finding M10: if this strategy ever reached RoundTrip it must fail
		// cleanly, not panic on a nil token source.
		rt := strategy.RoundTripper(http.DefaultTransport)
		req, _ := http.NewRequest("GET", "https://example.com/upstream", nil)
		_, err = rt.RoundTrip(req)
		if err == nil {
			t.Error("expected an error from the validate-only strategy's RoundTrip")
		}
		if !strings.Contains(err.Error(), "validate-only") {
			t.Errorf("error = %v, want the validate-only message", err)
		}
	})

	t.Run("empty client_id fails", func(t *testing.T) {
		cfg := &OAuth2Config{
			TokenURL:     "https://example.com/token",
			ClientID:     "",
			ClientSecret: "secret",
		}
		_, err := NewOAuth2StrategyValidateOnly(cfg)
		if err == nil {
			t.Fatal("expected error for empty client_id")
		}
	})

	t.Run("empty client_secret fails", func(t *testing.T) {
		cfg := &OAuth2Config{
			TokenURL:     "https://example.com/token",
			ClientID:     "client",
			ClientSecret: "",
		}
		_, err := NewOAuth2StrategyValidateOnly(cfg)
		if err == nil {
			t.Fatal("expected error for empty client_secret")
		}
	})

	t.Run("empty token_url fails", func(t *testing.T) {
		cfg := &OAuth2Config{
			TokenURL:     "",
			ClientID:     "client",
			ClientSecret: "secret",
		}
		_, err := NewOAuth2StrategyValidateOnly(cfg)
		if err == nil {
			t.Fatal("expected error for empty token_url")
		}
	})
}

func TestOAuth2Strategy_RoundTripper(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	t.Run("injects Bearer token", func(t *testing.T) {
		os.Clearenv()

		server, _ := mockTokenServer(t, "my-access-token", 3600)
		defer server.Close()

		cfg := &OAuth2Config{
			TokenURL:     server.URL,
			ClientID:     "client",
			ClientSecret: "secret",
		}

		strategy, err := NewOAuth2Strategy(context.Background(), cfg)
		if err != nil {
			t.Fatalf("NewOAuth2Strategy() error: %v", err)
		}

		// Track what the RoundTripper received
		var receivedReq *http.Request
		mockRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			receivedReq = req
			return &http.Response{StatusCode: 200}, nil
		})

		rt := strategy.RoundTripper(mockRT)

		// Create and execute request
		req := httptest.NewRequest("GET", "http://upstream.example.com/api", nil)
		_, err = rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip() error: %v", err)
		}

		// Verify Authorization header
		authHeader := receivedReq.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			t.Errorf("Authorization header = %q, want prefix 'Bearer '", authHeader)
		}
		if !strings.Contains(authHeader, "my-access-token") {
			t.Errorf("Authorization header = %q, should contain token", authHeader)
		}
	})

	t.Run("token reused across requests", func(t *testing.T) {
		os.Clearenv()

		server, requestCount := mockTokenServer(t, "reusable-token", 3600)
		defer server.Close()

		cfg := &OAuth2Config{
			TokenURL:     server.URL,
			ClientID:     "client",
			ClientSecret: "secret",
		}

		strategy, err := NewOAuth2Strategy(context.Background(), cfg)
		if err != nil {
			t.Fatalf("NewOAuth2Strategy() error: %v", err)
		}

		// Initial token request count
		initialCount := atomic.LoadInt32(requestCount)

		mockRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200}, nil
		})

		rt := strategy.RoundTripper(mockRT)

		// Make multiple requests
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", "http://upstream.example.com/api", nil)
			_, err := rt.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip() error: %v", err)
			}
		}

		// Verify no additional token requests (token was cached)
		finalCount := atomic.LoadInt32(requestCount)
		if finalCount != initialCount {
			t.Errorf("Expected %d token requests, got %d (token should be reused)", initialCount, finalCount)
		}
	})

	t.Run("original request not modified", func(t *testing.T) {
		os.Clearenv()

		server, _ := mockTokenServer(t, "test-token", 3600)
		defer server.Close()

		cfg := &OAuth2Config{
			TokenURL:     server.URL,
			ClientID:     "client",
			ClientSecret: "secret",
		}

		strategy, err := NewOAuth2Strategy(context.Background(), cfg)
		if err != nil {
			t.Fatalf("NewOAuth2Strategy() error: %v", err)
		}

		mockRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200}, nil
		})

		rt := strategy.RoundTripper(mockRT)

		// Create original request WITHOUT Authorization header
		originalReq := httptest.NewRequest("GET", "http://upstream.example.com/api", nil)
		originalAuthValue := originalReq.Header.Get("Authorization")

		// Execute
		_, err = rt.RoundTrip(originalReq)
		if err != nil {
			t.Fatalf("RoundTrip() error: %v", err)
		}

		// Verify original request was NOT modified
		afterAuthValue := originalReq.Header.Get("Authorization")
		if afterAuthValue != originalAuthValue {
			t.Errorf("Original request was modified: Authorization header changed from %q to %q",
				originalAuthValue, afterAuthValue)
		}
	})
}

func TestOAuth2Strategy_Singleflight(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	t.Run("concurrent requests don't cause thundering herd", func(t *testing.T) {
		os.Clearenv()

		// Track concurrent requests
		var maxConcurrent int32
		var currentConcurrent int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Track concurrent requests
			current := atomic.AddInt32(&currentConcurrent, 1)
			for {
				max := atomic.LoadInt32(&maxConcurrent)
				if current <= max {
					break
				}
				if atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
					break
				}
			}

			// Simulate slow token endpoint
			time.Sleep(50 * time.Millisecond)

			atomic.AddInt32(&currentConcurrent, -1)

			resp := tokenResponse{
				AccessToken: "concurrent-token",
				TokenType:   "Bearer",
				ExpiresIn:   3600,
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := &OAuth2Config{
			TokenURL:     server.URL,
			ClientID:     "client",
			ClientSecret: "secret",
		}

		strategy, err := NewOAuth2Strategy(context.Background(), cfg)
		if err != nil {
			t.Fatalf("NewOAuth2Strategy() error: %v", err)
		}

		// Reset counters after initial token fetch
		atomic.StoreInt32(&maxConcurrent, 0)
		atomic.StoreInt32(&currentConcurrent, 0)

		mockRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200}, nil
		})

		rt := strategy.RoundTripper(mockRT)

		// Launch many concurrent requests
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req := httptest.NewRequest("GET", "http://upstream.example.com/api", nil)
				_, err := rt.RoundTrip(req)
				if err != nil {
					t.Errorf("RoundTrip() error: %v", err)
				}
			}()
		}
		wg.Wait()

		// With singleflight, max concurrent should be 0 (token already cached from init)
		// If token wasn't cached, singleflight should still limit to 1
		max := atomic.LoadInt32(&maxConcurrent)
		if max > 1 {
			t.Errorf("Max concurrent token requests = %d, want <= 1 (singleflight should deduplicate)", max)
		}
	})
}

func TestOAuth2Strategy_TokenRefresh(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	t.Run("token refreshed before expiry", func(t *testing.T) {
		os.Clearenv()

		var tokenCounter int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&tokenCounter, 1)

			// Each token has a very short expiry (2 seconds)
			// With 30-second buffer, it should trigger refresh almost immediately
			// But since expiry is only 2 seconds, it will expire faster than the buffer
			resp := tokenResponse{
				AccessToken: "token-" + string('a'+count-1),
				TokenType:   "Bearer",
				ExpiresIn:   2, // Very short expiry for testing
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := &OAuth2Config{
			TokenURL:     server.URL,
			ClientID:     "client",
			ClientSecret: "secret",
		}

		strategy, err := NewOAuth2Strategy(context.Background(), cfg)
		if err != nil {
			t.Fatalf("NewOAuth2Strategy() error: %v", err)
		}

		mockRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200}, nil
		})

		rt := strategy.RoundTripper(mockRT)

		// Make initial request
		req := httptest.NewRequest("GET", "http://upstream.example.com/api", nil)
		_, err = rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip() error: %v", err)
		}

		// Wait for token to expire (2 seconds + buffer)
		time.Sleep(3 * time.Second)

		// Make another request - should trigger refresh
		req = httptest.NewRequest("GET", "http://upstream.example.com/api", nil)
		_, err = rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip() error after expiry: %v", err)
		}

		// Should have fetched at least 2 tokens (initial + refresh)
		count := atomic.LoadInt32(&tokenCounter)
		if count < 2 {
			t.Errorf("Expected at least 2 token requests after expiry, got %d", count)
		}
	})
}
