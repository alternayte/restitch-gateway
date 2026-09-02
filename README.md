# Restitch

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go version](https://img.shields.io/badge/go-1.25.7-blue.svg)](go.mod)
[![CI](https://img.shields.io/github/actions/workflow/status/alternayte/restitch-gateway/ci.yml?branch=main)](https://github.com/alternayte/restitch-gateway/actions)

A production-grade REST API composition gateway. Declare multi-step API
compositions in YAML — Restitch executes the DAG of upstream calls in
parallel, wires results together with expressions, and returns a single
merged response. No BFF code required.

## Install

Requirements: Go 1.25.7 and Node.js 24 (for the Studio).

```bash
git clone https://github.com/alternayte/restitch-gateway.git
cd restitch-gateway
make build-all         # gateway + Studio binaries under bin/
./bin/restitch version
```

Release binaries and container images are published with each tagged
release; `make docker` builds the image locally. See [VERSIONING.md] for the
compatibility policy.

## Architecture

RESTitch has two processes plus the data sources they talk to:

- **Gateway** (`./bin/restitch`): the data plane. It loads a YAML config
  (file or Studio registry), compiles every composition into a DAG once at
  startup, and serves the composed HTTP routes. Request handling is: inbound
  auth → rate limit → route → wave-parallel step execution (per-upstream
  retry, circuit breaker, cache, coalescing) → template evaluation → merged
  response. A separate admin server (port 9090, loopback by default) serves
  metrics, request records, stats, and config reload.
- **Studio** (`./bin/restitch-studio`): the control plane. An embedded React
  SPA for dashboards, request inspection, and a visual composition builder.
  It proxies read-only gateway admin calls, and optionally owns a config
  registry that the gateway polls instead of a file.
- **Upstreams**: plain HTTP services. The gateway never modifies them.

Requests never touch the Studio; the Studio going down does not affect the
data plane. The gateway keeps its last known-good config when the registry
is unreachable. See [docs/architecture.md] for the request lifecycle and the
trust boundaries between the processes.

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
  bind: "127.0.0.1"           # 0.0.0.0 only in a trusted network
  api_key: "${ADMIN_KEY}"     # required; requests without a key are rejected
  request_log_size: 500
  storage:                       # optional; in-memory ring buffer if omitted
    type: sqlite                 # memory | sqlite (turso is rejected)
    url: "file:studio.db"
    auth_token: ""
    retention: 168h

upstreams:
  users-api:
    url: "https://users.internal"
    timeout: 10s
    health_path: "/health"
    max_response_bytes: 10485760  # 10 MiB default
    auth:
      header: {name: "X-API-Key", value: "${API_KEY}"}
    transport:                    # optional; zero values fall through to defaults
      dial_timeout: 5s
      tls_handshake_timeout: 5s
      response_header_timeout: 10s
      max_idle_conns_per_host: 100 # Go's default of 2 costs 4-5x latency on fan-out
      insecure_skip_verify: false
    retry:
      max_attempts: 3
      interval: 250ms
      max_backoff: 5s
      backoff_on: [429, 502, 503, 504]
      drop_on: [400, 401, 403]    # never retry these
      retry_non_idempotent: false # POST/PATCH are not retried unless true
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
        depends_on: []             # usually inferred from template references
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
| `restitch_cache_misses_total` | counter | composition, step |
| `restitch_coalesced_total` | counter | composition, step |
| `restitch_registry_polls_total` | counter | result |
| `restitch_registry_poll_duration_seconds` | histogram | — |
| `restitch_registry_last_success_timestamp` | gauge | — |

**Admin API** (default port 9090):

| Endpoint | Description |
|----------|-------------|
| `GET /admin/api/info` | Version, uptime, config hash |
| `GET /admin/api/compositions` | Compositions with steps and wave layout |
| `GET /admin/api/compositions/{name}` | One composition |
| `GET /admin/api/upstreams` | Upstreams with health status |
| `GET /admin/api/requests?limit=100` | Request ring buffer (newest first) |
| `GET /admin/api/requests/{id}` | One request record with step detail |
| `GET /admin/api/stats` | Per-composition count/errors/avg/p95 |
| `GET /admin/api/stats/steps` | Per-step aggregates for a composition |
| `GET /admin/api/stats/timeseries` | Bucketed request/error/partial counts |
| `GET /admin/api/registry/status` | Last poll time, ETag, count, error state (registry mode only) |
| `POST /admin/api/validate` | Validate YAML config |
| `POST /admin/api/reload` | Trigger hot config reload |
| `GET /metrics` | Prometheus metrics |
| `GET /health` | Admin liveness |

Protected by `admin.api_key` (via `X-Admin-Key` header). The key is required:
with no key configured, every admin request is rejected. The admin server
binds `127.0.0.1` by default; set `admin.bind` only for a trusted network.

**Distributed Tracing**: OpenTelemetry support via OTLP HTTP exporter. Set
`OTEL_EXPORTER_OTLP_ENDPOINT` to enable. Spans are created per composition
and per step, and trace IDs are included in request records.

**Structured Logging**: JSON by default, every line includes `request_id`
from context. Use `-log-level debug` for step-level detail.

### CLI

```
restitch run [flags]            # Start the gateway (default when no subcommand)
restitch check -config f        # Validate config, print wave layout, exit 0/1
restitch dev [flags]            # Run gateway + Studio together (see Dev Mode)
restitch import openapi <spec>  # Generate a composition from an OpenAPI spec
restitch version                # Print version and Go version
```

`restitch -config restitch.yaml` (no subcommand) works as `run` for
backwards compatibility.

`run` flags: `-port`, `-tls-port`, `-cert`, `-key`, `-log-format`,
`-log-level`, `-config`, and the registry-mode flags `-registry-url`,
`-registry-key`, `-poll-interval`. `-config` and `-registry-url` are
mutually exclusive.

`import openapi` flags: `-upstream` (upstream name), `-base-url`,
`-ops` (comma-separated operation IDs), `-o` (output file).

### Hot Reload

Config can be reloaded without restart:

- **SIGHUP**: `kill -HUP <pid>`
- **File watch**: Automatic on config file change (debounced 500ms)
- **Admin API**: `POST /admin/api/reload`

Reload validates the new config first — a bad config never takes down the
running gateway (validate-then-atomic-swap via `atomic.Pointer`).

### Registry Mode

Instead of a config file, the gateway can poll Studio's registry for its
compositions:

```bash
restitch-studio -registry-key "$REGISTRY_KEY"
restitch run -registry-url http://localhost:3080 -registry-key "$REGISTRY_KEY"
```

The Studio registry API requires `X-Admin-Key`; the gateway's `-registry-key`
must match the Studio's `-registry-key`, or the poll is rejected. Studio
binds `127.0.0.1` by default.

The poller uses ETag change detection, so an unchanged bundle costs a 304.
Transient fetch failures back off exponentially and the gateway keeps serving
the last known-good config; a bundle that parses but fails validation is
rejected without disturbing the running pipeline. SIGHUP triggers an immediate
poll, skipping the backoff timer.

`-config` and `-registry-url` are mutually exclusive.

### Studio

Restitch Studio is a web UI for inspecting the running gateway.

```bash
# Build the frontend
make studio

# Run (gateway must be running with admin API on :9090)
./bin/restitch-studio

# Open http://localhost:3080
```

Pages: **Dashboard** (stat tiles, request-rate and latency charts, time-range
selector), **Compositions** (route table with pinning) and its detail view
(step metrics, dependency graph), **Requests** (live request log with status,
duration and partial filters), **Builder** (visual composition editor), and
**Config** (YAML editor with validation).

#### Config Registry

Studio can own the compositions rather than reading them from a file. Configs
are versioned, validated by compiling them through the gateway parser before
they are stored, and served to the gateway as a single bundle.

```
POST   /api/v1/configs                              Create
GET    /api/v1/configs                              List
GET    /api/v1/configs/{id}                         Get
PUT    /api/v1/configs/{id}                         Update content (new version)
PATCH  /api/v1/configs/{id}                         Update metadata
DELETE /api/v1/configs/{id}                         Delete
GET    /api/v1/configs/{id}/versions                Version history
POST   /api/v1/configs/{id}/versions/{n}/activate   Activate a version
POST   /api/v1/configs/validate                     Validate without storing
GET    /api/v1/registry/bundle                      Active configs as one YAML (ETag)
```

Point a gateway at it with `restitch run -registry-url ...` (see Registry Mode).
The registry API is protected by `-registry-key` (`X-Admin-Key`); Studio binds
loopback by default and warns when told to bind elsewhere.

#### Browser Preferences

Studio remembers per-browser UI state with no login. A `restitch_browser_id`
cookie (256-bit, `HttpOnly`, `SameSite=Strict`, one year) identifies the
browser; preferences are stored server-side against it.

```
GET /api/v1/preferences    # pinned compositions, sidebar state, default time range
PUT /api/v1/preferences
```

Pinned compositions sort to the top of the Compositions table, the sidebar
remembers whether it was collapsed, and the Dashboard reopens on the time range
you last chose. State is mirrored to `localStorage` so the first paint is
correct, then reconciled with the server.

### Dev Mode

`restitch dev` runs the gateway and Studio together with colour-prefixed logs,
health-check gating and automatic restart on crash:

```bash
restitch dev
# Gateway:  http://localhost:8080
# Admin:    http://localhost:9090
# Studio:   http://localhost:3080
```

Ports are fixed. Pass flags through to either child with `--gateway-args` and
`--studio-args`, e.g. `restitch dev --gateway-args="-log-level debug"`.
Ctrl+C shuts both down cleanly.

## Production Deployment

`deploy/` holds a Compose stack running the gateway, Studio, Prometheus and
Jaeger together:

```bash
cp deploy/.env.example deploy/.env    # set image tags and secrets
docker compose -f deploy/docker-compose.yml up -d
```

Prometheus ships with recording rules (pre-computed request rate, P50/P95/P99
latency and error rate per composition) and alert rules for sustained high
latency, elevated error rate, config reload failures and gateway-down. See
`deploy/README.md`.

Load tests live in `tests/loadtest/`. The k6 suite runs in CI on `release/*`
branches, tags, and manual dispatch — deliberately not on every push, where a
hard latency threshold would flake on shared-runner noise.

## Error Taxonomy

| Condition | Status | Body |
|-----------|--------|------|
| No matching route | 404 | ServeMux plain-text 404 |
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
make run            # Build and run the gateway
make test           # Run tests
make race           # Run tests with race detector
make vet            # Go vet
make lint           # golangci-lint
make ci             # vet + lint + race
make studio         # Build Studio frontend
make build-all      # Build gateway + studio binaries
make e2e            # Run E2E specs
make docker         # Build Docker image
make verify GATE=M3 # Run one milestone verification gate
make verify-all     # Run every gate
make ledger-check   # Check every plan task has green evidence
```

`build-all` builds the frontend automatically when `cmd/restitch-studio/dist`
has no assets, since that directory is a build artifact and is not committed.

### Repository notes

`PLAN.md` is the maintainers' planning record, not user documentation. The
verification gates under `scripts/gates/` and their evidence ledger are
maintained locally by the maintainers; the ledger directories are not part
of the public tree. The user-facing docs are `docs/` and this README.

### Project Structure

```
cmd/restitch/              Subcommand dispatch, run, check, dev, import, version
cmd/restitch-studio/       Embedded SPA + admin proxy + registry/preferences API
cmd/mockupstream/          Dev/demo mock upstream server
internal/gwconfig/         Root config, env expansion, validation
internal/composition/      Parser, template engine, DAG, executor, handler
internal/upstream/         Per-upstream client, retry, breaker, cache, coalesce, health
internal/auth/             Strategies (header, basic, passthrough, oauth2)
internal/inbound/          Inbound auth middleware (API key, JWT/JWKS)
internal/ratelimit/        Token-bucket limiters, global and per-composition
internal/server/           Server, router, middleware, TLS, shutdown
internal/admin/            Admin API server, request ring buffer, stats, storage
internal/hotreload/        File watcher, registry client, polling engine, signals
internal/registry/         Config registry store, validation, migrations
internal/session/          Browser session cookie, preferences store
internal/devmode/          Process supervision and prefixed output for `restitch dev`
internal/observability/    Request ID, slog setup, Prometheus metrics, OTel tracing
internal/reqlog/           Request record types (shared)
internal/mockupstream/     Mock upstream handlers (shared with cmd/)
internal/testenv/          Test helpers for spinning up gateways and upstreams
studio/                    React SPA (Vite + Tailwind v4)
deploy/                    Production Docker Compose stack, Prometheus rules
tests/                     E2E specs and k6 load tests
examples/                  Example configs
docs/                      Topic docs
```

## License

Apache-2.0 — see [LICENSE](LICENSE).
