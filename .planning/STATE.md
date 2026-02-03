# Project State: Restitch

**Last Updated:** 2026-02-03
**Current Phase:** 3 - Upstream Authentication (IN PROGRESS)
**Current Plan:** 02 of 05 complete
**Status:** Phase 3 in progress - header and basic auth strategies complete

## Project Reference

**Core Value:** Frontend teams can compose data from multiple backend services without writing, deploying, or maintaining BFF code.

**What We're Building:** REST API composition gateway (restitch-gateway) that eliminates hand-written BFF layers through declarative YAML configuration. Think Apollo Router for REST APIs.

**Current Focus:** Phase 3 IN PROGRESS. Building authentication strategies for upstream services. Header (AUTH-01) and Basic (AUTH-02) strategies complete.

## Current Position

**Phase:** 3 of 5 (Upstream Authentication) - IN PROGRESS
**Plan:** 02 of 05 complete
**Status:** Header and basic auth strategies complete
**Last activity:** 2026-02-03 - Completed 03-02-PLAN.md (Header and Basic auth strategies)

**Progress:**
```
[###################                               ] 40% (13/32)
Phase 3 plan 2 complete - header and basic auth working
```

**Next Actions:**
1. Execute Plan 03-03 (Passthrough authentication strategy) - NOTE: May already be done
2. Execute Plan 03-04 (OAuth2 client credentials strategy)
3. Execute Plan 03-05 (Integration and factory pattern)

## Performance Metrics

**Velocity:** 4 min for 01-01 (3 tasks), 3 min for 01-02 (2 tasks), 3 min for 01-03 (2 tasks), 5 min for 02-01 (3 tasks), 3 min for 02-02 (2 tasks), 9 min for 02-03 (2 tasks), 4 min for 02-04 (3 tasks), 2 min for 02-05 (3 tasks, gap closure), 2 min for 03-01 (3 tasks), 2 min for 03-02 (2 tasks)

**Phase Completion:**
- Phase 1: 6/6 requirements (100%) - COMPLETE
- Phase 2: 11/11 requirements (100%) - COMPLETE
- Phase 3: 2/6 requirements (33%) - IN PROGRESS
- Phase 4: 0/5 requirements (0%)
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
- [ ] Execute Plan 03-03 (Passthrough authentication strategy) - NOTE: May already be committed
- [ ] Execute Plan 03-04 (OAuth2 client credentials strategy)
- [ ] Execute Plan 03-05 (Integration and factory pattern)

### Known Blockers

None identified.

### Research Flags

**Phase 1:** SKIP research (standard Go patterns, well-documented) - COMPLETE
**Phase 2:** SKIP research (Expr and YAML libraries have excellent docs) - COMPLETE
**Phase 3:** Research COMPLETE - OAuth2 patterns documented in 03-RESEARCH.md
**Phase 4:** MAY NEED research for partial response UX (200 vs 207 Multi-Status decision)
**Phase 5:** SKIP research (standard observability patterns)

## Session Continuity

### What Just Happened

Phase 3 Plan 02 COMPLETE - Header and Basic auth strategies implemented:

Plan 03-02 deliverables:
- HeaderStrategy with RoundTripper pattern (internal/auth/header.go)
- BasicStrategy with SetBasicAuth (internal/auth/basic.go)
- Comprehensive tests for env var expansion and request cloning
- Both strategies clone requests before modification (retry safe)

Commits:
- cea63ce: Header authentication strategy
- 598ae7c: Basic authentication strategy

Patterns established:
- RoundTripper wrapper: clone request, modify, delegate to base
- Factory function validates env vars before returning strategy

Note: Passthrough strategy (501f959) was committed between header and basic - appears to be from previous session.

### What's Next

Phase 3 continues with remaining strategies:
- Check if Plan 03-03 (Passthrough) needs execution or is already done
- Execute Plan 03-04 (OAuth2 client credentials - token management)
- Execute Plan 03-05 (Integration - factory pattern, upstream client wiring)

### Context for Next Session

If returning after break:
1. Check "Current Position" above for phase/plan status
2. Review "Active TODOs" for immediate next actions
3. Check "Known Blockers" for anything preventing progress
4. Review `.planning/ROADMAP.md` for full phase structure
5. Review completed summaries at `.planning/phases/03-upstream-authentication/03-0*-SUMMARY.md`

Key files:
- `.planning/ROADMAP.md` - Phase structure and success criteria
- `.planning/REQUIREMENTS.md` - All 32 v1 requirements with traceability
- `.planning/phases/03-upstream-authentication/03-CONTEXT.md` - Phase 3 decisions
- `.planning/phases/03-upstream-authentication/03-RESEARCH.md` - OAuth2 and auth patterns
- `.planning/phases/03-upstream-authentication/03-01-SUMMARY.md` - Auth foundation summary
- `.planning/phases/03-upstream-authentication/03-02-SUMMARY.md` - Header/Basic auth summary
- `.planning/config.json` - Project configuration (mode: yolo, depth: standard)
- `cmd/restitch/main.go` - Application entrypoint with composition config loading
- `internal/server/` - Server, router, TLS, health, middleware, shutdown
- `internal/client/` - HTTP client with optimized connection pooling
- `internal/composition/` - Complete composition engine with parse-time validation
- `internal/config/env.go` - Environment variable expansion with validation
- `internal/auth/auth.go` - Strategy interface and config types
- `internal/auth/header.go` - HeaderStrategy implementation
- `internal/auth/basic.go` - BasicStrategy implementation

---

*State updated: 2026-02-03*
