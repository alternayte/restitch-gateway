package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
	strategy, _ := NewPassthroughStrategy(&PassthroughConfig{})
	mock := &mockRoundTripper{response: &http.Response{StatusCode: 200}}
	rt := strategy.RoundTripper(mock)

	req := httptest.NewRequest(http.MethodGet, "http://upstream.example.com/api", nil)
	ctx := WithClientAuthorization(req.Context(), "Bearer xyz123")
	req = req.WithContext(ctx)

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if mock.lastRequest.Header.Get("Authorization") != "Bearer xyz123" {
		t.Errorf("expected 'Bearer xyz123', got %q", mock.lastRequest.Header.Get("Authorization"))
	}
}

func TestPassthroughStrategy_ReturnsErrorWhenNoAuth(t *testing.T) {
	strategy, _ := NewPassthroughStrategy(&PassthroughConfig{})
	mock := &mockRoundTripper{response: &http.Response{StatusCode: 200}}
	rt := strategy.RoundTripper(mock)

	req := httptest.NewRequest(http.MethodGet, "http://upstream.example.com/api", nil)

	_, err := rt.RoundTrip(req)
	if !errors.Is(err, ErrMissingAuthHeader) {
		t.Errorf("expected ErrMissingAuthHeader, got %v", err)
	}
	if mock.lastRequest != nil {
		t.Error("request should not be forwarded when auth missing")
	}
}

func TestPassthroughStrategy_OriginalRequestUnmodified(t *testing.T) {
	strategy, _ := NewPassthroughStrategy(&PassthroughConfig{})
	mock := &mockRoundTripper{response: &http.Response{StatusCode: 200}}
	rt := strategy.RoundTripper(mock)

	req := httptest.NewRequest(http.MethodGet, "http://upstream.example.com/api", nil)
	req.Header.Set("X-Custom", "value")
	ctx := WithClientAuthorization(req.Context(), "Bearer token")
	req = req.WithContext(ctx)

	_, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}

	if mock.lastRequest == req {
		t.Error("forwarded request should be a clone")
	}
	if mock.lastRequest.Header.Get("Authorization") != "Bearer token" {
		t.Error("clone should have Authorization from context")
	}
}

func TestWithClientAuthorization(t *testing.T) {
	ctx := context.Background()
	if v := clientAuthorization(ctx); v != "" {
		t.Errorf("expected empty, got %q", v)
	}

	ctx = WithClientAuthorization(ctx, "Bearer abc")
	if v := clientAuthorization(ctx); v != "Bearer abc" {
		t.Errorf("expected 'Bearer abc', got %q", v)
	}
}

func TestIsMissingAuthHeaderError(t *testing.T) {
	if !IsMissingAuthHeaderError(ErrMissingAuthHeader) {
		t.Error("should match ErrMissingAuthHeader")
	}
	if IsMissingAuthHeaderError(nil) {
		t.Error("nil should not match")
	}
	if IsMissingAuthHeaderError(errors.New("other")) {
		t.Error("other error should not match")
	}
}
