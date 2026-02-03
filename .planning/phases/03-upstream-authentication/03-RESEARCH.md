# Phase 3: Upstream Authentication - Research

**Researched:** 2026-02-03
**Domain:** HTTP gateway authentication for upstream services (OAuth2, Basic Auth, API key injection, passthrough)
**Confidence:** HIGH

## Summary

Upstream authentication for Go HTTP gateways has well-established patterns centered on `golang.org/x/oauth2` for OAuth2 client credentials, Go's standard library for basic auth (`http.Request.SetBasicAuth`), and custom RoundTripper middleware for header injection. The key technical challenges are OAuth2 token caching with concurrent refresh (preventing "thundering herd"), environment variable substitution in YAML config, and fail-fast validation of missing secrets.

The standard approach for OAuth2 client credentials uses `golang.org/x/oauth2/clientcredentials` package which provides automatic token refresh with built-in concurrency safety via mutex-protected `ReuseTokenSource`. Token expiry buffering is configurable with `ReuseTokenSourceWithExpiry` to refresh tokens before they expire (30 seconds is a common buffer). For concurrent request deduplication during token refresh, `golang.org/x/sync/singleflight` provides production-tested protection against multiple goroutines simultaneously refreshing the same token.

For environment variable substitution in YAML, Go's standard library `os.ExpandEnv` handles `${VAR}` syntax but silently replaces undefined variables with empty strings. To achieve fail-fast behavior (startup failure on missing env vars), config parsing must explicitly validate after expansion using `os.LookupEnv`.

**Primary recommendation:** Use `golang.org/x/oauth2/clientcredentials` for OAuth2 with `ReuseTokenSourceWithExpiry` for early refresh buffering, wrap in `singleflight.Group` for concurrent safety, implement authentication via RoundTripper pattern to inject headers per upstream, and validate all `${VAR}` references resolve to non-empty values at startup.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| golang.org/x/oauth2 | latest | OAuth2 client implementation with token refresh | Official Go team library, handles 3-legged and 2-legged OAuth2, automatic refresh, concurrent-safe |
| golang.org/x/oauth2/clientcredentials | latest | OAuth2 client credentials (2-legged) flow | Specific implementation for service-to-service auth without user interaction |
| golang.org/x/sync/singleflight | latest | Duplicate function call suppression | Prevents thundering herd during token refresh, stdlib-adjacent |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| os.ExpandEnv | stdlib | Environment variable substitution in strings | Expanding `${VAR}` syntax in YAML after parsing |
| os.LookupEnv | stdlib | Environment variable lookup with exists check | Validating required env vars exist (vs. empty string) |
| crypto/subtle | stdlib | Constant-time comparison | Comparing secrets to prevent timing attacks |
| net/http.RoundTripper | stdlib | HTTP client middleware | Injecting authentication headers per-upstream |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| golang.org/x/oauth2 | Custom OAuth2 implementation | Never recommended - OAuth2 has many edge cases, token refresh logic is complex |
| singleflight.Group | Custom mutex/cache | Could use `sync.Once` per token but singleflight handles concurrent callers better |
| RoundTripper pattern | Modify httpClient before each request | RoundTripper is cleaner, composable, and how Kubernetes/major projects do it |
| os.ExpandEnv | Viper or custom parser | Viper is overkill for simple env var substitution; os.ExpandEnv is stdlib |

**Installation:**
```bash
go get golang.org/x/oauth2
go get golang.org/x/sync/singleflight
# stdlib packages require no installation
```

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── auth/                    # Authentication strategies
│   ├── auth.go             # Strategy interface, factory
│   ├── none.go             # No-auth strategy
│   ├── header.go           # Static header injection
│   ├── basic.go            # HTTP Basic Auth
│   ├── passthrough.go      # Forward client Authorization header
│   └── oauth2.go           # OAuth2 client credentials
├── client/                  # HTTP client (Phase 1)
│   └── client.go
└── composition/             # Composition engine (Phase 2)
    ├── config.go           # Add auth config to Upstream
    └── executor.go         # Inject auth via RoundTripper
