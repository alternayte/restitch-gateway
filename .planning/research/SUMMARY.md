# Project Research Summary

**Project:** Restitch - REST API composition gateway
**Domain:** API Gateway / BFF (Backend for Frontend)
**Researched:** 2026-02-03
**Confidence:** HIGH

## Executive Summary

Restitch is a REST API composition gateway built to eliminate BFF boilerplate for frontend teams. Research shows this domain is mature with established patterns from Kong, KrakenD, and Apollo Router. The winning approach combines Go's excellent HTTP performance with DAG-based parallel execution, declarative composition via YAML, and expression language for dynamic value interpolation. This enables GraphQL-level developer experience without GraphQL's complexity.

The recommended stack centers on Go 1.23+ with `goccy/go-yaml` (the actively maintained YAML parser since go-yaml/yaml was archived April 2025), `expr-lang/expr` for type-safe expression evaluation (proven at Google, Uber, ByteDance), and stdlib `net/http` with properly tuned connection pooling. The core differentiator is intelligent composition with graceful degradation - returning partial data when optional upstreams fail rather than all-or-nothing failures that make gateways more brittle than direct API calls.

Critical risks are primarily operational: goroutine/connection leaks from improper HTTP client usage (4-5x latency penalty), retry storms overwhelming struggling upstreams, expression language injection vulnerabilities, and timeout cascade failures. All are preventable with established Go patterns and must be addressed from Phase 1. The architecture follows a proven 3-layer pattern (HTTP server → DAG executor → HTTP client) with clear separation of concerns enabling testability and horizontal scaling.

## Key Findings

### Recommended Stack

Go provides the ideal foundation for high-throughput composition gateways: native goroutines enable true parallel execution, stdlib HTTP/2 support with connection pooling delivers excellent performance, and static binary deployment eliminates runtime dependencies. The stack is production-proven and actively maintained as of January 2026.

**Core technologies:**
- **Go 1.23+**: Native goroutines for parallel DAG execution, excellent HTTP performance, static binaries for easy deployment
- **goccy/go-yaml v1.19.2**: Active maintenance (go-yaml/yaml archived April 2025), superior error messages, passes 60 more YAML test cases
- **expr-lang/expr v1.17.7**: Type-safe expression evaluation for step dependencies, memory-safe (no side effects), optimizing compiler with bytecode VM, production-proven (Google, Uber, ByteDance)
- **net/http (stdlib)**: HTTP/2 support, connection pooling, context-aware, sufficient performance for gateway use case (fasthttp only 30-70% faster in real-world with API incompatibility)
- **golang.org/x/oauth2 v0.34.0**: Official OAuth2 implementation with auto-refreshing tokens for client credentials flow
- **golang.org/x/sync/errgroup**: Parallel execution with error handling and bounded concurrency (SetLimit since Go 1.20)

**Critical configuration:** Must set `MaxIdleConnsPerHost: 100` (default is 2) or suffer 4-5x latency penalty from connection churn. Use single shared `http.Client` instance per upstream, never create new clients per request.

### Expected Features

The research categorized 29 features into table stakes (15), differentiators (17), and anti-features (10). Restitch's v1 scope correctly prioritizes composition as the core differentiator.

**Must have (table stakes):**
- Request routing, error handling, timeout configuration (infrastructure baseline)
- Authentication (header, basic, passthrough, OAuth2) - users assume gateways handle auth
- TLS/SSL termination, CORS handling (security/browser requirements)
- Observability (logs, basic health checks) - cannot debug without visibility
- Declarative YAML configuration (GitOps expectation in 2026)

**Should have (competitive advantage):**
- **API Composition with parallel execution** - core value proposition, eliminates BFF code
- **Expression language for dependencies** - enables declarative step chaining without code
- **Graceful degradation** - return partial data when optional upstreams fail (KrakenD has this, Kong requires plugins)
- Distributed tracing (OpenTelemetry) - needed to debug multi-hop composition
- Response grouping/namespacing - prevent field name collisions in merged responses

**Defer (v2+):**
- Developer portal, entity-level caching, field selection (optimization features)
- Multi-protocol support (gRPC, GraphQL) - REST proves concept first
- Real-time config updates (hot-reload acceptable initially)
- AI/LLM integration (not target audience in 2026)

