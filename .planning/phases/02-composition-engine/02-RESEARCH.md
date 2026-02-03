# Phase 2: Composition Engine - Research

**Researched:** 2026-02-03
**Domain:** API composition, DAG execution, expression evaluation
**Confidence:** HIGH

## Summary

Phase 2 implements a composition engine that orchestrates multi-step API calls using YAML configuration, expr-lang/expr for dynamic values, and DAG-based parallel execution. The research identifies expr-lang/expr v1.17.7 as the locked expression language (user decision), establishes errgroup.WithContext as the standard pattern for fail-fast parallel execution, and confirms gopkg.in/yaml.v3 for YAML parsing despite its unmaintained status (widely used, stable API).

The standard approach is to build a custom DAG executor rather than use a workflow library, since existing Go DAG libraries are either too heavyweight (workflow orchestrators with persistence) or too simple (just graph structures without execution semantics). The composition engine requires specific semantics: parse YAML, build dependency graph from expr variable usage, execute with errgroup for fail-fast cancellation, and merge responses via template evaluation.

**Primary recommendation:** Build custom DAG execution with errgroup.WithContext for parallel steps, use expr's AST visitor to infer dependencies from variable usage, validate all expressions at YAML parse time (fail early), and reuse Phase 1's HTTP client patterns for upstream calls.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| expr-lang/expr | v1.17.7 | Expression evaluation | User decision (CONTEXT.md). Memory-safe, type-safe, used by Google Cloud Platform, Uber, OpenTelemetry. 7.6k stars, MIT license. |
| gopkg.in/yaml.v3 | v3 | YAML parsing | De facto standard (295/402 YAML test suite). Now unmaintained but widely used, stable API. |
| golang.org/x/sync/errgroup | latest | Parallel execution with error propagation | Stdlib extension, standard pattern for fail-fast goroutine groups with context cancellation. |
| github.com/google/uuid | v1.6+ | UUID generation | RFC 4122 implementation, used for X-Request-ID generation. Standard choice over gofrs/uuid. |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| github.com/goccy/go-yaml | v1.15+ | Alternative YAML parser | Only if gopkg.in/yaml.v3 issues arise. 60 more test cases passed, but less widely used. |
| net/http | stdlib | HTTP client | Already implemented in Phase 1 with proper connection pooling. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Custom DAG execution | github.com/Azure/go-workflow | Workflow library is production-ready but includes features we don't need (callbacks, retries handled in Phase 4). Custom DAG gives precise control over semantics. |
| Custom DAG execution | github.com/natessilva/dag | Pure graph structure, lacks execution semantics (no context cancellation, no result passing). Would need extensive wrapping. |
| gopkg.in/yaml.v3 | github.com/goccy/go-yaml | Better spec compliance (60 more test cases), but gopkg.in/yaml.v3 is more widely adopted. Switch only if specific features needed. |

**Installation:**
```bash
go get github.com/expr-lang/expr@v1.17.7
go get gopkg.in/yaml.v3
go get golang.org/x/sync/errgroup
go get github.com/google/uuid
```

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── composition/        # Composition engine core
│   ├── config.go      # YAML schema structs
│   ├── parser.go      # YAML parsing and validation
│   ├── dag.go         # DAG builder and dependency inference
│   ├── executor.go    # DAG execution with errgroup
│   ├── step.go        # Step execution logic
│   └── response.go    # Response merging via expr templates
├── client/            # HTTP client (Phase 1)
└── server/            # HTTP server (Phase 1)
```

### Pattern 1: YAML Configuration Schema
**What:** Single config file with named upstreams and composition definitions
**When to use:** User decided on single-file configuration (CONTEXT.md)
**Example:**
```yaml
# Source: User decision (CONTEXT.md)
upstreams:
  user-service:
    url: "https://users.example.com"
  order-service:
    url: "https://orders.example.com"

compositions:
  user-with-orders:
    steps:
      - name: user
        upstream: user-service
        path: "/users/{{ req.query.user_id }}"
        # method defaults to GET
        # headers default to empty

      - name: orders
        upstream: order-service
        path: "/orders?user_id={{ req.query.user_id }}"
        # No explicit depends_on needed - inferred from expr usage

    response:
      status: 200
      body:
        user: "{{ steps.user.body }}"
        orders: "{{ steps.orders.body }}"
