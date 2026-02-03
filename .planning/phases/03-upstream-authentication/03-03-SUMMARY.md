---
phase: 03-upstream-authentication
plan: 03
subsystem: auth
tags: [passthrough, authorization-header, security, roundtripper]

# Dependency graph
requires:
  - phase: 03-01
    provides: Strategy interface, auth config types, PassthroughConfig
provides:
  - PassthroughStrategy implementing auth.Strategy
  - ErrMissingAuthHeader sentinel error
  - IsMissingAuthHeaderError helper function
  - Security-first passthrough (reject unauthenticated requests)
affects: [composition-handler, 03-05-oauth2, 04-resilience]

# Tech tracking
tech-stack:
  added: []
  patterns: [passthrough-auth-pattern, fail-fast-auth-validation]

key-files:
  created:
    - internal/auth/passthrough.go
    - internal/auth/passthrough_test.go
  modified:
    - internal/auth/auth.go

key-decisions:
  - "Return ErrMissingAuthHeader immediately when no client auth (security best practice)"
  - "Clone request before forwarding to avoid modifying original"
  - "IsMissingAuthHeaderError helper for handler-layer error detection"

patterns-established:
  - "Passthrough auth: reject at RoundTripper, return 401 at handler layer"
  - "Sentinel errors for auth failures with Is() helper functions"

# Metrics
duration: 2min
completed: 2026-02-03
---

# Phase 3 Plan 03: Passthrough Authentication Summary

**Passthrough strategy forwards client Authorization header verbatim, rejecting unauthenticated requests immediately**

## Performance

- **Duration:** 2 min
- **Started:** 2026-02-03T08:58:36Z
- **Completed:** 2026-02-03T09:00:51Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- PassthroughStrategy forwards Bearer, Basic, and custom auth schemes verbatim
- ErrMissingAuthHeader returned when client omits Authorization header
- Security-first design: don't forward unauthenticated requests to upstream
- IsMissingAuthHeaderError helper for handler-layer 401 response logic
- Package documentation with example error handling code

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement passthrough authentication strategy** - `501f959` (feat)
2. **Task 2: Add passthrough error handling documentation** - `dead09c` (docs)

## Files Created/Modified
- `internal/auth/passthrough.go` - PassthroughStrategy implementation with ErrMissingAuthHeader
- `internal/auth/passthrough_test.go` - Tests for forwarding, error cases, clone verification
- `internal/auth/auth.go` - Package documentation with passthrough error handling section

## Decisions Made

1. **Return error immediately when no client auth**
   - Rationale: Per CONTEXT.md and RESEARCH.md security best practice - don't forward unauthenticated requests to upstreams that expect authentication
   - Impact: Handler layer must catch ErrMissingAuthHeader and return 401

2. **Clone request before forwarding**
   - Rationale: Per RESEARCH.md Pitfall 3 - avoid modifying original request (safe for retries)
   - Impact: Consistent with HeaderStrategy and BasicStrategy patterns

3. **IsMissingAuthHeaderError helper function**
   - Rationale: Handler layer needs clean error detection without importing errors package
   - Impact: Composition handler can use `auth.IsMissingAuthHeaderError(err)` for 401 logic

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - straightforward implementation following established patterns from HeaderStrategy.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Passthrough strategy complete (AUTH-03 requirement)
- Ready for Plan 03-04 (OAuth2 client credentials) or Plan 03-05 (strategy factory)
- Handler layer needs to implement ErrMissingAuthHeader -> 401 response

---
*Phase: 03-upstream-authentication*
*Completed: 2026-02-03*