**Anti-features to avoid:** Gateway as ESB (creates single point of failure), complex business logic in gateway (violates separation of concerns), synchronous fan-out to 20+ services (latency compounds), database access from gateway (bypasses service boundaries).

### Architecture Approach

High-performance Go API gateways follow a 3-layer pattern separating HTTP handling, orchestration logic, and upstream communication. This enables horizontal scaling (stateless design), testability (layers mock cleanly), and performance tuning (connection pool per layer).

**Major components:**
1. **HTTP Server Layer** (Router + Middleware) - Accept requests, auth/logging/CORS, route to composition config. Uses standard `http.Handler` chain pattern.
2. **DAG Execution Layer** (Scheduler + Expr evaluator) - Parse step dependencies, execute independent steps in parallel goroutines, evaluate dynamic expressions (`steps.auth.token`), collect results with proper error handling. This is the most complex component.
3. **HTTP Client Layer** (Connection pool + upstream clients) - Execute upstream requests with connection reuse, handle retries/timeouts, propagate context cancellation. Single shared client per upstream service, NOT per request.

**Key patterns:**
- **Middleware chain** - Composable cross-cutting concerns (auth, logging, tracing)
- **DAG parallel execution with errgroup** - Schedule independent steps concurrently, synchronize with proper error propagation and context cancellation
- **Connection pool tuning** - `MaxIdleConnsPerHost: 100` enables connection reuse (4-5x latency improvement)
- **Expression compilation** - Compile Expr templates once at config load, reuse across requests
- **Configuration hot-reload with fsnotify** - Watch YAML files, atomic config swap on valid changes

**Data flow:** Request → Middleware (auth/logging) → Route matching → DAG construction → Parallel goroutines (Expr eval → HTTP client → upstream) → Result aggregation → Response merge → JSON serialization → Client

**Scaling:** Stateless design enables horizontal scaling. First bottleneck at 1k-10k req/s is connection pool (fix with tuning), second is CPU (JSON marshal, Expr eval - use compilation caching), third is memory (GC pressure - use sync.Pool for buffers).

### Critical Pitfalls

Research identified 8 critical pitfalls, all preventable but commonly missed:

1. **Goroutine leaks from HTTP clients** - Response bodies not closed/drained, connections cannot be returned to pool, goroutines accumulate in `readLoop`/`writeLoop`. Prevention: Always `defer resp.Body.Close()` and `io.Copy(io.Discard, resp.Body)`, use context cancellation for all goroutines. Must fix in Phase 1, test with `goleak`.

2. **Connection pool misconfiguration** - Default `MaxIdleConnsPerHost: 2` causes connection churn with parallel execution (48 of 50 requests create new connections). Results in 4-5x latency penalty, port exhaustion. Prevention: Set `MaxIdleConnsPerHost: 100`, create ONE client per upstream at startup. Must fix in Phase 1.

3. **Context propagation failures** - Spawning goroutines with `context.Background()` instead of request context means timeouts/cancellations don't cascade, wasting resources on abandoned work. Prevention: Always derive from request context, test that cancellation terminates all goroutines within 100ms. Must fix in Phase 1 (DAG executor).

4. **Retry storms and cascade failures** - Retrying all 5xx errors immediately can amplify load 3.5x during upstream struggles, taking down the upstream completely. Prevention: Exponential backoff with jitter, only retry transient failures (network errors, 429, maybe 503), set max 2-3 attempts, implement retry budget (stop if >20% of requests retry). Must fix in Phase 1 (basic retry) and Phase 4 (circuit breakers).

5. **Expression language injection vulnerability** - If unsanitized user input flows into Expr evaluation, attackers can execute arbitrary code. Prevention: Whitelist allowed functions, disable os/exec packages, treat user input as variables NOT expression AST, set max execution time. Must fix in Phase 1 (Expr integration).

6. **Partial failure handling without graceful degradation** - Returning 500 when 1 of 5 upstreams fails (even if optional) makes gateway MORE fragile than direct calls. Prevention: Mark dependencies as required/optional in YAML, return partial results with `X-Partial-Response: true` header, consider 207 Multi-Status. Phase 2 (error handling) and Phase 3 (degradation).

