package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNewBasicStrategy(t *testing.T) {
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

	tests := []struct {
		name      string
		cfg       *BasicConfig
		envSetup  map[string]string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "valid literal credentials",
			cfg:      &BasicConfig{Username: "user", Password: "pass"},
			envSetup: map[string]string{},
			wantErr:  false,
		},
		{
			name:     "valid env var credentials",
			cfg:      &BasicConfig{Username: "${BASIC_USER}", Password: "${BASIC_PASS}"},
			envSetup: map[string]string{"BASIC_USER": "admin", "BASIC_PASS": "secret"},
			wantErr:  false,
		},
		{
			name:      "missing username env var",
			cfg:       &BasicConfig{Username: "${MISSING_USER}", Password: "pass"},
			envSetup:  map[string]string{},
			wantErr:   true,
			errSubstr: "basic auth username",
		},
		{
			name:      "missing password env var",
			cfg:       &BasicConfig{Username: "user", Password: "${MISSING_PASS}"},
			envSetup:  map[string]string{},
			wantErr:   true,
			errSubstr: "basic auth password",
		},
		{
			name:      "empty username env var",
			cfg:       &BasicConfig{Username: "${EMPTY_USER}", Password: "pass"},
			envSetup:  map[string]string{"EMPTY_USER": ""},
			wantErr:   true,
			errSubstr: "basic auth username",
		},
		{
			name:      "empty password env var",
			cfg:       &BasicConfig{Username: "user", Password: "${EMPTY_PASS}"},
			envSetup:  map[string]string{"EMPTY_PASS": ""},
			wantErr:   true,
			errSubstr: "basic auth password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envSetup {
				os.Setenv(k, v)
			}

			strategy, err := NewBasicStrategy(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("NewBasicStrategy() expected error, got nil")
				} else if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("NewBasicStrategy() error = %v, want substring %q", err, tt.errSubstr)
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

	tests := []struct {
		name         string
		cfg          *BasicConfig
		envSetup     map[string]string
		wantUsername string
		wantPassword string
	}{
		{
			name:         "basic auth with literal credentials",
			cfg:          &BasicConfig{Username: "user", Password: "pass"},
			envSetup:     map[string]string{},
			wantUsername: "user",
			wantPassword: "pass",
		},
		{
			name:         "basic auth with env var credentials",
			cfg:          &BasicConfig{Username: "${USER}", Password: "${PASS}"},
			envSetup:     map[string]string{"USER": "admin", "PASS": "secret123"},
			wantUsername: "admin",
			wantPassword: "secret123",
		},
		{
			name:         "basic auth with special characters",
			cfg:          &BasicConfig{Username: "user@domain.com", Password: "p@ss:word!"},
			envSetup:     map[string]string{},
			wantUsername: "user@domain.com",
			wantPassword: "p@ss:word!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envSetup {
				os.Setenv(k, v)
			}

			strategy, err := NewBasicStrategy(tt.cfg)
			if err != nil {
				t.Fatalf("NewBasicStrategy() error: %v", err)
			}

			// Track what the RoundTripper received
			var receivedReq *http.Request
			mockRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				receivedReq = req
				return &http.Response{StatusCode: 200}, nil
			})

			rt := strategy.RoundTripper(mockRT)

			// Create original request
			originalReq := httptest.NewRequest("GET", "http://example.com", nil)

			// Execute
			_, err = rt.RoundTrip(originalReq)
			if err != nil {
				t.Fatalf("RoundTrip() error: %v", err)
			}

			// Verify Authorization header format: "Basic base64(user:pass)"
			authHeader := receivedReq.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Basic ") {
				t.Errorf("Authorization header = %q, want prefix 'Basic '", authHeader)
				return
			}

			// Decode and verify credentials
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

	os.Clearenv()

	strategy, err := NewBasicStrategy(&BasicConfig{Username: "user", Password: "pass"})
	if err != nil {
		t.Fatalf("NewBasicStrategy() error: %v", err)
	}

	mockRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})

	rt := strategy.RoundTripper(mockRT)

	// Create original request WITHOUT Authorization header
	originalReq := httptest.NewRequest("GET", "http://example.com", nil)
	originalAuthValue := originalReq.Header.Get("Authorization")

	// Execute
	_, err = rt.RoundTrip(originalReq)
	if err != nil {
		t.Fatalf("RoundTrip() error: %v", err)
	}

	// Verify original request was NOT modified (clone was used)
	afterAuthValue := originalReq.Header.Get("Authorization")
	if afterAuthValue != originalAuthValue {
		t.Errorf("Original request was modified: Authorization header changed from %q to %q",
			originalAuthValue, afterAuthValue)
	}
}
