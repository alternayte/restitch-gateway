---
phase: 04-error-handling-resilience
plan: 02
subsystem: composition
tags: [error-handling, optional-steps, resilience, orchestration]

# Dependency graph
requires:
  - phase: 04-error-handling-resilience
    plan: 01
    provides: Error handling config schema (Optional, Timeout, ErrorRules)
provides:
  - Error collection types (StepErrorDetail, stepError)
  - Optional step orchestration without fail-fast
  - Dependency skip logic for failed steps
  - Partial response support (IsPartial, StepErrors fields)
affects: [04-03, 04-04]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "sync.WaitGroup for wave execution (replaced errgroup)"
    - "Error collection across waves with allErrors slice"
    - "Optional vs required step differentiation in error handling"
    - "Dependency skip cascade with 'dependency_failed' marker"

key-files:
  created:
    - internal/composition/errors.go
  modified:
    - internal/composition/executor.go

key-decisions:
  - "allErrors slice declared at start of Execute, aggregated after each wave"
  - "Optional step failures collected, composition continues"
  - "Required step failures fail-fast with detailed error message"
  - "Failed step dependencies cascade as 'dependency_failed' skips"
  - "Failed steps store nil result (available to dependents)"
  - "Error sanitization: timeout -> 'timeout', others -> 'upstream error'"

patterns-established:
  - "executeStepWithErrorHandling returns *stepError (nil on success)"
  - "checkDependenciesFailed helper for dependency validation"
  - "HasRequiredFailure for fail-fast decision"
  - "BuildErrorsArray for converting internal errors to user-facing array"
  - "SanitizeErrorMessage for hiding internal error details"

# Metrics
duration: 3min
completed: 2026-02-03
---

# Phase 04 Plan 02: Optional Step Orchestration Summary

**Implemented error collection and modified executor to handle optional steps without fail-fast cancellation**

## Performance

- **Duration:** 3 minutes (164 seconds)
- **Started:** 2026-02-03T10:13:05Z
- **Completed:** 2026-02-03T10:15:49Z
- **Tasks:** 3
- **Files created:** 1
- **Files modified:** 1

## Accomplishments
- Error collection types created (StepErrorDetail, stepError)
- Executor rewritten to use sync.WaitGroup instead of errgroup
- Optional step failures collected without failing composition
- Required step failures still fail-fast with detailed error
- Dependency skip logic prevents dependent steps from executing when dependencies fail
- Failed steps have nil results available to dependents
- Error sanitization prevents internal details from leaking to users

## Task Commits

Each task was committed atomically:

1. **Task 1: Create error types and helpers** - `580001e` (feat)
2. **Task 2: Modify CompositionResult to include errors** - `3bb129c` (feat)
3. **Task 3: Rewrite executor for optional step support** - `606eb8e` (feat)

## Files Created/Modified
- `internal/composition/errors.go` (NEW) - StepErrorDetail, stepError, SanitizeErrorMessage, BuildErrorsArray, HasRequiredFailure
- `internal/composition/executor.go` - Replaced errgroup with sync.WaitGroup, added allErrors tracking, new executeStepWithErrorHandling method, checkDependenciesFailed helper

## Decisions Made

**1. allErrors slice declared at start of Execute method**
- Rationale: Collect errors across ALL waves for complete partial response
- Implementation: `var allErrors []stepError` right after results map declaration
- Impact: Single source of truth for all failures throughout composition execution

**2. waveErrors aggregated into allErrors after each wave**
- Rationale: Per plan requirement "allErrors aggregates errors across all waves"
- Implementation: `allErrors = append(allErrors, waveErrors...)` after each wave
- Impact: Both required and optional failures tracked for complete error reporting

**3. Optional step failures collected, composition continues**
- Rationale: Per CONTEXT.md "Optional step failures do not cancel the composition"
- Implementation: Check `step.Optional` in executeStepWithErrorHandling, add to waveErrors but don't fail
- Impact: Compositions can return partial data when non-critical steps fail

**4. Required step failures fail-fast with detailed error**
- Rationale: Per CONTEXT.md "Required step failures cancel remaining steps"
- Implementation: HasRequiredFailure checks waveErrors, return first required error with context
- Impact: Clear error messages for debugging ("required step 'foo' failed: context canceled")

**5. Failed step dependencies cascade as 'dependency_failed' skips**
- Rationale: Per CONTEXT.md "Dependent steps skip when their dependency failed"
- Implementation: checkDependenciesFailed checks for nil results, mark as skipped with "dependency_failed"
- Impact: Predictable behavior - entire dependency chain skips on root failure

**6. Failed steps store nil result**
- Rationale: Per CONTEXT.md "Failed optional steps have nil result available to dependents"
- Implementation: `results[stepName] = nil` for both failures and skipped steps
- Impact: Expressions can check `steps.X == nil` for handling missing data

**7. Error sanitization hides internal details**
- Rationale: Per RESEARCH.md Pitfall 5 "Don't expose internal stack traces"
- Implementation: SanitizeErrorMessage converts errors to "timeout" or "upstream error"
- Impact: User-facing errors are clean, internal details logged separately

**8. Removed errgroup import, use sync.WaitGroup**
- Rationale: Per RESEARCH.md Pattern 2 "Replace errgroup.WithContext fail-fast"
- Implementation: Manual WaitGroup management with error collection
- Impact: Full control over error handling, optional steps don't cancel composition

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

**Test compatibility issue (resolved):**
- **Issue:** Existing test expected "context canceled" in error message
- **Cause:** New implementation wrapped error differently
- **Fix:** Improved error message to include step name and preserve underlying error with %w
- **Resolution:** Changed from generic "required step(s) failed" to "required step %q failed: %w" for better debugging

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

**Ready for 04-03:** Error matching rules

This plan established the core error orchestration. Next plan will:
- Implement error matching rules (status code list matching)
- Replace failed step body with configured value
- Add matched errors to _errors array for transparency

**Foundation solid:**
- Error collection working across all waves
- Optional vs required differentiation working
- Dependency skip cascade working
- nil results stored for failed steps
- Error sanitization hiding internal details

**No blockers identified.**

## Technical Notes

**Key architectural change:**
This plan fundamentally changed the executor from fail-fast (errgroup) to error-collecting (sync.WaitGroup). This is the core enabler for Phase 4's partial response feature.

**allErrors lifecycle:**
1. Declared at start of Execute method (empty slice)
2. Populated per wave in waveErrors
3. Aggregated after each wave: `allErrors = append(allErrors, waveErrors...)`
4. Checked for required failures (fail-fast if any)
5. Converted to StepErrorDetail array via BuildErrorsArray
6. Returned in CompositionResult (StepErrors field)

**Dependency skip cascade:**
When step A fails:
1. Result stored as nil: `results["A"] = nil`
2. Step B (depends on A) checks dependencies
3. checkDependenciesFailed returns true (A is nil)
4. Step B marked as skipped, stored as nil
5. Step C (depends on B) repeats cascade
6. All marked with "dependency_failed" in _errors array

---
*Phase: 04-error-handling-resilience*
*Completed: 2026-02-03*
