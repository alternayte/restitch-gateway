# Phase 4: Error Handling & Resilience - Research

**Researched:** 2026-02-03
**Domain:** Go error handling, context timeouts, partial response patterns
**Confidence:** HIGH

## Summary

This phase implements error handling and resilience patterns for the composition engine, allowing optional steps to fail gracefully while continuing execution and returning partial responses. The research focused on Go's standard library patterns for timeout management, error aggregation, and graceful degradation.

Go's `context` package provides the foundation for timeout management at both global composition and per-step levels. The current codebase already uses `errgroup.WithContext` which provides fail-fast semantics, but this phase requires modifying that to allow optional steps to fail without canceling remaining steps. Go 1.20's `errors.Join` enables collecting multiple step failures into a structured `_errors` array.

For partial responses, HTTP 200 with custom headers (X-Partial-Response) is the recommended approach over HTTP 207 Multi-Status, which is primarily for WebDAV multi-resource operations. The codebase's existing error passthrough pattern (upstream HTTP errors don't fail execution) aligns well with the partial response requirements.

**Primary recommendation:** Build custom errgroup-like orchestration that collects errors instead of failing fast, use `context.WithTimeout` per step, aggregate failures with `errors.Join`, and return HTTP 200 + X-Partial-Response header for partial data scenarios.

## Standard Stack

The established libraries/tools for this domain:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| context | stdlib | Timeout/cancellation | Go's official mechanism for deadlines and cancellation |
| errors | stdlib (1.20+) | Error aggregation | Standard library errors.Join for combining multiple errors |
| net/http | stdlib | HTTP client | Already in use, supports per-request context timeouts |
| sync | stdlib | Synchronization | Mutex for collecting errors from goroutines |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| golang.org/x/sync/errgroup | latest | Concurrency orchestration | Reference pattern only - need custom version for optional errors |
| time | stdlib | Timeout durations | Configure timeout values (30s default per spec) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Custom error collection | go-multierror/hashicorp | Adds dependency, but standard errors.Join available since Go 1.20 |
| HTTP 200 + header | HTTP 207 Multi-Status | 207 is WebDAV-specific, less widely supported, breaks HTTP semantics for single-resource operations |
| Custom retry logic | failsafe-go library | Phase defers retries, premature to add complex library |

**Installation:**
```bash
# All core dependencies already in stdlib
# errgroup already in use: go get golang.org/x/sync/errgroup
```

## Architecture Patterns

### Recommended Project Structure
```
internal/composition/
├── executor.go          # Modified to handle optional steps
├── step.go              # Add timeout per step, error matching
├── errors.go            # NEW: Error collection and aggregation
├── response.go          # Add partial response headers/body
└── config.go            # Add optional field, timeout config, error rules
```

### Pattern 1: Context Timeout Per Step
**What:** Each step gets its own context with timeout, derived from parent composition context
**When to use:** Every step execution
**Example:**
```go
// Source: https://pkg.go.dev/context
func executeStepWithTimeout(parentCtx context.Context, step *CompiledStep, timeout time.Duration) (*StepResult, error) {
    // Derive step context from parent with its own timeout
    stepCtx, cancel := context.WithTimeout(parentCtx, timeout)
    defer cancel() // Always release resources

    // Create request with step context
    req, err := http.NewRequestWithContext(stepCtx, step.Method, url, body)
    if err != nil {
        return nil, err
    }

    resp, err := httpClient.Do(req)
    if err != nil {
        // Check if timeout occurred
        if errors.Is(err, context.DeadlineExceeded) {
            return nil, fmt.Errorf("step timeout after %v: %w", timeout, err)
        }
        return nil, err
    }
    return parseResponse(resp), nil
}
```

### Pattern 2: Optional Step Error Collection (Not Fail-Fast)
**What:** Replace `errgroup.WithContext` fail-fast with custom orchestration that collects errors from optional steps
**When to use:** Wave execution when steps have optional: true
**Example:**
```go
// Source: Modified from https://pkg.go.dev/golang.org/x/sync/errgroup
type stepError struct {
    stepName string
    err      error
}

func executeWaveWithOptional(ctx context.Context, wave []string, steps map[string]*CompiledStep) (map[string]*StepResult, []stepError) {
    var wg sync.WaitGroup
    results := make(map[string]*StepResult)
    var stepErrors []stepError
    var mu sync.Mutex

    for _, stepName := range wave {
        wg.Add(1)
        go func(name string) {
            defer wg.Done()

            result, err := executeStep(ctx, name, steps[name])

            mu.Lock()
            defer mu.Unlock()

            if err != nil {
                if steps[name].Optional {
                    // Collect error but continue
                    stepErrors = append(stepErrors, stepError{name, err})
                    results[name] = nil // Mark as failed
                } else {
                    // Required step failure - this needs to propagate
                    stepErrors = append(stepErrors, stepError{name, err})
                }
            } else {
                results[name] = result
            }
        }(stepName)
    }

    wg.Wait()
    return results, stepErrors
}
```