```

### Pattern 2: Dependency Inference from Expression Usage
**What:** Use expr AST visitor to extract variables, infer dependencies from `steps.stepname` references
**When to use:** Default behavior (user decision: dependencies inferred from expr usage)
**Example:**
```go
// Source: expr-lang.org/docs/visitor + custom logic
type dependencyVisitor struct {
    dependencies map[string]bool
}

func (v *dependencyVisitor) Visit(node *ast.Node) {
    if member, ok := (*node).(*ast.MemberNode); ok {
        if ident, ok := member.Node.(*ast.IdentifierNode); ok {
            if ident.Value == "steps" {
                // Extract step name from steps.stepname reference
                if prop, ok := member.Property.(*ast.StringNode); ok {
                    v.dependencies[prop.Value] = true
                }
            }
        }
    }
}

// Parse expression and extract dependencies
tree, err := parser.Parse(`steps.user.body.id`)
visitor := &dependencyVisitor{dependencies: make(map[string]bool)}
ast.Walk(&tree.Node, visitor)
// visitor.dependencies["user"] == true
```

### Pattern 3: Fail-Fast Parallel Execution with errgroup
**What:** Execute independent steps in parallel, cancel all on first error
**When to use:** Always (user decision: fail fast on required step failure)
**Example:**
```go
// Source: pkg.go.dev/golang.org/x/sync/errgroup
g, ctx := errgroup.WithContext(parentCtx)

// Launch all ready steps
for _, step := range readySteps {
    step := step // Capture loop variable
    g.Go(func() error {
        result, err := executeStep(ctx, step)
        if err != nil {
            return err // Triggers context cancellation for all goroutines
        }
        results.Store(step.Name, result)
        return nil
    })
}

// Wait for all or first error
if err := g.Wait(); err != nil {
    return nil, fmt.Errorf("step failed: %w", err)
}
```

### Pattern 4: Expression Pre-Compilation and Validation
**What:** Compile all expressions at config parse time to fail early on syntax errors
**When to use:** Always - catch expression errors before serving traffic
**Example:**
```go
// Source: pkg.go.dev/github.com/expr-lang/expr
type CompiledStep struct {
    Name         string
    PathProgram  *vm.Program  // Compiled path expression
    BodyProgram  *vm.Program  // Compiled response body expression
    Dependencies []string     // Inferred from expressions
}

func compileStep(step StepConfig) (*CompiledStep, error) {
    // Define environment for type checking
    env := map[string]interface{}{
        "req": map[string]interface{}{
            "path":    "",
            "query":   map[string]string{},
            "headers": map[string]string{},
            "body":    map[string]interface{}{},
        },
        "steps": map[string]interface{}{
            // Populated with known step names
        },
    }

    // Compile path expression with type checking
    pathProg, err := expr.Compile(step.Path, expr.Env(env))
    if err != nil {
        return nil, fmt.Errorf("invalid path expression: %w", err)
    }

    return &CompiledStep{
        Name:        step.Name,
        PathProgram: pathProg,
    }, nil
}
```

### Pattern 5: Response Merging via Template Evaluation
**What:** YAML response body structure with expr placeholders, evaluated after steps complete
**When to use:** Always (user decision: template object approach)
**Example:**
```yaml
# Source: User decision (CONTEXT.md)
response:
  status: 200  # Can also be expr: "{{ steps.create.status }}"
  content_type: "application/json"
  body:
    # Template structure - output mirrors this shape
    user:
      id: "{{ steps.user.body.id }}"
      name: "{{ steps.user.body.name }}"
    orders: "{{ steps.orders.body | filter(.status == 'active') }}"
    total: "{{ len(steps.orders.body) }}"
