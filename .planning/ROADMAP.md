# Roadmap: Restitch

**Project:** REST API composition gateway
**Core Value:** Frontend teams can compose data from multiple backend services without writing, deploying, or maintaining BFF code.
**Created:** 2026-02-03
**Status:** Active

## Overview

Restitch delivers a production-ready REST API composition gateway through 5 phases. Phase 1 establishes HTTP infrastructure. Phase 2 implements the core composition engine with DAG-based parallel execution. Phase 3 adds authentication for real backend integrations. Phase 4 adds graceful degradation for production reliability. Phase 5 completes observability for operations.

Each phase delivers a complete, verifiable capability. Dependencies flow naturally: gateway infrastructure -> composition engine -> authentication -> error handling -> observability.

## Phases

### Phase 1: Gateway Foundation

**Goal:** Gateway can receive requests, route to handlers, and respond with basic health checks

**Dependencies:** None (foundational)

**Requirements:**
- GATE-01: Gateway routes requests by path pattern to compositions
- GATE-02: Gateway routes requests by HTTP method
- GATE-03: Gateway terminates TLS (HTTPS support)
- GATE-04: Gateway exposes /health endpoint for liveness checks
- GATE-05: Gateway exposes /ready endpoint for readiness checks
- GATE-06: Gateway drains connections gracefully on shutdown signal

**Success Criteria:**
1. User can send HTTP request to gateway and receive response
2. User can send HTTPS request to gateway with valid TLS certificate
3. User can query /health endpoint and receive 200 OK when gateway is running
4. User can query /ready endpoint and receive 200 OK when gateway can accept traffic
5. User can send SIGTERM signal and observe connections drain before shutdown

**Plans:** 3 plans
- [x] 01-01-PLAN.md — Project foundation, HTTP/HTTPS server, path/method routing
- [x] 01-02-PLAN.md — Health/ready endpoints, request logging middleware
- [x] 01-03-PLAN.md — Graceful shutdown, HTTP client foundation

**Status:** Complete
**Completed:** 2026-02-03

---

### Phase 2: Composition Engine

**Goal:** Users can define multi-step compositions that fetch data from multiple APIs in parallel and merge responses

**Dependencies:** Phase 1 (requires working HTTP server and routing)

**Requirements:**
- COMP-01: Gateway parses YAML configuration defining compositions
- COMP-02: Compositions define steps with upstream service calls
- COMP-03: Steps can declare dependencies on other steps
- COMP-04: Gateway builds DAG from step dependencies
- COMP-05: Independent steps execute in parallel via goroutines
- COMP-06: Dependent steps wait for their dependencies to complete
- COMP-07: Expr language evaluates dynamic values in step paths
- COMP-08: Expr language evaluates dynamic values in step params
- COMP-09: Expr language evaluates dynamic values in response body
- COMP-10: Steps can access results from completed dependency steps
- COMP-11: Response body merges/reshapes data from multiple steps

**Success Criteria:**
1. User can define composition in YAML with multiple steps and gateway executes them
2. User can define independent steps and observe them execute in parallel (logs show concurrent execution)
3. User can define step with dependency and observe it waits for dependency to complete before executing
4. User can use Expr syntax to reference values from previous steps in paths, params, and response (e.g., `steps.user.id`)
5. User receives merged response combining data from all successful steps

**Plans:** 5 plans
- [x] 02-01-PLAN.md — YAML config schema, parsing, expression compilation
- [x] 02-02-PLAN.md — DAG builder, dependency inference from expressions
- [x] 02-03-PLAN.md — Step executor, parallel execution with errgroup
- [x] 02-04-PLAN.md — Response merging, HTTP handler, main.go integration
- [x] 02-05-PLAN.md — Gap closure: validation timing fix (parse-time DAG validation)

**Status:** Complete
**Completed:** 2026-02-03

---

### Phase 3: Upstream Authentication

**Goal:** Users can configure authentication strategies for upstream services without writing code

**Dependencies:** Phase 2 (requires working composition engine to send authenticated requests)

