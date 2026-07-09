# Restitch

A production-grade REST API composition gateway. Declare multi-step API
compositions in YAML — Restitch executes the DAG of upstream calls in
parallel, wires results together with expressions, and returns a single
merged response. No BFF code required.

## Quick Start

```bash
# Build
make build

# Start the mock upstream (ships with the repo)
go run ./cmd/mockupstream &

# Write a config
cat > restitch.yaml <<'EOF'
upstreams:
  mock:
    url: "http://localhost:8081"

compositions:
  user-posts:
    path: "/api/user-posts"
    method: GET
    steps:
      - name: user
        upstream: mock
        path: "/users/{{ req.query.id }}"
      - name: orders
        upstream: mock
        path: "/orders?userId={{ steps.user.body.id }}"
    response:
      body:
        user: "{{ steps.user.body }}"
        orders: "{{ steps.orders.body }}"
EOF

# Run the gateway
./bin/restitch -config restitch.yaml

# Test
curl -s "http://localhost:8080/api/user-posts?id=1" | python3 -m json.tool
```

## Configuration

### Full Schema

```yaml
server:                          # all optional
  port: 8080
  tls_port: 8443
  tls_cert: ""
  tls_key: ""
  read_timeout: 10s
  write_timeout: 30s
  shutdown_timeout: 30s
  log_format: json               # json | text
  log_level: info                # debug | info | warn | error
  rate_limit:                    # optional global rate limiter
    requests_per_second: 1000
    burst: 50
    key: "ip"                    # ip | header:<name> | api-key
  auth:                          # inbound auth; omit = open
    api_keys: ["${GATEWAY_KEY}"]
    jwt:
      jwks_url: "https://issuer/.well-known/jwks.json"
      issuer: "https://issuer"
      audience: "restitch"

admin:
  enabled: true
  port: 9090
  api_key: "${ADMIN_KEY}"
  request_log_size: 500

upstreams:
  users-api:
    url: "https://users.internal"
    timeout: 10s
    health_path: "/health"
    max_response_bytes: 10485760  # 10 MiB default
    auth:
      header: {name: "X-API-Key", value: "${API_KEY}"}
    retry:
      max_attempts: 3
      interval: 250ms
      max_backoff: 5s
      backoff_on: [429, 502, 503, 504]
    circuit_breaker:
      max_failures: 5
      interval: 60s
      timeout: 10s

compositions:
  user-dashboard:
    path: "/api/users/{id}/dashboard"
    method: GET
    public: false                # true = skip inbound auth
    rate_limit:                  # optional per-composition rate limiter
      requests_per_second: 100
      burst: 10
      key: "ip"
    max_request_bytes: 1048576   # request body size limit (default 1 MiB)
    request_schema:              # optional JSON Schema for request body validation
      type: object
      properties:
        name: { type: string }
      required: [name]
    steps:
      - name: user
        upstream: users-api
        path: "/users/{{ req.params.id }}"
        method: GET
        headers: {X-Tenant: "{{ req.headers['X-Tenant'] }}"}
        optional: false
        timeout: 5s
        cache: {ttl: 30s}
        coalesce: true
        error_rules:
          - statuses: [404]
            body: null
    response:
      status: 200
      content_type: application/json
      body:
        user: "{{ steps.user.body }}"
```

### Expression Language

