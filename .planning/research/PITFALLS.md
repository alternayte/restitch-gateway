# Pitfalls Research

**Domain:** REST API Composition Gateway
**Researched:** 2026-02-03
**Confidence:** HIGH

## Critical Pitfalls

### Pitfall 1: Goroutine Leaks from HTTP Clients

**What goes wrong:**
HTTP client goroutines leak when response bodies are not properly closed or contexts are not cancelled, causing memory to grow unbounded. At scale, this manifests as persistent goroutines in `readLoop` and `writeLoop` that never terminate, with memory usage increasing with each request.

**Why it happens:**
Developers forget that Go's HTTP client connection pooling requires explicit cleanup. Even when you don't need the response body, it must be read and closed for connections to be reused. Additionally, spawning goroutines for parallel upstream requests without proper context cancellation leaves goroutines waiting indefinitely when parent requests abort.

**How to avoid:**
1. Always defer `resp.Body.Close()` immediately after checking error from HTTP request
2. Drain the response body even if you don't need it: `io.Copy(io.Discard, resp.Body)`
3. Use `defer cancel()` with context.WithCancel/WithTimeout for every goroutine spawned
4. In DAG parallel execution, ensure all goroutines receive cancellation signals when request context is cancelled
5. Configure reasonable `IdleConnTimeout` on Transport (default 90s may be too long for short-lived requests)

**Warning signs:**
- `pprof` shows increasing number of goroutines in `net/http.(*persistConn).readLoop` and `writeLoop`
- Memory grows linearly with request volume
- Number of goroutines grows unbounded (check with `runtime.NumGoroutine()`)
- Connections to upstream services remain open long after requests complete

**Phase to address:**
Phase 1 (Core HTTP handling) - Must be correct from the start. Add goroutine leak tests using `goleak` library in Phase 2 (Testing infrastructure).

---

### Pitfall 2: Connection Pool Misconfiguration

**What goes wrong:**
Go's default `http.Client` has `MaxIdleConnsPerHost` set to only 2, even though `MaxIdleConns` is 100. When your gateway spawns 50 goroutines to fetch data in parallel for a single request, only 2 connections per upstream host are reused, causing the other 48 to create new connections every time. This results in connection churn, TCP handshake overhead, increased latency (p99 can be 14x worse), and potential port exhaustion at scale.

**Why it happens:**
Developers use `http.DefaultClient` or create new `http.Client` instances per request without understanding connection pooling behavior. The default settings are optimized for simple use cases, not high-throughput parallel request orchestration.

**How to avoid:**
1. Create ONE dedicated `http.Client` per upstream service at gateway startup (not per request)
2. Configure Transport with appropriate pool settings:
   ```go
   &http.Transport{
       MaxIdleConns:        100,
       MaxIdleConnsPerHost: 100,  // NOT 2!
       MaxConnsPerHost:     100,  // Limit concurrent connections
       IdleConnTimeout:     90 * time.Second,
   }
   ```
3. For multi-tenant upstreams with many unique hosts, consider dynamic client management with LRU eviction
4. Monitor connection metrics: active connections, idle connections, connection wait time

**Warning signs:**
- High p99 latency despite fast p50 (indicates connection establishment overhead)
- Netstat shows many TIME_WAIT connections
- Upstream services see spiky connection patterns (sawtooth graph)
- Load testing reveals performance degradation that doesn't match upstream capacity

**Phase to address:**
Phase 1 (Core HTTP client setup) - Must be correct from the start. Add connection pool monitoring in Phase 3 (Observability).

---

### Pitfall 3: Context Propagation Failures in DAG Execution

**What goes wrong:**
When DAG nodes spawn goroutines for parallel execution, failing to properly propagate the request context means timeouts and cancellations don't cascade. This results in: upstream requests continuing after the client has disconnected, goroutine leaks (see Pitfall 1), wasted upstream capacity processing requests nobody cares about, and difficulty debugging why requests take longer than configured timeouts.

