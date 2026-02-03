# Project State: Restitch

**Last Updated:** 2026-02-03
**Current Phase:** 5 - Observability (IN PROGRESS)
**Current Plan:** 03 of 03 complete
**Status:** Phase 5 plans complete - awaiting verification

## Project Reference

**Core Value:** Frontend teams can compose data from multiple backend services without writing, deploying, or maintaining BFF code.

**What We're Building:** REST API composition gateway (restitch-gateway) that eliminates hand-written BFF layers through declarative YAML configuration. Think Apollo Router for REST APIs.

**Current Focus:** Phase 5 plans COMPLETE. Request ID tracing, step timing, and upstream health endpoint all implemented. Ready for final verification.

## Current Position

**Phase:** 5 of 5 (Observability) - IN PROGRESS
**Plan:** 03 of 03 complete
**Status:** All observability plans executed
**Last activity:** 2026-02-03 - Completed 05-03-PLAN.md (upstream health)

**Progress:**
```
[###############################################   ] 94% (31/33)
Phase 5 plans COMPLETE - awaiting verification
```

**Next Actions:**
1. Verify Phase 5 success criteria
2. Complete project!

## Performance Metrics

**Velocity:** 4 min for 01-01 (3 tasks), 3 min for 01-02 (2 tasks), 3 min for 01-03 (2 tasks), 5 min for 02-01 (3 tasks), 3 min for 02-02 (2 tasks), 9 min for 02-03 (2 tasks), 4 min for 02-04 (3 tasks), 2 min for 02-05 (3 tasks, gap closure), 2 min for 03-01 (3 tasks), 2 min for 03-02 (2 tasks), 2 min for 03-03 (2 tasks), 3 min for 03-04 (2 tasks), 5 min for 03-05 (3 tasks), 3 min for 04-01 (3 tasks), 3 min for 04-02 (3 tasks), 3 min for 04-03 (3 tasks), 2 min for 04-04 (3 tasks), 5 min for 05-01 (2 tasks), 4 min for 05-02 (2 tasks), 4 min for 05-03 (2 tasks)

**Phase Completion:**
- Phase 1: 6/6 requirements (100%) - COMPLETE
- Phase 2: 11/11 requirements (100%) - COMPLETE
- Phase 3: 6/6 requirements (100%) - COMPLETE
- Phase 4: 5/5 requirements (100%) - COMPLETE
- Phase 5: 4/4 requirements (100%) - AWAITING VERIFICATION

**Blockers:** None currently

## Accumulated Context

### Key Decisions

**2026-02-03 - Roadmap Creation**
- Decision: 5-phase structure following dependency chain (Gateway -> Composition -> Auth -> Resilience -> Observability)
- Rationale: Natural grouping by delivery boundaries, each phase delivers complete verifiable capability
- Impact: Foundation phase must establish HTTP patterns correctly (connection pooling, context propagation) as all later phases depend on this

**2026-02-03 - Phase 1 Must Avoid Critical Pitfalls**
- Decision: Phase 1 includes proper connection pooling, context propagation, and goroutine cleanup from start
- Rationale: Research identified these as common production issues; fixing later requires refactoring all phases
- Impact: Phase 1 includes patterns that seem like optimization but are actually correctness requirements

**2026-02-03 - Plan 01-01 Execution**
- Decision: Stdlib only for HTTP/TLS (no external dependencies)
- Decision: Router supports exact and prefix path matching
- Decision: TLS 1.2+ minimum with modern cipher preferences (P-256, X25519)
- Decision: HTTP and HTTPS servers run concurrently in goroutines
- Rationale: Clean foundation using Go's excellent stdlib, modern security defaults

**2026-02-03 - Plan 01-02 Execution**
- Decision: Health endpoint returns verbose JSON (status, uptime, version, memory)
- Decision: Ready endpoint checks gateway state only (upstream checking deferred)
- Decision: JSON logging by default, text format via --log-format=text flag
- Decision: Middleware pattern with ResponseWriter wrapper to capture status code
- Rationale: Standard Kubernetes probe patterns, 12-factor logging to stdout

**2026-02-03 - Plan 01-03 Execution**
- Decision: WaitForShutdownSignal returns channel for select-based coordination
- Decision: SetReady(false) called immediately on signal for instant /ready 503
- Decision: MaxIdleConnsPerHost: 100 to avoid 4-5x latency penalty from default 2
- Decision: DrainAndClose helper ensures proper connection pool return
- Rationale: Signal handling via channel + select allows coordination with error channel; HTTP client configured per research findings

