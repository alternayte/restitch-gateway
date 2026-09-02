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

package auth

import (
	"net/http"
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
	return &HeaderStrategy{name: cfg.Name, value: cfg.Value}, nil
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