```

### Pattern 1: Strategy Pattern for Auth Types
**What:** Each auth type (none, header, basic, passthrough, oauth2) implements a common interface that returns an `http.RoundTripper`.
**When to use:** Multiple authentication strategies that need to be selected at config-parse time and applied per-upstream.
**Example:**
```go
// Source: Standard Go pattern, used in Kubernetes client-go
package auth

import "net/http"

// Strategy represents an upstream authentication strategy.
type Strategy interface {
    // RoundTripper returns an http.RoundTripper that injects authentication.
    // The base RoundTripper is typically the http.DefaultTransport.
    RoundTripper(base http.RoundTripper) http.RoundTripper
}

// Config represents auth configuration from YAML (one strategy per upstream).
type Config struct {
    Header      *HeaderConfig      `yaml:"header"`
    Basic       *BasicConfig       `yaml:"basic"`
    Passthrough *PassthroughConfig `yaml:"passthrough"`
    OAuth2      *OAuth2Config      `yaml:"oauth2"`
}

// Build creates the appropriate Strategy from config.
func (c *Config) Build(ctx context.Context) (Strategy, error) {
    // Exactly one strategy must be set (validated at parse time)
    switch {
    case c.Header != nil:
        return NewHeaderStrategy(c.Header)
    case c.Basic != nil:
        return NewBasicStrategy(c.Basic)
    case c.Passthrough != nil:
        return NewPassthroughStrategy(c.Passthrough)
    case c.OAuth2 != nil:
        return NewOAuth2Strategy(ctx, c.OAuth2)
    default:
        return NewNoneStrategy(), nil
    }
}
```

### Pattern 2: RoundTripper Middleware for Auth Injection
**What:** Wrap the base http.Transport with a custom RoundTripper that modifies requests before they're sent.
**When to use:** Adding headers (auth, tracing, etc.) to every request for a specific upstream without modifying request code.
**Example:**
```go
// Source: Pattern from kubernetes/client-go/transport/round_trippers.go
package auth

import "net/http"

// HeaderRoundTripper injects a static header into every request.
type HeaderRoundTripper struct {
    base   http.RoundTripper
    name   string
    value  string
}

func (rt *HeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
    // Clone request to avoid modifying the original (critical for retries)
    reqCopy := req.Clone(req.Context())

    // Inject authentication header
    reqCopy.Header.Set(rt.name, rt.value)

    // Execute request with base transport
    return rt.base.RoundTrip(reqCopy)
}

// Usage: client := &http.Client{Transport: &HeaderRoundTripper{...}}
```

### Pattern 3: OAuth2 Token Refresh with Singleflight
**What:** Use `singleflight.Group` to deduplicate concurrent token refresh requests.
**When to use:** OAuth2 token caching where multiple concurrent requests might trigger refresh simultaneously.
**Example:**
```go
// Source: https://pkg.go.dev/golang.org/x/sync/singleflight and OAuth2 patterns
package auth

import (
    "context"
    "net/http"
    "golang.org/x/oauth2/clientcredentials"
    "golang.org/x/sync/singleflight"
)

type OAuth2RoundTripper struct {
    base        http.RoundTripper
    tokenSource oauth2.TokenSource
    group       *singleflight.Group
}

func (rt *OAuth2RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
    // Get token (singleflight ensures only one refresh at a time)
    tokenVal, err, _ := rt.group.Do("token", func() (interface{}, error) {
        return rt.tokenSource.Token()
    })

    if err != nil {
        return nil, err
    }

    token := tokenVal.(*oauth2.Token)

    // Clone and inject token
    reqCopy := req.Clone(req.Context())
    reqCopy.Header.Set("Authorization", token.Type()+" "+token.AccessToken)

    return rt.base.RoundTrip(reqCopy)
}
```

### Pattern 4: Environment Variable Validation at Startup
**What:** Parse YAML with `${VAR}` placeholders, expand with `os.ExpandEnv`, then validate all required vars were non-empty.
**When to use:** Config-driven secret injection where missing secrets should fail startup (not runtime).
**Example:**
```go
// Source: Standard Go pattern combining os.ExpandEnv with validation
package config

import (
    "fmt"
    "os"
    "regexp"
)