**2026-02-03 - Plan 02-01 Execution**
- Decision: All expressions compile at parse time (fail fast on syntax errors)
- Decision: Template-style {{ expr }} delimiters per CONTEXT.md
- Decision: BuildBaseEnvironment for consistent compile/runtime environments
- Decision: CompiledConfig separates parsing from compilation
- Rationale: Catch expression errors at startup (not request time); avoid expr-lang Pitfall 3 (function registration)

**2026-02-03 - Plan 02-02 Execution**
- Decision: Kahn's algorithm for topological sort with level detection
- Decision: AST visitor pattern for dependency extraction from expressions
- Decision: Automatic dependency inference from steps.X references
- Decision: Circular dependency detection at config parse time (not request time)
- Rationale: Standard DAG algorithm with O(V+E) complexity; AST visitor handles all expr syntax correctly; fail fast on cycles

**2026-02-03 - Plan 02-03 Execution**
- Decision: Upstream HTTP errors (500, 404) are passthrough - not step failures per CONTEXT.md
- Decision: Network/context errors trigger fail-fast via errgroup cancellation
- Decision: Header propagation uses case-insensitive lookup (HTTP headers canonicalized)
- Decision: Template interpolation checked BEFORE Program check (templates have Program==nil)
- Rationale: Upstream errors are results (Phase 2 completes all steps); network failures are true errors; HTTP header standard behavior; correct template detection order

**2026-02-03 - Plan 02-04 Execution**
- Decision: CompiledResponse stores original body template for runtime evaluation
- Decision: Single expressions return typed values, templates return interpolated strings
- Decision: Config file optional - gateway starts with health endpoints if missing
- Decision: Added HTTPClient() method to client.Client for interface compatibility
- Rationale: Template evaluation requires original structure; type preservation for complex objects; graceful degradation without config; standard http.Client interface needed by composition handler

**2026-02-03 - Plan 02-05 Execution (Gap Closure)**
- Decision: Move BuildDAG from request-time to parse-time for fail-fast validation
- Decision: Store ExecutionPlan in CompiledComposition to avoid redundant DAG builds
- Decision: Invalid configs prevent gateway startup with clear error messages
- Rationale: UAT tests 9 and 10 revealed validation timing bug - circular dependencies and missing step references were detected at request time instead of startup; moving BuildDAG to compileComposition() fixed both issues per CONTEXT.md fail-fast principle

**2026-02-03 - Plan 03-01 Execution (Auth Foundation)**
- Decision: ExpandEnvWithValidation validates ALL ${VAR} references before any expansion
- Decision: Empty env var treated as error (not just missing) for security
- Decision: NoneStrategy as default passthrough (base RoundTripper unchanged)
- Decision: Config.Validate() enforces exactly one strategy per upstream (mutual exclusivity)
- Rationale: Fail-fast validation catches missing secrets at startup not runtime; Strategy interface with RoundTripper pattern follows Go HTTP middleware conventions; mutual exclusivity prevents config confusion

**2026-02-03 - Plan 03-02 Execution (Header and Basic Auth)**
- Decision: Request cloning before header modification for retry safety
- Decision: Env var expansion at strategy creation time, not per-request
- Decision: Use stdlib SetBasicAuth instead of hand-rolling base64 encoding
- Rationale: Cloning prevents duplicate headers on retries; fail-fast philosophy extends to credential resolution; stdlib functions are tested and secure

**2026-02-03 - Plan 03-03 Execution (Passthrough Strategy)**
- Decision: Return ErrMissingAuthHeader immediately when no client auth (security best practice)
- Decision: Clone request before forwarding to avoid modifying original
- Decision: IsMissingAuthHeaderError helper for handler-layer error detection
- Rationale: Per RESEARCH.md Pitfall 5 - don't forward unauthenticated requests to upstreams that expect authentication; handler layer needs clean error detection for 401 response

**2026-02-03 - Plan 03-04 Execution (OAuth2 Strategy)**
- Decision: 30-second expiry buffer per CONTEXT.md (fixed, not percentage-based)
- Decision: Initial token fetch at startup for fail-fast credential validation
- Decision: Singleflight for concurrent token refresh deduplication
- Rationale: Per RESEARCH.md patterns - ReuseTokenSourceWithExpiry for configurable buffer, singleflight.Group.Do() prevents thundering herd during concurrent requests

**2026-02-03 - Plan 03-05 Execution (Strategy Factory and Integration)**
- Decision: Context parameter for CompileConfig (OAuth2 needs context for initial token fetch)
- Decision: Authorization header added to propagated headers for passthrough support
- Decision: Per-request http.Client with auth transport, reuses underlying connection pool
- Rationale: Fail-fast validation at startup; passthrough needs Authorization forwarded; transport is stateless so new Client is safe

