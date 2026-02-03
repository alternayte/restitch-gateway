---
status: diagnosed
phase: 02-composition-engine
source: [02-01-SUMMARY.md, 02-02-SUMMARY.md, 02-03-SUMMARY.md, 02-04-SUMMARY.md]
started: 2026-02-03T03:30:00Z
updated: 2026-02-03T03:30:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Gateway loads YAML config at startup
expected: Start gateway with --config flag pointing to valid YAML config. Gateway starts and logs composition count.
result: pass

### 2. Invalid expression syntax fails at startup
expected: Create config with invalid expression syntax (e.g., {{ @invalid }}). Gateway fails to start with clear error message identifying the problematic expression.
result: pass

### 3. Define composition with multiple steps
expected: Create composition with 2+ steps in YAML. Make HTTP request to gateway. Gateway executes all steps and returns composed response.
result: pass

### 4. Independent steps execute in parallel
expected: Create composition with 2 independent steps (no dependencies). Observe via logs or timing that both steps start simultaneously.
result: pass

### 5. Dependent steps wait for dependencies
expected: Create composition where step B references steps.A.body.id. Observe step B only executes after step A completes.
result: pass

### 6. Use Expr syntax in step paths
expected: Use {{ req.query.id }} in step path. Pass ?id=123 query param. Observe upstream receives /users/123 (not raw expression).
result: pass

### 7. Access previous step results in expressions
expected: First step returns { "id": 5 }. Second step uses {{ steps.first.body.id }}. Observe second step receives evaluated value 5.
result: pass

### 8. Response merges data from multiple steps
expected: Composition with 2 steps and response template combining both. Receive JSON response with merged data from both steps.
result: pass

### 9. Circular dependency detected at startup
expected: Create config where step A depends on B, step B depends on A. Gateway fails to start with clear cycle detection error.
result: issue
reported: "it doesnt fail to start when fails when a request is received"
severity: major

### 10. Missing step reference detected at startup
expected: Create config referencing steps.nonexistent in an expression. Gateway fails to start with clear "step not found" error.
result: issue
reported: "doesnt fail at startup but fails when a request is received"
severity: major

## Summary

total: 10
passed: 8
issues: 2
pending: 0
skipped: 0

## Gaps

- truth: "Circular dependencies are detected at config parse time (startup), not request time"
  status: failed
  reason: "User reported: it doesnt fail to start when fails when a request is received"
  severity: major
  test: 9
  root_cause: "BuildDAG called in executor.go:55 at request time instead of during CompileConfig in parser.go"
  artifacts:
    - path: "internal/composition/executor.go"
      issue: "BuildDAG called at line 55 during Execute(), should be pre-built"
    - path: "internal/composition/parser.go"
      issue: "CompileConfig does not call BuildDAG to pre-validate"
  missing:
    - "Add ExecutionPlan field to CompiledComposition"
    - "Call BuildDAG during CompileConfig for each composition"
    - "Update executor to use pre-built plan"
  debug_session: ""

- truth: "Missing step references are detected at config parse time (startup), not request time"
  status: failed
  reason: "User reported: doesnt fail at startup but fails when a request is received"
  severity: major
  test: 10
  root_cause: "Same as test 9 - BuildDAG validates step references but is called at request time"
  artifacts:
    - path: "internal/composition/executor.go"
      issue: "BuildDAG called at line 55 during Execute(), should be pre-built"
    - path: "internal/composition/parser.go"
      issue: "CompileConfig does not call BuildDAG to pre-validate"
  missing:
    - "Same fix as test 9 - move BuildDAG to CompileConfig"
  debug_session: ""