var envVarRegex = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// ExpandAndValidate expands environment variables and validates all are defined.
func ExpandAndValidate(raw string) (string, error) {
    // Find all ${VAR} references
    vars := envVarRegex.FindAllStringSubmatch(raw, -1)

    // Check each variable exists and is non-empty
    for _, match := range vars {
        varName := match[1]
        if val, exists := os.LookupEnv(varName); !exists || val == "" {
            return "", fmt.Errorf("required environment variable not set: %s", varName)
        }
    }

    // Expand (safe now that we've validated)
    return os.ExpandEnv(raw), nil
}
```

### Anti-Patterns to Avoid
- **Modifying original http.Request in RoundTripper:** Always clone with `req.Clone(req.Context())` - original may be reused for retries
- **Using == to compare secrets:** Use `subtle.ConstantTimeCompare` to prevent timing attacks
- **Storing tokens without expiry buffer:** Tokens expire mid-request without early refresh
- **Multiple goroutines refreshing same token:** Causes rate limiting, token invalidation - use singleflight
- **Panicking on token refresh failure:** Return HTTP 502 Bad Gateway, log details but hide from client

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| OAuth2 token refresh | Custom timer/cache logic | `golang.org/x/oauth2/clientcredentials` + `ReuseTokenSourceWithExpiry` | Handles token expiry, refresh, concurrent access, retry logic, error handling |
| Concurrent token refresh deduplication | Custom mutex/once pattern | `golang.org/x/sync/singleflight` | Handles waiting goroutines, error sharing, automatic cleanup |
| Basic auth header construction | Manual base64 encoding | `http.Request.SetBasicAuth(username, password)` | Handles encoding, header format, edge cases |
| Environment variable expansion | Custom string replacement | `os.ExpandEnv(s)` + validation | Handles `${VAR}` and `$VAR` syntax, nested braces |
| Secret comparison | `==` operator or `bytes.Equal` | `crypto/subtle.ConstantTimeCompare` | Prevents timing attacks that leak secret length/content |

**Key insight:** OAuth2 token lifecycle management is far more complex than it appears. Edge cases include: token refresh during startup, concurrent refresh requests, network failures during refresh, token revocation, clock skew between client and auth server, missing/expired refresh tokens. The `golang.org/x/oauth2` library has years of production hardening for these cases.

## Common Pitfalls

### Pitfall 1: os.ExpandEnv Silently Returns Empty String for Undefined Variables
**What goes wrong:** `os.ExpandEnv("${MISSING_VAR}")` returns `""` without error. Config parses successfully but auth fails at runtime with empty credentials.
**Why it happens:** `os.ExpandEnv` is designed for shell-like expansion where undefined variables expand to empty strings (not errors).
**How to avoid:** After YAML parsing, before calling `os.ExpandEnv`, extract all `${VAR}` references with regex and validate each with `os.LookupEnv(var)`. Return error if any variable is undefined or empty. Fail fast at startup.
**Warning signs:** Runtime auth failures with "401 Unauthorized" when credentials should be valid. Empty values in logged requests (but never log actual secrets).

### Pitfall 2: Token Refresh Thundering Herd
**What goes wrong:** 100 concurrent requests arrive when token is expired. All 100 goroutines call `tokenSource.Token()` simultaneously, triggering 100 token refresh requests to the auth server. Auth server rate-limits or token gets invalidated.
**Why it happens:** `oauth2.ReuseTokenSource` is concurrent-safe for reads but every goroutine independently checks expiry and calls refresh if needed. No built-in deduplication.
**How to avoid:** Wrap `tokenSource.Token()` call in `singleflight.Group.Do()`. First caller refreshes, others wait for result. All receive same token or same error.
**Warning signs:** Logs show multiple simultaneous token refresh requests. Auth server returns rate limit errors (429). Token refresh fails intermittently under load.

### Pitfall 3: Not Cloning http.Request in RoundTripper
**What goes wrong:** Modifying `req.Header` directly in RoundTripper affects the original request. If HTTP client retries the request (network error, redirect), modified headers persist or conflict.
**Why it happens:** `http.Request` is a pointer. Modifying it modifies the caller's request. Retries reuse the same request object.
**How to avoid:** Always start RoundTrip with `reqCopy := req.Clone(req.Context())`. Modify `reqCopy.Header`. Pass `reqCopy` to base transport.
**Warning signs:** Duplicate headers on retried requests. Authorization headers appearing in unexpected places. Flaky test failures.

### Pitfall 4: Missing Context Propagation in Token Refresh
**What goes wrong:** Client request has 5-second timeout. Token needs refresh, which takes 3 seconds. Request times out but token refresh continues, wasting auth server resources and potentially refreshing after request is canceled.
**Why it happens:** `oauth2.TokenSource.Token()` doesn't accept context (known limitation). Context from `Config.Client(ctx, token)` is captured at client creation, not per-request.
**How to avoid:** Pass context to `clientcredentials.Config.Client(ctx)` where `ctx` has appropriate timeout for token operations. For per-request contexts, this is a known tradeoff - token refresh uses client-creation context, not request context.
**Warning signs:** Token refresh operations continue after request cancellation. Logs show token fetch completing after client has disconnected.

### Pitfall 5: Passthrough Without Client Authorization Header
**What goes wrong:** Gateway configured with passthrough auth. Client sends request without `Authorization` header. Gateway forwards request to upstream with no auth, upstream returns 401, client confused.
**Why it happens:** Passthrough means "forward whatever client sent". If client sends nothing, upstream gets nothing.
**How to avoid:** Passthrough strategy should check if `Authorization` header exists in incoming request. If missing, return 401 with `WWW-Authenticate` header immediately (don't forward to upstream). Log this as client error, not gateway error.
**Warning signs:** Upstream 401 errors for passthrough upstreams when client forgot to send credentials.

### Pitfall 6: Mixing Auth Strategies per Upstream
**What goes wrong:** Config defines both `oauth2` and `header` for same upstream. Gateway uses both, sending duplicate `Authorization` headers or overwriting headers unpredictably.
**Why it happens:** YAML allows multiple keys. Parser doesn't validate mutual exclusivity.
**How to avoid:** At config parse time, validate exactly one auth strategy is set per upstream. Return parse error if multiple strategies defined. This is fail-fast.
**Warning signs:** Duplicate Authorization headers in requests. Flaky auth failures. Upstream sees unexpected auth type.

### Pitfall 7: Hardcoding 10-Second Expiry Buffer
**What goes wrong:** Default `ReuseTokenSource` uses 10-second expiry buffer (tokens considered expired 10 seconds before actual expiry). High-latency networks or slow auth servers mean tokens expire mid-request.
**Why it happens:** 10 seconds is sufficient for fast networks but not all environments. Decision states "30 seconds before expiry" buffer.
**How to avoid:** Use `ReuseTokenSourceWithExpiry(token, src, 30*time.Second)` instead of `ReuseTokenSource(token, src)`. Configurable buffer allows tuning for environment.
**Warning signs:** Upstream 401 errors during high load. Token refresh timing correlates with auth failures.

## Code Examples

Verified patterns from official sources:

### OAuth2 Client Credentials with Early Refresh
```go
// Source: https://pkg.go.dev/golang.org/x/oauth2/clientcredentials
// Source: https://pkg.go.dev/golang.org/x/oauth2#ReuseTokenSourceWithExpiry
package auth