**Why it happens:**
Developers pass `context.Background()` or create new contexts instead of deriving from the request context. In complex DAG execution with multiple levels of parallel fan-out, it's easy to lose the context chain. Additionally, custom context.WithValue usage can break cancellation if not derived correctly.

**How to avoid:**
1. Always derive contexts from the request context: `ctx := r.Context()`
2. When spawning goroutines for parallel DAG execution:
   ```go
   for _, node := range parallelNodes {
       go func(n *Node) {
           // Use same ctx, not context.Background()
           result := n.Execute(ctx, deps)
       }(node)
   }
   ```
3. Use `context.WithTimeout()` to derive child contexts with shorter deadlines, not to create new isolated contexts
4. Test that calling `cancel()` on request context terminates all in-flight goroutines within 100ms
5. Never store contexts in structs - pass them explicitly through function calls

**Warning signs:**
- Requests continue processing after client disconnect
- Gateway timeout fires but upstream logs show requests completing later
- Goroutine count spikes during load but doesn't return to baseline
- Trace spans show work happening after parent span ended

**Phase to address:**
Phase 1 (DAG executor) - Context wiring must be correct from day one. Add cancellation propagation tests in Phase 2 (Testing).

---

### Pitfall 4: Retry Storms and Cascade Failures

**What goes wrong:**
When an upstream service is struggling (high latency or partial failures), aggressive retry logic in the gateway can make things worse. If 1000 gateway requests each retry 3 times with no backoff, a 50% failure rate turns into 3500 upstream requests instead of 1500. This creates a "retry storm" that can take down the upstream completely, which then causes retries to OTHER upstreams (if they depend on the failing one), creating cascading failures across your entire backend.

**Why it happens:**
Developers add retries to improve reliability without considering the systemic effects. Default retry logic often retries on 5xx errors immediately, which is exactly when the upstream is least able to handle more traffic. In a composition gateway, retries multiply - if 3 parallel upstreams each retry 3 times, a single gateway request can trigger up to 27 upstream requests.

**How to avoid:**
1. Implement exponential backoff with jitter: `baseDelay * (2^attempt) + random(0, baseDelay*0.2)`
2. Only retry on specific transient failures:
   - Network errors (connection refused, timeout)
   - 503 Service Unavailable (maybe)
   - 429 Rate Limited (respect Retry-After header)
   - DO NOT retry on 5xx by default (could be non-transient)
3. Set maximum retry attempts (2-3 max) and total retry budget per request
4. Consider budget-based retry: track percentage of requests retried in last minute, stop retrying if >20%
5. For DAG execution: if node A fails and is retried, don't also retry dependent node B - fail fast instead
6. Implement circuit breaker pattern to stop hitting failing upstreams (skip for v1 per your roadmap, but design retry logic to not conflict with adding it later)

**Warning signs:**
- Upstream request volume is much higher than gateway request volume
- Retries increase during high load (should be opposite - back off when struggling)
- Multiple upstreams fail simultaneously (suggests cascade)
- Gateway latency increases while upstream latency stays the same (queuing/backpressure)

**Phase to address:**
Phase 1 (Basic retry logic with exponential backoff) - Include from start but keep simple. Phase 4+ (Advanced resilience) - Add circuit breakers and budget-based limiting.

---

### Pitfall 5: Expression Language Injection Vulnerability

**What goes wrong:**
If the Expr language evaluator processes expressions containing unsanitized user input, attackers can execute arbitrary code on the gateway server. For example, if a configuration allows `header: "X-User-Id-${request.body.userId}"` and `userId` comes from the request, an attacker could inject `${os.Exit(1)}` or other dangerous expressions. This is especially critical because your gateway runs with access to all upstream service credentials.

**Why it happens:**
Expression languages like Expr are powerful by design - they can call functions and access runtime values. Developers treat them like string templates without realizing they're code execution environments. The risk is highest when config allows expressions in fields that get evaluated with request data, especially when YAML configs can be modified by non-security-aware users.