```

### Pattern 6: DAG Topological Sort with Kahn's Algorithm
**What:** Build execution order from dependency graph using level-based scheduling
**When to use:** Always - determines which steps can run in parallel
**Example:**
```go
// Source: Kahn's algorithm (standard CS algorithm)
func buildExecutionPlan(steps []*CompiledStep) ([][]string, error) {
    // Build dependency graph
    inDegree := make(map[string]int)
    edges := make(map[string][]string)

    for _, step := range steps {
        inDegree[step.Name] = len(step.Dependencies)
        for _, dep := range step.Dependencies {
            edges[dep] = append(edges[dep], step.Name)
        }
    }

    // Level-based topological sort
    var levels [][]string
    queue := []string{}

    // Find all steps with no dependencies (level 0)
    for name, degree := range inDegree {
        if degree == 0 {
            queue = append(queue, name)
        }
    }

    for len(queue) > 0 {
        // All steps in queue can execute in parallel
        levels = append(levels, queue)

        nextQueue := []string{}
        for _, name := range queue {
            // Decrement in-degree for dependent steps
            for _, dependent := range edges[name] {
                inDegree[dependent]--
                if inDegree[dependent] == 0 {
                    nextQueue = append(nextQueue, dependent)
                }
            }
        }
        queue = nextQueue
    }

    // Check for cycles
    if len(levels) == 0 || sumSteps(levels) != len(steps) {
        return nil, errors.New("circular dependency detected")
    }

    return levels, nil
}
```

### Pattern 7: Header Propagation
**What:** Auto-forward tracing headers, generate X-Request-ID if missing
**When to use:** Always (user decision: auto-propagate specific headers)
**Example:**
```go
// Source: User decision (CONTEXT.md) + RFC standards
var propagatedHeaders = []string{
    "X-Request-ID",
    "X-Correlation-ID",
    "traceparent",        // W3C Trace Context
    "Accept",
    "Accept-Language",
}

