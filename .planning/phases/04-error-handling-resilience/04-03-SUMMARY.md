---
phase: 04-error-handling-resilience
plan: 03
subsystem: api
tags: [partial-response, error-handling, http-headers]

# Dependency graph
requires:
  - phase: 04-01
    provides: Error types (StepErrorDetail) and timeout execution
  - phase: 04-02
    provides: Optional step orchestration with error collection
provides:
  - Partial response building with _errors array injection
  - X-Partial-Response header signaling
  - Nil step result handling in expressions
affects: [04-04, 04-05, testing, client-integration]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Partial response pattern: HTTP 200 + X-Partial-Response header + _errors array"
    - "Nil value propagation in expression environment for failed optional steps"

key-files:
  created: []
  modified:
    - internal/composition/response.go
    - internal/composition/handler.go
    - internal/composition/step.go

key-decisions:
  - "BuildResponse accepts stepErrors parameter for _errors injection"
  - "X-Partial-Response header set only when IsPartial is true"
  - "Failed optional steps exposed as null in steps.X expressions"
  - "HTTP status remains 200 for partial responses (composition succeeded)"

patterns-established:
  - "Response builder signature: BuildResponse(template, results, request, stepErrors)"
  - "Header setting before WriteHeader: Content-Type, X-Partial-Response"
  - "Logging includes partial status and error count"

# Metrics
duration: 3min
completed: 2026-02-03
---

# Phase 4 Plan 3: Partial Response Building Summary

**HTTP 200 partial responses with X-Partial-Response header and _errors array for failed optional steps**

## Performance

- **Duration:** 3 min
- **Started:** 2026-02-03T11:18:51Z
- **Completed:** 2026-02-03T11:22:15Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- BuildResponse injects _errors array into response body when step failures occur
- X-Partial-Response header signals partial data to clients
- Failed optional steps produce null values in expression evaluation
- HTTP 200 status maintained for partial responses (composition succeeded, partial data is valid)

## Task Commits

Each task was committed atomically:

1. **Task 1: Modify BuildResponse to accept errors and inject _errors** - `77c336f` (feat)
2. **Task 1b: Handle nil step results in buildRequestEnv** - `e49f0c1` (feat)
3. **Task 2: Update handler to pass errors and set X-Partial-Response header** - `a2e7c6c` (feat)

## Files Created/Modified
- `internal/composition/response.go` - BuildResponse accepts stepErrors, injects _errors into body
- `internal/composition/handler.go` - Passes StepErrors to BuildResponse, sets X-Partial-Response header
- `internal/composition/step.go` - buildRequestEnv handles nil step results as null values

## Decisions Made

**BuildResponse signature change**
- Added stepErrors parameter to BuildResponse for error injection
- Maintains backward compatibility (empty stepErrors array = no _errors field)
- Rationale: Clean separation between response building and error tracking

**Nil step result handling**
- Failed optional steps stored as nil in results map
- buildRequestEnv exposes nil as null in expression environment
- Allows expressions like `steps.inventory ?? defaultValue` to work correctly
- Rationale: Per CONTEXT.md requirement for failed optional steps returning null

**X-Partial-Response header**
- Set only when result.IsPartial is true
- Not set for full success (no failed steps)
- Header presence is sufficient signal (value is "true")
- Rationale: Clear HTTP-level signaling separate from body content

**HTTP 200 for partial responses**
- Status code remains 200 even when steps fail
- Per CONTEXT.md: "Composition succeeded, partial data is valid"
- Rationale: Gateway successfully composed available data; step failures are reported via _errors array

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - implementation straightforward with existing error collection from 04-02.

## Next Phase Readiness

**Ready for 04-04 (Error matching rules):**
- Partial response infrastructure complete
- _errors array ready to receive error-matched failures
- Error types (StepErrorDetail) established in 04-01

**Ready for 04-05 (Circuit breaker):**
- Error signaling in place (X-Partial-Response header)
- Error collection working across waves
- Logging includes partial status for observability

**Ready for testing:**
- Full success responses: HTTP 200, no X-Partial-Response header, no _errors
- Partial responses: HTTP 200, X-Partial-Response: true, _errors array with step/message
- Failed step expressions return null in response template evaluation

**No blockers identified.**

---
*Phase: 04-error-handling-resilience*
*Completed: 2026-02-03*