### Pattern 3: Error Aggregation with errors.Join
**What:** Combine multiple step errors into structured _errors array for response body
**When to use:** Building final response when some steps failed
**Example:**
```go
// Source: https://pkg.go.dev/errors (Go 1.20+)
type StepErrorDetail struct {
    Step    string `json:"step"`
    Message string `json:"message"`
}

func buildErrorsArray(stepErrors []stepError) []StepErrorDetail {
    if len(stepErrors) == 0 {
        return nil
    }

    details := make([]StepErrorDetail, len(stepErrors))
    for i, se := range stepErrors {
        // Extract user-friendly message (not full stack trace)
        msg := se.err.Error()
        if errors.Is(se.err, context.DeadlineExceeded) {
            msg = "timeout"
        }
        details[i] = StepErrorDetail{
            Step:    se.stepName,
            Message: msg,
        }
    }
    return details
}

// For logging/debugging, can use errors.Join
func aggregateErrorsForLog(stepErrors []stepError) error {
    if len(stepErrors) == 0 {
        return nil
    }
    errs := make([]error, len(stepErrors))
    for i, se := range stepErrors {
        errs[i] = fmt.Errorf("step %s: %w", se.stepName, se.err)
    }
    return errors.Join(errs...)
}
```

### Pattern 4: Partial Response with Header
**What:** Return HTTP 200 with X-Partial-Response header and _errors field in body
**When to use:** Any step failure (optional or matched error rule)
**Example:**
```go
// Source: Based on https://boldlygo.tech/posts/2024-01-08-error-handling/
func writePartialResponse(w http.ResponseWriter, body interface{}, stepErrors []StepErrorDetail) {
    if len(stepErrors) > 0 {
        w.Header().Set("X-Partial-Response", "true")

        // Inject _errors into response body
        if bodyMap, ok := body.(map[string]interface{}); ok {
            bodyMap["_errors"] = stepErrors
        }
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK) // Always 200 per CONTEXT.md
    json.NewEncoder(w).Encode(body)
}
```

### Pattern 5: Timeout Configuration Hierarchy
**What:** Global composition timeout with per-step overrides, falling back to defaults
**When to use:** Configuration parsing and step execution
**Example:**
```go
// Source: https://betterstack.com/community/guides/scaling-go/golang-timeouts/
type StepConfig struct {
    Name     string
    Upstream string
    Timeout  *time.Duration // nil means use upstream default
    Optional bool
}

type UpstreamConfig struct {
    URL     string
    Timeout time.Duration // Default for all steps using this upstream
}

func getStepTimeout(step *StepConfig, upstream *UpstreamConfig) time.Duration {
    if step.Timeout != nil {
        return *step.Timeout
    }
    if upstream.Timeout > 0 {
        return upstream.Timeout
    }
    return 30 * time.Second // Default per CONTEXT.md
}
```

### Pattern 6: Error Matching Rules
**What:** Match upstream status codes and replace response body
**When to use:** Step returns status code matching error rule
**Example:**
```go
type ErrorRule struct {
    Statuses []int       // List of status codes to match (e.g., [404, 410])
    Body     interface{} // Replacement body value
}

func matchErrorRule(statusCode int, rules []ErrorRule) (interface{}, bool) {
    for _, rule := range rules {
        for _, status := range rule.Statuses {
            if status == statusCode {
                return rule.Body, true
            }
        }
    }
    return nil, false
}

// In step execution:
result, err := executeStep(ctx, step)
if err == nil && step.ErrorRules != nil {
    if replacement, matched := matchErrorRule(result.Status, step.ErrorRules); matched {
        // Replace step result with configured body, treat as success
        result.Body = replacement
        // Add to _errors array for transparency
        stepErrors = append(stepErrors, stepError{step.Name,
            fmt.Errorf("error rule matched status %d", result.Status)})
    }
}
```

### Anti-Patterns to Avoid

- **Using errgroup.WithContext for optional steps:** WithContext cancels all goroutines on first error, which breaks optional step semantics. Need custom orchestration or zero Group without context.