func buildUpstreamRequest(ctx context.Context, step *CompiledStep, incomingReq *http.Request) (*http.Request, error) {
    req, err := http.NewRequestWithContext(ctx, step.Method, step.URL, step.Body)
    if err != nil {
        return nil, err
    }

    // Generate X-Request-ID if not present
    requestID := incomingReq.Header.Get("X-Request-ID")
    if requestID == "" {
        requestID = uuid.New().String()
    }
    req.Header.Set("X-Request-ID", requestID)

    // Propagate other tracing headers
    for _, header := range propagatedHeaders {
        if value := incomingReq.Header.Get(header); value != "" {
            req.Header.Set(header, value)
        }
    }

    return req, nil
}
```

### Anti-Patterns to Avoid
- **Don't use sync.Map for step results:** Regular map with mutex or sync.Map are both viable, but sync.Map adds complexity (type assertions, no type safety) without clear benefit. Use regular map with mutex for simplicity unless profiling shows contention.
- **Don't build DAG at request time:** Build DAG once at config parse time, not per-request. DAG structure is static for a given configuration.
- **Don't ignore response body draining:** Phase 1 established DrainAndClose pattern. Always drain response bodies for connection reuse (4-5x latency penalty without it).
- **Don't inline upstream URLs in steps:** User decided on named upstreams pattern (nginx-style upstream blocks) for reusability.

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Expression evaluation | Custom parser/interpreter | expr-lang/expr v1.17.7 | Memory-safe, type-checked, battle-tested (Google, Uber use it). Supports filters, map operations, type coercion. |
| UUID generation | Custom random string generator | github.com/google/uuid | RFC 4122 compliant, handles version variants, proper entropy. |
| YAML parsing | Manual text parsing | gopkg.in/yaml.v3 | Handles anchors, tags, type coercion, edge cases (yes/no as bool, 0777 octals). |
| Parallel execution with cancellation | Manual goroutine coordination | errgroup.WithContext | Handles error propagation, context cancellation, wait synchronization. Well-tested stdlib extension. |
| Topological sort | Custom graph traversal | Kahn's algorithm (implement directly) | Standard algorithm, simple to implement (40 lines), no library needed for basic DAG execution. |
| JSON manipulation | String concatenation | encoding/json Marshal/Unmarshal | Handles escaping, type coercion, nested structures, UTF-8. |

**Key insight:** Expression evaluation and parallel execution coordination are complex with many edge cases. expr-lang/expr provides compile-time type checking and prevents infinite loops. errgroup provides correct context cancellation semantics (critical for fail-fast). Both are production-proven by major companies.

## Common Pitfalls

### Pitfall 1: Forgetting to Drain HTTP Response Bodies
**What goes wrong:** Connection pool exhaustion, 4-5x latency penalty under load
**Why it happens:** Go's http.Client requires reading body to EOF before close to reuse connection
**How to avoid:** Always use Phase 1's DrainAndClose pattern, even for error responses
**Warning signs:** High connection churn, MaxIdleConnsPerHost not helping latency
**Source:** [HTTP Connection reuse in Go clients](https://blog.cubieserver.de/2022/http-connection-reuse-in-go-clients/)

### Pitfall 2: expr Map Key Type Coercion Surprises
**What goes wrong:** `map[int]string` accessed with `mymap["1"]` fails at compile time, but `mymap := {"1": "foo"}` silently creates string keys
**Why it happens:** expr creates maps with string keys only, but Go maps passed to expr retain their key types
**How to avoid:** Always use string keys in expr expressions. Document that map access is string-only.
**Warning signs:** Runtime errors accessing numeric-keyed maps
**Source:** [10 Critical Gotchas with expr-lang/expr](https://buildsoftwaresystems.com/post/go-scripting-expr-lang-gotchas/)

### Pitfall 3: Passing expr Functions Only at Runtime
**What goes wrong:** Expression compiles successfully but fails at runtime with "unknown function"
**Why it happens:** expr requires functions to be passed at BOTH compile time (with expr.Function) AND runtime (in environment map)
**How to avoid:** Create helper that registers functions in both places. Validate at config parse time with full environment.
**Warning signs:** Expressions work in tests but fail in production when config changes
**Source:** [10 Critical Gotchas with expr-lang/expr](https://buildsoftwaresystems.com/post/go-scripting-expr-lang-gotchas/)

### Pitfall 4: Treating Gateway as a Monolith
**What goes wrong:** Single gateway configuration becomes a bottleneck, single point of failure
**Why it happens:** Convenient to put all compositions in one file/service
**How to avoid:** Document configuration modularity. Support multiple composition files (Phase 4 consideration).
**Warning signs:** Config file grows beyond 500 lines, deployment requires restarting all traffic
**Source:** [API gateway framework: The complete 2026 guide](https://www.digitalapi.ai/blogs/api-gateway-framework-the-complete-2026-guide-for-modern-microservices)

### Pitfall 5: Downstream Service Failure Handling
**What goes wrong:** Upstream 500 error causes gateway to return 500, but with gateway's error format instead of upstream's error details
**Why it happens:** Default HTTP client error handling wraps upstream errors
**How to avoid:** User decided on error passthrough - return upstream's status code and body directly (no wrapping)
**Warning signs:** Frontend team complains error messages are lost, logs show "upstream error" without details
**Source:** User decision (CONTEXT.md)

### Pitfall 6: Not Validating Expressions at Parse Time
**What goes wrong:** Composition serves traffic, user makes request, gets 500 from expression syntax error
**Why it happens:** Lazy evaluation of expressions at request time
**How to avoid:** Compile ALL expressions (path, params, response body) at YAML parse time. Fail gateway startup if any expression invalid.
**Warning signs:** Production traffic gets expression syntax errors, errors not caught in testing
**Source:** expr best practices

### Pitfall 7: Goroutine Leaks from Context Cancellation
**What goes wrong:** Step goroutines continue running after composition request is cancelled
**Why it happens:** Context not passed to http.NewRequestWithContext
**How to avoid:** ALWAYS use http.NewRequestWithContext with the errgroup's derived context. Phase 1's client.Do already does this.
**Warning signs:** Goroutine count grows over time, memory leak
**Source:** [Concurrent HTTP Requests in Go: Best Practices](https://medium.com/insiderengineering/concurrent-http-requests-in-golang-best-practices-and-techniques-f667e5a19dea)

### Pitfall 8: expr Struct Access vs Map Access Asymmetry
**What goes wrong:** `user.invalidField` throws compile error, but `userMap.invalidField` returns zero value at runtime
**Why it happens:** expr treats structs strictly (type-checked) but maps permissively (dynamic access)
**How to avoid:** Document behavior. Consider using structs for step results instead of map[string]interface{} to catch errors at compile time.
**Warning signs:** Silent failures where missing fields return empty values instead of errors
**Source:** [10 Critical Gotchas with expr-lang/expr](https://buildsoftwaresystems.com/post/go-scripting-expr-lang-gotchas/)

## Code Examples

Verified patterns from official sources:

### Parse YAML Configuration
```go
// Source: pkg.go.dev/gopkg.in/yaml.v3
type Config struct {
    Upstreams    map[string]Upstream    `yaml:"upstreams"`
    Compositions map[string]Composition `yaml:"compositions"`
}

