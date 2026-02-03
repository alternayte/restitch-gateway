package auth

import (
	"errors"
	"net/http"
)

// ErrMissingAuthHeader is returned when passthrough auth is configured
// but the client didn't provide an Authorization header.
var ErrMissingAuthHeader = errors.New("passthrough auth requires Authorization header from client")

// PassthroughStrategy implements authentication by forwarding the client's
// Authorization header verbatim to the upstream service.
// This strategy does NOT inject credentials - it forwards them.
//
// Per CONTEXT.md and RESEARCH.md security best practices:
//   - Returns ErrMissingAuthHeader if client sends no Authorization header
//   - Does NOT forward unauthenticated requests to upstream
//   - Forwards header verbatim when present ("Bearer xyz" -> "Bearer xyz")
type PassthroughStrategy struct{}

// NewPassthroughStrategy creates a passthrough strategy that forwards
// the client's Authorization header to upstreams.
func NewPassthroughStrategy(cfg *PassthroughConfig) (*PassthroughStrategy, error) {
	// No config to validate for passthrough
	return &PassthroughStrategy{}, nil
}

// RoundTripper returns an http.RoundTripper that forwards the client's
// Authorization header to the upstream.
func (s *PassthroughStrategy) RoundTripper(base http.RoundTripper) http.RoundTripper {
	return &passthroughRoundTripper{base: base}
}

// passthroughRoundTripper wraps a base RoundTripper to forward the client's
// Authorization header.
type passthroughRoundTripper struct {
	base http.RoundTripper
}

// RoundTrip implements http.RoundTripper by checking for and forwarding
// the Authorization header from the client request.
// CRITICAL: Returns ErrMissingAuthHeader if client didn't provide auth.
func (rt *passthroughRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Check if client provided Authorization header
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		// Per CONTEXT.md/RESEARCH.md: fail immediately, don't forward unauthenticated
		// The composition handler will catch this error and return 401 with
		// WWW-Authenticate header to the client.
		return nil, ErrMissingAuthHeader
	}

	// Authorization header exists - forward verbatim
	// Clone request to be consistent with other strategies (per RESEARCH.md Pitfall 3)
	reqCopy := req.Clone(req.Context())
	// Header already present from original request and survives clone.
	// Explicitly set it to ensure it's in the copy.
	reqCopy.Header.Set("Authorization", authHeader)

	return rt.base.RoundTrip(reqCopy)
}

// IsMissingAuthHeaderError returns true if the error indicates
// the client didn't provide required authentication.
// Use this in handler layers to return 401 Unauthorized with
// WWW-Authenticate header.
func IsMissingAuthHeaderError(err error) bool {
	return errors.Is(err, ErrMissingAuthHeader)
}
