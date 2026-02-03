// Package client provides an HTTP client with proper connection pooling
// for making requests to upstream services.
//
// CRITICAL: This client is configured with MaxIdleConnsPerHost: 100 to avoid
// the default value of 2, which causes 4-5x latency penalty under load.
//
// Usage:
//
//	c := client.New()
//	resp, err := c.Get(ctx, "https://api.example.com/data")
//	if err != nil {
//	    return err
//	}
//	defer client.DrainAndClose(resp.Body)
//	// read response body...
//
// IMPORTANT: Callers MUST call DrainAndClose(resp.Body) after reading the
// response to properly return the connection to the pool.
package client

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"time"
)

// Client wraps http.Client with proper configuration for upstream requests.
// It is safe for concurrent use and should be shared across goroutines.
type Client struct {
	httpClient *http.Client
}

// New creates a new Client with optimized connection pooling settings.
// The client is configured with:
//   - MaxIdleConns: 100
//   - MaxIdleConnsPerHost: 100 (CRITICAL: default is 2)
//   - IdleConnTimeout: 90s
//   - TLS 1.2+ for HTTPS upstreams
//   - Default timeout: 30s (can be overridden per-request via context)
func New() *Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100, // CRITICAL: default is 2, causes 4-5x latency penalty
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
		// TLS config for HTTPS upstreams
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second, // Default timeout, overridden per-request via context
		},
	}
}

// Do executes an HTTP request with the given context.
// The context is used for cancellation and deadline propagation.
//
// IMPORTANT: Caller MUST call DrainAndClose(resp.Body) after reading the response.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)
	return c.httpClient.Do(req)
}

// Get performs an HTTP GET request to the specified URL.
// The context is used for cancellation and deadline propagation.
//
// IMPORTANT: Caller MUST call DrainAndClose(resp.Body) after reading the response.
func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

// Transport returns the underlying http.Transport for inspection or testing.
func (c *Client) Transport() *http.Transport {
	return c.httpClient.Transport.(*http.Transport)
}

// HTTPClient returns the underlying http.Client.
// This is useful when a component needs the standard http.Client interface.
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

// DrainAndClose properly drains and closes a response body to return the
// connection to the pool. This MUST be called after reading the response,
// typically in a defer statement.
//
// Example:
//
//	resp, err := client.Get(ctx, url)
//	if err != nil {
//	    return err
//	}
//	defer client.DrainAndClose(resp.Body)
//
// It is safe to call with a nil body.
func DrainAndClose(body io.ReadCloser) {
	if body != nil {
		// Drain any remaining data to allow connection reuse
		_, _ = io.Copy(io.Discard, body)
		_ = body.Close()
	}
}
