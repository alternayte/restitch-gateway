package auth

import (
	"fmt"
	"net/http"

	"github.com/restitch/restitch-gateway/internal/gwconfig"
)

// HeaderStrategy implements static header injection for upstream authentication.
// It injects a configured header name/value pair into every request to the upstream.
// The header value supports ${VAR} syntax for environment variable expansion.
type HeaderStrategy struct {
	name  string
	value string
}

// NewHeaderStrategy creates a header strategy that injects a static header.
// The header value is expanded using environment variables at creation time.
// Returns an error if referenced environment variables are missing or empty.
func NewHeaderStrategy(cfg *HeaderConfig) (*HeaderStrategy, error) {
	// Expand environment variables with validation
	value, err := gwconfig.ExpandEnvStrict(cfg.Value)
	if err != nil {
		return nil, fmt.Errorf("header auth value: %w", err)
	}
	return &HeaderStrategy{name: cfg.Name, value: value}, nil
}

// RoundTripper returns an http.RoundTripper that injects the configured header.
func (s *HeaderStrategy) RoundTripper(base http.RoundTripper) http.RoundTripper {
	return &headerRoundTripper{base: base, name: s.name, value: s.value}
}

// headerRoundTripper wraps a base RoundTripper to inject the configured header.
type headerRoundTripper struct {
	base  http.RoundTripper
	name  string
	value string
}

// RoundTrip implements http.RoundTripper by cloning the request and adding the header.
// CRITICAL: Request is cloned to avoid modifying the original (safe for retries).
func (rt *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid modifying original (per RESEARCH.md Pitfall 3)
	reqCopy := req.Clone(req.Context())
	reqCopy.Header.Set(rt.name, rt.value)
	return rt.base.RoundTrip(reqCopy)
}