**How to avoid:**
1. NEVER concatenate user input into expressions - expressions should only reference safe variables
2. Create a whitelist of allowed functions and disable dangerous ones in Expr:
   ```go
   env := expr.Env{
       "os": nil,     // Disable os package
       "exec": nil,   // Disable exec
       // Only expose safe functions
   }
   ```
3. Separate "template variables" from "expression evaluation" - user input should only populate variables, never be part of expression AST
4. If expressions can access request data, validate and sanitize FIRST:
   ```go
   // BAD: expr.Eval("userId: " + request.Body["userId"])
   // GOOD:
   validUserId := validateUserId(request.Body["userId"])
   expr.Eval(expression, map[string]any{"userId": validUserId})
   ```
5. Run security-focused static analysis on all YAML configs that contain expressions
6. Consider sandboxing: limit memory, CPU time, and disable reflection for expression evaluation
7. Document clearly in examples which fields support expressions and security implications

**Warning signs:**
- Config examples show concatenating request data into expressions
- Expr environment allows access to os, exec, or other system packages
- No input validation before passing request data to expression evaluator
- Complex expressions in configs that manipulate user-provided strings

**Phase to address:**
Phase 1 (Initial Expr integration) - Lock down the environment and establish safe patterns. Phase 2 (Security audit) - Add automated checks for dangerous expression patterns in configs.

---

### Pitfall 6: Partial Failure Handling Without Graceful Degradation

**What goes wrong:**
In a composition gateway, when 1 of 5 upstream requests fails, naive error handling returns 500 to the client with NO data, even though 4/5 requests succeeded. This makes your gateway MORE fragile than directly calling services. Users see blank pages when they could have seen 80% of content. This is particularly bad for optional/enhancement data - if "user recommendations" service is down, showing the user profile without recommendations is better than showing nothing.

**Why it happens:**
Developers think in terms of "request succeeds or fails" rather than "request produces partial results." The DAG executor short-circuits on first error instead of collecting all results. Error propagation logic doesn't distinguish between critical dependencies (must succeed) and optional dependencies (failure is acceptable).

**How to avoid:**
1. Design DAG nodes with dependency criticality:
   ```yaml
   nodes:
     - id: user-profile
       required: true    # Gateway fails if this fails
     - id: recommendations
       required: false   # Include if available, omit if fails
   ```
2. Return partial results object:
   ```go
   type CompositionResult struct {
       Data    map[string]any
       Errors  map[string]error  // Per-node errors
       Status  ResultStatus      // Complete, Partial, Failed
   }
   ```
3. Let the YAML config define error handling strategy per node:
   - `fail-fast`: Return 5xx immediately
   - `omit`: Exclude failed data, return partial response
   - `default`: Use fallback value
4. Include `X-Partial-Response: true` header when returning degraded data
5. Consider returning 207 Multi-Status for partial failures instead of 200
6. Add health-based degradation: if upstream has <95% success rate, automatically mark as optional

**Warning signs:**
- Gateway availability is lower than the average of its upstreams (should be higher via graceful degradation)
- User complaints about "all or nothing" behavior ("site was completely down when just one feature wasn't working")
- P99 latency tied to slowest upstream (should have timeout-based degradation)
- No way to test gateway behavior when upstreams have partial failures

**Phase to address:**
Phase 2 (Error handling) - Add required/optional dependency marking. Phase 3 (Graceful degradation) - Implement partial response patterns.

---

### Pitfall 7: Timeout Configuration Cascade Failures

**What goes wrong:**
Gateway has 30s request timeout. Upstream A has 25s timeout. DAG executes 3 sequential nodes each with 10s timeout. Request takes 29s, all upstreams succeed, but gateway returns 504 Gateway Timeout. Alternatively: gateway timeout is TOO SHORT (5s) for legitimate multi-hop composition (fetching 5 services in parallel, each taking 2s), causing false timeout errors. Or worst: no timeouts configured, so a hanging upstream holds gateway connections and goroutines forever.