**2026-02-03 - Plan 04-01 Execution (Error Handling Config and Timeout)**
- Decision: Steps required by default (Optional field defaults to false)
- Decision: Timeout hierarchy: step > upstream > 30s default
- Decision: Step timeout as pointer (*time.Duration) to distinguish "not set" from "set to 0"
- Decision: Defer cancel() immediately after context creation for resource cleanup
- Rationale: Per CONTEXT.md "Steps required by default"; hierarchy allows fine-grained control; pointer enables nil fallback; defer prevents resource leaks per RESEARCH.md

**2026-02-03 - Plan 04-02 Execution (Optional Step Orchestration)**
- Decision: Replace errgroup with sync.WaitGroup for optional step support
- Decision: allErrors slice declared at start of Execute, aggregated after each wave
- Decision: Optional step failures collected in waveErrors, composition continues
- Decision: Required step failures still fail-fast with detailed error message
- Decision: Failed step dependencies cascade as "dependency_failed" skips
- Decision: Failed steps store nil result (available to dependents)
- Decision: Error sanitization hides internal details (timeout -> "timeout", others -> "upstream error")
- Rationale: Per RESEARCH.md Pattern 2 - custom orchestration enables error collection; allErrors tracks across all waves for complete partial response; nil results allow expressions to check for missing data

**2026-02-03 - Plan 04-03 Execution (Partial Response Building)**
- Decision: BuildResponse accepts stepErrors parameter for _errors injection
- Decision: X-Partial-Response header set only when IsPartial is true
- Decision: Failed optional steps exposed as null in steps.X expressions
- Decision: HTTP status remains 200 for partial responses (composition succeeded)
- Rationale: Per CONTEXT.md "Top-level `_errors` field in response body with failure details"; X-Partial-Response provides HTTP-level signaling; nil step results allow expressions like steps.inventory ?? defaultValue

**2026-02-03 - Plan 04-04 Execution (Error Matching Rules)**
- Decision: Error rule matches replace body but treat step as successful
- Decision: Matched errors always recorded in _errors array for transparency
- Decision: Error rule matches marked as optional=true to avoid failing composition
- Decision: matchErrorRule checks exact status code matches (no ranges/wildcards)
- Rationale: Per CONTEXT.md "Error matching rules replace the failed step's slot with the configured body value"; transparency via _errors maintains client awareness; simple list matching defers complexity to future

**2026-02-03 - Plan 05-01 Execution (Request ID and Enhanced Logging)**
- Decision: ULID format for request IDs (26 chars, time-sortable, collision-safe)
- Decision: Honor incoming X-Request-ID if present, generate ULID if not
- Decision: RequestIDMiddleware runs before LoggingMiddleware for context availability
- Decision: Snake_case field names for JSON logs (request_id, status_code, duration_ms)
- Rationale: ULID enables time-based log analysis; honoring incoming IDs supports distributed tracing; middleware order ensures context propagation; snake_case per CONTEXT.md requirements

**2026-02-03 - Plan 05-02 Execution (Step Timing)**
- Decision: Wave numbers 1-indexed for human readability in logs
- Decision: Timing recorded for all step outcomes (success, failed, skipped)
- Decision: Duration calculated from microseconds/1000 for ms precision
- Decision: findSlowestStep helper returns map[string]interface{} for structured logging
- Rationale: Human-readable wave numbers (wave: 1 vs wave: 0); complete timing data for all outcomes; float64 milliseconds for precision; structured map enables JSON logging

**2026-02-03 - Plan 05-03 Execution (Upstream Health Endpoint)**
- Decision: UpstreamInfo bridge type to avoid import cycle between server and composition
- Decision: HEAD request by default for minimal overhead (no body)
- Decision: GET request when health_path configured (assumes health endpoint returns body)
- Decision: 2xx/3xx = healthy, 4xx/5xx = unhealthy
- Decision: 10 second timeout for all upstream health checks
- Decision: /ready independent of upstream health (per CONTEXT.md)
- Decision: Always return HTTP 200 for monitoring tool compatibility
- Rationale: Bridge type pattern cleanly avoids import cycle; HEAD is efficient for connectivity check; GET appropriate for dedicated health endpoints; standard HTTP success ranges; timeout prevents hanging; /ready checks gateway state not upstreams

### Active TODOs

