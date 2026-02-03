# Technology Stack: REST API Gateway

**Project:** Restitch - REST API composition gateway
**Researched:** 2026-02-03
**Confidence:** HIGH

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go | 1.23+ | Runtime and language | Native goroutines for parallel execution, excellent HTTP performance, strong stdlib, static binaries for easy deployment |
| goccy/go-yaml | v1.19.2 | YAML configuration parsing | Active maintenance (published Jan 2026), better error messages than go-yaml/yaml, passes 60 more YAML test cases, supports path queries, preserves comments. Official go-yaml/yaml is archived as of April 2025 |
| expr-lang/expr | v1.17.7 | Expression evaluation | Production-proven (Google, Uber, ByteDance), memory-safe, side-effect-free, optimizing compiler with bytecode VM, excellent Go type integration, active development (Dec 2025 release) |
| net/http | stdlib | HTTP client for upstream requests | Standard library, HTTP/2 support, connection pooling built-in, sufficient performance for most use cases, context-aware, no external dependencies |
| golang.org/x/oauth2 | v0.34.0 | OAuth2 client credentials flow | Official Google library, auto-refreshing tokens, standard for OAuth2 in Go ecosystem, well-maintained (Dec 2025 release) |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| golang.org/x/sync/errgroup | Latest | Parallel execution with error handling | For executing multiple upstream HTTP requests concurrently with proper error propagation and context cancellation |
| hashicorp/go-retryablehttp | Latest | HTTP retry with backoff | For resilient upstream calls, automatic exponential backoff, respects Retry-After headers, production-proven in Terraform/Vault/Consul |
| spf13/cobra | Latest | CLI framework | For building the gateway CLI with subcommands (serve, validate, version), automatic help generation, flag handling |
| spf13/viper | Latest | Configuration management | If config needs env vars or multi-format support (pairs well with Cobra), allows precedence: flags > env > config files |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| testing (stdlib) | Unit testing | Use table-driven tests (Go idiom), t.Run for subtests, t.Parallel for concurrent tests |
| golang.org/x/tools/cmd/goimports | Code formatting | Auto-organizes imports, superset of gofmt |
| golangci-lint | Linting | Aggregates 50+ linters, configurable, fast, industry standard |

## Installation

