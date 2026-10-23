# Restitch

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go version](https://img.shields.io/badge/go-1.25.7-blue.svg)](go.mod)
[![CI](https://img.shields.io/github/actions/workflow/status/alternayte/restitch-gateway/ci.yml?branch=main)](https://github.com/alternayte/restitch-gateway/actions)

A REST API composition gateway. You declare compositions in YAML. Restitch
runs the graph of upstream calls in parallel, connects the results with
expressions, and returns one merged response. You write no BFF code.

## Install

Container image (one image for every role):

```bash
docker pull ghcr.io/alternayte/restitch-gateway:<version>
```

From source. You need Go 1.25.7 and Node.js 24:

```bash
git clone https://github.com/alternayte/restitch-gateway.git
cd restitch-gateway
make build-all         # builds the frontend, then bin/restitch
./bin/restitch version
```

Each tagged release publishes the image and the binaries. See
[VERSIONING.md](VERSIONING.md) for the compatibility policy.

## Architecture

Restitch is one binary, `restitch`. The command picks the role:

- **`restitch gateway`**: the data plane. It loads an artifact, compiles every
  composition into a step graph once, and serves the composed routes. A
  request goes through inbound auth, the rate limit, routing, wave-parallel
  step execution (retry, circuit breaker, cache and coalescing per
  upstream), template evaluation and the merged response.
- **`restitch studio`**: the control plane. A web UI and API on Postgres.
  People edit the draft of each environment and publish numbered artifact
  versions.
- **Upstreams**: plain HTTP services. The gateway never changes them.

A gateway runs in one of two modes:

- **Managed mode**: `RESTITCH_STUDIO_URL` and `RESTITCH_GATEWAY_TOKEN`. The
  gateway pulls the current artifact version of its environment and sends
  heartbeats and telemetry to Studio. Studio never calls a gateway.
- **Standalone mode**: `RESTITCH_CONFIG` names one artifact file. No Studio.

Requests never reach Studio. When Studio is down, managed gateways keep
serving the last version they applied. See
[docs/architecture.md](docs/architecture.md).

## Quick Start

### With Studio

```bash
make build-all
./bin/restitch dev
```

`restitch dev` runs Studio and a managed gateway in one process on SQLite,
with no setup:

- Studio: http://localhost:3080. Sign in as `dev@restitch.local` /
  `restitch-dev-password`.
- Gateway: http://localhost:8080, which serves the `dev` environment.

Add an upstream and a composition in the `dev` environment, then publish.
The gateway applies the version within one second.

### Standalone

```bash
make build-all

# Start the mock upstream (ships with the repo)
go run ./cmd/mockupstream &

# Write an artifact
cat > restitch.yaml <<'YAML'
upstreams:
  mock:
    url: "http://localhost:8081"

compositions:
  user-posts:
    path: "/api/user-posts"
    method: GET
    public: true
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
YAML

# Run the gateway
RESTITCH_CONFIG=restitch.yaml ./bin/restitch gateway

# Test
curl -s "http://localhost:8080/api/user-posts?id=1" | python3 -m json.tool
```

## Configuration

Process settings are flags and `RESTITCH_` environment variables. The
artifact holds the rest. [docs/configuration.md](docs/configuration.md)
lists every variable and every field.

### Process settings

| Role | Required variables |
|------|--------------------|
| Studio | `RESTITCH_DATABASE_URL`, `RESTITCH_BOOTSTRAP_EMAIL`, `RESTITCH_BOOTSTRAP_PASSWORD`, `RESTITCH_PUBLIC_URL` |
| Managed gateway | `RESTITCH_STUDIO_URL`, `RESTITCH_GATEWAY_TOKEN` |
| Standalone gateway | `RESTITCH_CONFIG` |

Each flag has one variable: `RESTITCH_` plus the flag name in upper snake
case. A flag wins over its variable, and the variable wins over the default.

### Artifact schema

```yaml
auth:                            # inbound auth policy; required unless every composition is public
  api_keys: ["${GATEWAY_KEY}"]
  jwt:
    jwks_url: "https://issuer/.well-known/jwks.json"
    issuer: "https://issuer"
    audience: "restitch"

rate_limit:                      # optional gateway-wide rate limiter
  requests_per_second: 1000
  burst: 50
  key: "ip"                      # ip | header:<name> | api-key

block_private_upstreams: false   # true refuses literal private IP upstream hosts

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

`${VAR}` values expand from the environment of the gateway, so secrets
never reach Studio. A v1 file with a `server:` or `admin:` block is refused,
and the error names the replacement variable.

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

**Inbound** (the top-level `auth` policy):
- **API keys**: `X-API-Key` header, constant-time compare
- **JWT/JWKS**: `Authorization: Bearer <token>`, validates issuer/audience/expiry
- Per-composition `public: true` to skip auth. A composition that is not public needs the `auth` policy, or the artifact is refused

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
- Per-composition `rate_limit` and the top-level `rate_limit`
- Exceeded requests receive 429 with `Retry-After` header

### Observability

**Prometheus metrics** are on `GET /metrics` on the ops port. Set
`RESTITCH_OPS_PORT` (for example `9090`) to turn it on.

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
| `restitch_telemetry_dropped_total` | counter | kind |
| `restitch_registry_polls_total` | counter | result |
| `restitch_registry_poll_duration_seconds` | histogram | — |
| `restitch_registry_last_success_timestamp` | gauge | — |

The `restitch_registry_*` metrics count the pulls of a managed gateway from
Studio.

**Probes** are on the data port. `GET /health` answers 200 while the process
runs. `GET /ready` answers 503 until the gateway applies its first artifact,
then 200.

**Studio telemetry**: each managed gateway sends heartbeats, one-minute usage
aggregates and sampled requests to Studio. The environment overview in
Studio shows them for every replica.

**Distributed tracing**: OpenTelemetry through the OTLP HTTP exporter. Set
`OTEL_EXPORTER_OTLP_ENDPOINT` to turn it on. The gateway makes one span per
composition and one per step.

**Structured logging**: JSON by default. Every line has the `request_id`.
Set `RESTITCH_LOG_LEVEL=debug` for step-level detail.

See [docs/observability.md](docs/observability.md).

### CLI

```
restitch gateway [flags]                 # Run a gateway (managed or standalone)
restitch studio [flags]                  # Run Studio
restitch dev [flags]                     # Run Studio and a managed gateway in one process
restitch publish -env NAME [-m MSG] FILE # Import a file into a draft and publish it
restitch check -config FILE              # Validate an artifact, print the wave layout
restitch import openapi <spec> [flags]   # Generate an artifact from an OpenAPI spec
restitch healthcheck <url>               # Exit 0 when the URL answers 2xx (container probes)
restitch version                         # Print the version
```

A bare `restitch` prints usage and exits with status 2.

### Hot reload

- **Standalone mode**: the gateway reloads the file when it changes, and on
  SIGHUP.
- **Managed mode**: the gateway pulls every `RESTITCH_POLL_INTERVAL` (10s).
  SIGHUP pulls at once.

The gateway validates each new artifact first. An invalid artifact never
replaces the running one. The swap is atomic, so a reload never blocks a
request.

### Studio

Studio has environments, drafts of typed resources, publish with a diff,
numbered artifact versions, rollback, promote, gateway tokens, users with
the roles viewer, editor and owner, and API keys for CI.

Publish from CI:

```bash
RESTITCH_STUDIO_URL=https://studio.example.com RESTITCH_API_KEY=rak_... \
  restitch publish -env production -m "Release 42" restitch.yaml
```

See [docs/studio.md](docs/studio.md).

## Production Deployment

Studio needs Postgres and four variables. A managed gateway needs two. The
generic guide for any platform that runs an image with env vars is
[docs/deployment.md](docs/deployment.md).

`deploy/` holds a Docker Compose stack (Postgres, Studio, two managed
gateways, a mock upstream and Prometheus) and plain Kubernetes manifests.
CI lints both with `make deploy-lint`. See
[deploy/README.md](deploy/README.md).

Prometheus ships with recording rules (request rate, P50/P95/P99 latency and
error rate per composition) and alert rules.

Load tests live in `tests/loadtest/`. The k6 suite runs in CI on
`release/*` branches, tags and manual dispatch, not on every push: a hard
latency threshold flakes on shared runners.

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
make studio         # Build the Studio frontend into internal/studio/dist
make build          # Build bin/restitch
make build-all      # Build the frontend when it is missing, then bin/restitch
make run            # Run the gateway
make test           # Run tests
make race           # Run tests with the race detector
make vet            # Go vet
make lint           # golangci-lint
make ci             # vet + lint + race
make e2e            # Run the golden specs in tests/
make docker         # Build the image restitch:dev
make deploy-lint    # Lint deploy/ with docker compose config and kubeconform
```

The binary embeds `internal/studio/dist`. That directory is a build output
and is not committed, so `go build ./...` fails on a fresh clone until
`make studio` has run.

The Studio tests in `internal/studio` also run against Postgres when
`RESTITCH_TEST_DATABASE_URL` is set.

### Repository notes

`PLAN.md` is the maintainers' planning record, not user documentation. The
verification gates under `scripts/gates/` and their evidence ledger are
maintained locally by the maintainers; the ledger directories are not part
of the public tree. The user-facing docs are `docs/` and this README.

### Project Structure

```
cmd/restitch/              Subcommands: gateway, studio, dev, publish, check, import, healthcheck, version
cmd/mockupstream/          Mock upstream server for development and demos
internal/gwconfig/         Env naming rule, env expansion, artifact key checks
internal/composition/      Parser, validator, template engine, DAG, executor, handler
internal/upstream/         Per-upstream client, retry, breaker, cache, coalesce, health
internal/auth/             Upstream auth strategies (header, basic, passthrough, oauth2)
internal/inbound/          Inbound auth middleware (API key, JWT/JWKS)
internal/ratelimit/        Token-bucket limiters, gateway-wide and per composition
internal/server/           Server, router, middleware, TLS, probes
internal/hotreload/        Studio client, poller, heartbeat, usage and request push
internal/admin/            Usage aggregation of a managed gateway
internal/studio/           Studio server, API, store, migrations, embedded UI
internal/observability/    Request ID, slog setup, Prometheus metrics, OTel tracing
internal/reqlog/           Request record types
internal/mockupstream/     Mock upstream handlers (shared with cmd/)
internal/testenv/          Test helpers for gateways and upstreams
studio/                    Studio web UI (Vite, React, TanStack, Tailwind v4)
api/openapi.yaml           Studio API description, generated and committed
deploy/                    Docker Compose stack, Kubernetes manifests, Prometheus rules
tests/                     E2E specs and k6 load tests
examples/                  Example artifacts
docs/                      Topic docs
```

## License

Apache-2.0 — see [LICENSE](LICENSE).
