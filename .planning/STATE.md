# Project State: Restitch

**Last Updated:** 2026-02-03
**Current Phase:** 4 - Error Handling & Resilience (COMPLETE)
**Current Plan:** 04 of 04 complete
**Status:** Phase 4 COMPLETE - all error handling and resilience features implemented

## Project Reference

**Core Value:** Frontend teams can compose data from multiple backend services without writing, deploying, or maintaining BFF code.

**What We're Building:** REST API composition gateway (restitch-gateway) that eliminates hand-written BFF layers through declarative YAML configuration. Think Apollo Router for REST APIs.

**Current Focus:** Phase 4 COMPLETE. Error handling foundation, optional step orchestration, partial response building, and error rule matching all implemented. Ready for Phase 5 (Observability).

## Current Position

**Phase:** 4 of 5 (Error Handling & Resilience) - COMPLETE
**Plan:** 04 of 04 complete
**Status:** All error handling features verified
**Last activity:** 2026-02-03 - Completed Phase 4 verification (20/20 must-haves passed)

**Progress:**
```
[############################################      ] 88% (28/32)
Phase 4 COMPLETE - error handling and resilience working
```

**Next Actions:**
1. Begin Phase 5 planning (Observability)

## Performance Metrics

**Velocity:** 4 min for 01-01 (3 tasks), 3 min for 01-02 (2 tasks), 3 min for 01-03 (2 tasks), 5 min for 02-01 (3 tasks), 3 min for 02-02 (2 tasks), 9 min for 02-03 (2 tasks), 4 min for 02-04 (3 tasks), 2 min for 02-05 (3 tasks, gap closure), 2 min for 03-01 (3 tasks), 2 min for 03-02 (2 tasks), 2 min for 03-03 (2 tasks), 3 min for 03-04 (2 tasks), 5 min for 03-05 (3 tasks), 3 min for 04-01 (3 tasks), 3 min for 04-02 (3 tasks), 3 min for 04-03 (3 tasks), 2 min for 04-04 (3 tasks)

**Phase Completion:**
- Phase 1: 6/6 requirements (100%) - COMPLETE
- Phase 2: 11/11 requirements (100%) - COMPLETE
- Phase 3: 6/6 requirements (100%) - COMPLETE
- Phase 4: 5/5 requirements (100%) - COMPLETE
- Phase 5: 0/4 requirements (0%)

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
- Decision: Error sanitization hides internal details (timeout → "timeout", others → "upstream error")
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
- [ ] Begin Phase 5 planning (Observability)

### Known Blockers

None identified.

### Research Flags

**Phase 1:** SKIP research (standard Go patterns, well-documented) - COMPLETE
**Phase 2:** SKIP research (Expr and YAML libraries have excellent docs) - COMPLETE
**Phase 3:** Research COMPLETE - OAuth2 patterns documented in 03-RESEARCH.md
**Phase 4:** Research COMPLETE - Error handling patterns documented in 04-RESEARCH.md
**Phase 5:** SKIP research (standard observability patterns)

## Session Continuity

### What Just Happened

Phase 4 COMPLETE - All error handling and resilience features verified:

Plan deliverables:
- 04-01: Config schema with Optional, Timeout, ErrorRules; step timeout execution with context.WithTimeout
- 04-02: Error types (StepErrorDetail), executor rewritten with sync.WaitGroup for optional step support
- 04-03: Partial response building with _errors array injection, X-Partial-Response header
- 04-04: Error rule matching for status code replacement, matches recorded in _errors

Verification: 20/20 must-haves passed (100%)

Key commits:
- 94edaed, f0258e5, 3f529ee: Error handling config and timeout
- 580001e, 3bb129c, 606eb8e: Optional step orchestration
- 77c336f, e49f0c1, a2e7c6c: Partial response building
- 97b63de, 520caee, 8b086cd: Error matching rules

Patterns established:
- Timeout hierarchy: step > upstream > 30s default
- sync.WaitGroup replaces errgroup for optional step support
- allErrors aggregates errors across all waves
- X-Partial-Response header for partial response signaling
- _errors array injected into response body
- Error rule matches recorded transparently

### What's Next

Phase 5 (Observability):
- Structured JSON logging for all requests
- Request ID, method, path, status, duration in logs
- Per-step timing for waterfall debugging
- DAG execution order logging

### Context for Next Session

If returning after break:
1. Check "Current Position" above for phase/plan status
2. Review "Active TODOs" for immediate next actions
3. Check "Known Blockers" for anything preventing progress
4. Review `.planning/ROADMAP.md` for full phase structure
5. Review completed summaries at `.planning/phases/04-error-handling-resilience/04-0X-SUMMARY.md`

Key files:
- `.planning/ROADMAP.md` - Phase structure and success criteria
- `.planning/REQUIREMENTS.md` - All 32 v1 requirements with traceability
- `.planning/phases/04-error-handling-resilience/04-CONTEXT.md` - Phase 4 decisions
- `.planning/phases/04-error-handling-resilience/04-RESEARCH.md` - Error handling and resilience patterns
- `.planning/phases/04-error-handling-resilience/04-VERIFICATION.md` - Phase 4 verification report
- `.planning/config.json` - Project configuration (mode: yolo, depth: standard)
- `cmd/restitch/main.go` - Application entrypoint with composition config loading
- `internal/server/` - Server, router, TLS, health, middleware, shutdown
- `internal/client/` - HTTP client with optimized connection pooling
- `internal/composition/` - Complete composition engine with error handling
- `internal/config/env.go` - Environment variable expansion with validation
- `internal/auth/` - All authentication strategies (header, basic, passthrough, oauth2)

---

*State updated: 2026-02-03*