**Why it happens:**
Timeout configuration requires understanding the entire request flow. Developers set arbitrary numbers without calculating: `gateway_timeout >= max_parallel_execution_time + processing_overhead`. In DAG execution, sequential node timeouts need to sum to less than gateway timeout, and parallel node timeouts should be based on slowest node, not sum of all. Plus, upstream service timeouts, load balancer timeouts, and client timeouts all interact.

**How to avoid:**
1. Establish timeout hierarchy:
   ```
   Client timeout (60s)
   ↳ Load balancer timeout (50s)
     ↳ Gateway request timeout (45s)
       ↳ Gateway upstream timeout (10s per request)
         ↳ Upstream service timeout (8s)
   ```
2. For DAG execution, calculate timeout budget:
   - Parallel nodes: `timeout = max(node_timeouts) + 500ms`
   - Sequential nodes: `timeout = sum(node_timeouts) + 500ms`
   - Reserve 10% of gateway timeout for processing overhead
3. Make timeouts configurable per-node in YAML:
   ```yaml
   nodes:
     - id: fast-service
       timeout: 2s
     - id: slow-analytics
       timeout: 10s
   ```
4. Implement deadline propagation: if request has 5s remaining, don't start 10s upstream call
5. Monitor timeout metrics: track how often timeouts fire vs actual failures
6. Default to 10s for upstream requests, 30s for gateway requests (reasonable starting point)

**Warning signs:**
- 504 errors when upstreams show successful responses
- Logs show "context deadline exceeded" but upstream logs show request completed
- Gateway p99 latency exactly matches timeout setting (requests being cut off artificially)
- Timeouts are round numbers (10s, 30s) without justification from actual latency measurements

**Phase to address:**
Phase 1 (Timeout infrastructure) - Implement context deadlines and per-node timeout configuration. Phase 3 (Timeout tuning) - Add deadline-aware execution and monitoring.

---

### Pitfall 8: Authentication Token Management Memory Leaks

**What goes wrong:**
OAuth2 token refresh logic caches tokens in-memory with no eviction policy. After running for days/weeks, memory usage grows to gigabytes as the gateway accumulates tokens for every upstream service and user combination. Alternatively: tokens are not cached at all, so every request triggers token refresh, overwhelming the auth provider and adding 200ms+ latency to every request.

**Why it happens:**
OAuth2 token management seems simple ("call token endpoint, cache result") but production scenarios are complex: tokens expire and need refresh, refresh tokens themselves can be revoked, token cache needs thread-safe concurrent access, multi-tenant scenarios mean potentially millions of cached tokens, and there's no built-in cache eviction in golang.org/x/oauth2.

**How to avoid:**
1. Use a bounded LRU cache for tokens with max size limit (e.g., 10,000 entries)
2. Implement token cache key strategy: `cacheKey = hash(upstream_id + auth_config)`
3. Set cache TTL to token expiration minus safety margin: `cache_ttl = token.ExpiresIn - 60s`
4. Handle token refresh failures gracefully:
   ```go
   // If cached token expired and refresh fails, try NEW token request
   // Don't let one stuck refresh block all requests
   ```
5. For client credentials grant (most common in service-to-service): cache per upstream service, not per user
6. For OAuth2 with user context: consider external cache (Redis) to share across gateway instances
7. Monitor cache hit rate, eviction rate, and cache memory usage
8. Add background goroutine to clean expired tokens every 5 minutes

**Warning signs:**
- Memory grows linearly with unique upstream/user combinations
- Auth provider rate limiting errors
- Token refresh happens on every request (cache not working)
- Spiky latency when tokens expire (thundering herd)
- `pprof` shows large maps in token cache structures

**Phase to address:**
Phase 2 (Auth implementation) - Implement basic token caching. Phase 3 (Cache management) - Add LRU eviction and expiry cleanup.

---

## Technical Debt Patterns

