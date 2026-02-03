# Phase 02 Plan 03: Step Execution and DAG Executor Summary

**HTTP step execution with parallel DAG execution using errgroup, header propagation, and upstream error passthrough**

---
phase: 02-composition-engine
plan: 03
type: execution
status: complete
subsystem: composition-executor
tags: [http-client, parallel-execution, errgroup, dag, uuid, header-propagation]

dependencies:
  requires:
    - phase: "02-01"
      provides: "CompiledConfig with expression compilation"
    - phase: "02-02"
      provides: "ExecutionPlan with wave-grouped steps"
    - phase: "01-03"
      provides: "HTTP client with connection pooling"
  provides:
    - "ExecuteStep function for upstream HTTP requests"
    - "Executor for parallel DAG execution"
    - "Header propagation with X-Request-ID generation"
    - "Upstream error passthrough (non-2xx as results, not failures)"
  affects: ["02-04"]

tech-stack:
  added:
    - "github.com/google/uuid v1.6.0"
    - "golang.org/x/sync/errgroup"
  patterns:
    - "errgroup.WithContext for fail-fast parallel execution"
    - "Template interpolation for {{ expr }} patterns"
    - "Header propagation with case-insensitive lookup"
    - "Upstream error passthrough (non-2xx status as StepResult)"

key-files:
  created:
    - "internal/composition/step.go"
    - "internal/composition/step_test.go"
    - "internal/composition/executor.go"
    - "internal/composition/executor_test.go"
  modified:
    - "go.mod"
    - "go.sum"

decisions:
  - "Upstream HTTP errors (500, 404) are passthrough - not step failures per CONTEXT.md"
  - "Network/context errors trigger fail-fast via errgroup cancellation"
  - "Header propagation uses case-insensitive lookup (HTTP headers canonicalized)"
  - "Template interpolation checked BEFORE Program check (templates have Program==nil)"
  - "JSON numbers unmarshaled as float64 (standard Go behavior)"

metrics:
  tasks: 2
  commits: 2
  tests: 15
  duration: "9 min"
  completed: "2026-02-03"
---

## What Was Built

Created step execution and DAG executor with parallel execution, header propagation, and proper error handling.

### Core Capabilities

**Step Execution (step.go):**
- `ExecuteStep`: Makes HTTP requests to upstreams with evaluated expressions
- `buildRequestEnv`: Constructs environment with req data + step results
- `propagateHeaders`: Auto-propagates X-Request-ID, X-Correlation-ID, traceparent, Accept, Accept-Language
- `parseJSONBody`: Parses JSON responses (with Content-Type check)
- `interpolateTemplate`: Replaces {{ expr }} patterns with evaluated values
- Expression evaluation for paths, bodies, and headers
- UUID generation for X-Request-ID if not present
- DrainAndClose pattern for connection reuse

**DAG Executor (executor.go):**
- `Executor`: Runs compositions according to DAG execution plan
- `Execute`: Executes waves sequentially, steps within waves in parallel
- `NewExecutor`: Creates executor with compiled config and HTTP client
- errgroup.WithContext for fail-fast on network/context errors
- resultsMutex for concurrent access to results map
- Logging: step start/complete events with duration

### Execution Flow

```
Execute(ctx, compositionName, req)
  ↓
BuildDAG(comp) → ExecutionPlan
  ↓
For each wave in plan.Waves:
  errgroup.WithContext(ctx)
    ↓
  For each step in wave:
    g.Go(func() {
      executeStep(waveCtx, stepName, ...)
        ↓
      ExecuteStep(ctx, step, upstream, env, httpClient)
        ↓
      Store result in results[stepName]
    })
    ↓
  g.Wait() // Fail-fast on first error
  ↓
Return CompositionResult{Steps: results}
```

**Wave Semantics:**
- Waves execute sequentially (wave N+1 starts only after wave N completes)
- Steps within same wave execute in parallel via errgroup
- First network/context error cancels all goroutines in that wave
- Upstream HTTP errors (500, 404) are results, not failures

## Tasks Completed

### Task 1: Create Step Execution with HTTP Client Integration
**Commit:** `f3962cf`
**Files:** `internal/composition/step.go`, `internal/composition/step_test.go`, `go.mod`, `go.sum`

Implemented HTTP step execution with expression evaluation and header propagation:

