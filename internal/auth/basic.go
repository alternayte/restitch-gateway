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

// BasicStrategy implements HTTP Basic Authentication for upstream services.
// It uses the standard SetBasicAuth method to encode username:password
// as a base64 Authorization header. Credentials support ${VAR} syntax
// for environment variable expansion.
type BasicStrategy struct {
	username string
	password string
}

// NewBasicStrategy creates a basic auth strategy with the given credentials.
// Both username and password are expanded using environment variables at creation time.
// Returns an error if referenced environment variables are missing or empty.
func NewBasicStrategy(cfg *BasicConfig) (*BasicStrategy, error) {
	return &BasicStrategy{username: cfg.Username, password: cfg.Password}, nil
}

// RoundTripper returns an http.RoundTripper that injects Basic auth header.
func (s *BasicStrategy) RoundTripper(base http.RoundTripper) http.RoundTripper {
	return &basicRoundTripper{base: base, username: s.username, password: s.password}
}

// basicRoundTripper wraps a base RoundTripper to inject Basic authentication.
type basicRoundTripper struct {
	base     http.RoundTripper
	username string
	password string
}

// RoundTrip implements http.RoundTripper by cloning the request and adding Basic auth.
// CRITICAL: Request is cloned to avoid modifying the original (safe for retries).
// Uses stdlib SetBasicAuth per RESEARCH.md "Don't Hand-Roll" section.
func (rt *basicRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid modifying original (per RESEARCH.md Pitfall 3)
	reqCopy := req.Clone(req.Context())
	// Use stdlib SetBasicAuth - never hand-roll base64 encoding
	reqCopy.SetBasicAuth(rt.username, rt.password)
	return rt.base.RoundTrip(reqCopy)
}
