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

// Package composition provides YAML configuration parsing and expression compilation
// for the composition engine.
//
// The composition engine allows users to define multi-step API compositions that
// fetch data from multiple upstreams in parallel and merge responses using expressions.
//
// Key concepts:
//   - Upstreams: Named upstream services referenced in steps
//   - Compositions: Multi-step workflows with response templates
//   - Steps: Individual upstream requests that can depend on each other
//   - Expressions: Dynamic values using {{ expr }} template syntax
//
// All expressions are compiled at parse time (not request time) to fail fast on
// syntax errors.
package composition

import (
	"time"

	"github.com/alternayte/restitch-gateway/internal/auth"
	"github.com/alternayte/restitch-gateway/internal/ratelimit"
)

// Config represents the complete composition configuration loaded from YAML.
// It defines all upstreams and compositions available to the gateway.
type Config struct {
	Upstreams    map[string]Upstream    `yaml:"upstreams"`
	Compositions map[string]Composition `yaml:"compositions"`
}

// Upstream represents a named backend service that steps can call.
// Auth configuration is optional - if omitted, no authentication is applied.
type Upstream struct {
	URL              string           `yaml:"url"`
	Auth             *auth.Config     `yaml:"auth"`
	Timeout          time.Duration    `yaml:"timeout"`
	HealthPath       string           `yaml:"health_path"`
	MaxResponseBytes int64            `yaml:"max_response_bytes"`
	Transport        *TransportConfig `yaml:"transport"`
	Retry            *RetryConfig     `yaml:"retry"`
	CircuitBreaker   *BreakerConfig   `yaml:"circuit_breaker"`
}

// TransportConfig configures the HTTP transport for an upstream. All fields are
// optional; zero values fall through to the defaults in upstream.BuildTransport.
type TransportConfig struct {
	DialTimeout           time.Duration `yaml:"dial_timeout"`
	TLSHandshakeTimeout   time.Duration `yaml:"tls_handshake_timeout"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"`
	MaxIdleConnsPerHost   int           `yaml:"max_idle_conns_per_host"`
	InsecureSkipVerify    bool          `yaml:"insecure_skip_verify"`
}

// RetryConfig configures retry behavior for an upstream or step.
type RetryConfig struct {
	MaxAttempts        int           `yaml:"max_attempts"`
	Interval           time.Duration `yaml:"interval"`
	MaxBackoff         time.Duration `yaml:"max_backoff"`
	BackoffOn          []int         `yaml:"backoff_on"`
	DropOn             []int         `yaml:"drop_on"`
	RetryNonIdempotent bool          `yaml:"retry_non_idempotent"`
}

// BreakerConfig configures circuit breaker for an upstream.
type BreakerConfig struct {
	MaxFailures int           `yaml:"max_failures"`
	Interval    time.Duration `yaml:"interval"`
	Timeout     time.Duration `yaml:"timeout"`
}

// CacheConfig configures per-step response caching.
type CacheConfig struct {
	TTL time.Duration `yaml:"ttl"`
}

// Composition represents a multi-step API composition with a response template.
// Compositions are matched to incoming requests by path and method.
type Composition struct {
	Path            string            `yaml:"path"`
	Method          string            `yaml:"method"`
	Public          bool              `yaml:"public"`
	Steps           []Step            `yaml:"steps"`
	Response        ResponseTemplate  `yaml:"response"`
	RateLimit       *ratelimit.Config `yaml:"rate_limit"`
	MaxRequestBytes int64             `yaml:"max_request_bytes"`
	RequestSchema   map[string]any    `yaml:"request_schema"`
}

// Step represents a single upstream request in a composition.
// Steps can depend on other steps either implicitly (via expression usage)
// or explicitly (via DependsOn).
type Step struct {
	Name       string            `yaml:"name"`
	Upstream   string            `yaml:"upstream"`
	Path       string            `yaml:"path"`
	Method     string            `yaml:"method"`
	Headers    map[string]string `yaml:"headers"`
	Body       string            `yaml:"body"`
	DependsOn  []string          `yaml:"depends_on"`
	Optional   bool              `yaml:"optional"`
	Timeout    *time.Duration    `yaml:"timeout"`
	ErrorRules []ErrorRule       `yaml:"error_rules"`
	Retry      *RetryConfig      `yaml:"retry"`
	Cache      *CacheConfig      `yaml:"cache"`
	Coalesce   bool              `yaml:"coalesce"`
}

// ErrorRule defines a rule for matching upstream error status codes
// and replacing the response body with a configured value.
type ErrorRule struct {
	Statuses []int       `yaml:"statuses"` // List of status codes to match (e.g., [404, 410])
	Body     interface{} `yaml:"body"`     // Replacement body value
}

// ResponseTemplate defines the structure of the composition response.
// The body can contain nested maps with expression templates that reference
// step results.
type ResponseTemplate struct {
	Status      interface{} `yaml:"status"`       // int or expression string
	ContentType string      `yaml:"content_type"` // MIME type (defaults to application/json)
	Body        interface{} `yaml:"body"`         // Nested map with expression templates
}