**Functions:**
- `ExecuteStep`: Main function that makes HTTP request to upstream
- `buildRequestEnv`: Constructs environment with req data and step results
- `propagateHeaders`: Auto-propagates tracing headers with UUID generation
- `parseJSONBody`: Parses JSON response body
- `evaluatePath`: Evaluates path expressions with template interpolation
- `evaluateBody`: Evaluates body expressions for POST/PUT requests
- `evaluateHeader`: Evaluates header expressions
- `interpolateTemplate`: Replaces {{ expr }} patterns with evaluated values
- `convertHeaders`: Converts http.Header to map[string]string

**Key Implementation Details:**
- http.NewRequestWithContext for cancellation support (RESEARCH.md Pitfall 7)
- DrainAndClose pattern for connection reuse (RESEARCH.md Pitfall 1)
- Case-insensitive header lookup (HTTP headers canonicalized)
- Template interpolation checked FIRST before Program check
- JSON numbers as float64 (standard Go unmarshaling)

**Dependencies Added:**
- github.com/google/uuid v1.6.0 for X-Request-ID generation

**Tests (14 cases across 4 functions):**
- TestExecuteStep: 6 test cases
  - Simple GET request
  - Path with expression evaluation
  - Header propagation
  - X-Request-ID generation when missing
  - Context cancellation
  - Upstream error passthrough (non-2xx)
- TestBuildRequestEnv: 2 test cases
  - Parses request data (path, query, headers)
  - Includes step results
- TestParseJSONBody: 4 test cases
  - Parses JSON object
  - Parses JSON array
  - Handles non-JSON content type
  - Handles invalid JSON
- TestInterpolateTemplate: 3 test cases (including fix)
  - Simple interpolation
  - Multiple interpolations
  - Interpolate with step results

### Task 2: Create DAG Executor with Parallel Execution
**Commit:** `71d7efb`
**Files:** `internal/composition/executor.go`, `internal/composition/executor_test.go`, `internal/composition/step.go`, `internal/composition/step_test.go`, `go.mod`, `go.sum`

Implemented DAG executor using errgroup for parallel execution:

**Structures:**
- `CompositionResult`: Holds results from all steps
- `Executor`: Runs compositions according to DAG execution plan

**Functions:**
- `NewExecutor`: Creates executor with compiled config and HTTP client
- `Execute`: Executes composition by running waves sequentially
- `executeStep`: Executes single step with proper locking
- `countSteps`: Utility to count total steps across waves

**Key Implementation Details:**
- errgroup.WithContext creates derived context for wave execution
- resultsMutex protects concurrent access to results map
- Wave-by-wave execution: each wave completes before next starts
- Within wave: steps execute in parallel via errgroup.Go
- First error in wave cancels all goroutines (fail-fast)
- buildRequestEnv called with mutex lock to get consistent snapshot

**Dependencies Added:**
- golang.org/x/sync/errgroup for parallel execution

**Tests (6 test cases):**
- Parallel execution of independent steps (timing verification)
- Dependent steps wait for dependencies (execution order verification)
- Upstream 500 is not step failure - composition completes
- Step results accessible to dependent steps (diamond pattern)
- Context cancellation stops execution
- Composition not found error

**Step.go Improvements:**
- Fixed evaluatePath to check template syntax FIRST (before Program check)
- Fixed interpolateTemplate to handle step results correctly
- Added test for interpolating step results in paths

## Integration Points

**Consumes:**
- `CompiledConfig` from 02-01 (parser.go)
- `CompiledComposition` with compiled expressions
- `ExecutionPlan` from 02-02 (dag.go)
- `*http.Client` from Phase 1 (internal/client/client.go)

**Provides:**
- `ExecuteStep` function for upstream HTTP requests
- `Executor` for parallel DAG execution
- `CompositionResult` with all step results
- `StepResult` with status, headers, and parsed body

**Next Phase (02-04):**
- Response merging will use CompositionResult
- HTTP handler will create Executor and call Execute
- Integration with server/router from Phase 1

## Testing Coverage

**Unit Tests:**
- 14 test cases for step execution
- 6 test cases for executor
- 20 total test cases across 10 test functions

**Integration with Previous Phases:**
- Uses BuildDAG from 02-02
- Uses CompiledConfig from 02-01
- Uses Phase 1's HTTP client patterns

**Test Scenarios:**
1. Simple GET requests to upstreams
2. Path expression evaluation with request data
3. Header propagation (X-Request-ID, traceparent, etc.)
4. X-Request-ID generation when missing
5. Context cancellation propagation
6. Upstream error passthrough (500, 404)
7. Parallel execution verification (timing)
8. Dependent step execution order
9. Step results accessible to dependent steps
10. Diamond pattern (two parallel, one dependent)

All tests pass: `go test ./internal/composition/... -v`

## Architectural Decisions

