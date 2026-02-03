---
phase: 04-error-handling-resilience
plan: 01
subsystem: composition
tags: [timeout, context, error-handling, resilience]

# Dependency graph
requires:
  - phase: 03-upstream-authentication
    provides: CompiledUpstream with auth strategies
provides:
  - Error handling configuration schema (Optional, Timeout, ErrorRules)
  - Per-step timeout execution with context.WithTimeout
  - Timeout hierarchy resolution (step > upstream > 30s default)
affects: [04-02, 04-03, 04-04]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Timeout hierarchy: step-level override > upstream default > global default"
    - "Context timeout with defer cancel() for resource cleanup"
    - "Timeout error detection via errors.Is(context.DeadlineExceeded)"

key-files:
  created: []
  modified:
    - internal/composition/config.go
    - internal/composition/parser.go
    - internal/composition/step.go

key-decisions:
  - "Optional field defaults to false (steps required by default)"
  - "Timeout hierarchy: step > upstream > 30s default"
  - "Timeout errors wrapped with duration for debugging"
  - "Step timeout resolved at execution time using hierarchy"

patterns-established:
  - "resolveTimeout() hierarchy pattern for config precedence"
  - "ExecuteStepWithTimeout with explicit timeout parameter"
  - "Context derivation from parent with defer cancel()"

# Metrics
duration: 3min
completed: 2026-02-03
---

# Phase 04 Plan 01: Error Handling Foundation Summary

**Extended config schema for error handling (optional steps, timeouts, error rules) plus timeout-aware step execution with context.WithTimeout**

## Performance

- **Duration:** 3 minutes
- **Started:** 2026-02-03T10:06:37Z
- **Completed:** 2026-02-03T10:09:23Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments
- Config schema supports optional steps, per-step timeouts, and error matching rules
- Timeout hierarchy implemented (step > upstream > default 30s)
- Step execution respects timeout via context.WithTimeout with proper cancellation
- Timeout errors distinguishable from other errors via context.DeadlineExceeded

## Task Commits

Each task was committed atomically:

1. **Task 1: Add error handling config types** - `94edaed` (feat)
2. **Task 2: Add compiled config fields and timeout resolution** - `f0258e5` (feat)
3. **Task 3: Implement step timeout execution** - `3f529ee` (feat)

## Files Created/Modified
- `internal/composition/config.go` - Added Optional, Timeout, ErrorRules to Step; Timeout to Upstream; ErrorRule type
- `internal/composition/parser.go` - Added Optional, Timeout, ErrorRules to CompiledStep; Timeout to CompiledUpstream; DefaultStepTimeout constant
- `internal/composition/step.go` - Added ExecuteStepWithTimeout with context.WithTimeout; resolveTimeout hierarchy; timeout error detection

## Decisions Made

**1. Steps required by default (Optional defaults to false)**
- Rationale: Per CONTEXT.md "Steps required by default, must be explicitly marked optional"
- Impact: Users must opt-in to optional semantics with `optional: true`

**2. Timeout hierarchy: step > upstream > 30s default**
- Rationale: Per CONTEXT.md "Timeouts configured at upstream level with step-level override"
- Implementation: resolveTimeout() checks step.Timeout, then upstream.Timeout, then DefaultStepTimeout
- Impact: Fine-grained control per step, sensible defaults

**3. Step timeout as pointer (*time.Duration)**
- Rationale: Distinguish between "not set" (nil) and "set to 0" for hierarchy resolution
- Impact: nil means "use upstream default", allowing proper fallback

**4. Timeout errors wrapped with duration**
- Rationale: Debugging - error message includes timeout duration for troubleshooting
- Example: `"timeout after 5s: context deadline exceeded"`
- Impact: Clearer error messages for operators

**5. Defer cancel() immediately after context creation**
- Rationale: Per RESEARCH.md Pitfall 3 - prevent resource leaks
- Impact: Resources always released even if function panics

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - implementation straightforward using Go stdlib context package.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

**Ready for 04-02:** Optional step orchestration

This plan established the configuration schema and timeout foundation. Next plan will modify DAG executor to:
- Handle optional step failures without failing composition
- Collect errors instead of fail-fast
- Skip dependent steps when dependencies fail

**Foundation solid:**
- Config types support all Phase 4 requirements (optional, timeout, error_rules)
- Timeout execution working with proper context management
- Error detection pattern established (errors.Is for DeadlineExceeded)

**No blockers identified.**

---
*Phase: 04-error-handling-resilience*
*Completed: 2026-02-03*