type Upstream struct {
    URL string `yaml:"url"`
}

type Composition struct {
    Steps    []Step            `yaml:"steps"`
    Response ResponseTemplate  `yaml:"response"`
}

type Step struct {
    Name     string            `yaml:"name"`
    Upstream string            `yaml:"upstream"`
    Path     string            `yaml:"path"`
    Method   string            `yaml:"method,omitempty"`   // Default: GET
    Headers  map[string]string `yaml:"headers,omitempty"`  // Default: empty
}

type ResponseTemplate struct {
    Status      interface{} `yaml:"status"`       // int or expr string
    ContentType string      `yaml:"content_type"` // Default: application/json
    Body        interface{} `yaml:"body"`         // Nested map with expr strings
}

func parseConfig(data []byte) (*Config, error) {
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("invalid YAML: %w", err)
    }
    return &cfg, nil
}
```

### Compile and Validate Expression
```go
// Source: pkg.go.dev/github.com/expr-lang/expr
func compileExpression(exprStr string, env map[string]interface{}) (*vm.Program, error) {
    program, err := expr.Compile(exprStr,
        expr.Env(env),           // Type checking
        expr.AllowUndefinedVariables(), // Allow partial env for validation
    )
    if err != nil {
        return nil, fmt.Errorf("expression compilation failed: %w", err)
    }
    return program, nil
}

// Example usage
env := map[string]interface{}{
    "req": map[string]interface{}{
        "query": map[string]string{},
    },
    "steps": map[string]interface{}{
        "user": map[string]interface{}{
            "body": map[string]interface{}{},
        },
    },
}

program, err := compileExpression("steps.user.body.id", env)
```

### Execute Expression at Runtime
```go
// Source: pkg.go.dev/github.com/expr-lang/expr
func evaluateExpression(program *vm.Program, env map[string]interface{}) (interface{}, error) {
    result, err := expr.Run(program, env)
    if err != nil {
        return nil, fmt.Errorf("expression evaluation failed: %w", err)
    }
    return result, nil
}

// Example: Evaluate path template
runtimeEnv := map[string]interface{}{
    "req": map[string]interface{}{
        "query": map[string]string{"user_id": "123"},
    },
}

path, err := evaluateExpression(pathProgram, runtimeEnv)
// path: "/users/123"
```

### Build DAG and Execute with errgroup
```go
// Source: pkg.go.dev/golang.org/x/sync/errgroup
type StepResult struct {
    Status  int
    Headers http.Header
    Body    map[string]interface{}
}

func executeComposition(ctx context.Context, plan [][]string, steps map[string]*CompiledStep) (map[string]*StepResult, error) {
    results := make(map[string]*StepResult)
    resultsMutex := sync.Mutex{}

    // Execute each level in sequence (levels contain parallel steps)
    for levelIdx, level := range plan {
        g, levelCtx := errgroup.WithContext(ctx)

        for _, stepName := range level {
            stepName := stepName // Capture loop variable
            step := steps[stepName]

            g.Go(func() error {
                // Build environment with completed step results
                resultsMutex.Lock()
                env := buildEnvironment(results)
                resultsMutex.Unlock()

                // Execute step
                result, err := executeStep(levelCtx, step, env)
                if err != nil {
                    // Fail fast - context cancelled for all goroutines
                    return fmt.Errorf("step %s failed: %w", stepName, err)
                }

                // Store result
                resultsMutex.Lock()
                results[stepName] = result
                resultsMutex.Unlock()

                return nil
            })
        }

        // Wait for level to complete or first error
        if err := g.Wait(); err != nil {
            return nil, err
        }
    }

    return results, nil
}
```

### Generate UUID for X-Request-ID
```go
// Source: pkg.go.dev/github.com/google/uuid
import "github.com/google/uuid"