**Why errgroup.WithContext:**
- Standard Go pattern for parallel execution with fail-fast
- Automatic context cancellation on first error
- Clean error propagation from goroutines
- Well-tested stdlib extension

**Why Mutex Instead of sync.Map:**
- Simple read-modify pattern (add results sequentially)
- No high contention (writes happen between waves, not during)
- Better type safety (no interface{} assertions)
- Clearer code (explicit locking visible)

**Why Upstream Errors Are Not Step Failures:**
- Per CONTEXT.md: "Upstream error passthrough"
- Phase 2 focus: successful request execution, not response validation
- Allows composition to complete and return all step results
- Phase 4 will handle optional steps and partial responses
- Gateway's job is to execute DAG correctly, not validate upstream behavior

**Why Template Interpolation Runtime:**
- Template expressions reference step results (runtime data)
- Compile-time validation done in parser.go (syntax check)
- Runtime interpolation evaluates with actual step results
- Efficient: only re-compiles expressions for validation

## Performance Notes

**Step Execution:**
- Single HTTP request per step (no retries in Phase 2)
- Connection pooling from Phase 1 (MaxIdleConnsPerHost: 100)
- DrainAndClose ensures connection reuse
- Template interpolation: O(expressions) regex + evaluation

**DAG Execution:**
- Wave-based parallelism: O(waves) sequential, O(max_wave_size) parallel
- Typical composition: 2-3 waves, 2-4 steps per wave
- errgroup overhead: minimal (goroutine creation + channel coordination)
- Mutex contention: low (lock held briefly for map access)

**Memory:**
- Results stored in map (one entry per step)
- No intermediate buffers (streaming not needed for Phase 2)
- Step results contain parsed JSON (unmarshaled once)

## Deviations from Plan

**Auto-fixed Issues:**

**1. [Rule 3 - Blocking] Fixed header propagation case-sensitivity**
- **Found during:** Task 1 testing (TestExecuteStep/header_propagation)
- **Issue:** HTTP headers are case-insensitive, but Go map keys are case-sensitive. Test was failing because headers were stored with canonical casing (e.g., "X-Request-Id" instead of "X-Request-ID")
- **Fix:** Added case-insensitive lookup in propagateHeaders using strings.EqualFold
- **Files modified:** internal/composition/step.go
- **Verification:** TestExecuteStep/header_propagation passes
- **Committed in:** f3962cf (Task 1 commit)

**2. [Rule 1 - Bug] Fixed template interpolation order in evaluatePath**
- **Found during:** Task 2 testing (TestExecutor/dependent_steps_wait_for_dependencies)
- **Issue:** evaluatePath was checking `if pathExpr.Program == nil` before checking for template syntax. Templates have Program==nil, so they were being returned raw without interpolation.
- **Fix:** Moved template syntax check BEFORE Program check in evaluatePath
- **Files modified:** internal/composition/step.go
- **Verification:** TestExecutor tests pass, template interpolation working
- **Committed in:** 71d7efb (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking)
**Impact on plan:** Both auto-fixes necessary for correctness. No scope creep.

## Issues Encountered

**Issue 1: Upstream errors treated as failures initially**
- **Problem:** Test "fail-fast on first error" was expecting upstream 500 to fail the composition
- **Root cause:** Misunderstanding of CONTEXT.md "upstream error passthrough"
- **Resolution:** Clarified that upstream HTTP errors (500, 404) are results, not failures. Only network/context errors trigger fail-fast. Updated test to reflect correct behavior.

**Issue 2: Template interpolation not working**
- **Problem:** Tests showed URL still containing {{ expr }} instead of interpolated values
- **Root cause:** evaluatePath logic was checking Program==nil before template syntax
- **Resolution:** Reordered checks to detect template syntax first (Rule 1 - Bug fix)
- **Debugging:** Added temporary debug logging to trace evaluatePath execution

All issues resolved successfully with no remaining blockers.

## Next Phase Readiness

**For 02-04 (Response Merging):**
- ✅ Executor ready to be integrated with HTTP handler
- ✅ Step results include all data needed for response merging (status, headers, body)
- ✅ CompositionResult provides clean interface for response builder
- ✅ Error handling patterns established (network vs upstream errors)
- ✅ Header propagation working correctly

**Future Considerations:**
- Response merging will need to handle upstream errors in final response
- HTTP handler will need to map composition routes to executor
- Main.go integration will wire up executor with server
- Per-step timeout configuration deferred to Phase 4
- Optional steps and partial responses deferred to Phase 4

---

**Status:** Complete
**Duration:** 9 minutes
**Next Plan:** 02-04 (Response merging with HTTP handler integration)