Expressions use [Expr](https://expr-lang.org/) syntax inside `{{ }}` delimiters.

| Variable | Type | Notes |
|----------|------|-------|
| `req.method` | string | Incoming HTTP method |
| `req.path` | string | Incoming URL path |
| `req.params` | map[string]string | Path parameters (`{id}` → `req.params.id`) |
| `req.query` | map[string]string | First query value per key |
| `req.query_all` | map[string][]string | All query values |
| `req.headers` | map[string]string | Canonical-cased, first value |
| `req.body` | any | Parsed JSON body (nil if absent) |
| `req.auth` | map | JWT claims (nil if no JWT) |
| `request` | — | Alias for `req` |
| `steps.<name>.status` | int | Upstream HTTP status |
| `steps.<name>.headers` | map[string]string | Upstream response headers |
| `steps.<name>.body` | any | Parsed JSON body |
| `steps.<name>` | nil | When step failed or was skipped |

```yaml (fragment)
# Path escaping is automatic
path: "/users/{{ req.params.id }}"

# Null coalescing for optional steps
body:
  points: "{{ steps.loyalty.body.points ?? 0 }}"

# Ternary
body:
  status: "{{ steps.user.body.active ? 'active' : 'inactive' }}"
```

### Partial Responses

When optional steps fail, the composition still returns HTTP 200:

- `X-Restitch-Complete: false` — at least one step failed/was skipped
- `X-Partial-Response: true` — legacy alias
- `_errors` array in the body:
  ```json
  {"step": "loyalty", "message": "upstream error", "status": "failed"}
  ```

Failed step references evaluate to `null` in templates (no 500).

### Authentication

**Inbound** (gateway-level, per `server.auth`):
- **API keys**: `X-API-Key` header, constant-time compare
- **JWT/JWKS**: `Authorization: Bearer <token>`, validates issuer/audience/expiry
- Per-composition `public: true` to skip auth

**Upstream** (per-upstream, one of):
- `header`: Static header injection (`X-API-Key`, etc.)
- `basic`: HTTP Basic Auth
- `passthrough`: Forwards client's `Authorization` header via context (only passthrough upstreams receive it — no credential leak to other upstreams)
- `oauth2`: Client credentials with automatic token refresh (10s bounded)

### Resilience

**Retry** (per-upstream or per-step override):
- Status-code-driven: `backoff_on` retries, `drop_on` gives up immediately
- Exponential backoff with jitter, honors `Retry-After`
- Idempotent methods only by default (`retry_non_idempotent: true` to override)

**Circuit Breaker** (per-upstream):
- Trips after `max_failures` consecutive failures (status ≥ 500 or network error)
- Half-open probe after `timeout`; client cancellations don't count

**Response Cache** (per-step, GET only):
- In-memory TTL cache, keyed by method + URL + auth identity hash
- Only caches status < 500, non-error-rule responses

**Request Coalescing** (per-step, GET only):
- `coalesce: true` — deduplicates concurrent identical requests via singleflight

**Rate Limiting** (per-composition or global):
- Token-bucket limiter keyed by IP, header, or API key
- Per-composition `rate_limit` and global `server.rate_limit`
- Exceeded requests receive 429 with `Retry-After` header

### Observability

**Prometheus Metrics** (on admin port):

| Metric | Type | Labels |
|--------|------|--------|
| `restitch_requests_total` | counter | composition, method, status |
| `restitch_request_duration_seconds` | histogram | composition |
| `restitch_partial_responses_total` | counter | composition |
| `restitch_step_duration_seconds` | histogram | composition, step, upstream, status |
| `restitch_upstream_requests_total` | counter | upstream, status_class |
| `restitch_retries_total` | counter | upstream |
| `restitch_breaker_state` | gauge | upstream |
| `restitch_cache_hits_total` | counter | composition, step |

**Admin API** (default port 9090):

| Endpoint | Description |
|----------|-------------|
| `GET /admin/api/info` | Version, uptime, config hash |
| `GET /admin/api/requests?limit=100` | Request ring buffer (newest first) |
| `GET /admin/api/stats` | Per-composition count/errors/avg/p95 |
| `POST /admin/api/validate` | Validate YAML config |
| `POST /admin/api/reload` | Trigger hot config reload |
| `GET /metrics` | Prometheus metrics |
| `GET /health` | Admin liveness |

Protected by `admin.api_key` (via `X-Admin-Key` header) when configured.

**Distributed Tracing**: OpenTelemetry support via OTLP HTTP exporter. Set
`OTEL_EXPORTER_OTLP_ENDPOINT` to enable. Spans are created per composition
and per step, and trace IDs are included in request records.

**Structured Logging**: JSON by default, every line includes `request_id`
from context. Use `-log-level debug` for step-level detail.

### CLI

```
restitch run [flags]        # Start the gateway (default when no subcommand)
restitch check -config f    # Validate config, print wave layout, exit 0/1
restitch version            # Print version and Go version
```

`restitch -config restitch.yaml` (no subcommand) works as `run` for
backwards compatibility.

### Hot Reload

Config can be reloaded without restart:

- **SIGHUP**: `kill -HUP <pid>`
- **File watch**: Automatic on config file change (debounced 500ms)
- **Admin API**: `POST /admin/api/reload`

Reload validates the new config first — a bad config never takes down the
running gateway (validate-then-atomic-swap via `atomic.Pointer`).

### Studio

Restitch Studio is a web UI for inspecting the running gateway.

```bash
# Build the frontend
make studio

# Run (gateway must be running with admin API on :9090)
./bin/restitch-studio

# Open http://localhost:3080
```

Pages: Dashboard (stats tiles, per-composition table), Requests (live
request log with status badges), Config (YAML validation).

## Error Taxonomy

| Condition | Status | Body |
|-----------|--------|------|
| No matching route | 404 | `{"error":"not found"}` |
| Method mismatch | 405 + `Allow` | ServeMux default |
| Inbound auth failed | 401 + `WWW-Authenticate` | `{"error":"unauthorized"}` |
| Passthrough, no client auth | 401 | `{"error":"authorization header required"}` |
| Required step failed | 502 | `{"error":"upstream error","step":"<name>"}` |
| Required step timeout | 504 | `{"error":"upstream timeout","step":"<name>"}` |
| Template bug (no failed dep) | 500 | `{"error":"internal error"}` |
| Optional failures only | 200 | Template output + `_errors` |

Internal error details are never exposed to clients (D17).

## Development

```bash
make build          # Build gateway binary
make test           # Run tests
make race           # Run tests with race detector
make vet            # Go vet
make lint           # golangci-lint
make ci             # vet + lint + race
make studio         # Build Studio frontend
make build-all      # Build gateway + studio binaries
make e2e            # Run E2E specs
make docker         # Build Docker image
```

### Project Structure

```
cmd/restitch/              Subcommand dispatch, run, check, version, pipeline
cmd/restitch-studio/       Embedded SPA + admin proxy
cmd/mockupstream/          Dev/demo mock upstream server
internal/gwconfig/         Root config, env expansion, validation
internal/composition/      Parser, template engine, DAG, executor, handler
internal/upstream/         Per-upstream client, retry, breaker, cache, coalesce, health
internal/auth/             Strategies (header, basic, passthrough, oauth2)
internal/inbound/          Inbound auth middleware (API key, JWT/JWKS)
internal/server/           Server, router, middleware, TLS, shutdown
internal/admin/            Admin API server, request ring buffer, stats
internal/observability/    Request ID, slog setup, Prometheus metrics
internal/reqlog/           Request record types (shared)
studio/                    React SPA (Vite + Tailwind v4)
examples/                  Example configs
docs/                      Topic docs
```

## License

Apache-2.0 — see [LICENSE](LICENSE).