import (
    "context"
    "net/http"
    "time"
    "golang.org/x/oauth2"
    "golang.org/x/oauth2/clientcredentials"
)

type OAuth2Strategy struct {
    tokenSource oauth2.TokenSource
}

func NewOAuth2Strategy(ctx context.Context, cfg *OAuth2Config) (*OAuth2Strategy, error) {
    ccConfig := &clientcredentials.Config{
        ClientID:     cfg.ClientID,
        ClientSecret: cfg.ClientSecret,
        TokenURL:     cfg.TokenURL,
        Scopes:       cfg.Scopes,
    }

    // Get initial token
    token, err := ccConfig.Token(ctx)
    if err != nil {
        return nil, fmt.Errorf("initial token fetch failed: %w", err)
    }

    // Create reusable token source with 30-second early refresh buffer
    baseSource := ccConfig.TokenSource(ctx)
    tokenSource := oauth2.ReuseTokenSourceWithExpiry(
        token,
        baseSource,
        30*time.Second, // Refresh 30 seconds before expiry
    )

    return &OAuth2Strategy{
        tokenSource: tokenSource,
    }, nil
}

func (s *OAuth2Strategy) RoundTripper(base http.RoundTripper) http.RoundTripper {
    return &oauth2.Transport{
        Source: s.tokenSource,
        Base:   base,
    }
}
```

### Basic Authentication with Constant-Time Comparison (for reference - not needed for client)
```go
// Source: https://www.alexedwards.net/blog/basic-authentication-in-go
// Source: https://pkg.go.dev/crypto/subtle
package auth