Shortcuts that seem reasonable but create long-term problems.

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Using `http.DefaultClient` instead of custom client | Fast to implement, no configuration needed | Connection pool exhaustion, can't tune performance, no per-upstream isolation | Only for low-traffic prototypes or non-critical internal tools |
| Retrying all 5xx errors | Higher success rate | Can cause retry storms, takes down struggling upstreams | Never acceptable - always use retry budget and exponential backoff |
| Global timeout instead of per-node timeouts | Simple configuration | Fast upstreams pay latency penalty, slow upstreams not given enough time | Acceptable for MVP, must fix by v1.1 |
| In-memory token cache without eviction | Simple implementation | Memory leaks, OOM crashes | Only for single-tenant scenarios with <100 unique tokens |
| No partial failure handling | Simpler error logic | Brittle system, poor user experience | Acceptable for Phase 1 MVP if documented, must fix before public release |
| Panic instead of error returns in DAG execution | Fails fast, easy to write | Crashes entire gateway on single bad node | Never acceptable - always use error returns |
| Stringifying all response bodies | Easy composition | Memory exhaustion with large responses, slow | Never acceptable - stream responses when possible |

## Integration Gotchas

Common mistakes when connecting to external services.

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| OAuth2 client credentials | Requesting new token on every request | Cache token until 60s before expiry, refresh in background |
| Header-based auth | Copying all request headers to upstream | Whitelist allowed headers, sanitize `Host`, `Authorization` |
| Basic auth | Storing plain-text credentials in config | Use environment variables or secret manager, never commit creds |
| Passthrough auth | Forwarding auth header to all upstreams | Configure per-upstream which auth to use, validate tokens |
| HTTP/2 upstreams | Creating new client per request | Reuse client to benefit from HTTP/2 connection multiplexing |
| Large JSON responses | Reading entire response into memory | Stream response body, use `json.Decoder` for partial parsing |
| Multipart responses | Buffering entire response before returning | Stream chunks as they arrive (harder with composition though) |
| gRPC upstreams | Using separate TCP connection per request | Use connection pooling with grpc.WithBlock() options |

## Performance Traps

Patterns that work at small scale but fail as usage grows.

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| JSON marshal/unmarshal for every composition | CPU usage >60%, slow p99 | Use gjson for reading values without full unmarshal | >100 req/s with 5+ upstreams |
| Copying large request/response bodies | Memory usage spikes, GC pressure | Stream bodies, reference original bytes | Bodies >1MB or >50 req/s |
| Synchronous logging on hot path | High p99 latency, inconsistent | Use buffered async logger | >500 req/s |
| Unbounded DAG parallelism | Port exhaustion, file descriptor limits | Limit concurrent goroutines with semaphore | >50 parallel nodes or >200 req/s |
| Global mutex for config access | Low throughput, high lock contention | Use atomic.Value or sync.Map for read-heavy config | >1000 req/s |
| Creating new Expr evaluator per request | CPU and memory waste | Pool evaluators with sync.Pool | >200 req/s |
| No request size limits | OOM from large requests | Enforce max body size (e.g., 10MB) with http.MaxBytesReader | First malicious/buggy client |
| Middleware that clones request context | Memory allocation overhead | Pass context by value, avoid cloning | >1000 req/s |

## Security Mistakes

Domain-specific security issues beyond general web security.

| Mistake | Risk | Prevention |
|---------|------|------------|
| Exposing upstream error details in gateway response | Information disclosure (internal IPs, service names, stack traces) | Scrub errors, return generic "upstream service unavailable" |
| No validation on YAML config expressions | Remote code execution via malicious config | Whitelist Expr functions, disable os/exec packages, validate configs |
| Forwarding all request headers upstream | Header injection, auth bypass | Whitelist allowed headers, drop sensitive ones like `X-Forwarded-For` |
| Trusting upstream SSL without verification | Man-in-the-middle attacks | Always verify certificates, pin expected CAs |
| No rate limiting per upstream | Gateway can DDoS upstreams | Per-upstream rate limit, backpressure when limit reached |
| Logging auth tokens | Credential exposure in logs | Sanitize logs, redact `Authorization`, `X-API-Key` headers |
| Evaluating expressions with request body data | Injection attacks | Only use request data as variables, not in expression AST |
| No timeout on Expr evaluation | DoS via expensive expressions | Set max execution time (100ms) for expression evaluation |
| Caching responses with auth tokens | Token leakage across users | Never cache responses containing auth headers |