**Requirements:**
- AUTH-01: Upstream auth strategy: header (static API key/token injection)
- AUTH-02: Upstream auth strategy: basic (username/password)
- AUTH-03: Upstream auth strategy: passthrough (forward caller's auth header)
- AUTH-04: Upstream auth strategy: oauth2_client_credentials
- AUTH-05: OAuth2 tokens are cached and auto-refreshed before expiry
- AUTH-06: Each upstream can have its own auth configuration

**Success Criteria:**
1. User can configure header auth in YAML and observe API key sent to upstream
2. User can configure basic auth in YAML and observe username/password sent to upstream
3. User can configure passthrough auth and observe client's Authorization header forwarded to upstream
4. User can configure OAuth2 client credentials and observe gateway automatically fetch and use access token
5. User can observe OAuth2 tokens reused across requests (not fetched every time) and refreshed before expiry

**Plans:** 5 plans
- [x] 03-01-PLAN.md — Environment variable validation, auth config schema, strategy interface
- [x] 03-02-PLAN.md — Header and basic auth strategies with RoundTripper pattern
- [x] 03-03-PLAN.md — Passthrough auth strategy with missing header handling
- [x] 03-04-PLAN.md — OAuth2 client credentials with token caching and singleflight
- [x] 03-05-PLAN.md — Integration wiring: auth strategies into composition engine

**Status:** Complete
**Completed:** 2026-02-03

---

### Phase 4: Error Handling & Resilience

**Goal:** Users receive partial data when optional upstreams fail, making gateway more reliable than direct API calls

**Dependencies:** Phase 3 (requires working compositions with real upstreams to test failure scenarios)

**Requirements:**
- ERR-01: Compositions define error matching rules on step status codes
- ERR-02: Matched errors return configured status code and body
- ERR-03: Steps can be marked as optional (non-blocking on failure)
- ERR-04: Optional step failures return partial response with remaining data
- ERR-05: Upstream timeouts are configurable per step

**Success Criteria:**
1. User can define error matching rules and observe gateway return configured status/body when upstream returns specific error
2. User can mark step as optional and observe gateway continue execution when that step fails
3. User receives partial response with data from successful steps when optional step fails (not 500 error)
4. User can configure timeout per step and observe gateway cancel request after timeout expires
5. User receives response indicating partial data (via header or status code) when some steps fail

**Plans:** 4 plans
- [x] 04-01-PLAN.md — Config schema (optional, timeout, error_rules) and step timeout execution
- [x] 04-02-PLAN.md — Error types and optional step orchestration (sync.WaitGroup)
- [x] 04-03-PLAN.md — Partial response builder and X-Partial-Response header
- [x] 04-04-PLAN.md — Error matching rules for status code replacement

**Status:** Complete
**Completed:** 2026-02-03

---

### Phase 5: Observability

**Goal:** Users can debug composition failures and monitor gateway performance through structured logs and health checks

**Dependencies:** Phase 4 (requires complete composition functionality to observe)

**Requirements:**
- OBS-01: Structured JSON logging for all requests
- OBS-02: Logs include request ID, method, path, status, duration
- OBS-03: Per-step timing logged (which step took how long)
- OBS-04: DAG execution order logged for debugging

**Success Criteria:**
1. User can view structured JSON logs for each request with all required fields (request ID, method, path, status, duration)
2. User can trace specific request through logs using request ID
3. User can identify slow steps by examining per-step timing in logs
4. User can understand composition execution order by examining DAG execution logs
5. User can query health endpoint and receive upstream connectivity status

**Status:** Pending

---

## Progress

| Phase | Name | Requirements | Status | Completion |
|-------|------|--------------|--------|------------|
| 1 | Gateway Foundation | 6 | Complete | 100% |
| 2 | Composition Engine | 11 | Complete | 100% |
| 3 | Upstream Authentication | 6 | Complete | 100% |
| 4 | Error Handling & Resilience | 5 | Complete | 100% |
| 5 | Observability | 4 | Pending | 0% |

**Overall:** 28/32 requirements complete (88%)

---

## Dependencies

```
Phase 1: Gateway Foundation
    |
Phase 2: Composition Engine
    |
Phase 3: Upstream Authentication
    |
Phase 4: Error Handling & Resilience
    |
Phase 5: Observability
```

**Rationale:**
- Phase 2 requires Phase 1's HTTP server to handle composition requests
- Phase 3 requires Phase 2's composition engine to send authenticated upstream requests
- Phase 4 requires Phase 3's real upstream integrations to test failure scenarios
- Phase 5 requires Phase 4's complete functionality to observe and monitor

---

## Research Context

Research identified critical patterns and pitfalls that inform phase execution:

**Phase 1 must establish:**
- Proper HTTP connection pooling (`MaxIdleConnsPerHost: 100`)
- Context propagation from request through all goroutines
- Graceful goroutine cleanup (defer body close, io.Copy drain)

**Phase 2 core technologies:**
- gopkg.in/yaml.v3 for YAML parsing
- expr-lang/expr v1.17.7 for expression evaluation
- golang.org/x/sync/errgroup for parallel DAG execution

**Phase 3 authentication:**
- golang.org/x/oauth2 for OAuth2 client credentials
- golang.org/x/sync/singleflight for concurrent token refresh protection
- RoundTripper pattern for auth injection per upstream

**Phase 4 resilience patterns:**
- Optional vs required dependency marking
- Partial response with X-Partial-Response header
- Context timeout per step with hierarchy (step > upstream > 30s default)
- sync.WaitGroup replaces errgroup for optional step support

**Phase 5 observability:**
- Structured logging with request ID propagation
- Per-step timing for waterfall debugging
- Health checks with upstream connectivity validation

**Critical pitfalls to avoid:**
1. Goroutine leaks from unclosed HTTP response bodies
2. Connection pool misconfiguration (default is only 2 connections)
3. Context propagation failures causing orphaned goroutines
4. Retry storms overwhelming struggling upstreams
5. Expression language injection vulnerabilities
6. Timeout cascade failures (wrong hierarchy)
7. Authentication token memory leaks from unbounded caching

See `.planning/research/SUMMARY.md` for complete analysis.

---

*Last updated: 2026-02-03 (Phase 4 complete)*