7. **Timeout configuration cascade failures** - Gateway timeout too short for multi-hop composition, or timeout hierarchy inverted (gateway < upstream), or no timeouts at all. Prevention: Establish hierarchy (client > load balancer > gateway > upstream), calculate timeout budget (parallel = max + 500ms, sequential = sum + 500ms), propagate deadlines. Must fix in Phase 1.

8. **Authentication token management memory leaks** - Caching OAuth2 tokens without eviction causes unbounded memory growth, OR not caching causes 200ms+ latency per request. Prevention: Use LRU cache with max size, TTL = token expiry - 60s, cache per upstream (not per user for client credentials). Phase 2 (auth) and Phase 3 (cache management).

## Implications for Roadmap

Based on research, v1 should deliver core composition functionality with production-grade reliability. The architecture naturally groups into 4 phases based on dependencies.

### Phase 1: Foundation (HTTP + DAG + Expr)
**Rationale:** Must establish end-to-end request flow before adding complexity. Single step execution validates HTTP client patterns, then DAG adds parallelism, then Expr enables dependencies.

**Delivers:**
- HTTP server with basic routing
- Connection-pooled HTTP client (proper configuration from day 1)
- DAG executor with parallel goroutine scheduling
- Expr integration for dynamic values
- Context propagation (request → goroutines → upstreams)
- Basic retry logic with exponential backoff

**Addresses (from FEATURES.md):**
- API Composition (Basic) - core value proposition
- Expression Language - differentiator enabling declarative dependencies
- Automatic Parallelization - performance critical for composition viability
- Request Routing, Error Handling, Timeout Configuration - baseline features

**Avoids (from PITFALLS.md):**
- MUST: Goroutine leaks (proper body close, context cancellation)
- MUST: Connection pool misconfiguration (MaxIdleConnsPerHost: 100)
- MUST: Context propagation failures (derive from request context)
- MUST: Retry storms (exponential backoff, limited attempts)
- MUST: Expression injection (whitelist functions, no os/exec)
- MUST: Timeout cascade (establish hierarchy, propagate deadlines)

**Research flag:** SKIP - well-documented patterns (Go stdlib, Expr library docs, established gateway patterns)

### Phase 2: Configuration System
**Rationale:** Core components (Phase 1) must work first since config drives their behavior. YAML parsing, validation, and hot-reload enable external configuration.

**Delivers:**
- YAML composition definition parsing (goccy/go-yaml)
- Config validation (detect cycles, required fields, invalid expressions)
- fsnotify-based hot-reload with atomic swap
- Error handling and rollback (keep previous config on invalid changes)
- Startup validation (reject invalid configs before serving traffic)

**Uses (from STACK.md):**
- goccy/go-yaml v1.19.2 (go-yaml/yaml archived April 2025)
- fsnotify for file watching
- atomic.Value or sync.RWMutex for safe config swap

**Implements (from ARCHITECTURE.md):**
- Configuration hot-reload pattern (watch → parse → validate → compile Expr → atomic swap)
- Zero-downtime updates

**Avoids (from PITFALLS.md):**
- Config errors crashing gateway (validation before swap)
- Missing startup validation (fail fast on invalid configs)

**Research flag:** SKIP - standard Go patterns (YAML parsing, fsnotify, atomic updates)

### Phase 3: Authentication
**Rationale:** With composition and config working, add auth to support real backend integrations. All four auth types needed for production use.

**Delivers:**
- Header-based auth (forward static headers)
- Basic auth (username/password per upstream)
- Passthrough auth (forward client auth to upstream)
- OAuth2 client credentials flow with token caching

**Uses (from STACK.md):**
- golang.org/x/oauth2 for client credentials
- LRU cache for token management (bounded, with TTL)

**Implements (from ARCHITECTURE.md):**
- Middleware pattern for auth injection
- Per-upstream auth configuration

**Avoids (from PITFALLS.md):**
- MUST: Token cache memory leaks (LRU with max size, TTL-based expiry)
- MUST: Logging sensitive data (redact Authorization headers)
- Header injection (whitelist forwarded headers, sanitize)

