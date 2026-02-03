# Architecture Research

**Domain:** REST API Composition Gateway
**Researched:** 2026-02-03
**Confidence:** MEDIUM-HIGH

## Standard Architecture

### System Overview

High-performance API gateways in Go follow a layered architecture separating concerns:

```
┌─────────────────────────────────────────────────────────────┐
│                    HTTP Server Layer                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   Router     │  │  Middleware  │  │   TLS/HTTP2  │      │
│  │   Handler    │  │    Chain     │  │   Config     │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
│         │                  │                  │              │
├─────────┴──────────────────┴──────────────────┴──────────────┤
│                    Request Processing Layer                   │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ DAG Executor │  │   Context    │  │ Expr Engine  │      │
│  │  (Parallel)  │  │ Propagation  │  │  (Dynamic)   │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
│         │                  │                  │              │
├─────────┴──────────────────┴──────────────────┴──────────────┤
│                    HTTP Client Layer                          │
│  ┌───────────────────────────────────────────────────────┐   │
│  │         Connection Pool Manager                       │   │
│  │  (MaxIdleConns: 100, MaxIdleConnsPerHost: 100)      │   │
│  └───────────────────────────────────────────────────────┘   │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                   │
│  │ Upstream │  │ Upstream │  │ Upstream │                   │
│  │ Client 1 │  │ Client 2 │  │ Client N │                   │
│  └──────────┘  └──────────┘  └──────────┘                   │
└─────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| **Router Handler** | Accept incoming HTTP requests, extract path/method/headers, route to appropriate composition definition | `net/http` with `gorilla/mux` or `chi` for routing |
| **Middleware Chain** | Authentication, logging, rate limiting, CORS, request tracing | Chain of `http.Handler` wrappers |
| **DAG Executor** | Parse step dependencies, schedule parallel execution via goroutines, collect results | Custom scheduler using `sync.WaitGroup` or DAG library |
| **Context Manager** | Propagate timeouts, cancellation signals, tracing metadata across goroutines | Go's `context.Context` with `context.WithTimeout` |
| **Expr Engine** | Evaluate dynamic values (e.g., `steps.auth.userId`) for request construction | `github.com/expr-lang/expr` library |
| **Connection Pool** | Reuse HTTP connections to upstream services, handle keep-alive | `http.Transport` with tuned connection pool settings |
| **Upstream Client** | Execute HTTP requests to backend services, handle retries and circuit breaking | `http.Client` with custom `RoundTripper` |
| **Config Loader** | Load YAML compositions, watch for changes, hot-reload without restart | `fsnotify` watching config directory, `gopkg.in/yaml.v3` parsing |

## Recommended Project Structure

```
restitch/
├── cmd/
│   └── restitch-gateway/       # Binary entry point
│       └── main.go             # Wire up dependencies, start server
├── internal/
│   ├── config/                 # Configuration loading & hot-reload
│   │   ├── loader.go           # YAML parsing, validation
│   │   ├── watcher.go          # fsnotify-based file watching
│   │   └── types.go            # CompositionConfig structs
│   ├── executor/               # DAG execution engine
│   │   ├── dag.go              # Build dependency graph
│   │   ├── scheduler.go        # Parallel goroutine scheduling
│   │   └── step.go             # Step execution logic
│   ├── http/                   # HTTP layer components
│   │   ├── handler.go          # Main request handler
│   │   ├── middleware.go       # Middleware chain
│   │   └── client.go           # Upstream HTTP client with pooling
│   ├── expr/                   # Expression evaluation
│   │   ├── evaluator.go        # Expr wrapper
│   │   └── context.go          # Build evaluation context from steps
│   └── server/                 # HTTP server setup
│       ├── server.go           # net/http server configuration
│       └── router.go           # Route registration
├── pkg/                        # Public interfaces (if building library)
│   └── composition/            # Public API types
└── configs/                    # Example composition YAML files
    └── example.yaml