- **HTTP 207 Multi-Status for single composition:** 207 is for WebDAV multi-resource operations. Gateway returns single composition result, not multiple independent resources. Use 200 + X-Partial-Response.

- **Not calling cancel() on context:** Memory leak. Always `defer cancel()` after creating context with timeout, even if you think operation will complete first.

- **Global timeout only:** Per-step timeouts critical for heterogeneous upstreams (slow analytics API vs fast cache). Must support both global composition timeout AND per-step overrides.

- **Returning full error stack traces:** The `_errors` array is for users, not debugging. Return sanitized messages like "timeout" or "upstream error", not internal stack traces.

- **Optional dependents not inheriting optional status:** If step A is optional and step B depends on A, B must be skipped (not fail) when A fails. Mark as "dependency_failed" in _errors.

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Error aggregation from goroutines | Custom slice + mutex pattern | errors.Join (Go 1.20+) | Thread-safe, structured unwrapping with errors.Is/As, standard library |
| Context timeout management | Custom timeout channels | context.WithTimeout | Standard library, integrates with http.Client, properly propagates cancellation |
| Connection pooling with timeouts | Custom HTTP client | http.Client with Transport config | Already using Phase 1 client, just add context to requests |
| Retry with backoff | Custom retry loops | (Deferred to future phase) | This phase explicitly avoids retries per CONTEXT.md "fail immediately on timeout" |

**Key insight:** Go's standard library provides all necessary primitives for timeout management and error aggregation. Third-party libraries like failsafe-go or go-multierror add complexity without benefit for this phase's scope.

## Common Pitfalls

### Pitfall 1: Timeout Context Not Propagated to HTTP Request
**What goes wrong:** Setting `http.Client.Timeout` but not using `http.NewRequestWithContext(ctx, ...)`, causing per-step timeouts to be ignored
**Why it happens:** Easy to forget to thread context through request creation
**How to avoid:** Always use `http.NewRequestWithContext` and pass step context, not parent context. The existing codebase already does this correctly (step.go:61).
**Warning signs:** Requests don't respect configured timeouts, hang longer than expected

### Pitfall 2: Race Condition in Error Collection
**What goes wrong:** Multiple goroutines appending to shared `[]error` slice without mutex causes data corruption or missing errors
**Why it happens:** Slices are not thread-safe for concurrent writes
**How to avoid:** Always protect error slice with sync.Mutex when collecting from goroutines (see Pattern 2 example)
**Warning signs:** Intermittent missing errors, test flakiness, race detector warnings

### Pitfall 3: Forgetting to Mark Dependents as Skipped
**What goes wrong:** When optional step A fails, dependent step B tries to execute and panics accessing nil `steps.A` in expression
**Why it happens:** Dependency graph doesn't account for optional step failures
**How to avoid:** Check dependency results before executing step, skip with "dependency_failed" message if any dependency is nil
**Warning signs:** Panic in expression evaluation, nil pointer dereference

### Pitfall 4: Using context.Background() Instead of Parent Context
**What goes wrong:** Per-step timeout context created from `context.Background()` instead of parent composition context, breaking timeout hierarchy
**Why it happens:** Convenience of not threading context through functions
**How to avoid:** Always derive step context from parent: `context.WithTimeout(parentCtx, stepTimeout)`. This ensures if parent times out, all steps are canceled.
**Warning signs:** Steps don't respect global composition timeout, continue after composition deadline

### Pitfall 5: Error Rule Matching Without Adding to _errors
**What goes wrong:** Error rule matches and replaces body, but failure not recorded in `_errors` array, hiding that upstream failed
**Why it happens:** Treating error rule match as "fixed" rather than "handled"
**How to avoid:** Always add matched error rule to `_errors` array with step name and "error rule matched" message for transparency
**Warning signs:** Client thinks all steps succeeded when actually some failed and were masked

### Pitfall 6: Not Distinguishing Timeout from Other Context Errors
**What goes wrong:** All context errors reported as generic "context canceled", making timeout debugging hard
**Why it happens:** Not checking `errors.Is(err, context.DeadlineExceeded)` specifically
**How to avoid:** Check for DeadlineExceeded specifically and report as "timeout" in `_errors` array, per CONTEXT.md spec
**Warning signs:** Unclear error messages, hard to distinguish intentional cancellation from timeout

