// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hotreload

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/alternayte/restitch-gateway/internal/upstream"
)

// maxBundleBytes bounds the registry bundle response body. A broken or
// malicious registry cannot exhaust gateway memory (finding H4).
const maxBundleBytes = 10 << 20 // 10 MiB

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
	defer upstream.DrainAndClose(resp.Body)

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
	// The bundle body is bounded so a broken or malicious registry cannot
	// exhaust gateway memory with an unbounded yaml_content (finding H4).
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBundleBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read bundle: %w", err)
	}
	if len(body) > maxBundleBytes {
		return nil, fmt.Errorf("bundle exceeds %d bytes", maxBundleBytes)
	}
	if err := json.Unmarshal(body, &bundle); err != nil {
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