## UX Pitfalls

Common user experience mistakes in this domain.

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Generic error messages ("internal server error") | Users don't know if retry will help or what went wrong | Return structured errors with error codes and retry guidance |
| No indication of partial failures | Users think feature is broken when only non-critical part failed | Include partial response flag and list of failed components |
| Timeout errors indistinguishable from failures | Users don't know if request succeeded but was slow | Return 504 for timeout vs 502 for failure, include timing info |
| No request tracing | Debugging failures requires correlating multiple logs | Return `X-Request-ID` header, log with request ID |
| Silent degradation | Users confused why feature is incomplete | Include `X-Degraded-Mode: true` header or response field |
| Retry logic visible to users | Users experience duplicate operations (e.g., double-charge) | Make gateway retries transparent, ensure upstream idempotency |
| No health check endpoint | Operations can't tell if gateway is healthy | Expose `/health` with upstream dependency status |
| Configuration errors only visible in logs | Users see cryptic failures after config changes | Validate config on startup, expose `/config-status` endpoint |

## "Looks Done But Isn't" Checklist

Things that appear complete but are missing critical pieces.

- [ ] **HTTP client setup:** Often missing - connection pool configuration (MaxIdleConnsPerHost) — verify load test shows connection reuse
- [ ] **Retry logic:** Often missing - exponential backoff and jitter — verify retries don't cause thundering herd
- [ ] **Context propagation:** Often missing - cancellation handling in parallel goroutines — verify cancelling request stops all upstreams
- [ ] **Error handling:** Often missing - distinction between retryable and non-retryable errors — verify 5xx doesn't always retry
- [ ] **OAuth2 token management:** Often missing - token cache eviction and refresh token handling — verify memory doesn't grow unbounded
- [ ] **Timeout configuration:** Often missing - timeout hierarchy and deadline propagation — verify 504s don't fire when upstreams succeed
- [ ] **Graceful degradation:** Often missing - partial response handling for optional dependencies — verify one upstream failure doesn't fail whole request
- [ ] **Expression security:** Often missing - function whitelisting and input sanitization — verify expressions can't execute arbitrary code
- [ ] **Response body handling:** Often missing - draining and closing response bodies — verify goroutine count stays stable under load
- [ ] **Logging:** Often missing - sanitization of auth headers and tokens — verify logs don't contain sensitive data
- [ ] **Health checks:** Often missing - deep health that checks upstream connectivity — verify health endpoint fails when upstreams are down
- [ ] **Configuration validation:** Often missing - startup validation and schema checking — verify invalid configs are rejected before serving traffic

## Recovery Strategies

When pitfalls occur despite prevention, how to recover.

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Goroutine leak | LOW | Deploy patched version with proper context cancellation; existing leaks clear on restart |
| Connection pool misconfiguration | LOW | Rolling restart with corrected Transport settings; immediate effect |
| Context propagation failures | MEDIUM | Add context to function signatures (breaking change); requires code changes throughout DAG executor |
| Retry storms | LOW | Deploy circuit breaker or reduce retry attempts; can hotfix by temporarily disabling retries |
| Expression injection vulnerability | HIGH | Audit all configs for malicious expressions, deploy patched Expr environment, rotate upstream credentials if compromised |
| Partial failure handling missing | MEDIUM | Add required/optional flags to configs, update DAG executor to collect partial results; requires design change |
| Timeout cascade failures | LOW | Recalculate timeout hierarchy, update configs, rolling restart; monitor for false timeouts |
| Token cache memory leak | LOW | Deploy with LRU cache, restart to clear memory; future growth prevented |
| Missing response body close | LOW | Add defer close statements; rolling restart clears existing leaks |
| Header injection vulnerability | MEDIUM | Add header whitelist, sanitize forwarded headers; may break clients sending non-standard headers |