### Pitfall 7: Timeout Values Don't Account for Upstream Variability
**What goes wrong:** Setting same timeout for all steps (e.g., 30s) causes fast services to wait unnecessarily or slow services to timeout too early
**Why it happens:** One-size-fits-all configuration
**How to avoid:** Support timeout hierarchy: per-step override > upstream default > global default (30s). Configure based on actual upstream SLAs.
**Warning signs:** Frequent timeouts on slow but working services, or unnecessarily long waits for failed fast services

## Code Examples

Verified patterns from official sources:

### Timeout Configuration and Execution
```go
// Source: https://pkg.go.dev/context + https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
func ExecuteStepWithTimeout(
    parentCtx context.Context,
    step *CompiledStep,
    upstream *CompiledUpstream,
    env map[string]interface{},
    httpClient *http.Client,
) (*StepResult, error) {
    // Get timeout for this step (hierarchy: step > upstream > default)
    timeout := getStepTimeout(step, upstream)

    // Create step context with timeout derived from parent
    stepCtx, cancel := context.WithTimeout(parentCtx, timeout)
    defer cancel() // CRITICAL: Always release resources

    // Build request (existing logic)
    path, err := evaluatePath(step.PathExpr, env)
    if err != nil {
        return nil, fmt.Errorf("failed to evaluate path: %w", err)
    }

    url := buildURL(upstream.URL, path)

    // Create request with step context (not parent context!)
    req, err := http.NewRequestWithContext(stepCtx, step.Method, url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    // Execute request
    resp, err := httpClient.Do(req)
    if err != nil {
        // Check for timeout specifically
        if errors.Is(err, context.DeadlineExceeded) {
            return nil, fmt.Errorf("timeout after %v: %w", timeout, err)
        }
        return nil, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()

    return parseResponse(resp), nil
}
```

### Optional Step Orchestration Without Fail-Fast
```go
// Source: Modified from https://pkg.go.dev/golang.org/x/sync/errgroup
type WaveResult struct {
    StepResults map[string]*StepResult
    StepErrors  []StepErrorDetail
}

func executeWave(
    ctx context.Context,
    wave []string,
    composition *CompiledComposition,
    env map[string]interface{},
) (*WaveResult, error) {
    var wg sync.WaitGroup
    results := make(map[string]*StepResult)
    var errors []StepErrorDetail
    var mu sync.Mutex

    // Track if any required step failed
    var requiredStepFailed atomic.Bool

    for _, stepName := range wave {
        wg.Add(1)
        go func(name string) {
            defer wg.Done()

            step := composition.Steps[name]

            // Check if dependencies failed
            mu.Lock()
            depFailed := checkDependenciesFailed(step, results)
            mu.Unlock()

            if depFailed {
                mu.Lock()
                errors = append(errors, StepErrorDetail{
                    Step:    name,
                    Message: "dependency_failed",
                })
                results[name] = nil
                mu.Unlock()
                return
            }

            // Execute step
            result, err := ExecuteStepWithTimeout(ctx, step, env)

            mu.Lock()
            defer mu.Unlock()

            if err != nil {
                if step.Optional {
                    // Optional step failed - collect error, mark as nil, continue
                    errors = append(errors, StepErrorDetail{
                        Step:    name,
                        Message: sanitizeErrorMessage(err),
                    })
                    results[name] = nil
                } else {
                    // Required step failed - collect error and signal failure
                    errors = append(errors, StepErrorDetail{
                        Step:    name,
                        Message: sanitizeErrorMessage(err),
                    })
                    requiredStepFailed.Store(true)
                }
            } else {
                results[name] = result
            }
        }(stepName)
    }

    wg.Wait()

    // If any required step failed, return error (fail composition)
    if requiredStepFailed.Load() {
        return nil, fmt.Errorf("required step(s) failed")
    }

    return &WaveResult{
        StepResults: results,
        StepErrors:  errors,
    }, nil
}

func checkDependenciesFailed(step *CompiledStep, results map[string]*StepResult) bool {
    for _, depName := range step.DependsOn {
        if result, exists := results[depName]; !exists || result == nil {
            return true
        }
    }
    return false
}

func sanitizeErrorMessage(err error) string {
    if errors.Is(err, context.DeadlineExceeded) {
        return "timeout"
    }
    // Don't expose internal details
    return "upstream error"
}
```

