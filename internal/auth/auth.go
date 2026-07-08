// Package auth provides authentication strategies for upstream services.
//
// The auth package implements a Strategy interface that allows different
// authentication methods to be configured per-upstream in YAML configuration.
// Supported strategies include:
//   - Header: Static header injection (e.g., API keys)
//   - Basic: HTTP Basic Authentication
//   - Passthrough: Forward client's Authorization header
//   - OAuth2: OAuth2 client credentials flow
//
// All strategies are configured at startup via YAML, with secrets referenced
// using ${VAR_NAME} environment variable syntax.
//
// # Passthrough Error Handling
//
// When using passthrough authentication, the client MUST include an Authorization
// header. If missing, the RoundTripper returns ErrMissingAuthHeader.
//
// The composition handler should catch this error using IsMissingAuthHeaderError
// and return:
//   - Status: 401 Unauthorized
//   - Header: WWW-Authenticate: Bearer (or appropriate scheme)
//   - Body: {"error": "authorization header required"}
//
// This is a security best practice - don't forward unauthenticated requests
// to upstreams that expect authentication.
//
// Example error handling in handler:
//
//	if auth.IsMissingAuthHeaderError(err) {
//	    w.Header().Set("WWW-Authenticate", "Bearer")
//	    http.Error(w, `{"error":"authorization header required"}`, 401)
//	    return
//	}
package auth

import (
	"context"
	"errors"
	"net/http"
)

// Strategy represents an upstream authentication strategy.
// Each strategy wraps a base RoundTripper to inject authentication
// headers into outgoing requests.
type Strategy interface {
	// RoundTripper returns an http.RoundTripper that injects authentication.
	// The base RoundTripper is typically http.DefaultTransport or a custom
	// transport with connection pooling.
	RoundTripper(base http.RoundTripper) http.RoundTripper
}

// Config represents auth configuration from YAML.
// Exactly one strategy per upstream - strategies are mutually exclusive.
type Config struct {
	Header      *HeaderConfig      `yaml:"header"`
	Basic       *BasicConfig       `yaml:"basic"`
	Passthrough *PassthroughConfig `yaml:"passthrough"`
	OAuth2      *OAuth2Config      `yaml:"oauth2"`
}

// Validate ensures exactly zero or one strategy is configured.
// Multiple strategies are not allowed per upstream.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}

	count := 0
	if c.Header != nil {
		count++
	}
	if c.Basic != nil {
		count++
	}
	if c.Passthrough != nil {
		count++
	}
	if c.OAuth2 != nil {
		count++
	}

	if count > 1 {
		return errors.New("exactly one auth strategy allowed per upstream, found multiple")
	}

	return nil
}

// HeaderConfig for static header injection (AUTH-01).
// Injects a fixed header value into every request to the upstream.
// Value supports ${VAR} syntax for environment variable expansion.
type HeaderConfig struct {
	Name  string `yaml:"name"`  // Header name (e.g., "X-API-Key", "Authorization")
	Value string `yaml:"value"` // Header value with optional ${VAR} syntax
}

// BasicConfig for HTTP Basic Auth (AUTH-02).
// Credentials support ${VAR} syntax for environment variable expansion.
type BasicConfig struct {
	Username string `yaml:"username"` // Username with optional ${VAR} syntax
	Password string `yaml:"password"` // Password with optional ${VAR} syntax
}

// PassthroughConfig for forwarding client Authorization header (AUTH-03).
// Forwards the client's Authorization header verbatim to the upstream.
// No transformation is applied (e.g., "Bearer xyz" stays "Bearer xyz").
type PassthroughConfig struct {
	// No fields - presence of this config indicates passthrough mode.
	// The Authorization header from the incoming request will be forwarded.
}

// OAuth2Config for OAuth2 client credentials flow (AUTH-04, AUTH-05).
// Automatically fetches and refreshes tokens using client credentials grant.
// ClientID and ClientSecret support ${VAR} syntax for environment variable expansion.
type OAuth2Config struct {
	TokenURL     string   `yaml:"token_url"`     // Token endpoint URL
	ClientID     string   `yaml:"client_id"`     // Client ID with optional ${VAR} syntax
	ClientSecret string   `yaml:"client_secret"` // Client Secret with optional ${VAR} syntax
	Scopes       []string `yaml:"scopes"`        // Optional scopes to request
}

// Build creates the appropriate Strategy based on which config option is set.
// Returns nil if no auth is configured.
// Returns an error if environment variable expansion fails or credentials are invalid.
func (c *Config) Build(ctx context.Context) (Strategy, error) {
	if c == nil {
		return nil, nil
	}

	switch {
	case c.Header != nil:
		return NewHeaderStrategy(c.Header)
	case c.Basic != nil:
		return NewBasicStrategy(c.Basic)
	case c.Passthrough != nil:
		return NewPassthroughStrategy(c.Passthrough)
	case c.OAuth2 != nil:
		return NewOAuth2Strategy(ctx, c.OAuth2)
	default:
		return nil, nil // No auth configured
	}
}
