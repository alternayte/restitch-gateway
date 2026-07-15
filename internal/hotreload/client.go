package hotreload

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type FetchResult struct {
	YAML             []byte
	ETag             string
	CompositionCount int
	NotModified      bool
}

type ClientOption func(*RegistryClient)

func WithAdminKey(key string) ClientOption {
	return func(c *RegistryClient) { c.adminKey = key }
}

func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *RegistryClient) { c.httpClient = hc }
}

type RegistryClient struct {
	url        string
	httpClient *http.Client
	adminKey   string
}

func NewRegistryClient(url string, opts ...ClientOption) *RegistryClient {
	c := &RegistryClient{
		url:        url,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *RegistryClient) Fetch(ctx context.Context, lastETag string) (*FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/api/v1/registry/bundle", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if lastETag != "" {
		req.Header.Set("If-None-Match", lastETag)
	}
	if c.adminKey != "" {
		req.Header.Set("X-Admin-Key", c.adminKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch bundle: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return &FetchResult{
			NotModified: true,
			ETag:        resp.Header.Get("ETag"),
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	var bundle struct {
		YAMLContent      string   `json:"yaml_content"`
		ETag             string   `json:"etag"`
		CompositionCount int      `json:"composition_count"`
		CompositionNames []string `json:"composition_names"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&bundle); err != nil {
		return nil, fmt.Errorf("decode bundle: %w", err)
	}

	etag := resp.Header.Get("ETag")
	if etag == "" {
		etag = bundle.ETag
	}

	return &FetchResult{
		YAML:             []byte(bundle.YAMLContent),
		ETag:             etag,
		CompositionCount: bundle.CompositionCount,
	}, nil
}