```bash
# Core dependencies
go get github.com/goccy/go-yaml@v1.19.2
go get github.com/expr-lang/expr@v1.17.7
go get golang.org/x/oauth2@v0.34.0

# Supporting libraries
go get golang.org/x/sync/errgroup
go get github.com/hashicorp/go-retryablehttp
go get github.com/spf13/cobra
go get github.com/spf13/viper

# Dev tools (install globally)
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

## Alternatives Considered

| Category | Recommended | Alternative | Why Not Alternative |
|----------|-------------|-------------|---------------------|
| YAML parsing | goccy/go-yaml | gopkg.in/yaml.v3 (go-yaml/yaml) | Archived/unmaintained as of April 2025, inferior error messages, fails 60 more YAML test cases |
| Expression language | expr-lang/expr | text/template, govaluate, otto | expr is purpose-built, faster (bytecode VM), safer (no side effects), better type checking |
| HTTP client | net/http | fasthttp | fasthttp lacks HTTP/2, incompatible API, only 30-70% faster in real-world (not 6x), net/http sufficient for gateway use case |
| Concurrency | errgroup + goroutines | Worker pool libraries (pond, ants) | errgroup in stdlib (as of Go 1.20 with SetLimit), simpler, official, sufficient for bounded parallelism |
| OAuth2 | golang.org/x/oauth2 | Custom implementation | Industry standard, auto-refresh, battle-tested, no reason to reinvent |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| gopkg.in/yaml.v2 | Old API, YAML 1.1 only, missing features | goccy/go-yaml v1.19.2 (YAML 1.2, better errors) |
| gopkg.in/yaml.v3 (go-yaml/yaml) | **Archived April 2025**, unmaintained, creator cannot maintain it | goccy/go-yaml v1.19.2 |
| fasthttp | No HTTP/2, marginal real-world gains, API incompatibility | net/http stdlib |
| text/template for expressions | Designed for text generation, not safe evaluation, no type checking | expr-lang/expr |
| github.com/antonmedv/expr | Old import path | github.com/expr-lang/expr (official path) |

## Stack Patterns by Use Case

**If config is simple (YAML only):**
- Just goccy/go-yaml
- Parse directly into structs
- Validate after unmarshal

**If config needs environment variables:**
- Add Viper
- Bind YAML keys to env vars
- Allows 12-factor config pattern

**If upstream services are unreliable:**
- Add go-retryablehttp
- Configure backoff strategy
- Set max retries per phase

**If parallelism needs bounds:**
- Use errgroup.SetLimit(N)
- Available in stdlib since Go 1.20
- No need for third-party worker pools

## Version Compatibility

| Package | Compatible With | Notes |
|---------|-----------------|-------|
| expr-lang/expr@v1.17.7 | Go 1.18+ | Requires generics support (if/else expressions) |
| goccy/go-yaml@v1.19.2 | Go 1.16+ | Works with all modern Go versions |
| golang.org/x/oauth2@v0.34.0 | Go 1.19+ | Uses context package extensively |
| errgroup.SetLimit | Go 1.20+ | Older Go needs neilotoole/errgroup fork |

## HTTP Client Configuration Recommendations

### Default net/http.Client Settings

```go
client := &http.Client{
    Timeout: 30 * time.Second, // Overall request timeout
    Transport: &http.Transport{
        MaxIdleConns:        100,              // Total idle connections
        MaxIdleConnsPerHost: 10,               // Idle per upstream host
        IdleConnTimeout:     90 * time.Second, // How long idle conns live
        DisableCompression:  false,            // Enable gzip by default
        ForceAttemptHTTP2:   true,             // Prefer HTTP/2
    },
}
```

### With Retries (go-retryablehttp)

```go
retryClient := retryablehttp.NewClient()
retryClient.RetryMax = 3
retryClient.RetryWaitMin = 1 * time.Second
retryClient.RetryWaitMax = 30 * time.Second
retryClient.HTTPClient.Timeout = 90 * time.Second
standardClient := retryClient.StandardClient() // Wraps to http.Client
```

### Context-Aware Requests

```go
// Always use context for cancellation and timeouts
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
resp, err := client.Do(req)
```

## Project Structure Recommendation

Based on Go 2026 best practices for CLI services:

```
restitch/
├── cmd/
│   └── restitch/
│       └── main.go              # CLI entry point (minimal, calls internal)
├── internal/
│   ├── config/                  # YAML parsing, validation
│   ├── evaluator/               # Expr wrapper, expression compilation
│   ├── gateway/                 # Core composition logic
│   ├── upstream/                # HTTP client, OAuth2, retry logic
│   └── server/                  # HTTP server (if needed)
├── go.mod
├── go.sum
└── README.md
```

**Rationale:**
- `/cmd/restitch` for CLI entry (short install path)
- `/internal` for all implementation (prevents external imports)
- Organize by **feature/domain**, not layer (config, gateway, upstream)
- Avoid `/pkg` unless code is explicitly for external use
- Keep flat (1-2 levels max)

## Sources

### High Confidence (Official Documentation & Context7)
- [goccy/go-yaml v1.19.2](https://pkg.go.dev/github.com/goccy/go-yaml) - Package documentation
- [goccy/go-yaml GitHub](https://github.com/goccy/go-yaml) - Advantages, features, published Jan 8, 2026
- [go-yaml/yaml GitHub](https://github.com/go-yaml/yaml) - Archived status confirmed April 2025
- [expr-lang/expr v1.17.7](https://github.com/expr-lang/expr) - Latest release Dec 15, 2025
- [expr-lang/expr releases](https://github.com/expr-lang/expr/releases) - Version history
- [golang.org/x/oauth2 v0.34.0](https://pkg.go.dev/golang.org/x/oauth2) - Published Dec 1, 2025
- [golang.org/x/oauth2/clientcredentials](https://pkg.go.dev/golang.org/x/oauth2/clientcredentials) - Client credentials flow API
- [Go context package](https://pkg.go.dev/context) - Official stdlib documentation
- [Go wiki: TableDrivenTests](https://go.dev/wiki/TableDrivenTests) - Official testing pattern
- [Go modules layout](https://go.dev/doc/modules/layout) - Official project structure

### Medium Confidence (Verified Community Sources)
- [Leapcell: Working with YAML in Go](https://leapcell.io/blog/working-with-yaml-in-go) - YAML library comparison
- [WunderGraph: Expr Lang](https://wundergraph.com/blog/expr-lang-go-centric-expression-language) - Use cases
- [HashiCorp go-retryablehttp](https://github.com/hashicorp/go-retryablehttp) - Production-proven retry library
- [Cobra documentation](https://cobra.dev/docs/learning-resources/learning-journey/) - CLI framework
- [Spf13 Viper](https://github.com/spf13/viper) - Configuration management
- [Code with Hugo: Parallel HTTP Requests](https://codewithhugo.com/parallel-http-request-golang/) - Concurrency patterns
- [Medium: Golang Concurrency Patterns](https://medium.com/@ninucium/golang-concurrency-patterns-for-select-done-errgroup-and-worker-pool-645bec0bd3c9) - errgroup usage
- [OneUptime: Go Project Structure](https://oneuptime.com/blog/post/2026-01-07-go-project-structure/view) - Jan 2026 guidance
- [OneUptime: Context Propagation](https://oneuptime.com/blog/post/2026-02-01-go-context-propagation-microservices/view) - Feb 2026 patterns
- [OneUptime: Table-Driven Tests](https://oneuptime.com/blog/post/2026-01-07-go-table-driven-tests/view) - Jan 2026 testing
- [BytesizeGo: CLI Structure](https://www.bytesizego.com/blog/structure-go-cli-app) - CLI patterns

### Low Confidence (Community Opinions, Not Verified)
- Various Stack Overflow discussions on net/http vs fasthttp performance
- Blog posts on Go project structure (golang-standards/project-layout controversy)

---
*Stack research for: REST API composition gateway*
*Researched: 2026-02-03*
*Overall confidence: HIGH - All core libraries verified with official documentation or package registries*
