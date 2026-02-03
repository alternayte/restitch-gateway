package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"golang.org/x/sync/singleflight"

	"github.com/restitch/restitch-gateway/internal/config"
)

// oauth2ExpiryDelta is the buffer before token expiry to trigger refresh.
// Per CONTEXT.md: "Refresh 30 seconds before expiry (fixed buffer, not percentage)"
const oauth2ExpiryDelta = 30 * time.Second

// OAuth2Strategy implements OAuth2 client credentials authentication for upstream services.
// It automatically fetches and refreshes access tokens, injecting them as Bearer tokens.
// The strategy uses singleflight to prevent thundering herd during token refresh.
type OAuth2Strategy struct {
	tokenSource oauth2.TokenSource
	group       *singleflight.Group
}

// NewOAuth2Strategy creates an OAuth2 client credentials strategy.
// It immediately fetches an initial token to fail fast on configuration errors.
// ClientID, ClientSecret, and TokenURL support ${VAR} syntax for environment variable expansion.
// Returns an error if environment variables are missing/empty or initial token fetch fails.
func NewOAuth2Strategy(ctx context.Context, cfg *OAuth2Config) (*OAuth2Strategy, error) {
	// Expand environment variables with validation
	clientID, err := config.ExpandEnvWithValidation(cfg.ClientID)
	if err != nil {
		return nil, fmt.Errorf("oauth2 client_id: %w", err)
	}
	clientSecret, err := config.ExpandEnvWithValidation(cfg.ClientSecret)
	if err != nil {
		return nil, fmt.Errorf("oauth2 client_secret: %w", err)
	}
	// Token URL typically doesn't contain secrets, but support ${VAR} for flexibility
	tokenURL, err := config.ExpandEnvWithValidation(cfg.TokenURL)
	if err != nil {
		return nil, fmt.Errorf("oauth2 token_url: %w", err)
	}

	// Create client credentials config
	ccConfig := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
		Scopes:       cfg.Scopes,
	}

	// Get initial token at startup (fail-fast per CONTEXT.md)
	// Use context with timeout for initial fetch
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	token, err := ccConfig.Token(fetchCtx)
	if err != nil {
		return nil, fmt.Errorf("initial oauth2 token fetch failed: %w", err)
	}

	// Create token source with 30-second early refresh buffer
	// Per RESEARCH.md: Use ReuseTokenSourceWithExpiry for configurable buffer
	baseSource := ccConfig.TokenSource(ctx)
	tokenSource := oauth2.ReuseTokenSourceWithExpiry(token, baseSource, oauth2ExpiryDelta)

	return &OAuth2Strategy{
		tokenSource: tokenSource,
		group:       &singleflight.Group{},
	}, nil
}

// RoundTripper returns an http.RoundTripper that injects OAuth2 Bearer tokens.
func (s *OAuth2Strategy) RoundTripper(base http.RoundTripper) http.RoundTripper {
	return &oauth2RoundTripper{
		base:        base,
		tokenSource: s.tokenSource,
		group:       s.group,
	}
}

// oauth2RoundTripper wraps a base RoundTripper to inject OAuth2 Bearer tokens.
type oauth2RoundTripper struct {
	base        http.RoundTripper
	tokenSource oauth2.TokenSource
	group       *singleflight.Group
}

// RoundTrip implements http.RoundTripper by fetching/refreshing the token and injecting it.
// Uses singleflight to prevent thundering herd during concurrent token refresh.
// CRITICAL: Request is cloned to avoid modifying the original (safe for retries).
func (rt *oauth2RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Get token using singleflight to prevent thundering herd
	// Per RESEARCH.md Pitfall 2: Multiple goroutines refreshing same token
	tokenVal, err, _ := rt.group.Do("token", func() (interface{}, error) {
		return rt.tokenSource.Token()
	})
	if err != nil {
		// Per CONTEXT.md: "Gateway auth failures return 502 Bad Gateway"
		// The handler layer will catch this and return 502
		return nil, fmt.Errorf("oauth2 token refresh failed: %w", err)
	}

	token := tokenVal.(*oauth2.Token)

	// Clone request and inject token (per RESEARCH.md Pitfall 3)
	reqCopy := req.Clone(req.Context())
	reqCopy.Header.Set("Authorization", token.Type()+" "+token.AccessToken)

	return rt.base.RoundTrip(reqCopy)
}
