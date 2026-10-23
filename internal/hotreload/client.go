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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alternayte/restitch-gateway/v2/internal/composition"
	"github.com/alternayte/restitch-gateway/v2/internal/upstream"
)

// ErrNoVersion means the environment has no published version yet. The
// gateway keeps polling and stays not ready.
var ErrNoVersion = errors.New("the environment has no published version yet")

// ErrTokenRejected means Studio refused the gateway token.
var ErrTokenRejected = errors.New("studio rejected the gateway token")

// FetchResult is one pull of the environment's current artifact.
type FetchResult struct {
	YAML        []byte
	ETag        string
	Version     int
	NotModified bool
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithHTTPClient replaces the HTTP client (tests use it).
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) { c.httpClient = hc }
}

// Client pulls the current artifact version from Studio with a gateway
// token. The token names the environment.
type Client struct {
	url        string
	token      string
	httpClient *http.Client
}

// NewClient returns a client for the Studio at studioURL.
func NewClient(studioURL, token string, opts ...ClientOption) *Client {
	c := &Client{
		url:        strings.TrimRight(studioURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Fetch pulls the current version. lastETag makes an unchanged version
// answer NotModified without a body.
func (c *Client) Fetch(ctx context.Context, lastETag string) (*FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/gateway/v1/artifact", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if lastETag != "" {
		req.Header.Set("If-None-Match", lastETag)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pull artifact: %w", err)
	}
	defer upstream.DrainAndClose(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotModified:
		return &FetchResult{NotModified: true, ETag: resp.Header.Get("ETag")}, nil
	case http.StatusNotFound:
		return nil, ErrNoVersion
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrTokenRejected
	default:
		return nil, fmt.Errorf("studio returned status %d", resp.StatusCode)
	}

	// The body is bounded so a broken Studio cannot exhaust gateway memory.
	// Studio refuses to publish anything larger.
	body, err := io.ReadAll(io.LimitReader(resp.Body, composition.MaxArtifactBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read artifact: %w", err)
	}
	if len(body) > composition.MaxArtifactBytes {
		return nil, fmt.Errorf("artifact exceeds %d bytes", composition.MaxArtifactBytes)
	}
	version, _ := strconv.Atoi(resp.Header.Get("Restitch-Artifact-Version"))
	return &FetchResult{YAML: body, ETag: resp.Header.Get("ETag"), Version: version}, nil
}
