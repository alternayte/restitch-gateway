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
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewBasicStrategy(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *BasicConfig
		wantErr bool
	}{
		{
			name:    "valid credentials",
			cfg:     &BasicConfig{Username: "user", Password: "pass"},
			wantErr: false,
		},
		{
			name:    "pre-expanded env var credentials",
			cfg:     &BasicConfig{Username: "admin", Password: "secret"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy, err := NewBasicStrategy(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("NewBasicStrategy() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("NewBasicStrategy() unexpected error: %v", err)
				return
			}

			if strategy == nil {
				t.Error("NewBasicStrategy() returned nil strategy")
			}
		})
	}
}

func TestBasicStrategy_RoundTripper(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *BasicConfig
		wantUsername string
		wantPassword string
	}{
		{
			name:         "basic auth with literal credentials",
			cfg:          &BasicConfig{Username: "user", Password: "pass"},
			wantUsername: "user",
			wantPassword: "pass",
		},
		{
			name:         "basic auth with special characters",
			cfg:          &BasicConfig{Username: "user@domain.com", Password: "p@ss:word!"},
			wantUsername: "user@domain.com",
			wantPassword: "p@ss:word!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy, err := NewBasicStrategy(tt.cfg)
			if err != nil {
				t.Fatalf("NewBasicStrategy() error: %v", err)
			}

			var receivedReq *http.Request
			mockRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				receivedReq = req
				return &http.Response{StatusCode: 200}, nil
			})

			rt := strategy.RoundTripper(mockRT)
			originalReq := httptest.NewRequest("GET", "http://example.com", nil)

			_, err = rt.RoundTrip(originalReq)
			if err != nil {
				t.Fatalf("RoundTrip() error: %v", err)
			}

			authHeader := receivedReq.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Basic ") {
				t.Errorf("Authorization header = %q, want prefix 'Basic '", authHeader)
				return
			}

			encoded := strings.TrimPrefix(authHeader, "Basic ")
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Errorf("Failed to decode base64: %v", err)
				return
			}

			expected := tt.wantUsername + ":" + tt.wantPassword
			if string(decoded) != expected {
				t.Errorf("Decoded credentials = %q, want %q", string(decoded), expected)
			}
		})
	}
}

func TestBasicStrategy_OriginalRequestUnmodified(t *testing.T) {
	strategy, err := NewBasicStrategy(&BasicConfig{Username: "user", Password: "pass"})
	if err != nil {
		t.Fatalf("NewBasicStrategy() error: %v", err)
	}

	mockRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})

	rt := strategy.RoundTripper(mockRT)
	originalReq := httptest.NewRequest("GET", "http://example.com", nil)
	originalAuthValue := originalReq.Header.Get("Authorization")

	_, err = rt.RoundTrip(originalReq)
	if err != nil {
		t.Fatalf("RoundTrip() error: %v", err)
	}

	afterAuthValue := originalReq.Header.Get("Authorization")
	if afterAuthValue != originalAuthValue {
		t.Errorf("Original request was modified: Authorization header changed from %q to %q",
			originalAuthValue, afterAuthValue)
	}
}