### Building Partial Response
```go
// Source: Based on https://boldlygo.tech/posts/2024-01-08-error-handling/
func BuildPartialResponse(
    template ResponseTemplate,
    stepResults map[string]*StepResult,
    stepErrors []StepErrorDetail,
    env map[string]interface{},
) (int, http.Header, interface{}) {
    // Build response body from template (existing logic)
    body := evaluateResponseBody(template, stepResults, env)

    // Inject _errors array if any step failed
    if len(stepErrors) > 0 {
        if bodyMap, ok := body.(map[string]interface{}); ok {
            bodyMap["_errors"] = stepErrors
        }
    }

    // Build headers
    headers := http.Header{}
    headers.Set("Content-Type", "application/json")

    // Set X-Partial-Response header if any errors occurred
    if len(stepErrors) > 0 {
        headers.Set("X-Partial-Response", "true")
    }

    // Always return 200 per CONTEXT.md (composition succeeded, partial data valid)
    return http.StatusOK, headers, body
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| errgroup first-error semantics | errors.Join for collecting all | Go 1.20 (Feb 2023) | Can now aggregate multiple errors in stdlib without third-party libs |
| Request.Cancel channel | context.Context cancellation | Go 1.7 (Aug 2016) | Context is now the standard, Request.Cancel deprecated |
| go-multierror/hashicorp | errors.Join stdlib | Go 1.20 (Feb 2023) | No longer need external dependency for error aggregation |
| HTTP 207 for partial responses | HTTP 200 + custom header | Ongoing shift | 207 recognized as WebDAV-specific, not RESTful for single-resource APIs |

**Deprecated/outdated:**
- `Request.Cancel` channel: Replaced by `context.Context` in Go 1.7+. The codebase correctly uses context everywhere.
- Third-party multierror packages for simple aggregation: `errors.Join` available since Go 1.20 provides equivalent functionality in stdlib

## Open Questions

Things that couldn't be fully resolved:

1. **Error rule syntax: status list vs ranges vs wildcards**
   - What we know: CONTEXT.md shows status code list is simplest ([404, 410])
   - What's unclear: Whether to support ranges (400-499) or wildcards (4xx) in future
   - Recommendation: Start with explicit list only (simplest, most explicit). Add ranges in future phase if needed.

2. **Whether dependents of optional steps inherit optional status**
   - What we know: CONTEXT.md says "If a step with dependents fails, dependent steps are skipped and marked as `dependency_failed`"
   - What's unclear: If step A is optional and step B depends on A, when A fails, should B be marked "dependency_failed" (treating A failure as acceptable) or should the entire composition fail because B couldn't run?
   - Recommendation: Treat as cascade skip - B marked "dependency_failed" when optional A fails. This preserves "optional" semantics (A failure doesn't fail composition).

3. **Global composition timeout default value**
   - What we know: Per-step default is 30s (CONTEXT.md), but global composition timeout not specified
   - What's unclear: Should global timeout be sum of step timeouts, fixed value, or derived heuristic?
   - Recommendation: Start without global composition timeout (nil = no limit). Steps control their own timeouts. Add global timeout in future phase if needed.

## Sources

### Primary (HIGH confidence)
- [context package - Go Packages](https://pkg.go.dev/context) - Official Go context documentation
- [errors package - Go Packages](https://pkg.go.dev/errors) - Official errors.Join documentation (Go 1.20+)
- [errgroup package - Go Packages](https://pkg.go.dev/golang.org/x/sync/errgroup) - Official errgroup patterns
- [The complete guide to Go net/http timeouts - Cloudflare](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/) - Comprehensive timeout configuration guide

### Secondary (MEDIUM confidence)
- [Timeouts in Go: A Comprehensive Guide - Better Stack](https://betterstack.com/community/guides/scaling-go/golang-timeouts/) - Best practices verified against official docs
- [Error handling in Go web apps - Boldly Go](https://boldlygo.tech/posts/2024-01-08-error-handling/) - Error handling patterns verified
- [How to Use errgroup for Parallel Operations in Go](https://oneuptime.com/blog/post/2026-01-07-go-errgroup/view) - Recent 2026 errgroup patterns
- [207 Multi-Status - MDN](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Status/207) - HTTP 207 specification

### Tertiary (LOW confidence)
- [HTTP Status Codes - Partial FAILURES discussion](https://groups.google.com/g/api-craft/c/86i25AMXM7c) - Community discussion on partial responses
- Various Go resilience library documentation (failsafe-go, gobreaker, go-resiliency) - Deferred for future phases

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All stdlib packages with official documentation
- Architecture: HIGH - Patterns verified against official Go docs and existing codebase
- Pitfalls: HIGH - Based on official documentation warnings and common Go concurrency issues

**Research date:** 2026-02-03
**Valid until:** 2026-03-03 (30 days - Go stdlib stable, patterns unlikely to change)