```

### Structure Rationale

- **cmd/restitch-gateway/**: Single binary entry point. Keep main.go minimal (dependency injection, config loading, server start). This is Go's standard pattern for executable binaries.
- **internal/**: Private packages enforced by Go compiler. Cannot be imported by external projects. Core business logic lives here.
- **internal/config/**: Isolate all configuration concerns (YAML parsing, validation, hot-reload).
- **internal/executor/**: Isolate DAG scheduling logic. Complex enough to warrant own package.
- **internal/http/**: All HTTP-specific code (handlers, middleware, client). Clear boundary between HTTP and business logic.
- **pkg/**: Optional. Only if building reusable library components for external consumption. Many projects skip this.

## Architectural Patterns

### Pattern 1: Middleware Chain (Adapter Pattern)

**What:** Each middleware wraps the next `http.Handler`, forming a chain. Each middleware can inspect/modify request before passing to next, and inspect/modify response after.

**When to use:** Cross-cutting concerns like logging, authentication, rate limiting that apply to all or many endpoints.

**Trade-offs:**
- **Pros:** Composable, testable in isolation, standard Go idiom
- **Cons:** Order matters (auth before logging user ID), can be hard to debug with many layers

**Example:**
```go
type Middleware func(http.Handler) http.Handler

func LoggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)
        log.Printf("%s %s took %v", r.Method, r.URL.Path, time.Since(start))
    })
}

func Chain(middlewares ...Middleware) Middleware {
    return func(final http.Handler) http.Handler {
        for i := len(middlewares) - 1; i >= 0; i-- {
            final = middlewares[i](final)
        }
        return final
    }
}

// Usage:
handler := Chain(
    LoggingMiddleware,
    AuthMiddleware,
    RateLimitMiddleware,
)(compositionHandler)
```

### Pattern 2: DAG Parallel Execution with Context

**What:** Build dependency graph of steps, execute independent steps in parallel goroutines, synchronize with channels or WaitGroup, propagate cancellation via context.

**When to use:** When you have multiple API calls where some depend on results of others, but many are independent.

**Trade-offs:**
- **Pros:** Dramatic latency reduction (3 sequential 100ms calls = 300ms, parallel = 100ms)
- **Cons:** Complexity in error handling, cancellation propagation, result collection

**Example:**
```go
type Step struct {
    Name         string
    DependsOn    []string
    Execute      func(ctx context.Context, inputs map[string]interface{}) (interface{}, error)
}

func ExecuteDAG(ctx context.Context, steps []Step) (map[string]interface{}, error) {
    results := make(map[string]interface{})
    var mu sync.Mutex
    var wg sync.WaitGroup
    errCh := make(chan error, len(steps))

    for _, step := range steps {
        wg.Add(1)
        go func(s Step) {
            defer wg.Done()

            // Wait for dependencies
            for _, dep := range s.DependsOn {
                // Wait until dep result available
            }

            // Execute step
            result, err := s.Execute(ctx, getInputs(s.DependsOn, results))
            if err != nil {
                errCh <- err
                return
            }

            mu.Lock()
            results[s.Name] = result
            mu.Unlock()
        }(step)
    }

    wg.Wait()
    close(errCh)

    if err := <-errCh; err != nil {
        return nil, err
    }
    return results, nil
}
```

**Better approach:** Use existing DAG libraries like `github.com/heimdalr/dag` or `github.com/dmarticus/directed-acyclic-graph` which handle topological sort, cycle detection, and parallel execution.

### Pattern 3: Connection Pool Tuning for High Throughput

**What:** Configure `http.Transport` with appropriate connection pool limits to enable connection reuse and prevent port exhaustion.

**When to use:** Always in production. Default Go settings (MaxIdleConnsPerHost: 2) are inadequate for high-traffic gateways.

**Trade-offs:**
- **Pros:** 4-5x latency improvement via connection reuse, prevents TCP handshake overhead
- **Cons:** Holds more file descriptors open, requires tuning based on upstream service count

**Example:**
```go
transport := &http.Transport{
    MaxIdleConns:        100,  // Total idle connections across all hosts
    MaxIdleConnsPerHost: 100,  // CRITICAL: Default is 2, increase to match concurrency
    MaxConnsPerHost:     100,  // Total connections per host (including active)
    IdleConnTimeout:     90 * time.Second,
    TLSHandshakeTimeout: 10 * time.Second,
    DialContext: (&net.Dialer{
        Timeout:   10 * time.Second,
        KeepAlive: 30 * time.Second,
    }).DialContext,
}

