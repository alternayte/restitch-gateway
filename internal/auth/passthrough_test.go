package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockRoundTripper captures the request for inspection.
type mockRoundTripper struct {
	lastRequest *http.Request
	response    *http.Response
	err         error
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.lastRequest = req
	if m.err != nil {
		return nil, m.err
	}
	if m.response != nil {
		return m.response, nil
	}
	return &http.Response{StatusCode: 200}, nil
}

func TestPassthroughStrategy_ForwardsBearerToken(t *testing.T) {
	// Create passthrough strategy
	strategy, err := NewPassthroughStrategy(&PassthroughConfig{})
	if err != nil {
		t.Fatalf("NewPassthroughStrategy failed: %v", err)
	}

	// Create mock transport
	mock := &mockRoundTripper{
		response: &http.Response{StatusCode: 200},
	}

	// Wrap with passthrough
	rt := strategy.RoundTripper(mock)

	// Create request with Bearer token
	req := httptest.NewRequest(http.MethodGet, "http://upstream.example.com/api", nil)
	req.Header.Set("Authorization", "Bearer xyz123")

	// Execute
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify header was forwarded verbatim
	if mock.lastRequest == nil {
		t.Fatal("expected request to be forwarded")
	}

	gotAuth := mock.lastRequest.Header.Get("Authorization")
	if gotAuth != "Bearer xyz123" {
		t.Errorf("expected 'Bearer xyz123', got %q", gotAuth)
	}
}

func TestPassthroughStrategy_ForwardsBasicAuth(t *testing.T) {
	// Create passthrough strategy
	strategy, err := NewPassthroughStrategy(&PassthroughConfig{})
	if err != nil {
		t.Fatalf("NewPassthroughStrategy failed: %v", err)
	}

	// Create mock transport
	mock := &mockRoundTripper{
		response: &http.Response{StatusCode: 200},
	}

	// Wrap with passthrough
	rt := strategy.RoundTripper(mock)

	// Create request with Basic auth
	req := httptest.NewRequest(http.MethodGet, "http://upstream.example.com/api", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // base64(user:pass)

	// Execute
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify header was forwarded verbatim
	gotAuth := mock.lastRequest.Header.Get("Authorization")
	if gotAuth != "Basic dXNlcjpwYXNz" {
		t.Errorf("expected 'Basic dXNlcjpwYXNz', got %q", gotAuth)
	}
}

func TestPassthroughStrategy_ForwardsCustomScheme(t *testing.T) {
	// Create passthrough strategy
	strategy, err := NewPassthroughStrategy(&PassthroughConfig{})
	if err != nil {
		t.Fatalf("NewPassthroughStrategy failed: %v", err)
	}

	// Create mock transport
	mock := &mockRoundTripper{
		response: &http.Response{StatusCode: 200},
	}

	// Wrap with passthrough
	rt := strategy.RoundTripper(mock)

	// Create request with custom auth scheme
	req := httptest.NewRequest(http.MethodGet, "http://upstream.example.com/api", nil)
	req.Header.Set("Authorization", "HMAC-SHA256 abc:signature")

	// Execute
	_, err = rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}

	// Verify header was forwarded verbatim
	gotAuth := mock.lastRequest.Header.Get("Authorization")
	if gotAuth != "HMAC-SHA256 abc:signature" {
		t.Errorf("expected 'HMAC-SHA256 abc:signature', got %q", gotAuth)
	}
}

func TestPassthroughStrategy_ReturnsErrorWhenNoAuth(t *testing.T) {
	// Create passthrough strategy
	strategy, err := NewPassthroughStrategy(&PassthroughConfig{})
	if err != nil {
		t.Fatalf("NewPassthroughStrategy failed: %v", err)
	}

	// Create mock transport
	mock := &mockRoundTripper{
		response: &http.Response{StatusCode: 200},
	}

	// Wrap with passthrough
	rt := strategy.RoundTripper(mock)

	// Create request WITHOUT Authorization header
	req := httptest.NewRequest(http.MethodGet, "http://upstream.example.com/api", nil)

	// Execute
	resp, err := rt.RoundTrip(req)

	// Should return error
	if err == nil {
		t.Fatal("expected error when no Authorization header")
	}

	// Response should be nil
	if resp != nil {
		t.Error("expected nil response on error")
	}

	// Should be ErrMissingAuthHeader
	if !errors.Is(err, ErrMissingAuthHeader) {
		t.Errorf("expected ErrMissingAuthHeader, got %v", err)
	}

	// Mock should NOT have received request
	if mock.lastRequest != nil {
		t.Error("request should not be forwarded when auth missing")
	}
}

func TestPassthroughStrategy_OriginalRequestUnmodified(t *testing.T) {
	// Create passthrough strategy
	strategy, err := NewPassthroughStrategy(&PassthroughConfig{})
	if err != nil {
		t.Fatalf("NewPassthroughStrategy failed: %v", err)
	}

	// Create mock transport
	mock := &mockRoundTripper{
		response: &http.Response{StatusCode: 200},
	}

	// Wrap with passthrough
	rt := strategy.RoundTripper(mock)

	// Create original request
	originalReq := httptest.NewRequest(http.MethodGet, "http://upstream.example.com/api", nil)
	originalReq.Header.Set("Authorization", "Bearer original-token")
	originalReq.Header.Set("X-Custom", "value")

	// Store original pointer for comparison
	originalAuthHeader := originalReq.Header.Get("Authorization")
	originalCustomHeader := originalReq.Header.Get("X-Custom")

	// Execute
	_, err = rt.RoundTrip(originalReq)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}

	// Verify original request headers are unchanged
	if originalReq.Header.Get("Authorization") != originalAuthHeader {
		t.Error("original request Authorization header was modified")
	}
	if originalReq.Header.Get("X-Custom") != originalCustomHeader {
		t.Error("original request X-Custom header was modified")
	}

	// Verify the forwarded request is a different object
	if mock.lastRequest == originalReq {
		t.Error("forwarded request should be a clone, not the original")
	}

	// Verify both have same Authorization header
	if mock.lastRequest.Header.Get("Authorization") != originalAuthHeader {
		t.Error("forwarded request should have same Authorization header")
	}
}

func TestPassthroughStrategy_EmptyAuthHeaderTreatedAsMissing(t *testing.T) {
	// Create passthrough strategy
	strategy, err := NewPassthroughStrategy(&PassthroughConfig{})
	if err != nil {
		t.Fatalf("NewPassthroughStrategy failed: %v", err)
	}

	// Create mock transport
	mock := &mockRoundTripper{
		response: &http.Response{StatusCode: 200},
	}

	// Wrap with passthrough
	rt := strategy.RoundTripper(mock)

	// Create request with empty Authorization header
	req := httptest.NewRequest(http.MethodGet, "http://upstream.example.com/api", nil)
	req.Header.Set("Authorization", "")

	// Execute
	_, err = rt.RoundTrip(req)

	// Should return error (empty is same as missing)
	if !errors.Is(err, ErrMissingAuthHeader) {
		t.Errorf("expected ErrMissingAuthHeader for empty header, got %v", err)
	}
}

func TestIsMissingAuthHeaderError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "direct ErrMissingAuthHeader",
			err:      ErrMissingAuthHeader,
			expected: true,
		},
		{
			name:     "wrapped ErrMissingAuthHeader",
			err:      errors.New("wrapper: " + ErrMissingAuthHeader.Error()),
			expected: false, // string wrapping doesn't work with errors.Is
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "different error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMissingAuthHeaderError(tt.err)
			if got != tt.expected {
				t.Errorf("IsMissingAuthHeaderError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}
