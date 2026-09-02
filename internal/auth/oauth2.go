package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const oauth2ExpiryDelta = 30 * time.Second

// OAuth2Strategy implements OAuth2 client credentials authentication.
type OAuth2Strategy struct {
	tokenSource oauth2.TokenSource
}

// NewOAuth2Strategy creates an OAuth2 client credentials strategy.
// Uses a dedicated refresh client with 10s timeout to bound hung IdP calls.
func NewOAuth2Strategy(ctx context.Context, cfg *OAuth2Config) (*OAuth2Strategy, error) {
	ccConfig := &clientcredentials.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		TokenURL:     cfg.TokenURL,
		Scopes:       cfg.Scopes,
	}

	// Dedicated refresh client with bounded timeout (D6 fix)
	refreshClient := &http.Client{Timeout: 10 * time.Second}
	tsCtx := context.WithValue(context.Background(), oauth2.HTTPClient, refreshClient)

	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	token, err := ccConfig.Token(fetchCtx)
	if err != nil {
		return nil, fmt.Errorf("initial oauth2 token fetch failed: %w", err)
	}

	// ReuseTokenSourceWithExpiry serializes refresh internally — no singleflight needed
	baseSource := ccConfig.TokenSource(tsCtx)
	tokenSource := oauth2.ReuseTokenSourceWithExpiry(token, baseSource, oauth2ExpiryDelta)

	return &OAuth2Strategy{
		tokenSource: tokenSource,
	}, nil
}

// NewOAuth2StrategyValidateOnly validates OAuth2 config fields without fetching a token.
func NewOAuth2StrategyValidateOnly(cfg *OAuth2Config) (*OAuth2Strategy, error) {
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("oauth2 client_id is required")
	}
	if cfg.ClientSecret == "" {
		return nil, fmt.Errorf("oauth2 client_secret is required")
	}
	if cfg.TokenURL == "" {
		return nil, fmt.Errorf("oauth2 token_url is required")
	}
	// A nil tokenSource would panic in RoundTrip if this strategy ever
	// reached it; store a failing source instead (finding M10).
	return &OAuth2Strategy{
		tokenSource: failingTokenSource{err: fmt.Errorf("oauth2 validate-only strategy has no token source")},
	}, nil
}

// failingTokenSource always returns its error. It stands in for a real token
// source in strategies that must never perform a request.
type failingTokenSource struct {
	err error
}

func (s failingTokenSource) Token() (*oauth2.Token, error) {
	return nil, s.err
}

// RoundTripper returns an http.RoundTripper that injects OAuth2 Bearer tokens.
func (s *OAuth2Strategy) RoundTripper(base http.RoundTripper) http.RoundTripper {
	return &oauth2RoundTripper{
		base:        base,
		tokenSource: s.tokenSource,
	}
}

type oauth2RoundTripper struct {
	base        http.RoundTripper
	tokenSource oauth2.TokenSource
}

func (rt *oauth2RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := rt.tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("oauth2 token refresh failed: %w", err)
	}

	reqCopy := req.Clone(req.Context())
	reqCopy.Header.Set("Authorization", token.Type()+" "+token.AccessToken)
	return rt.base.RoundTrip(reqCopy)
}
