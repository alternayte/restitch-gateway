# Restitch

A REST API composition gateway that eliminates hand-written Backend-for-Frontend (BFF) layers. Declaratively compose multiple REST API endpoints into unified responses using YAML configuration — no backend code changes required.

Think Apollo Router + GraphQL Hive, but for REST APIs.

## Why Restitch?

**The Problem:** Frontend teams need data from multiple backend services. The typical solution is writing BFF services — glue code wrapping fetch calls, each requiring its own repo, deployment, CI pipeline, monitoring, and on-call rotation.

**The Solution:** Restitch lets you define API compositions in YAML. The gateway handles parallel execution, dependency resolution, authentication, error handling, and response merging automatically.

```yaml
# Before: 200+ lines of BFF code
# After: 15 lines of YAML

compositions:
  user-dashboard:
    path: "/api/dashboard"
    method: GET
    steps:
      - name: user
        upstream: users-api
        path: "/users/{{ request.query.id }}"
      - name: orders
        upstream: orders-api
        path: "/orders?userId={{ steps.user.body.id }}"
      - name: loyalty
        upstream: loyalty-api
        path: "/points/{{ steps.user.body.id }}"
    response:
      body:
        user: "{{ steps.user.body }}"
        recent_orders: "{{ steps.orders.body[:5] }}"
        loyalty_points: "{{ steps.loyalty.body.points }}"
```

## Features