func getOrGenerateRequestID(r *http.Request) string {
    requestID := r.Header.Get("X-Request-ID")
    if requestID == "" {
        requestID = uuid.New().String() // RFC 4122 v4 UUID
    }
    return requestID
}
```

### Extract Dependencies from Expression (AST Visitor)
```go
// Source: expr-lang.org/docs/visitor
type dependencyExtractor struct {
    dependencies []string
}

func (d *dependencyExtractor) Visit(node *ast.Node) {
    // Look for steps.stepname patterns
    if member, ok := (*node).(*ast.MemberNode); ok {
        if ident, ok := member.Node.(*ast.IdentifierNode); ok && ident.Value == "steps" {
            if prop, ok := member.Property.(*ast.StringNode); ok {
                d.dependencies = append(d.dependencies, prop.Value)
            }
        }
    }
}

func extractDependencies(exprStr string) ([]string, error) {
    tree, err := parser.Parse(exprStr)
    if err != nil {
        return nil, err
    }

    extractor := &dependencyExtractor{}
    ast.Walk(&tree.Node, extractor)

    return extractor.dependencies, nil
}

// Example: extractDependencies("steps.user.body.id + steps.orders.body[0]")
// Returns: ["user", "orders"]
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| String-based expression parsing | expr-lang/expr with compile-time type checking | expr v1.0 (2019) | Catch errors at config load, not request time. Performance: compiled bytecode vs interpreted. |
| Manual goroutine coordination | errgroup.WithContext | golang.org/x/sync (2018) | Automatic context cancellation, simpler error handling, no risk of goroutine leaks. |
| sync.RWMutex for all concurrent maps | Context-specific choice (sync.RWMutex vs sync.Map) | Ongoing debate | Use sync.RWMutex by default; sync.Map only for read-heavy disjoint keys (profiling required). |
| gopkg.in/yaml.v2 | gopkg.in/yaml.v3 | 2019 | YAML 1.2 support, better spec compliance. v2 deprecated. |
| YAML validation via JSON Schema | Go struct tags + type system | Standard practice | Simpler for static config, JSON Schema for dynamic/user-supplied YAML. |

**Deprecated/outdated:**
- **gopkg.in/yaml.v2:** Use v3 for YAML 1.2 support
- **Manual context cancellation with channels:** Use errgroup.WithContext for simpler error propagation
- **goccy/go-yaml for this use case:** gopkg.in/yaml.v3 sufficient unless specific features needed (better test coverage vs wider adoption tradeoff)

**Note on gopkg.in/yaml.v3 maintenance status:** The library was marked unmaintained in April 2025, but remains the de facto standard with stable API and 7k stars. No critical bugs reported. Consider goccy/go-yaml as future migration path if issues arise.

## Open Questions

Things that couldn't be fully resolved:

1. **How does expr handle deeply nested JSON with unknown schema?**
   - What we know: expr supports map access with `.` notation and array indexing
   - What's unclear: Performance characteristics with deeply nested structures (10+ levels), error messages for invalid paths
   - Recommendation: Test with real upstream responses. Consider response schema validation in Phase 4.

2. **Should step results be stored as structs or map[string]interface{}?**
   - What we know: expr supports both. Structs give compile-time type checking, maps give flexibility.
   - What's unclear: User hasn't decided. Affects whether `steps.user.invalidField` fails at compile vs runtime.
   - Recommendation: Start with map[string]interface{} for flexibility (matches JSON unmarshaling). Consider typed response schemas in Phase 4 if expr errors become problematic.

3. **What happens if circular dependencies exist despite Kahn's algorithm validation?**
   - What we know: Kahn's algorithm detects cycles at config parse time
   - What's unclear: Edge case with dynamic step names or conditional steps (Phase 4 feature)
   - Recommendation: For Phase 2 (static steps only), Kahn's algorithm is sufficient. Document that dynamic steps require cycle detection at request time.