## Pitfall-to-Phase Mapping

How roadmap phases should address these pitfalls.

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Goroutine leaks from HTTP clients | Phase 1: Core HTTP handling | Run goleak tests showing 0 leaked goroutines after 1000 requests |
| Connection pool misconfiguration | Phase 1: HTTP client setup | Load test shows connection reuse, MaxIdleConnsPerHost set to >10 |
| Context propagation failures | Phase 1: DAG executor | Cancellation test shows all goroutines stop within 100ms |
| Retry storms | Phase 1: Basic retry logic | Retry count per request is bounded, exponential backoff implemented |
| Expression injection | Phase 1: Expr integration | Security audit shows no dangerous functions exposed |
| Partial failure handling | Phase 2: Error handling | Test with one upstream down returns partial data for optional nodes |
| Timeout configuration | Phase 1: Timeout setup | Gateway timeout > sum of sequential node timeouts |
| Token cache memory leak | Phase 2: Auth implementation | Memory usage stable after 10k requests with 1k unique token combinations |
| No health checks | Phase 2: Observability | `/health` endpoint returns 503 when upstreams are down |
| Logging sensitive data | Phase 2: Logging | Audit logs shows Authorization headers are redacted |
| Missing config validation | Phase 1: Config loading | Invalid YAML rejected on startup, not at runtime |
| Unbounded parallelism | Phase 3: Performance optimization | Goroutine count bounded even with 100 parallel nodes |

## Sources

