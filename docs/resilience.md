# Resilience

Restitch provides four resilience layers — retries, circuit breakers, request
coalescing, and response caching. Each is configured per upstream or per step
and only active when explicitly enabled.

## RoundTripper Chain

Layers compose as a decorator chain around each upstream's HTTP transport:

```
request → metrics → retry → circuit breaker → auth → base transport → upstream
```

Each layer is skipped when unconfigured (zero overhead).

---

## Retries

Status-code-driven retry policy with exponential backoff and jitter.

### Configuration

Per upstream (applies to all steps using that upstream):

```yaml
upstreams:
  users-api:
    url: "https://users.internal"
    retry:
      max_attempts: 3           # total attempts including the first
      interval: 250ms           # initial backoff
      max_backoff: 5s           # backoff cap
      backoff_on: [429, 502, 503, 504]   # retry on these status codes
      drop_on: []               # give up immediately on these
      retry_non_idempotent: false        # retry POST/PATCH only if true
```

Per step (overrides the upstream retry config for that step):

```yaml
steps:
  - name: create-order
    upstream: orders-api
    path: "/orders"
    method: POST
    retry:
      max_attempts: 2
      retry_non_idempotent: true
```

### Defaults

When a `retry:` block is present but fields are omitted:

| Field | Default |
|-------|---------|
| `max_attempts` | 3 |
| `interval` | 250ms |
| `max_backoff` | 5s |
| `backoff_on` | [429, 502, 503, 504] |
| `drop_on` | [] |
| `retry_non_idempotent` | false |

### Behavior

1. **First attempt** executes normally.
2. If the response status is in `drop_on` → return immediately (no retry).
3. If the response status is in `backoff_on`, or a network error occurs →
   retryable.
4. Non-idempotent methods (POST, PATCH, DELETE) are retried **only** if
   `retry_non_idempotent: true`.
5. Before each retry:
   - Drain and close the previous response body (for connection reuse).
   - Sleep `min(interval × 2^(attempt-1), max_backoff)` with ±20% jitter.
   - If the response had a `Retry-After` header (seconds integer or HTTP
     date), sleep that duration instead, capped at `max_backoff`.
   - Abort immediately if the request context is canceled.
6. After `max_attempts` total attempts, return the last response or error.

---

## Circuit Breaker