- **DAG-based parallel execution** — Independent steps run concurrently
- **Expression language** — Dynamic paths, params, and response shaping with [Expr](https://expr-lang.org/)
- **Four auth strategies** — Header, Basic, Passthrough, OAuth2 client credentials
- **Graceful degradation** — Optional steps, partial responses, error rules
- **Structured observability** — Request IDs, JSON logging, per-step timing
- **Production ready** — TLS termination, health checks, graceful shutdown

## Quick Start

### Build

```bash
go build -o restitch ./cmd/restitch
```

### Configure

Create `restitch.yaml`:

```yaml
upstreams:
  jsonplaceholder:
    url: "https://jsonplaceholder.typicode.com"

compositions:
  user-posts:
    path: "/api/user-posts"
    method: GET
    steps:
      - name: user
        upstream: jsonplaceholder
        path: "/users/{{ request.query.id }}"
      - name: posts
        upstream: jsonplaceholder
        path: "/posts?userId={{ steps.user.body.id }}"
    response:
      body:
        user: "{{ steps.user.body }}"
        posts: "{{ steps.posts.body }}"
```

### Run

```bash
./restitch --config restitch.yaml
# restitch vdev listening on :8080 (HTTP only, no TLS certificate provided)
```

### Test

```bash
curl "http://localhost:8080/api/user-posts?id=1" | jq .
```

## Configuration Reference

### Upstreams

Define backend services that steps can call:

```yaml
upstreams:
  users-api:
    url: "https://api.example.com"
    timeout: 10s                    # Default timeout for steps (optional, default: 30s)
    health_path: "/health"          # Health check path (optional, default: HEAD to base URL)
    auth:                           # Authentication (optional)
      header:
        name: "X-API-Key"
        value: "${API_KEY}"         # Environment variable expansion
```

### Authentication Strategies

#### Header Auth (API Keys)

```yaml
upstreams:
  api:
    url: "https://api.example.com"
    auth:
      header:
        name: "X-API-Key"
        value: "${API_KEY}"
```

#### Basic Auth

```yaml
upstreams:
  api:
    url: "https://api.example.com"
    auth:
      basic:
        username: "${SERVICE_USER}"
        password: "${SERVICE_PASS}"
```

#### Passthrough Auth

Forwards the client's `Authorization` header to the upstream. Returns 401 if the client doesn't provide one.

```yaml
upstreams:
  api:
    url: "https://api.example.com"
    auth:
      passthrough: {}
```

#### OAuth2 Client Credentials

Automatically fetches and refreshes tokens:

```yaml
upstreams:
  api:
    url: "https://api.example.com"
    auth:
      oauth2:
        token_url: "https://auth.example.com/oauth/token"
        client_id: "${OAUTH_CLIENT_ID}"
        client_secret: "${OAUTH_CLIENT_SECRET}"
        scopes:
          - read:users
          - read:orders
```

### Compositions

Define API endpoints that compose multiple upstream calls:

```yaml
compositions:
  user-dashboard:
    path: "/api/dashboard"          # Route path
    method: GET                     # HTTP method (default: GET)
    steps:
      - name: user                  # Step name (referenced in expressions)
        upstream: users-api         # Which upstream to call
        path: "/users/{{ request.query.id }}"
        method: GET                 # Step method (default: GET)
        headers:                    # Additional headers (optional)
          X-Correlation-ID: "{{ request.headers['X-Request-ID'] }}"
        optional: false             # If true, failure doesn't fail composition
        timeout: 5s                 # Override upstream timeout (optional)
        error_rules:                # Replace body on specific status codes
          - statuses: [404]
            body: null
    response:
      status: 200
      body:
        user: "{{ steps.user.body }}"
```

### Step Dependencies

Dependencies are inferred automatically from expressions:

```yaml
steps:
  - name: user
    upstream: users-api
    path: "/users/{{ request.query.id }}"

  - name: orders
    upstream: orders-api
    # This step depends on 'user' (inferred from expression)
    path: "/orders?userId={{ steps.user.body.id }}"

  - name: products
    upstream: products-api
    # Independent of user/orders — runs in parallel with 'user'
    path: "/products/featured"
```

Execution order:
```
Wave 1: user, products (parallel)
Wave 2: orders (waits for user)
```

You can also declare explicit dependencies:

```yaml
steps:
  - name: audit
    upstream: audit-api
    path: "/log"
    depends_on: [user, orders]  # Explicit dependency
```

### Expression Language

Restitch uses [Expr](https://expr-lang.org/) for dynamic values. Expressions are wrapped in `{{ }}`.

#### Available Variables

| Variable | Description |
|----------|-------------|
| `request.path` | Request URL path |
| `request.query` | Query parameters (map) |
| `request.headers` | Request headers (map) |
| `steps.<name>.body` | Parsed JSON body from step |
| `steps.<name>.status` | HTTP status code from step |
| `steps.<name>.headers` | Response headers from step |

#### Example Expressions

```yaml
# Access query parameter
path: "/users/{{ request.query.id }}"

# Access nested JSON field
path: "/orders?userId={{ steps.user.body.data.id }}"

# Array indexing
body:
  latest_order: "{{ steps.orders.body[0] }}"

# Array slicing
body:
  recent: "{{ steps.orders.body[:5] }}"

# Built-in functions
body:
  count: "{{ len(steps.orders.body) }}"
  has_orders: "{{ len(steps.orders.body) > 0 }}"

# Conditional (ternary)
body:
  status: "{{ steps.user.body.active ? 'Active' : 'Inactive' }}"

# Null coalescing for optional steps
body:
  loyalty: "{{ steps.loyalty.body ?? {} }}"
```

### Error Handling

#### Optional Steps

Mark steps as optional to continue on failure:

```yaml
steps:
  - name: recommendations
    upstream: ml-api
    path: "/recommend/{{ steps.user.body.id }}"
    optional: true  # Composition continues if this fails
```

When optional steps fail:
- Response includes `X-Partial-Response: true` header
- Response body includes `_errors` array with failure details
- Failed step results are `null` in expressions

#### Error Rules

Replace response body when upstream returns specific status codes:

```yaml
steps:
  - name: user
    upstream: users-api
    path: "/users/{{ request.query.id }}"
    error_rules:
      - statuses: [404, 410]
        body: null              # Replace with null
      - statuses: [403]
        body:
          error: "forbidden"
          default_user: true
```

#### Timeouts

```yaml
upstreams:
  slow-api:
    url: "https://slow.example.com"
    timeout: 30s  # Upstream default

compositions:
  example:
    steps:
      - name: fast
        upstream: slow-api
        path: "/quick"
        timeout: 2s  # Override for this step
```

Timeout hierarchy: Step timeout > Upstream timeout > Default (30s)

## CLI Options

```
Usage: restitch [options]

Options:
  -config string
        Path to composition config file (default "restitch.yaml")
  -port int
        HTTP server port (default 8080)
  -tls-port int
        HTTPS server port (default 8443)
  -cert string
        Path to TLS certificate file
  -key string
        Path to TLS private key file
  -log-format string
        Log format: json or text (default "json")
```

### TLS Example

```bash
./restitch \
  --config restitch.yaml \
  --cert server.crt \
  --key server.key \
  --port 8080 \
  --tls-port 8443
```

## Health Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Liveness check — returns gateway status, uptime, version |
| `GET /ready` | Readiness check — returns 200 when ready, 503 during shutdown |
| `GET /health/upstreams` | Upstream connectivity status with latency |

### Example Responses

```bash
# Liveness
curl http://localhost:8080/health
{
  "status": "healthy",
  "version": "dev",
  "uptime": "5m32s",
  "memory_mb": 12
}

# Upstream health
curl http://localhost:8080/health/upstreams
{
  "status": "healthy",
  "upstreams": {
    "users-api": {
      "status": "healthy",
      "url": "https://api.example.com",
      "latency_ms": 45,
      "last_check": "2026-02-03T12:00:00Z"
    }
  }
}
```

## Observability

### Structured Logging

All requests are logged as JSON with consistent fields:

```json
{
  "time": "2026-02-03T12:00:00.000Z",
  "level": "INFO",
  "msg": "request complete",
  "request_id": "01HQXK5V2NZJK3M8W9P4R6T7Y",
  "method": "GET",
  "path": "/api/dashboard",
  "status_code": 200,
  "duration_ms": 127,
  "remote_addr": "192.168.1.100:54321"
}
```

### Request Tracing

Pass `X-Request-ID` header to trace requests:

```bash
curl -H "X-Request-ID: my-trace-123" http://localhost:8080/api/dashboard
# Response includes: X-Request-ID: my-trace-123
# All logs include: "request_id": "my-trace-123"
```

If not provided, Restitch generates a ULID.

### Step Timing

Composition completion logs include per-step timing:

```json
{
  "msg": "composition complete",
  "request_id": "01HQXK5V2NZJK3M8W9P4R6T7Y",
  "composition": "user-dashboard",
  "duration_ms": 127,
  "step_timings": {
    "user": {"wave": 1, "duration_ms": 45, "status": "success"},
    "orders": {"wave": 2, "duration_ms": 82, "status": "success"},
    "loyalty": {"wave": 2, "duration_ms": 31, "status": "success"}
  },
  "slowest_step": "orders"
}
```

## Example: Complete Configuration

```yaml
upstreams:
  users:
    url: "https://users.internal.example.com"
    timeout: 10s
    health_path: "/health"
    auth:
      header:
        name: "X-Internal-Token"
        value: "${INTERNAL_TOKEN}"

  orders:
    url: "https://orders.internal.example.com"
    timeout: 15s
    auth:
      oauth2:
        token_url: "https://auth.example.com/token"
        client_id: "${ORDERS_CLIENT_ID}"
        client_secret: "${ORDERS_CLIENT_SECRET}"
        scopes: [read:orders]

  recommendations:
    url: "https://ml.example.com"
    timeout: 5s
    auth:
      passthrough: {}

compositions:
  user-dashboard:
    path: "/api/v1/dashboard"
    method: GET
    steps:
      - name: user
        upstream: users
        path: "/users/{{ request.query.userId }}"
        error_rules:
          - statuses: [404]
            body: null

      - name: orders
        upstream: orders
        path: "/users/{{ steps.user.body.id }}/orders?limit=10"
        timeout: 5s

      - name: recommendations
        upstream: recommendations
        path: "/recommend?userId={{ steps.user.body.id }}"
        optional: true
        timeout: 2s

    response:
      status: 200
      body:
        user:
          id: "{{ steps.user.body.id }}"
          name: "{{ steps.user.body.name }}"
          email: "{{ steps.user.body.email }}"
        orders: "{{ steps.orders.body.items }}"
        order_count: "{{ len(steps.orders.body.items) }}"
        recommendations: "{{ steps.recommendations.body ?? [] }}"
```

## Graceful Shutdown

Restitch handles SIGTERM/SIGINT gracefully:

1. `/ready` immediately returns 503 (stop receiving new traffic)
2. In-flight requests complete (up to 30s timeout)
3. Process exits cleanly

This integrates with Kubernetes, ECS, and other orchestrators.

## Roadmap

### v2 (Planned)

**Caching**
- Response caching with configurable TTL per step
- Cache key derivation from request parameters
- Cache invalidation via API

**Advanced Resilience**
- Circuit breaker per upstream service
- Request coalescing for duplicate in-flight requests
- Retry policies with exponential backoff

**Configuration**
- Hot-reload configuration without restart
- Configuration validation with helpful error messages
- OpenAPI spec import to auto-generate upstream definitions

**Studio (Control Plane)**
- Web UI for viewing and editing compositions
- Visual DAG editor
- Latency waterfall visualization
- Configuration audit log with rollback
- Real-time metrics dashboard

### Out of Scope

| Feature | Reason |
|---------|--------|
| WASM plugins | Complexity; hardcode what's needed |
| Inbound JWT validation | Use API gateway or service mesh |
| GraphQL support | GraphQL has Apollo Federation |
| Rate limiting | Use upstream rate limits or external solution |
| Load balancing | Use service mesh |
| WebSocket support | REST-only |

## Development

### Build

```bash
go build ./cmd/restitch
```

### Test

```bash
go test ./...
```

### Project Structure

```
.
├── cmd/restitch/          # Application entrypoint
├── internal/
│   ├── auth/              # Authentication strategies
│   ├── client/            # HTTP client with connection pooling
│   ├── composition/       # Config parsing, DAG execution, response building
│   ├── config/            # Environment variable expansion
│   ├── observability/     # Request ID middleware
│   └── server/            # HTTP server, routing, health endpoints
└── restitch.yaml          # Example configuration
```

## License

[TBD]
