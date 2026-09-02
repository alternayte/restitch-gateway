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
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewHeaderStrategy(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *HeaderConfig
		wantErr bool
	}{
		{
			name:    "valid literal value",
			cfg:     &HeaderConfig{Name: "X-API-Key", Value: "secret123"},
			wantErr: false,
		},
		{
			name:    "pre-expanded value",
			cfg:     &HeaderConfig{Name: "Authorization", Value: "Bearer abc123"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy, err := NewHeaderStrategy(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("NewHeaderStrategy() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("NewHeaderStrategy() unexpected error: %v", err)
				return
			}

			if strategy == nil {
				t.Error("NewHeaderStrategy() returned nil strategy")
			}
		})
	}
}

func TestHeaderStrategy_RoundTripper(t *testing.T) {
	tests := []struct {
		name          string
		cfg           *HeaderConfig
		wantHeader    string
		wantValue     string
		originalValue string
	}{
		{
			name:       "injects header with literal value",
			cfg:        &HeaderConfig{Name: "X-API-Key", Value: "secret123"},
			wantHeader: "X-API-Key",
			wantValue:  "secret123",
		},
		{
			name:       "injects header with pre-expanded value",
			cfg:        &HeaderConfig{Name: "Authorization", Value: "Bearer mytoken"},
			wantHeader: "Authorization",
			wantValue:  "Bearer mytoken",
		},
		{
			name:          "overwrites existing header",
			cfg:           &HeaderConfig{Name: "X-API-Key", Value: "new-value"},
			wantHeader:    "X-API-Key",
			wantValue:     "new-value",
			originalValue: "old-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy, err := NewHeaderStrategy(tt.cfg)
			if err != nil {
				t.Fatalf("NewHeaderStrategy() error: %v", err)
			}

			var receivedReq *http.Request
			mockRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				receivedReq = req
				return &http.Response{StatusCode: 200}, nil
			})

			rt := strategy.RoundTripper(mockRT)
			originalReq := httptest.NewRequest("GET", "http://example.com", nil)
			if tt.originalValue != "" {
				originalReq.Header.Set(tt.wantHeader, tt.originalValue)
			}

			_, err = rt.RoundTrip(originalReq)
			if err != nil {
				t.Fatalf("RoundTrip() error: %v", err)
			}

			gotValue := receivedReq.Header.Get(tt.wantHeader)
			if gotValue != tt.wantValue {
				t.Errorf("Header %q = %q, want %q", tt.wantHeader, gotValue, tt.wantValue)
			}
		})
	}
}

func TestHeaderStrategy_OriginalRequestUnmodified(t *testing.T) {
	strategy, err := NewHeaderStrategy(&HeaderConfig{Name: "X-API-Key", Value: "secret"})
	if err != nil {
		t.Fatalf("NewHeaderStrategy() error: %v", err)
	}

	mockRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})

	rt := strategy.RoundTripper(mockRT)
	originalReq := httptest.NewRequest("GET", "http://example.com", nil)
	originalHeaderValue := originalReq.Header.Get("X-API-Key")

	_, err = rt.RoundTrip(originalReq)
	if err != nil {
		t.Fatalf("RoundTrip() error: %v", err)
	}

	afterHeaderValue := originalReq.Header.Get("X-API-Key")
	if afterHeaderValue != originalHeaderValue {
		t.Errorf("Original request was modified: header X-API-Key changed from %q to %q",
			originalHeaderValue, afterHeaderValue)
	}
}

// roundTripperFunc is a helper to create RoundTripper from a function.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