4. **Performance impact of compiling expressions at parse time vs runtime?**
   - What we know: expr.Compile is fast enough for config parsing (one-time cost)
   - What's unclear: Whether re-compilation is needed for hot-reload scenarios (Phase 4)
   - Recommendation: Compile all expressions at config load. Hot-reload requires new compiled config instance.

## Sources

### Primary (HIGH confidence)
- [expr-lang/expr v1.17.7](https://github.com/expr-lang/expr) - Expression language features, version, API
- [pkg.go.dev/github.com/expr-lang/expr](https://pkg.go.dev/github.com/expr-lang/expr) - Official Go package documentation
- [expr-lang.org/docs/getting-started](https://expr-lang.org/docs/getting-started) - Language syntax, built-in functions
- [expr-lang.org/docs/visitor](https://expr-lang.org/docs/visitor) - AST visitor pattern for dependency extraction
- [pkg.go.dev/golang.org/x/sync/errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup) - Official errgroup documentation
- [pkg.go.dev/gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) - YAML v3 package documentation
- [github.com/go-yaml/yaml](https://github.com/go-yaml/yaml) - YAML library status, maintenance notice
- [pkg.go.dev/github.com/google/uuid](https://pkg.go.dev/github.com/google/uuid) - UUID generation library
- [Go blog: Pipelines and cancellation](https://go.dev/blog/pipelines) - Official Go concurrency patterns
- [Go blog: Context](https://go.dev/blog/context) - Context usage and cancellation

### Secondary (MEDIUM confidence)
- [10 Critical Gotchas with expr-lang/expr](https://buildsoftwaresystems.com/post/go-scripting-expr-lang-gotchas/) - Production pitfalls (WebSearch verified with expr docs)
- [How to Use errgroup for Parallel Operations in Go](https://oneuptime.com/blog/post/2026-01-07-go-errgroup/view) - January 2026 article with examples
- [Concurrent HTTP Requests in Go: Best Practices](https://medium.com/insiderengineering/concurrent-http-requests-in-golang-best-practices-and-techniques-f667e5a19dea) - Context cancellation patterns
- [HTTP Connection reuse in Go clients](https://blog.cubieserver.de/2022/http-connection-reuse-in-go-clients/) - Connection pooling pitfalls
- [API gateway framework: The complete 2026 guide](https://www.digitalapi.ai/blogs/api-gateway-framework-the-complete-2026-guide-for-modern-microservices) - API composition anti-patterns
- [Go sync.Map: The Right Tool for the Right Job](https://victoriametrics.com/blog/go-sync-map/) - sync.Map vs RWMutex tradeoffs
- [Working with YAML in Go](https://leapcell.io/blog/working-with-yaml-in-go) - YAML parsing best practices

### Tertiary (LOW confidence)
- [Kazaam JSON transformation library](https://github.com/qntfy/kazaam) - Alternative approach to response merging (not recommended for this use case, expr is sufficient)
- [Azure/go-workflow](https://github.com/Azure/go-workflow) - DAG workflow library (not used, building custom executor)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - expr-lang/expr is user decision, errgroup and gopkg.in/yaml.v3 are de facto standards verified via official docs
- Architecture: HIGH - Patterns verified with official Go blog posts, expr documentation, and Phase 1 implementation
- Pitfalls: MEDIUM-HIGH - expr gotchas verified with official docs, HTTP client issues verified with multiple sources, gateway anti-patterns from recent 2026 article

**Research date:** 2026-02-03
**Valid until:** 30 days (March 5, 2026) - expr-lang/expr and errgroup are stable; gopkg.in/yaml.v3 unmaintained but API frozen

**Note on USER decisions from CONTEXT.md:**
- Expression language: expr-lang/expr (locked decision)
- YAML structure: Single file, named upstreams, smart defaults (locked decision)
- Execution: Fail fast, 30s default timeout, error passthrough (locked decision)
- Response merging: Template object approach (locked decision)
- Dependencies: Inferred from expr usage, explicit depends_on available (locked decision)