client := &http.Client{
    Transport: transport,
    Timeout:   30 * time.Second, // Overall request timeout
}
```

**CRITICAL:** Always read response body to completion and close, otherwise connections cannot be reused:
```go
resp, err := client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()
io.Copy(io.Discard, resp.Body) // Drain body even if you don't need it
```

### Pattern 4: Configuration Hot-Reload with fsnotify

**What:** Watch configuration directory for changes using `fsnotify`, reload compositions on file modify events, swap in new config atomically without restarting server.

**When to use:** Production gateways where downtime is unacceptable for config changes.

**Trade-offs:**
- **Pros:** Zero-downtime updates, faster iteration on compositions
- **Cons:** Need to handle invalid configs gracefully (rollback to previous), watch for excessive reload loops

**Example:**
```go
func WatchConfigs(configDir string, onReload func(map[string]*Composition)) error {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return err
    }
    defer watcher.Close()

    if err := watcher.Add(configDir); err != nil {
        return err
    }

    for {
        select {
        case event := <-watcher.Events:
            if event.Op&fsnotify.Write == fsnotify.Write {
                configs, err := loadConfigs(configDir)
                if err != nil {
                    log.Printf("Invalid config, keeping previous: %v", err)
                    continue
                }
                onReload(configs) // Atomic swap
            }
        case err := <-watcher.Errors:
            log.Printf("Watcher error: %v", err)
        }
    }
}
```

**Production enhancement:** Use `github.com/ancalabrese/reload` package which provides auto-rollback on invalid config.

### Pattern 5: Expression Evaluation for Dynamic Values

**What:** Use `github.com/expr-lang/expr` to evaluate dynamic expressions (e.g., `steps.auth.token`) at runtime without reflection overhead.

**When to use:** When composition steps need to reference outputs from previous steps (very common in API composition).

**Trade-offs:**
- **Pros:** Type-safe, fast (compiles to bytecode), safer than `text/template`
- **Cons:** Limited to expression language syntax, not Turing-complete (but that's a feature for safety)

**Example:**
```go
import "github.com/expr-lang/expr"

type EvalContext struct {
    Steps  map[string]interface{}
    Request map[string]interface{}
}

func EvaluateURL(template string, ctx EvalContext) (string, error) {
    program, err := expr.Compile(template, expr.Env(ctx))
    if err != nil {
        return "", err
    }

    result, err := expr.Run(program, ctx)
    if err != nil {
        return "", err
    }

    return result.(string), nil
}

// Usage:
ctx := EvalContext{
    Steps: map[string]interface{}{
        "auth": map[string]interface{}{"token": "abc123"},
    },
}
url, _ := EvaluateURL(`"https://api.example.com?token=" + steps.auth.token`, ctx)
// url = "https://api.example.com?token=abc123"
```

**Performance tip:** Compile expressions once at config load time, reuse compiled programs across requests.

## Data Flow

### Request Lifecycle

```
1. HTTP Request Received (net/http)
   ↓
2. Middleware Chain
   - Logging (capture start time)
   - Authentication (validate API key)
   - Rate Limiting (check quota)
   - Context enrichment (add trace ID)
   ↓
3. Route Matching (find composition config)
   ↓
4. DAG Construction
   - Parse step dependencies
   - Build execution graph
   - Detect cycles (fail fast)
   ↓
5. Parallel Execution (goroutines)
   - For each step with no pending dependencies:
     a. Create child context with timeout
     b. Evaluate dynamic expressions (Expr)
     c. Build HTTP request to upstream
     d. Execute via connection pool
     e. Store result in shared map (mutex)
     f. Signal dependent steps
   ↓
6. Result Aggregation
   - Wait for all goroutines
   - Check for errors (any step failed?)
   - Merge results into response shape
   ↓
7. HTTP Response
   - Serialize to JSON
   - Apply response middleware (logging duration)
   - Write to client
