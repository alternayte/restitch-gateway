package auth

import (
	"fmt"
	"net/http"

	"github.com/restitch/restitch-gateway/internal/gwconfig"
)

// BasicStrategy implements HTTP Basic Authentication for upstream services.
// It uses the standard SetBasicAuth method to encode username:password
// as a base64 Authorization header. Credentials support ${VAR} syntax
// for environment variable expansion.
type BasicStrategy struct {
	username string
	password string
}

// NewBasicStrategy creates a basic auth strategy with the given credentials.
// Both username and password are expanded using environment variables at creation time.
// Returns an error if referenced environment variables are missing or empty.
func NewBasicStrategy(cfg *BasicConfig) (*BasicStrategy, error) {
	// Expand environment variables with validation
	username, err := gwconfig.ExpandEnvStrict(cfg.Username)
	if err != nil {
		return nil, fmt.Errorf("basic auth username: %w", err)
	}
	password, err := gwconfig.ExpandEnvStrict(cfg.Password)
	if err != nil {
		return nil, fmt.Errorf("basic auth password: %w", err)
	}
	return &BasicStrategy{username: username, password: password}, nil
}

// RoundTripper returns an http.RoundTripper that injects Basic auth header.
func (s *BasicStrategy) RoundTripper(base http.RoundTripper) http.RoundTripper {
	return &basicRoundTripper{base: base, username: s.username, password: s.password}
}

// basicRoundTripper wraps a base RoundTripper to inject Basic authentication.
type basicRoundTripper struct {
	base     http.RoundTripper
	username string
	password string
}

// RoundTrip implements http.RoundTripper by cloning the request and adding Basic auth.
// CRITICAL: Request is cloned to avoid modifying the original (safe for retries).
// Uses stdlib SetBasicAuth per RESEARCH.md "Don't Hand-Roll" section.
func (rt *basicRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid modifying original (per RESEARCH.md Pitfall 3)
	reqCopy := req.Clone(req.Context())
	// Use stdlib SetBasicAuth - never hand-roll base64 encoding
	reqCopy.SetBasicAuth(rt.username, rt.password)
	return rt.base.RoundTrip(reqCopy)
}