### Go HTTP Client and Connection Pooling
- [HTTP Resource Leak Mysteries in Go - Blog - Coder](https://coder.com/blog/go-leak-mysteries)
- [HTTP Connection Pooling in Go by David Bacisin](https://davidbacisin.com/writing/golang-http-connection-pools-1)
- [HTTP Connection churn in GO - DEV Community](https://dev.to/gkampitakis/http-connection-churn-in-go-34pl)
- [Solving Memory Leak Issues in Go HTTP Clients | by Chaewonkong | Medium](https://medium.com/@chaewonkong/solving-memory-leak-issues-in-go-http-clients-ba0b04574a83)
- [How to Use the HTTP Client in GO To Enhance Performance](https://www.loginradius.com/blog/engineering/tune-the-go-http-client-for-high-performance)
- [Avoiding Memory Leak in Golang API | by Iman Tumorang | Easyread](https://medium.com/hackernoon/avoiding-memory-leak-in-golang-api-1843ef45fca8)

### Goroutine Leaks and Context
- [net/http: using Transport.IdleConnTimeout may lead to goroutine leak in an edge case · Issue #25621 · golang/go](https://github.com/golang/go/issues/25621)
- [Understanding and Preventing Goroutine Leaks in Go | by SONU RAJ | Medium](https://medium.com/@srajsonu/understanding-and-preventing-goroutine-leaks-in-go-623cac542954)
- [How to Implement Request Context Propagation in Go Microservices](https://oneuptime.com/blog/post/2026-02-01-go-context-propagation-microservices/view)
- [Propagating an Inappropriate Context in Go: Pitfalls and Solutions | by José Paulo Zanardo Marciano | Medium](https://medium.com/@marcianojosepaulo/propagating-an-inappropriate-context-in-go-pitfalls-and-solutions-531b6cc692ad)

### API Gateway Patterns and Mistakes
- [10 Common API Development Mistakes and How to Avoid Them - DEV Community](https://dev.to/neelp03/10-common-api-development-mistakes-and-how-to-avoid-them-489o)
- [Common Mistakes in API Gateway and How to Avoid Them](https://www.syncloop.com/blogs/13-04-2025/common-mistakes-in-api-gateway-and-how-to-avoid-them.html)
- [6 Common API Gateway Monitoring Mistakes - API7.ai](https://api7.ai/blog/6-api-gateway-monitoring-mistakes)
- [Mastering High-Performance API Gateways in Go | by Sandeep | Medium](https://medium.com/@ksandeeptech07/mastering-high-performance-api-gateways-in-go-833310e8aeb4)

### Retry Patterns and Resilience
- [10 Common API Resilience Design Patterns with API Gateway - API7.ai](https://api7.ai/blog/10-common-api-resilience-design-patterns)
- [How to Configure Retries and Timeouts in Istio](https://oneuptime.com/blog/post/2026-01-07-istio-retries-timeouts/view)
- [HTTP Headers: Retry-After — Practical Patterns, Pitfalls, and Production-Ready Use – TheLinuxCode](https://thelinuxcode.com/http-headers-retry-after-practical-patterns-pitfalls-and-production-ready-use/)
- [Retry with backoff pattern - AWS Prescriptive Guidance](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/retry-backoff.html)

### Graceful Degradation and Error Handling
- [Best Practices of API Degradation in API Gateway - API7.ai](https://api7.ai/blog/degradation-in-api-gateway)
- [REL05-BP01 Implement graceful degradation - AWS Well-Architected Framework](https://docs.aws.amazon.com/wellarchitected/latest/framework/rel_mitigate_interaction_failure_graceful_degradation.html)
- [Error Handling in APIs: Crafting Meaningful Responses - API7.ai](https://api7.ai/learning-center/api-101/error-handling-apis)

### Expression Language Security
- [Expression Language Injection | OWASP Foundation](https://owasp.org/www-community/vulnerabilities/Expression_Language_Injection)
- [Resecurity | CVE-2025-68613: Remote Code Execution via Expression Injection in n8n](https://www.resecurity.com/blog/article/cve-2025-68613-remote-code-execution-via-expression-injection-in-n8n-2)
- [Expression Language Injection Stefano Di Paola, Minded Security](https://mindedsecurity.com/wp-content/uploads/2020/10/ExpressionLanguageInjection.pdf)
- [Expression Language (EL) Injection | by XploitByte | Medium](https://medium.com/@rafiul.pentester/expression-language-el-injection-fa6c1279fd9f)

### DAG Orchestration
- [Troubleshooting Apache Airflow: Optimizing DAG Scheduling, Parallelism, and Performance](https://www.mindfulchase.com/explore/troubleshooting-tips/troubleshooting-apache-airflow-optimizing-dag-scheduling,-parallelism,-and-performance.html)
- [Building a Custom DAG Orchestration System for Experimentation](https://www.geteppo.com/blog/building-a-custom-dag-orchestration-system-for-experimentation)

### OAuth2 and Authentication
- [oauth2 package - golang.org/x/oauth2 - Go Packages](https://pkg.go.dev/golang.org/x/oauth2)
- [Creating an OAuth2 Client in Golang (With Full Examples)](https://www.sohamkamani.com/golang/oauth/)
- [A Guide to Go's `x/oauth2` Package: OAuth2 Authentication | Reintech media](https://reintech.io/blog/guide-to-go-x-oauth2-package-oauth2-authentication)

### BFF Pattern
- [Backend for Frontend (BFF) Pattern: The Dos and Don'ts of the BFF Pattern | AKF Partners](https://akfpartners.com/growth-blog/backend-for-frontend)
- [Mastering the Backend for Frontend (BFF) Pattern: Origins, Implementation, and Real-World Banking Use Cases | by Bhuman Soni | Jan, 2026 | Medium](https://medium.com/@bhuman.soni/mastering-the-backend-for-frontend-bff-pattern-origins-implementation-and-real-world-banking-c30aa5141ea6)

### Configuration Validation
- [Common Mistakes in Kubernetes Gateway API and How to Avoid Them](https://www.syncloop.com/blogs/18-04-2025/common-mistakes-in-kubernetes-gateway-api-and-how-to-avoid-them.html)
- [Istio / Configuration Validation Problems](https://istio.io/latest/docs/ops/common-problems/validation/)

---
*Pitfalls research for: REST API Composition Gateway (Restitch)*
*Researched: 2026-02-03*
*Confidence: HIGH (verified with official Go docs, recent 2025-2026 articles, and OWASP security guidance)*