```

### Context Propagation Flow

```
Parent Request Context (from net/http)
    ↓ (with timeout)
DAG Execution Context
    ↓ (fork per goroutine)
Step 1 Context ──→ HTTP Client ──→ Upstream Service 1
Step 2 Context ──→ HTTP Client ──→ Upstream Service 2
Step 3 Context ──→ HTTP Client ──→ Upstream Service 3
    ↑
    └─ If parent cancels (client disconnect), all children cancel
```

**Key insight:** Context cancellation cascades. If client disconnects, gateway stops all in-flight upstream requests via `context.Done()` channel.

### Configuration Reload Flow

```
1. fsnotify detects file change
   ↓
2. Trigger reload
   ↓
3. Parse YAML files
   ↓
4. Validate compositions (check cycles, required fields)
   ↓
5. If valid:
     - Compile all Expr templates
     - Build routing table
     - Atomic swap (sync.RWMutex or atomic.Value)
     - Log success
   If invalid:
     - Log error
     - Keep previous config
     - Alert monitoring
```

## Scaling Considerations

| Scale | Architecture Adjustments |
|-------|--------------------------|
| **0-1k req/s** | Single instance, default settings fine. Focus on correctness over optimization. |
| **1k-10k req/s** | Tune connection pool (MaxIdleConnsPerHost: 100), add basic monitoring (Prometheus), enable pprof for profiling. Connection pool tuning is first bottleneck. |
| **10k-100k req/s** | Multiple instances behind load balancer (stateless design enables horizontal scaling), add response caching (in-memory or Redis), optimize hot paths (reduce allocations), consider connection limits to upstream services. |
| **100k+ req/s** | Add edge caching (CDN), implement request coalescing (deduplicate identical in-flight requests), consider protocol optimization (HTTP/2, gRPC), monitor GC pressure (reduce allocations). |

### Scaling Priorities

1. **First bottleneck: Connection pool exhaustion**
   - **Symptom:** High latency, many requests creating new connections
   - **Fix:** Increase `MaxIdleConnsPerHost` to match concurrency (e.g., 100)
   - **Validation:** Check `netstat` for TIME_WAIT connections, should drop significantly

2. **Second bottleneck: CPU (expression evaluation, JSON marshaling)**
   - **Symptom:** High CPU usage, slow response times despite low upstream latency
   - **Fix:** Compile Expr templates once (not per request), use `json.Marshal` pools, consider `jsoniter` library
   - **Validation:** Profile with `pprof` to identify hot functions

3. **Third bottleneck: Memory (GC pressure from allocations)**
   - **Symptom:** GC pauses visible in latency percentiles (p99 spikes)
   - **Fix:** Reuse buffers with `sync.Pool`, reduce allocations in hot paths
   - **Validation:** Use `GODEBUG=gctrace=1` to monitor GC behavior

## Anti-Patterns

### Anti-Pattern 1: Creating New HTTP Client Per Request

**What people do:**
```go
func callUpstream(url string) (*http.Response, error) {
    client := &http.Client{} // NEW CLIENT EVERY TIME
    return client.Get(url)
}
```

**Why it's wrong:**
- No connection pooling, every request pays TCP handshake cost (latency +50-200ms)
- Port exhaustion on high traffic (TIME_WAIT connections pile up)
- 4-5x slower than reusing connections

**Do this instead:**
```go
var sharedClient = &http.Client{
    Transport: &http.Transport{
        MaxIdleConnsPerHost: 100,
        // ... other tuning
    },
}