- [x] Create Phase 1 execution plan
- [x] Execute Plan 01-01 (HTTP/HTTPS server with routing)
- [x] Execute Plan 01-02 (Health endpoints)
- [x] Execute Plan 01-03 (Graceful shutdown)
- [x] Verify Phase 1 success criteria (PASSED)
- [x] Begin Phase 2 planning (Composition Engine)
- [x] Execute Plan 02-01 (YAML config parsing and expression compilation)
- [x] Execute Plan 02-02 (DAG construction with dependency inference)
- [x] Execute Plan 02-03 (Step execution and DAG executor)
- [x] Execute Plan 02-04 (Response merging and HTTP handler integration)
- [x] Verify Phase 2 success criteria (PASSED - with 2 issues identified)
- [x] Diagnose Phase 2 UAT gaps (Tests 9 and 10 - validation timing bugs)
- [x] Execute Plan 02-05 (Gap closure - move validation to parse-time)
- [x] Verify Phase 2 complete with all issues resolved
- [x] Begin Phase 3 planning (Authentication)
- [x] Execute Plan 03-01 (Auth foundation: env expansion, Strategy interface, config schema)
- [x] Execute Plan 03-02 (Header and Basic authentication strategies)
- [x] Execute Plan 03-03 (Passthrough authentication strategy)
- [x] Execute Plan 03-04 (OAuth2 client credentials strategy)
- [x] Execute Plan 03-05 (Strategy factory and integration)
- [x] Verify Phase 3 success criteria (PASSED)
- [x] Begin Phase 4 planning (Resilience)
- [x] Execute Plan 04-01 (Error handling config and timeout execution)
- [x] Execute Plan 04-02 (Optional step orchestration)
- [x] Execute Plan 04-03 (Partial response building)
- [x] Execute Plan 04-04 (Error matching rules)
- [x] Verify Phase 4 success criteria (PASSED - 20/20 must-haves)
- [x] Begin Phase 5 planning (Observability)
- [x] Execute Plan 05-01 (Request ID and enhanced logging) - commits 4f5ca97, 5f2beab
- [x] Execute Plan 05-02 (Step timing collection) - commits 7654c42, f61a4b8
- [x] Execute Plan 05-03 (Upstream health checks) - commits bec963b, 3a1b5b8
- [ ] Verify Phase 5 success criteria

### Known Blockers

None - import cycle issue in 05-03 resolved via UpstreamInfo bridge type.

### Research Flags

**Phase 1:** SKIP research (standard Go patterns, well-documented) - COMPLETE
**Phase 2:** SKIP research (Expr and YAML libraries have excellent docs) - COMPLETE
**Phase 3:** Research COMPLETE - OAuth2 patterns documented in 03-RESEARCH.md
**Phase 4:** Research COMPLETE - Error handling patterns documented in 04-RESEARCH.md
**Phase 5:** SKIP research (standard observability patterns) - COMPLETE

## Session Continuity

### What Just Happened

Plan 05-03 COMPLETE - Upstream health endpoint implemented:

Plan deliverables:
- HealthPath field in Upstream config for custom health check paths
- UpstreamHealthResponse and UpstreamStatus types for response structure
- UpstreamInfo bridge type to avoid import cycle
- checkUpstreamHealth function with HEAD/GET selection
- UpstreamHealthHandler for /health/upstreams endpoint
- Integration in main.go with UpstreamInfo map

Key commits:
- bec963b: Upstream config health path and checker
- 3a1b5b8: Upstream health endpoint and integration

Patterns established:
- Bridge types for avoiding import cycles
- Concurrent upstream checks with mutex-protected results
- HEAD by default, GET with health_path

Deviation: Import cycle required UpstreamInfo bridge type (Rule 3 - Blocking)

### What's Next

Phase 5 verification:
- All observability plans complete (05-01, 05-02, 05-03)
- Ready for final verification of Phase 5 success criteria

After Phase 5:
- Project complete! All 32 v1 requirements implemented

### Context for Next Session

If returning after break:
1. Check "Current Position" above for phase/plan status
2. Review "Active TODOs" for immediate next actions
3. Check "Known Blockers" for anything preventing progress
4. Review `.planning/ROADMAP.md` for full phase structure
5. Review completed summaries at `.planning/phases/05-observability/05-0X-SUMMARY.md`

Key files:
- `.planning/ROADMAP.md` - Phase structure and success criteria
- `.planning/REQUIREMENTS.md` - All 32 v1 requirements with traceability
- `.planning/phases/05-observability/05-CONTEXT.md` - Phase 5 decisions
- `.planning/config.json` - Project configuration (mode: yolo, depth: standard)
- `cmd/restitch/main.go` - Application entrypoint with middleware chain
- `internal/server/` - Server, router, TLS, health, middleware, shutdown
- `internal/server/health.go` - Health handlers including UpstreamHealthHandler
- `internal/client/` - HTTP client with optimized connection pooling
- `internal/composition/` - Complete composition engine with step timing
- `internal/observability/` - Request ID infrastructure (ULID)
- `internal/auth/` - All authentication strategies

---

*State updated: 2026-02-03*