Per-upstream circuit breaker using consecutive failure counting. Powered by
[sony/gobreaker](https://github.com/sony/gobreaker).

### Configuration

```yaml
upstreams:
  flaky-api:
    url: "https://flaky.internal"
    circuit_breaker:
      max_failures: 5     # consecutive failures to trip
      interval: 60s       # rolling counter reset period
      timeout: 10s        # open → half-open after this duration
```

### States

| State | Behavior |
|-------|----------|
| **Closed** | Requests pass through normally. Consecutive failures are counted. |
| **Open** | All requests fail immediately with 502 (no upstream call). |
| **Half-open** | After `timeout`, one probe request is allowed through. Success → closed. Failure → open. |

### Failure Counting

- **Transport errors** (network failures, DNS errors) count as failures.
- **HTTP 5xx responses** count as failures (the response is still returned to
  the caller — only the breaker counter increments).
- **Client-canceled contexts** (`context.Canceled`) do **not** count as
  failures. This prevents client disconnects from tripping the breaker.
- **HTTP 4xx and other responses** do not count as failures.

The breaker trips when `max_failures` consecutive failures occur. The counter
resets every `interval`.

### Position in the Chain

The breaker sits **inside** the retry layer: `retry → breaker → auth`. A
retry attempt against an open breaker fails fast without calling the upstream,
and the retry layer handles it as a retryable network error.

---

## Request Coalescing

In-flight request deduplication using singleflight. When multiple concurrent
requests to the same composition trigger the same upstream call, only one
HTTP request is made and the result is shared.

### Configuration

Per step:

```yaml
steps:
  - name: catalog
    upstream: catalog-api
    path: "/products/{{ req.params.id }}"
    coalesce: true     # GET only
```

### Behavior

- **GET only** — coalescing is only applied when the step method is GET.
  Non-GET steps with `coalesce: true` are ignored.
- **Key construction** — the dedup key is
  `method + URL + sha256(authorization)[:16]`. The authorization hash
  prevents cross-user data leaks through coalescing.
- **Shared results** — step results are value types. The same result is
  returned to all coalesced callers.
- **Cache interaction** — coalescing is checked **after** the cache. If the
  cache has a hit, coalescing is skipped.

---

## Response Caching

Per-step, in-memory, TTL-based response caching.

### Configuration

Per step:

```yaml
steps:
  - name: user-profile
    upstream: users-api
    path: "/users/{{ req.params.id }}"
    cache:
      ttl: 30s
```

### Behavior

- **GET only** — cache is only applied to GET steps. Configuring `cache` on
  a non-GET step is a validation error.
- **TTL-based expiry** — cached responses expire after the configured `ttl`.
  A background janitor sweeps expired entries every 30 seconds.
- **Key construction** — same as coalescing:
  `method + URL + sha256(authorization)[:16]`.
- **What is cached** — responses with status < 500 that did not match an
  error rule.
- **Cache bypass** — responses with status ≥ 500 or error-rule matches are
  never cached.
- **In-memory only** — there is no distributed cache. For shared caching
  across instances, use an external CDN or cache layer.

### Execution Order

```
request → cache check → coalesce → execute step → cache fill → response
```

A cache hit skips coalescing and upstream execution entirely.

---

## Rate Limiting

Token-bucket rate limiting at two levels: per-composition and global gateway.

### Per-Composition

```yaml
compositions:
  user-api:
    path: "/api/users/{id}"
    method: GET
    rate_limit:
      requests_per_second: 100
      burst: 10
      key: "ip"               # ip | header:<name> | api-key
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `requests_per_second` | float | — | Sustained request rate |
| `burst` | int | — | Maximum burst size above the sustained rate |
| `key` | string | `"ip"` | Rate limit key: `ip` (client IP), `header:<name>` (value of the named request header), or `api-key` (the `X-API-Key` value) |

### Global Gateway

Applied before composition dispatch. Configured under `server.rate_limit`
with the same fields as above:

```yaml
server:
  rate_limit:
    requests_per_second: 1000
    burst: 50
    key: "ip"
```

When both global and per-composition limits are configured, a request must
pass both.

### Response

Requests that exceed the rate limit receive:

- HTTP **429 Too Many Requests**
- `Retry-After: 1` header
- Body: `{"error":"rate limit exceeded"}`

### Admin API Rate Limiting

Admin mutation endpoints (`POST /admin/api/reload`, `POST /admin/api/validate`)
are rate-limited at 10 requests per second per IP with a burst of 5. This is
always active and not configurable.

---

## Configuration Examples

### Full resilience setup

```yaml
upstreams:
  backend:
    url: "https://backend.internal"
    timeout: 10s
    retry:
      max_attempts: 3
      backoff_on: [502, 503, 504]
    circuit_breaker:
      max_failures: 5
      timeout: 10s

compositions:
  dashboard:
    path: "/api/dashboard"
    steps:
      - name: data
        upstream: backend
        path: "/data"
        coalesce: true
        cache:
          ttl: 30s
    response:
      body:
        data: "{{ steps.data.body }}"
```

### Per-step retry override

```yaml
upstreams:
  api:
    url: "https://api.internal"
    retry:
      max_attempts: 3
      backoff_on: [503]

compositions:
  create:
    path: "/api/create"
    method: POST
    steps:
      - name: submit
        upstream: api
        path: "/submit"
        method: POST
        retry:
          max_attempts: 2
          retry_non_idempotent: true    # override upstream default
    response:
      body: "{{ steps.submit.body }}"
```
