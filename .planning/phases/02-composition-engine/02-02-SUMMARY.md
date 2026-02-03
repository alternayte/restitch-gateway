# Phase 02 Plan 02: DAG Construction Summary

**One-liner:** Automatic dependency inference from expression AST with Kahn's algorithm for parallel execution waves

---
phase: 02-composition-engine
plan: 02
type: execution
status: complete
subsystem: composition-dag
tags: [dag, topological-sort, dependency-analysis, parallel-execution, expr-ast]

dependencies:
  requires: ["02-01"]
  provides: ["dag-execution-plan", "cycle-detection", "dependency-inference"]
  affects: ["02-03"]

tech-stack:
  added: []
  patterns: ["kahn-topological-sort", "ast-visitor", "wave-based-parallelism"]

key-files:
  created:
    - "internal/composition/deps.go"
    - "internal/composition/dag.go"
  modified: []

decisions: []

metrics:
  tasks: 2
  commits: 2
  tests: 27
  duration: "3 min"
  completed: "2026-02-03"
---

## What Was Built

Created DAG construction system that automatically infers dependencies from expression usage and produces parallel execution plans.

### Core Capabilities

**Dependency Inference (deps.go):**
- AST visitor pattern to extract step references from expressions
- Supports both dot notation (`steps.user`) and bracket notation (`steps["user-service"]`)
- Handles nested access, array indexing, and complex expressions
- `ExtractDependencies`: Parse single expression for step references
- `ExtractAllDependencies`: Deduplicate across multiple expressions
- `MergeDependencies`: Combine explicit `depends_on` with inferred dependencies

**DAG Execution Planning (dag.go):**
- `ExecutionPlan`: Wave-grouped steps for parallel execution
- `BuildDAG`: Analyzes composition and produces execution plan
- Kahn's algorithm for topological sorting with level detection
- Circular dependency detection at config parse time
- Validates all step references exist before execution
- Dependencies extracted from path, body, and header expressions

### Execution Flow

```
CompiledComposition
  ↓
analyzeDependencies (extract from expressions)
  ↓
validateDependencies (check references exist)
  ↓
buildExecutionPlan (Kahn's algorithm)
  ↓
ExecutionPlan { Waves: [][]string }
```

**Wave Semantics:**
- Wave 0: Steps with no dependencies (parallel)
- Wave N+1: Steps depending only on waves 0..N (parallel within wave)
- Waves execute sequentially; steps within wave execute in parallel

**Example:**
```yaml
steps:
  - name: user
    path: /users/{{ req.query.id }}
  - name: orders
    path: /orders/{{ steps.user.body.id }}
  - name: profile
    path: /profile/{{ steps.user.body.id }}
```

Produces:
```
Wave 0: [user]
Wave 1: [orders, profile]  # Both depend only on user
```

## Tasks Completed

### Task 1: Create Dependency Extractor Using Expr AST Visitor
**Commit:** `d6a4f75`
**Files:** `internal/composition/deps.go`, `internal/composition/deps_test.go`

Implemented AST visitor to parse expressions and extract step dependencies:
- `dependencyVisitor` walks expr AST looking for `steps.X` patterns
- Handles both `ast.IdentifierNode` (dot notation) and `ast.StringNode` (bracket notation)
- Deduplicates dependencies across expressions
- Returns error on invalid expression syntax

**Tests (13 cases):**
- Simple/multiple/nested references
- Bracket and mixed notation
- Array access and complex expressions
- Duplicate references (deduplication)
- Invalid syntax error handling

### Task 2: Create DAG Builder With Cycle Detection
**Commit:** `ad2c0fd`
**Files:** `internal/composition/dag.go`, `internal/composition/dag_test.go`

Implemented DAG builder using Kahn's algorithm:
- `analyzeDependencies`: Extract from path/body/header expressions
- `validateDependencies`: Ensure all references exist
- `buildExecutionPlan`: Topological sort with level detection
- Cycle detection identifies all steps in circular dependency

**Tests (14 cases):**
- No dependencies (parallel execution)
- Linear chain (sequential waves)
- Diamond pattern (parallel converge)
- Explicit vs inferred dependencies
- Direct and indirect cycle detection
- Missing step reference validation
- Complex multi-wave DAG

## Integration Points

**Consumes:**
- `CompiledComposition` from 02-01 (parser.go)
- `CompiledStep` with expressions
- `expr` library for AST parsing

**Provides:**
- `ExecutionPlan` with wave-grouped steps
- `ExtractDependencies` for expression analysis
- `BuildDAG` for execution order determination

**Next Phase (02-03):**
- Response merging will use ExecutionPlan to execute steps
- Step results passed between waves via environment
- Fail-fast execution cancels remaining waves on error

## Testing Coverage

**Unit Tests:**
- 27 total test cases across 7 test functions
- Dependency extraction: 13 cases
- DAG building: 14 cases
- All edge cases covered (cycles, missing refs, empty deps)

**Integration with 02-01:**
- Uses `CompiledExpr.Raw` for expression content
- Uses `ExtractExpressions` from expr.go
- Compatible with `BuildBaseEnvironment` structure

**Test Scenarios:**
1. Independent steps → Wave 0 parallel
2. Linear dependencies → Sequential waves
3. Diamond pattern → Parallel then merge
4. Circular dependencies → Error at parse time
5. Missing references → Error at validation
6. Complex DAG → Multi-wave grouping

All tests pass: `go test ./internal/composition/... -v`

## Architectural Decisions

**Why Kahn's Algorithm:**
- Standard algorithm for topological sort (well-understood)
- Level detection built-in (waves = levels)
- Cycle detection guaranteed (remaining nodes = cycle)
- O(V + E) complexity - optimal for DAG

**Why AST Visitor:**
- Correct parsing of expression syntax
- Handles all expr language features (operators, functions, etc.)
- No regex fragility (handles nested structures)
- Returns same results as expr runtime evaluation

**Why Inferred Dependencies:**
- User doesn't need to specify obvious dependencies
- Reduces configuration verbosity
- Matches user mental model (if I reference it, I depend on it)
- Explicit `depends_on` available for edge cases

**Why Validate at Parse Time:**
- Fail fast on configuration errors
- No runtime surprises during request handling
- Clear error messages with step names
- Prevents serving traffic with invalid config

## Next Phase Readiness

**For 02-03 (Response Merging):**
- ✅ ExecutionPlan ready for wave-by-wave execution
- ✅ Step dependencies tracked for result passing
- ✅ Validation ensures safe execution (no cycles, no missing refs)

**Future Considerations:**
- Optional steps (Phase 4): May need conditional DAG building
- Dynamic step names (Phase 4): Would require runtime cycle detection
- Step retries (Phase 4): DAG structure unchanged, retry within wave

## Performance Notes

**Dependency Analysis:**
- AST parsing per expression (one-time at config load)
- Visitor pattern: O(nodes in AST)
- Typically < 100 nodes per expression

**DAG Building:**
- Kahn's algorithm: O(V + E) where V = steps, E = dependencies
- Typical composition: 3-10 steps → microseconds
- No runtime overhead (plan built at parse time)

**Memory:**
- ExecutionPlan stored per composition (small footprint)
- No additional memory during request execution

## Deviations from Plan

None - plan executed exactly as written.

All tasks completed successfully with comprehensive test coverage.

---

**Status:** Complete
**Duration:** 3 minutes
**Next Plan:** 02-03 (Response merging with expression evaluation)
