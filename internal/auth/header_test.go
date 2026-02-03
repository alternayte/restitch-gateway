package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNewHeaderStrategy(t *testing.T) {
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
		cfg       *HeaderConfig
		envSetup  map[string]string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "valid literal value",
			cfg:      &HeaderConfig{Name: "X-API-Key", Value: "secret123"},
			envSetup: map[string]string{},
			wantErr:  false,
		},
		{
			name:     "valid env var value",
			cfg:      &HeaderConfig{Name: "Authorization", Value: "Bearer ${API_TOKEN}"},
			envSetup: map[string]string{"API_TOKEN": "abc123"},
			wantErr:  false,
		},
		{
			name:      "missing env var",
			cfg:       &HeaderConfig{Name: "X-API-Key", Value: "${MISSING_KEY}"},
			envSetup:  map[string]string{},
			wantErr:   true,
			errSubstr: "header auth value",
		},
		{
			name:      "empty env var",
			cfg:       &HeaderConfig{Name: "X-API-Key", Value: "${EMPTY_KEY}"},
			envSetup:  map[string]string{"EMPTY_KEY": ""},
			wantErr:   true,
			errSubstr: "header auth value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envSetup {
				os.Setenv(k, v)
			}

			strategy, err := NewHeaderStrategy(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Error("NewHeaderStrategy() expected error, got nil")
				} else if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("NewHeaderStrategy() error = %v, want substring %q", err, tt.errSubstr)
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
		name          string
		cfg           *HeaderConfig
		envSetup      map[string]string
		wantHeader    string
		wantValue     string
		originalValue string // value to set on original request
	}{
		{
			name:       "injects header with literal value",
			cfg:        &HeaderConfig{Name: "X-API-Key", Value: "secret123"},
			envSetup:   map[string]string{},
			wantHeader: "X-API-Key",
			wantValue:  "secret123",
		},
		{
			name:       "injects header with expanded env var",
			cfg:        &HeaderConfig{Name: "Authorization", Value: "Bearer ${TOKEN}"},
			envSetup:   map[string]string{"TOKEN": "mytoken"},
			wantHeader: "Authorization",
			wantValue:  "Bearer mytoken",
		},
		{
			name:          "overwrites existing header",
			cfg:           &HeaderConfig{Name: "X-API-Key", Value: "new-value"},
			envSetup:      map[string]string{},
			wantHeader:    "X-API-Key",
			wantValue:     "new-value",
			originalValue: "old-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envSetup {
				os.Setenv(k, v)
			}

			strategy, err := NewHeaderStrategy(tt.cfg)
			if err != nil {
				t.Fatalf("NewHeaderStrategy() error: %v", err)
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
			if tt.originalValue != "" {
				originalReq.Header.Set(tt.wantHeader, tt.originalValue)
			}

			// Execute
			_, err = rt.RoundTrip(originalReq)
			if err != nil {
				t.Fatalf("RoundTrip() error: %v", err)
			}

			// Verify header was injected
			gotValue := receivedReq.Header.Get(tt.wantHeader)
			if gotValue != tt.wantValue {
				t.Errorf("Header %q = %q, want %q", tt.wantHeader, gotValue, tt.wantValue)
			}
		})
	}
}

func TestHeaderStrategy_OriginalRequestUnmodified(t *testing.T) {
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

	strategy, err := NewHeaderStrategy(&HeaderConfig{Name: "X-API-Key", Value: "secret"})
	if err != nil {
		t.Fatalf("NewHeaderStrategy() error: %v", err)
	}

	mockRT := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200}, nil
	})

	rt := strategy.RoundTripper(mockRT)

	// Create original request WITHOUT the header
	originalReq := httptest.NewRequest("GET", "http://example.com", nil)
	originalHeaderValue := originalReq.Header.Get("X-API-Key")

	// Execute
	_, err = rt.RoundTrip(originalReq)
	if err != nil {
		t.Fatalf("RoundTrip() error: %v", err)
	}

	// Verify original request was NOT modified (clone was used)
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