func callUpstream(url string) (*http.Response, error) {
    return sharedClient.Get(url) // Reuses connections
}
```

### Anti-Pattern 2: Ignoring Context Cancellation

**What people do:**
```go
func executeStep(ctx context.Context, url string) (interface{}, error) {
    resp, err := http.Get(url) // Ignores ctx, keeps going even if parent cancelled
    // ...
}
```

**Why it's wrong:**
- Wastes resources on work nobody cares about (client disconnected)
- Can cause goroutine leaks if operation blocks indefinitely
- Delays error propagation

**Do this instead:**
```go
func executeStep(ctx context.Context, url string) (interface{}, error) {
    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
    resp, err := client.Do(req) // Context cancellation stops request
    if err != nil {
        return nil, err
    }
    // Always check context in long operations
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
        // continue processing
    }
}
```

### Anti-Pattern 3: Not Draining Response Bodies

**What people do:**
```go
resp, err := client.Get(url)
if err != nil {
    return err
}
defer resp.Body.Close()
if resp.StatusCode != 200 {
    return errors.New("bad status") // BODY NOT READ
}
```

**Why it's wrong:**
- Connection cannot be returned to pool, stays in TIME_WAIT
- Under load, exhausts connection pool
- Difficult to debug (manifests as high latency, not clear error)

**Do this instead:**
```go
resp, err := client.Get(url)
if err != nil {
    return err
}
defer func() {
    io.Copy(io.Discard, resp.Body) // Always drain
    resp.Body.Close()
}()
if resp.StatusCode != 200 {
    return errors.New("bad status") // Connection returned to pool
}
```

### Anti-Pattern 4: Panicking in Goroutines

**What people do:**
```go
for _, step := range steps {
    go func(s Step) {
        result := s.Execute() // If this panics, crashes entire process
        results[s.Name] = result
    }(step)
}
```

**Why it's wrong:**
- Panic in goroutine crashes entire gateway process
- No way to recover or log partial results
- Violates "make zero values useful" principle (whole gateway down)

**Do this instead:**
```go
for _, step := range steps {
    go func(s Step) {
        defer func() {
            if r := recover(); r != nil {
                log.Printf("Step %s panicked: %v", s.Name, r)
                errCh <- fmt.Errorf("step panicked: %v", r)
            }
        }()
        result, err := s.Execute()
        if err != nil {
            errCh <- err
            return
        }
        results[s.Name] = result
    }(step)
}
```

### Anti-Pattern 5: Using Mutexes in Hot Path Instead of Channels

**What people do:**
```go
var mu sync.Mutex
results := make(map[string]interface{})

for _, step := range steps {
    go func(s Step) {
        result := s.Execute()
        mu.Lock()         // Contention on every step
        results[s.Name] = result
        mu.Unlock()
    }(step)
}
```

**Why it's wrong:**
- Mutex contention slows down parallel execution
- Not idiomatic Go (channels are often clearer)
- Harder to reason about ordering

**Do this instead:**
```go
type stepResult struct {
    Name   string
    Result interface{}
}

resultCh := make(chan stepResult, len(steps))

for _, step := range steps {
    go func(s Step) {
        result := s.Execute()
        resultCh <- stepResult{s.Name, result} // No contention
    }(step)
}

