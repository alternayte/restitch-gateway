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

package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRequestID(t *testing.T) {
	// Generate a request ID
	id := NewRequestID()

	// ULID format is exactly 26 characters
	if len(id) != 26 {
		t.Errorf("expected request ID length 26, got %d: %s", len(id), id)
	}

	// Generate another ID - should be different
	id2 := NewRequestID()
	if id == id2 {
		t.Errorf("expected unique IDs, got same ID twice: %s", id)
	}
}

func TestGetRequestID_EmptyContext(t *testing.T) {
	// Context without request ID should return empty string
	ctx := context.Background()
	id := GetRequestID(ctx)

	if id != "" {
		t.Errorf("expected empty string for context without request ID, got %q", id)
	}
}

func TestGetRequestID_WithRequestID(t *testing.T) {
	// Context with request ID should return it
	ctx := withRequestID(context.Background(), "test-id-123")
	id := GetRequestID(ctx)

	if id != "test-id-123" {
		t.Errorf("expected 'test-id-123', got %q", id)
	}
}

func TestRequestIDMiddleware_HonorsIncomingHeader(t *testing.T) {
	// Create a handler that records the request ID from context
	var capturedID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with middleware
	wrapped := RequestIDMiddleware(handler)

	// Make request with existing X-Request-ID header
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(RequestIDHeader, "my-custom-trace-id")
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Should honor incoming header
	if capturedID != "my-custom-trace-id" {
		t.Errorf("expected context to contain 'my-custom-trace-id', got %q", capturedID)
	}

	// Should set response header with same value
	responseID := rec.Header().Get(RequestIDHeader)
	if responseID != "my-custom-trace-id" {
		t.Errorf("expected response header 'my-custom-trace-id', got %q", responseID)
	}
}

func TestRequestIDMiddleware_GeneratesULID(t *testing.T) {
	// Create a handler that records the request ID from context
	var capturedID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with middleware
	wrapped := RequestIDMiddleware(handler)

	// Make request without X-Request-ID header
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	// Should generate a ULID (26 characters)
	if len(capturedID) != 26 {
		t.Errorf("expected generated ULID of length 26, got %d: %s", len(capturedID), capturedID)
	}

	// Should set response header with generated value
	responseID := rec.Header().Get(RequestIDHeader)
	if responseID != capturedID {
		t.Errorf("expected response header to match context ID, got %q vs %q", responseID, capturedID)
	}
}

func TestRequestIDMiddleware_SetsResponseHeader(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := RequestIDMiddleware(handler)

	// Test 1: Without incoming header - should generate and set
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec1 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec1, req1)

	if rec1.Header().Get(RequestIDHeader) == "" {
		t.Error("expected X-Request-ID response header to be set")
	}

	// Test 2: With incoming header - should preserve and set
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set(RequestIDHeader, "existing-id")
	rec2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec2, req2)

	if rec2.Header().Get(RequestIDHeader) != "existing-id" {
		t.Errorf("expected X-Request-ID 'existing-id', got %q", rec2.Header().Get(RequestIDHeader))
	}
}