import (
    "crypto/subtle"
    "net/http"
)

func (s *BasicStrategy) RoundTripper(base http.RoundTripper) http.RoundTripper {
    return &basicAuthRoundTripper{
        username: s.username,
        password: s.password,
        base:     base,
    }
}

type basicAuthRoundTripper struct {
    username string
    password string
    base     http.RoundTripper
}

func (rt *basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
    reqCopy := req.Clone(req.Context())

    // SetBasicAuth handles base64 encoding and header format
    reqCopy.SetBasicAuth(rt.username, rt.password)

    return rt.base.RoundTrip(reqCopy)
}

// Note: subtle.ConstantTimeCompare is for server-side validation (not client-side)
// but included for completeness
func validatePassword(expected, provided string) bool {
    expectedBytes := []byte(expected)
    providedBytes := []byte(provided)

    // Constant-time comparison prevents timing attacks
    return subtle.ConstantTimeCompare(expectedBytes, providedBytes) == 1
}
```

### Environment Variable Validation
```go
// Source: https://pkg.go.dev/os#ExpandEnv
// Source: https://pkg.go.dev/os#LookupEnv
package config

import (
    "fmt"
    "os"
    "regexp"
)

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// ExpandEnvWithValidation expands ${VAR} and ensures all variables are defined.
func ExpandEnvWithValidation(value string) (string, error) {
    // Find all ${VAR} references
    matches := envVarPattern.FindAllStringSubmatch(value, -1)

    // Validate each variable exists and is non-empty
    for _, match := range matches {
        varName := match[1] // Capture group 1 is the variable name

        val, exists := os.LookupEnv(varName)
        if !exists {
            return "", fmt.Errorf("environment variable %s is not set", varName)
        }
        if val == "" {
            return "", fmt.Errorf("environment variable %s is empty", varName)
        }
    }

    // Safe to expand now that we've validated
    return os.ExpandEnv(value), nil
}
```

### Passthrough Strategy with Missing Header Check
```go
// Source: Standard pattern for forwarding headers
package auth

import (
    "fmt"
    "net/http"
)

type PassthroughStrategy struct{}

func (s *PassthroughStrategy) RoundTripper(base http.RoundTripper) http.RoundTripper {
    return &passthroughRoundTripper{base: base}
}

type passthroughRoundTripper struct {
    base http.RoundTripper
}

