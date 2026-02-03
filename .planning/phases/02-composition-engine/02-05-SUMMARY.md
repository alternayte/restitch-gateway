---
phase: 02-composition-engine
plan: 05
subsystem: validation
tags: [dag, validation, circular-dependency, startup-validation, fail-fast]

# Dependency graph
requires:
  - phase: 02-composition-engine
    provides: "DAG execution engine with BuildDAG function"
provides:
  - "Startup-time validation for circular dependencies"
  - "Startup-time validation for missing step references"
  - "Pre-built execution plans stored in CompiledComposition"
  - "Gateway fails to start with invalid configs (fail-fast behavior)"
affects: [03-authentication, 04-advanced-features]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Validation happens at config parse time (CompileConfig), not request time"
    - "DAG execution plans pre-built and stored in CompiledComposition.ExecutionPlan"
    - "BuildDAG called once per composition during startup, not per request"

key-files:
  created: []
  modified:
    - internal/composition/parser.go
    - internal/composition/executor.go
    - internal/composition/parser_test.go

key-decisions:
  - "Move BuildDAG from request-time to parse-time for fail-fast validation"
  - "Store ExecutionPlan in CompiledComposition to avoid redundant DAG builds"
  - "Invalid configs prevent gateway startup with clear error messages"

patterns-established:
  - "Parse-time validation: All structural validation (cycles, missing refs) happens at startup"
  - "Pre-built plans: Execution plans built once during config compilation, reused for all requests"

# Metrics
duration: 2min
completed: 2026-02-03
---

# Phase 02 Plan 05: Validation Timing Fix Summary

**DAG validation moved to startup-time with pre-built execution plans, fixing circular dependency and missing step reference detection**

## Performance

- **Duration:** 2 min
- **Started:** 2026-02-03T07:51:40Z
- **Completed:** 2026-02-03T07:53:33Z
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments

- Circular dependencies now detected at config parse time (startup), not request time
- Missing step references now detected at config parse time (startup), not request time
- Gateway fails to start with clear error messages for invalid configs
- Execution plans pre-built during CompileConfig(), eliminating redundant DAG builds per request
- UAT test 9 and test 10 issues resolved

## Task Commits

Each task was committed atomically:

1. **Task 1: Move BuildDAG from request-time to parse-time** - `c6f311b` (feat)
   - Added ExecutionPlan field to CompiledComposition struct
   - Call BuildDAG during compileComposition() to validate at startup
   - Updated executor.Execute() to use pre-built plan instead of building on every request

2. **Task 2: Add tests verifying startup failure on invalid configs** - `3f18df0` (test)
   - TestCompileConfig_CircularDependency verifies cycle detection at parse time
   - TestCompileConfig_MissingStepReference verifies missing step detection at parse time

3. **Task 3: Verify fix with end-to-end test** - (verification only, no commit)
   - All composition tests pass
   - Circular dependency config fails at startup with clear error
   - Missing step reference config fails at startup with clear error
   - Valid configs continue to work normally

## Files Created/Modified

- `internal/composition/parser.go` - Added ExecutionPlan field to CompiledComposition, BuildDAG call during compileComposition()
- `internal/composition/executor.go` - Uses pre-built ExecutionPlan instead of calling BuildDAG per request
- `internal/composition/parser_test.go` - Added tests for circular dependency and missing step reference validation at startup

## Decisions Made

None - followed plan as specified. Plan correctly identified the timing bug and prescribed the exact fix.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - validation logic in dag.go already had correct error detection. Issue was purely timing: BuildDAG was called at request-time instead of parse-time. Moving the call to compileComposition() fixed both UAT issues immediately.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Phase 2 (Composition Engine) is now fully complete. All 11 requirements satisfied:

✅ 1. Gateway loads YAML config at startup
✅ 2. Invalid expression syntax fails at startup
✅ 3. Define composition with multiple steps
✅ 4. Independent steps execute in parallel
✅ 5. Dependent steps wait for dependencies
✅ 6. Use Expr syntax in step paths
✅ 7. Access previous step results in expressions
✅ 8. Response merges data from multiple steps
✅ 9. Circular dependency detected at startup (FIXED)
✅ 10. Missing step reference detected at startup (FIXED)
✅ 11. HTTP handler integration complete

Ready for Phase 3 (Authentication).

---
*Phase: 02-composition-engine*
*Completed: 2026-02-03*
