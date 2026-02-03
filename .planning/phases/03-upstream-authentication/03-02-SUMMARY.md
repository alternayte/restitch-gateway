---
phase: 03-upstream-authentication
plan: 02
subsystem: auth
tags: [http, roundtripper, header-injection, basic-auth, env-vars]

# Dependency graph
requires:
  - phase: 03-01
    provides: Strategy interface, HeaderConfig, BasicConfig, ExpandEnvWithValidation
provides:
  - HeaderStrategy implementing RoundTripper pattern with env var expansion
  - BasicStrategy implementing RoundTripper pattern with env var expansion
  - Request cloning pattern for retry safety
affects: [03-03, 03-04, 03-05]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "RoundTripper wrapper pattern for auth injection"
    - "Request cloning before header modification"
    - "Env var expansion at strategy creation time"

key-files:
  created:
    - "internal/auth/header.go"
    - "internal/auth/header_test.go"
    - "internal/auth/basic.go"
    - "internal/auth/basic_test.go"
  modified: []

key-decisions:
  - "Request cloning prevents modification of original request (safe for retries)"
  - "Env var expansion happens at strategy creation, not per-request"
  - "Use stdlib SetBasicAuth instead of hand-rolling base64 encoding"

patterns-established:
  - "RoundTripper wrapper: clone request, modify, delegate to base"
  - "Factory function validates env vars before returning strategy"

# Metrics
duration: 2min
completed: 2026-02-03
---

# Phase 3 Plan 02: Header and Basic Auth Strategies Summary

**Header and basic auth strategies implementing RoundTripper pattern with env var expansion and request cloning for retry safety**

## Performance

- **Duration:** 2 min
- **Started:** 2026-02-03T08:58:03Z
- **Completed:** 2026-02-03T09:00:09Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- HeaderStrategy injects configured header name/value with ${VAR} expansion
- BasicStrategy uses stdlib SetBasicAuth for proper base64 encoding
- Both strategies clone requests before modification (retry safe)
- Comprehensive test coverage for env var expansion, header injection, and clone verification

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement header authentication strategy** - `cea63ce` (feat)
2. **Task 2: Implement basic authentication strategy** - `598ae7c` (feat)

## Files Created/Modified

- `internal/auth/header.go` - HeaderStrategy with RoundTripper pattern
- `internal/auth/header_test.go` - Tests for header injection, env vars, cloning
- `internal/auth/basic.go` - BasicStrategy with SetBasicAuth
- `internal/auth/basic_test.go` - Tests for basic auth, env vars, cloning

## Decisions Made

- **Request cloning:** Both strategies clone the incoming request before adding auth headers. This is critical for retry safety - if a retry mechanism reuses the request, it won't have duplicate auth headers.
- **Env var expansion at creation time:** Credentials are expanded once when the strategy is created, not on every request. This matches the fail-fast philosophy and avoids per-request overhead.
- **Stdlib SetBasicAuth:** BasicStrategy uses the standard library's SetBasicAuth method rather than hand-rolling base64 encoding. Per RESEARCH.md guidance to avoid hand-rolling crypto.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- **Module path typo:** Initial header.go used `restitch/internal/config` instead of `github.com/restitch/restitch-gateway/internal/config`. Fixed immediately after first test run failed.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- HeaderStrategy (AUTH-01) and BasicStrategy (AUTH-02) complete
- RoundTripper pattern established for remaining strategies
- Ready for Plan 03-03 (Passthrough) and 03-04 (OAuth2)
- Note: Passthrough strategy files exist (committed as 03-03) but were not part of this plan

---
*Phase: 03-upstream-authentication*
*Completed: 2026-02-03*