func (rt *passthroughRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
    // Check if client provided Authorization header
    authHeader := req.Header.Get("Authorization")
    if authHeader == "" {
        // Client didn't provide auth - fail immediately
        // Note: This check should happen earlier (at handler level) to return
        // proper 401 with WWW-Authenticate header. RoundTripper returns *http.Response.
        return nil, fmt.Errorf("passthrough auth requires Authorization header from client")
    }

    // Authorization header exists - forward verbatim
    reqCopy := req.Clone(req.Context())
    // Header already present, no modification needed

    return rt.base.RoundTrip(reqCopy)
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Custom OAuth2 client | `golang.org/x/oauth2` | ~2014 | Standard library-adjacent package eliminates custom implementations |
| `ReuseTokenSource` with default buffer | `ReuseTokenSourceWithExpiry` with configurable buffer | ~2019 | Allows tuning expiry buffer for high-latency environments |
| Manual mutex for token refresh | `ReuseTokenSource` built-in mutex | ~2014 | Concurrent-safe by default |
| Custom deduplication logic | `golang.org/x/sync/singleflight` | ~2017 | Standard pattern for deduplicating concurrent operations |
| String comparison for secrets | `crypto/subtle.ConstantTimeCompare` | Always available | Prevents timing attacks |

**Deprecated/outdated:**
- `oauth2.Transport.CancelRequest`: Deprecated, use context cancellation instead
- Custom base64 encoding for basic auth: Use `http.Request.SetBasicAuth`
- Comparing passwords/tokens with `==`: Use `subtle.ConstantTimeCompare`

## Open Questions

Things that couldn't be fully resolved:

1. **TokenSource context propagation limitation**
   - What we know: `oauth2.TokenSource.Token()` doesn't accept per-request context (GitHub issue #262)
   - What's unclear: Best practice for per-request timeout vs. client-level context
   - Recommendation: Use client-creation context with reasonable timeout for token operations (30s). Accept that per-request cancellation won't cancel in-flight token refresh. Document this as known limitation.

2. **Optimal expiry buffer for different auth servers**
   - What we know: 30 seconds chosen per CONTEXT.md decisions
   - What's unclear: Whether all OAuth2 providers tolerate 30-second buffer, clock skew implications
   - Recommendation: Start with 30 seconds (configurable via `ReuseTokenSourceWithExpiry`), monitor auth failures, adjust if needed. Log token refresh timing at debug level.

3. **Passthrough auth with no client header - exact behavior**
   - What we know: CONTEXT.md says "security best practice" needed
   - What's unclear: Return 401 immediately vs. forward to upstream and let it return 401
   - Recommendation: Return 401 immediately from gateway with `WWW-Authenticate: Bearer` header. Don't forward unauthenticated requests to upstream (security best practice). This prevents accidental exposure if upstream misconfigured.

## Sources

### Primary (HIGH confidence)
- [golang.org/x/oauth2 package documentation](https://pkg.go.dev/golang.org/x/oauth2) - OAuth2 client implementation, TokenSource interface, ReuseTokenSource
- [golang.org/x/oauth2/clientcredentials package](https://pkg.go.dev/golang.org/x/oauth2/clientcredentials) - Client credentials flow implementation
- [golang.org/x/sync/singleflight package](https://pkg.go.dev/golang.org/x/sync/singleflight) - Duplicate call suppression
- [golang.org/x/oauth2 source code - oauth2.go](https://github.com/golang/oauth2/blob/master/oauth2.go) - ReuseTokenSource internal implementation
- [golang.org/x/oauth2 source code - clientcredentials.go](https://github.com/golang/oauth2/blob/master/clientcredentials/clientcredentials.go) - Token() method implementation

### Secondary (MEDIUM confidence)
- [Alex Edwards - Basic Authentication in Go](https://www.alexedwards.net/blog/basic-authentication-in-go) - Basic auth best practices, verified with stdlib docs
- [Kubernetes client-go RoundTripper patterns](https://github.com/kubernetes/client-go/blob/master/transport/round_trippers.go) - Production RoundTripper examples
- [Go singleflight explanation - DEV Community](https://dev.to/nickytonline/gos-singleflight-package-and-why-its-awesome-for-concurrent-requests-4122) - Singleflight use cases
- [API Gateway Authentication patterns - Solo.io](https://www.solo.io/topics/api-gateway/api-gateway-authentication) - Gateway auth strategies 2026
- [API Security Best Practices 2026 - Qodex.ai](https://qodex.ai/blog/15-api-security-best-practices-to-secure-your-apis-in-2026) - Token exchange, workload identity patterns

### Tertiary (LOW confidence)
- [WebSearch] golang os.ExpandEnv undefined variable behavior - Multiple sources confirm empty string behavior
- [WebSearch] OAuth2 token refresh pitfalls - GitHub issues highlight common problems
- [WebSearch] RoundTripper authentication injection pattern - Community consensus despite docs

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Official packages, well-documented, production-proven
- Architecture: HIGH - Patterns verified in official docs and Kubernetes codebase
- Pitfalls: MEDIUM - Combination of official issues, community experience, and inference from API design
- Code examples: HIGH - All examples derived from official documentation or verified source code
- Environment variable handling: HIGH - Standard library behavior, well-documented
- OAuth2 concurrent safety: HIGH - Verified in source code (mutex implementation)
- Singleflight usage: MEDIUM - Pattern inferred from package docs and community usage

**Research date:** 2026-02-03
**Valid until:** 2026-03-03 (30 days - stable domain, Go stdlib and x/ packages have slow update cadence)