results := make(map[string]interface{})
for i := 0; i < len(steps); i++ {
    r := <-resultCh
    results[r.Name] = r.Result
}
```

**Note:** For Restitch use case with dependencies, mutex is acceptable since writes are infrequent (one per completed step). For highly contentious maps, channels or sync.Map may be better.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| **Upstream REST APIs** | HTTP client with connection pooling, circuit breaker for fault tolerance | Use context for timeout propagation, wrap errors with service name |
| **Authentication providers** | Middleware intercepts all requests, validates JWT/API key, injects user context | Cache validation results (short TTL) to avoid auth service overload |
| **Observability (Prometheus)** | Instrument with `prometheus/client_golang`, expose /metrics endpoint | Track: request count, duration histogram, upstream latency, error rate |
| **Tracing (OpenTelemetry)** | Inject trace context into upstream requests, span per DAG step | Use W3C Trace Context headers for cross-service tracing |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| **Handler ↔ Executor** | Function call passing context + config | Handler is thin, executor owns orchestration logic |
| **Executor ↔ HTTP Client** | Function call with context + request params | Executor doesn't know HTTP details, client handles protocols |
| **Config Loader ↔ Handler** | Atomic pointer swap (`atomic.Value` or `sync.RWMutex`) | Hot-reload without blocking requests |
| **Middleware ↔ Handler** | `http.Handler` interface (Go standard) | Middleware chain wraps handler, clear separation |

## Build Order Implications

### Phase 1: Minimal Gateway (Core Request Flow)
**Build in this order:**
1. Basic HTTP server with single route
2. Simple router matching request to hardcoded config
3. Single step execution (no DAG yet)
4. HTTP client with basic connection pool
5. Response serialization

**Rationale:** Establish end-to-end flow first. Can test with `curl` immediately.

**Dependency:** HTTP server → Router → Single step execution → HTTP client → Response

### Phase 2: DAG Execution (Parallel Composition)
**Build in this order:**
1. Step dependency parser
2. Topological sort (or integrate DAG library)
3. Goroutine scheduler with WaitGroup
4. Result collection with mutex
5. Error aggregation

**Rationale:** Core value proposition is parallel execution. This is the most complex component.

**Dependency:** Single step execution must work first (provides building block for DAG)

### Phase 3: Dynamic Expressions (Expr Integration)
**Build in this order:**
1. Integrate `expr-lang/expr` library
2. Build evaluation context from step results
3. Expression compilation at config load
4. Expression evaluation in request building

**Rationale:** Enables step chaining. Needed before real compositions work.

**Dependency:** DAG execution must work first (provides step results to evaluate against)

### Phase 4: Configuration System (Hot-Reload)
**Build in this order:**
1. YAML parsing for composition definitions
2. Config validation
3. fsnotify integration
4. Atomic config swap
5. Error handling and rollback

**Rationale:** Enables external configuration. Last because early phases can use hardcoded configs.

**Dependency:** All core components must work first (config drives their behavior)

### Phase 5: Production Hardening
**Build in this order:**
1. Middleware chain (logging, auth, rate limiting)
2. Context propagation and timeout enforcement
3. Observability (metrics, tracing)
4. Graceful shutdown
5. Error wrapping and structured logging

**Rationale:** Nice-to-haves for production, not required for core functionality.

**Dependency:** Core gateway must work first

## Key Architectural Decisions for Restitch

### Decision 1: Stateless Design
**Choice:** No shared state between requests, all state in request context
**Rationale:** Enables horizontal scaling, simplifies deployment, prevents subtle bugs
**Trade-off:** Can't do request coalescing (deduplicating identical in-flight requests) without shared state

### Decision 2: DAG Library vs. Custom
**Recommendation:** Use existing DAG library (`github.com/heimdalr/dag` or `github.com/natessilva/dag`)
**Rationale:** Cycle detection and topological sort are tricky to get right, battle-tested libraries exist
**Trade-off:** Additional dependency, but reduces risk of subtle concurrency bugs

### Decision 3: Expression Language (Expr)
**Choice:** Use `github.com/expr-lang/expr` (not Go templates)
**Rationale:** Type-safe, fast, safer than templates (no code execution), widely adopted (Google Cloud, Uber, GoDaddy)
**Trade-off:** Users must learn Expr syntax, but it's simpler than full programming language

### Decision 4: Configuration Format (YAML)
**Choice:** YAML for composition definitions
**Rationale:** Human-friendly, supports comments, standard in DevOps ecosystem
**Trade-off:** YAML parsing is slower than JSON, but config is loaded rarely (hot-reload, not per-request)

### Decision 5: Hot-Reload Mechanism
**Choice:** fsnotify-based file watching
**Rationale:** Zero-downtime updates, fast feedback loop for composition changes
**Trade-off:** Need robust validation and rollback to prevent bad configs from crashing gateway

## Sources

**Architecture Patterns:**
- [Mastering High-Performance API Gateways in Go](https://medium.com/@ksandeeptech07/mastering-high-performance-api-gateways-in-go-833310e8aeb4)
- [The Anatomy of an API Gateway in Golang](https://hackernoon.com/the-anatomy-of-an-api-gateway-in-golang)
- [API Gateway Design Pattern in Go](https://medium.com/@rajamanohar.mummidi/api-gateway-design-pattern-in-go-c8741ce48af8)
- [Microservices Pattern: API Gateway](https://microservices.io/patterns/apigateway.html)

**Reverse Proxy & Performance:**
- [Creating a reverse proxy server in Go](https://reintech.io/blog/creating-a-reverse-proxy-server-in-go)
- [Writing a reverse proxy in Go](https://developer20.com/writing-proxy-in-go/)

**Middleware Patterns:**
- [Middleware Patterns in Go](https://drstearns.github.io/tutorials/gomiddleware/)
- [Making and using HTTP Middleware in Go](https://www.alexedwards.net/blog/making-and-using-middleware)
- [Go HTTP Middleware: Build Better APIs with These Patterns](https://dev.to/jones_charles_ad50858dbc0/go-http-middleware-build-better-apis-with-these-patterns-2nl2)

**Connection Pooling:**
- [HTTP Connection Pooling in Go](https://davidbacisin.com/writing/golang-http-connection-pools-1)
- [How to Use the HTTP Client in GO To Enhance Performance](https://www.loginradius.com/blog/engineering/tune-the-go-http-client-for-high-performance)
- [Deep Dive into Go's HTTP Client Transport Layer](https://leapcell.io/blog/deep-dive-into-go-s-http-client-transport-layer)
- [Go HTTP Client Patterns: A Production-Ready Implementation Guide](https://jsschools.com/golang/go-http-client-patterns-a-production-ready-implem/)

**DAG Execution:**
- [dag package - github.com/natessilva/dag](https://pkg.go.dev/github.com/natessilva/dag)
- [GitHub - heimdalr/dag](https://github.com/heimdalr/dag)
- [Building a Simple DAG Manager in Go](https://medium.com/@ahamrouni/building-a-simple-dag-manager-in-go-a-step-by-step-guide-31880db7a4a8)
- [GO-DAG Framework](https://docs.go-dag.dev.rho.social/)

**Configuration Hot-Reload:**
- [Clean and simple hot-reloading on uninterrupted go applications](https://itnext.io/clean-and-simple-hot-reloading-on-uninterrupted-go-applications-5974230ab4c5)
- [Effortless Hot Reloading in Golang: Harnessing the Power of Viper](https://medium.com/@adamszpilewicz/effortless-hot-reloading-in-golang-harnessing-the-power-of-viper-4b54703f7424)
- [Golang hot configuration reload](https://www.openmymind.net/Golang-Hot-Configuration-Reload/)

**Context Patterns:**
- [How to Implement Request Context Propagation in Go Microservices](https://oneuptime.com/blog/post/2026-02-01-go-context-propagation-microservices/view)
- [Golang Context - Cancellation, Timeout and Propagation](https://golangbot.com/context-timeout-cancellation/)
- [Go Concurrency Patterns: Context](https://go.dev/blog/context)

**Expression Evaluation:**
- [GitHub - expr-lang/expr](https://github.com/expr-lang/expr)
- [Expr | Expression language](https://expr-lang.org/)

**API Composition:**
- [Microservices Pattern: API Composition](https://microservices.io/patterns/data/api-composition.html)
- [API Composition Pattern in Microservices](https://www.geeksforgeeks.org/system-design/api-composition-pattern-in-microservices/)
- [API Composition - BFF Patterns](https://bff-patterns.com/patterns/api-composition)

**Project Structure:**
- [GitHub - golang-standards/project-layout](https://github.com/golang-standards/project-layout)
- [Organizing a Go module](https://go.dev/doc/modules/layout)
- [Go Project Structure: Practices & Patterns](https://www.glukhov.org/post/2025/12/go-project-structure/)

**Error Handling:**
- [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)
- [Error Handling in Go: Idiomatic Patterns for Clean Code](https://www.djamware.com/post/6926eda9eca0e67f5e7d5d34/error-handling-in-go-idiomatic-patterns-for-clean-code)
- [Go Error Handling Techniques](https://arashtaher.wordpress.com/2024/09/05/go-error-handling-techniques-exploring-sentinel-errors-custom-types-and-client-facing-errors/)

**Gateway Implementations:**
- [KrakenD Design Principles](https://www.krakend.io/docs/design/)
- [Kong Gateway Architecture](https://deepwiki.com/Kong/kong/2-architecture-and-core-components)

---
*Architecture research for: Restitch REST API Composition Gateway*
*Researched: 2026-02-03*
