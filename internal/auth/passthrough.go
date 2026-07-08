package auth

import (
	"context"
	"errors"
	"net/http"
)

// ErrMissingAuthHeader is returned when passthrough auth is configured
// but the client didn't provide an Authorization header.
var ErrMissingAuthHeader = errors.New("passthrough auth requires Authorization header from client")

type clientAuthKey struct{}

// WithClientAuthorization stores the inbound Authorization header value in the context.
func WithClientAuthorization(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, clientAuthKey{}, value)
}

func clientAuthorization(ctx context.Context) string {
	if v, ok := ctx.Value(clientAuthKey{}).(string); ok {
		return v
	}
	return ""
}

// PassthroughStrategy implements authentication by forwarding the client's
// Authorization header (read from context) to the upstream service.
type PassthroughStrategy struct{}

// NewPassthroughStrategy creates a passthrough strategy.
func NewPassthroughStrategy(cfg *PassthroughConfig) (*PassthroughStrategy, error) {
	return &PassthroughStrategy{}, nil
}

// RoundTripper returns an http.RoundTripper that forwards the client's
// Authorization header from context to the upstream.
func (s *PassthroughStrategy) RoundTripper(base http.RoundTripper) http.RoundTripper {
	return &passthroughRoundTripper{base: base}
}

type passthroughRoundTripper struct {
	base http.RoundTripper
}

// RoundTrip reads Authorization from request context (not headers).
func (rt *passthroughRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	authValue := clientAuthorization(req.Context())
	if authValue == "" {
		return nil, ErrMissingAuthHeader
	}

	reqCopy := req.Clone(req.Context())
	reqCopy.Header.Set("Authorization", authValue)
	return rt.base.RoundTrip(reqCopy)
}

// IsMissingAuthHeaderError returns true if the error indicates
// the client didn't provide required authentication.
func IsMissingAuthHeaderError(err error) bool {
	return errors.Is(err, ErrMissingAuthHeader)
}