**Research flag:** SKIP for basic auth patterns, MAY NEED for OAuth2 edge cases (token refresh failures, concurrent requests, cache invalidation)

### Phase 4: Graceful Degradation
**Rationale:** With basic composition working, add resilience to make gateway LESS fragile than direct calls. This is the key to production readiness.

**Delivers:**
- Required vs optional dependency marking in YAML
- Partial response handling (return data from successful steps)
- Response status indicators (X-Partial-Response header, 207 Multi-Status)
- Error detail exposure control (scrub internal details)

**Implements (from ARCHITECTURE.md):**
- Result aggregation with per-node error collection
- Partial response serialization

**Addresses (from FEATURES.md):**
- Graceful Degradation - differentiator (KrakenD has this, others don't)
- Better UX than all-or-nothing failures

**Avoids (from PITFALLS.md):**
- MUST: Partial failure handling missing (mark dependencies as optional)
- Gateway more fragile than direct calls (degradation improves reliability)
- Silent degradation (include headers/fields indicating partial response)

**Research flag:** SKIP - well-documented pattern (AWS graceful degradation best practices, KrakenD implementation)

### Phase 5: Observability
**Rationale:** Core functionality complete, add visibility for production operation and debugging.

**Delivers:**
- Structured logging with request IDs
- Health check endpoint with upstream connectivity checks
- Basic metrics (request count, latency, error rate)
- Distributed tracing preparation (trace ID propagation)

**Uses (from STACK.md):**
- stdlib log or structured logger
- Prometheus client (if metrics needed)
- OpenTelemetry for tracing (Phase 5+)

**Implements (from ARCHITECTURE.md):**
- Middleware for logging and tracing context
- Health check integration with DAG executor

**Avoids (from PITFALLS.md):**
- No health checks (expose /health endpoint)
- Generic error messages (return request ID, structured errors)
- Missing request tracing (include X-Request-ID)

**Research flag:** SKIP - standard observability patterns (Go logging, Prometheus, OpenTelemetry)

### Phase Ordering Rationale

- **Phase 1 before 2:** Must have working HTTP client and DAG executor before configuration can drive them
- **Phase 1 before 3:** Auth requires working HTTP client to inject credentials into requests
- **Phase 3 before 4:** Degradation needs real upstreams to test partial failures (auth enables real backends)
- **Phase 5 last:** Observability is nice-to-have, core must work first

**Dependency chain:** HTTP client → DAG executor → Expr → Config → Auth → Degradation → Observability

**Pitfall prevention:** Phase 1 must get HTTP patterns right (connection pooling, context propagation, goroutine cleanup) because all later phases depend on this foundation. Late fixes to HTTP client patterns require refactoring all phases.

### Research Flags

**Phases needing deeper research during planning:**
- **Phase 3 (Auth):** OAuth2 edge cases (token refresh during high concurrency, cache invalidation strategies, multiple token endpoints) - niche domain with sparse production examples
- **Phase 4 (Degradation):** Decision on 200 vs 207 Multi-Status for partial failures, client expectations for partial responses - requires UX research

**Phases with standard patterns (skip research-phase):**
- **Phase 1 (Foundation):** Well-documented Go patterns (stdlib HTTP, errgroup, context), established gateway architectures (KrakenD, Kong design docs)
- **Phase 2 (Config):** Standard YAML parsing, fsnotify usage, hot-reload patterns documented
- **Phase 5 (Observability):** Prometheus, OpenTelemetry, structured logging all have official docs

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All core libraries verified with official pkg.go.dev, recent releases (expr v1.17.7 Dec 2025, oauth2 v0.34.0 Dec 2025, goccy/go-yaml v1.19.2 Jan 2026) |
| Features | HIGH | Cross-referenced Kong, KrakenD, Tyk, Apollo Router official docs, AWS/Azure BFF patterns, 2026 industry analysis from multiple sources |
| Architecture | MEDIUM-HIGH | Based on authoritative sources (official Go blogs, stdlib docs, production gateway implementations), some patterns inferred from community sources |
| Pitfalls | HIGH | Verified with Go issue tracker (#25621 goroutine leak), OWASP security guidance, AWS Well-Architected Framework, recent 2025-2026 articles from production teams |

**Overall confidence:** HIGH

Research is comprehensive with official sources for all core technologies. Stack recommendations are conservative (proven libraries, avoid cutting-edge). Feature analysis cross-references multiple production gateways. Pitfalls verified against real-world production issues documented in Go issue tracker and recent blog posts.

### Gaps to Address

**Minor gaps requiring validation during implementation:**

- **OAuth2 concurrent token refresh:** golang.org/x/oauth2 handles this, but edge cases (multiple gateway instances, cache invalidation) need testing - validate in Phase 3 with concurrent request test
- **DAG library choice:** Research identified `github.com/heimdalr/dag` and `github.com/natessilva/dag` as options but didn't deeply compare - evaluate both in Phase 1, may need custom implementation if neither handles timeout propagation well
- **Expr expression compilation caching strategy:** Docs show how to compile but not best practice for caching compiled programs (per-route, global cache, sync.Pool) - design in Phase 1 based on profiling
- **Graceful degradation status codes:** Industry inconsistent on 200 vs 207 Multi-Status for partial failures, no clear winner - user research in Phase 4 to determine client expectations
- **Connection pool sizing:** Research suggests 100 as reasonable default but actual optimal value depends on upstream count and request parallelism - tune in Phase 1 based on load testing

All gaps are implementation details, not fundamental uncertainties. Core approach is validated.

## Sources

### Primary (HIGH confidence)
- [goccy/go-yaml v1.19.2](https://pkg.go.dev/github.com/goccy/go-yaml) - Official package docs, verified Jan 8 2026 release
- [expr-lang/expr v1.17.7](https://github.com/expr-lang/expr) - Official GitHub, verified Dec 15 2025 release, production users listed
- [golang.org/x/oauth2 v0.34.0](https://pkg.go.dev/golang.org/x/oauth2) - Official Google library, verified Dec 1 2025 release
- [Go stdlib documentation](https://pkg.go.dev/net/http) - net/http, context, sync packages
- [Kong Gateway Documentation](https://developer.konghq.com/gateway/) - Official docs, feature comparison
- [KrakenD Documentation](https://www.krakend.io/docs/) - Official docs, composition patterns, graceful degradation implementation
- [Apollo Router Releases](https://github.com/apollographql/router/releases) - GraphQL composition approach
- [AWS Well-Architected Framework](https://docs.aws.amazon.com/wellarchitected/latest/reliability-pillar/rel_mitigate_interaction_failure_graceful_degradation.html) - Graceful degradation best practices
- [Go issue #25621](https://github.com/golang/go/issues/25621) - Goroutine leak edge case verification
- [OWASP Expression Language Injection](https://owasp.org/www-community/vulnerabilities/Expression_Language_Injection) - Security guidance

### Secondary (MEDIUM confidence)
- [Mastering High-Performance API Gateways in Go](https://medium.com/@ksandeeptech07/mastering-high-performance-api-gateways-in-go-833310e8aeb4) - Architecture patterns, verified with multiple sources
- [HTTP Connection Pooling in Go](https://davidbacisin.com/writing/golang-http-connection-pools-1) - Connection pool tuning, cross-verified with Go stdlib docs
- [OneUptime Blog: Go Context Propagation](https://oneuptime.com/blog/post/2026-02-01-go-context-propagation-microservices/view) - Jan 2026 patterns
- [Best API Gateway in 2026: Top Tools, Features](https://www.digitalapi.ai/blogs/best-api-gateway) - Market landscape, feature expectations
- [The 5 Worst Anti-Patterns in API Management](https://thenewstack.io/the-5-worst-anti-patterns-in-api-management/) - Anti-feature validation
- [BFF Pattern - Sam Newman](https://samnewman.io/patterns/architectural/bff/) - Original pattern definition
- [10 Common API Resilience Design Patterns](https://api7.ai/blog/10-common-api-resilience-design-patterns) - Retry/circuit breaker patterns

### Tertiary (LOW confidence)
- Various Stack Overflow discussions on net/http vs fasthttp performance (claims of 6x improvement not verified in production benchmarks, real-world shows 30-70%)
- Golang project structure debates (golang-standards/project-layout controversy - used official Go module docs instead)

---
*Research completed: 2026-02-03*
*Ready for roadmap: yes*
