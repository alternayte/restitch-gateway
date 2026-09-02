# Configuration Reference

Restitch is configured via a YAML file. By default the gateway looks for
`restitch.yaml` in the working directory. Use `-config <path>` to specify a
different file.

## Config Precedence

Values are resolved in this order (highest wins):

```
CLI flags  >  RESTITCH_* env vars  >  YAML file  >  built-in defaults
```

## Environment Variable Expansion

All string values in the YAML file support variable expansion:

| Syntax | Behavior |
|--------|----------|
| `${VAR}` | Replaced with the value of `VAR`. Error if unset. |
| `${VAR:default}` | Replaced with `VAR` if set, otherwise `default`. |
| `$$` | Literal `$` character. |

A bare `$` that is not part of `$$` or `${...}` is a parse error.

## RESTITCH_* Environment Overrides

These environment variables override `server.*` and `admin.*` fields without
editing the YAML file:

| Variable | Overrides |
|----------|-----------|
| `RESTITCH_CONFIG` | Config file path (equivalent to `-config`) |
| `RESTITCH_PORT` | `server.port` |
| `RESTITCH_TLS_PORT` | `server.tls_port` |
| `RESTITCH_TLS_CERT` | `server.tls_cert` |
| `RESTITCH_TLS_KEY` | `server.tls_key` |
| `RESTITCH_LOG_FORMAT` | `server.log_format` |
| `RESTITCH_LOG_LEVEL` | `server.log_level` |
| `RESTITCH_ADMIN_PORT` | `admin.port` |
| `RESTITCH_ADMIN_BIND` | `admin.bind` |
| `RESTITCH_ADMIN_ENABLED` | `admin.enabled` |
| `RESTITCH_ADMIN_API_KEY` | `admin.api_key` |
| `RESTITCH_REGISTRY_URL` | `-registry-url` (registry mode) |
| `RESTITCH_REGISTRY_KEY` | `-registry-key` (registry auth) |
| `RESTITCH_POLL_INTERVAL` | `-poll-interval` |

## Duration Format

Duration fields use Go duration strings: a sequence of decimal numbers with a
unit suffix. Valid units: `ns`, `us`/`µs`, `ms`, `s`, `m`, `h`.

Examples: `"5s"`, `"250ms"`, `"1m30s"`, `"10s"`.

---

## server

Top-level server settings. All fields are optional.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `port` | int | `8080` | HTTP listen port |
| `tls_port` | int | `8443` | HTTPS listen port |
| `tls_cert` | string | `""` | Path to TLS certificate. TLS is enabled only when both `tls_cert` and `tls_key` are set. |
| `tls_key` | string | `""` | Path to TLS private key |
| `read_timeout` | duration | `"10s"` | HTTP server read timeout |
| `write_timeout` | duration | `"30s"` | HTTP server write timeout |
| `shutdown_timeout` | duration | `"30s"` | Graceful shutdown deadline |
| `log_format` | string | `"json"` | Log output format: `json` or `text` |
| `log_level` | string | `"info"` | Minimum log level: `debug`, `info`, `warn`, `error` |

### server.rate_limit

Optional global rate limiter applied to all incoming requests before
composition dispatch.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `requests_per_second` | float | — | Sustained request rate |
| `burst` | int | — | Maximum burst size above the sustained rate |
| `key` | string | `"ip"` | Rate limit key: `ip`, `header:<name>`, or `api-key` |

```yaml
server:
  rate_limit:
    requests_per_second: 1000
    burst: 50
    key: "ip"
```

### server.auth

Inbound authentication applied to all compositions (unless `public: true`).
Omit the entire `auth` block to run without inbound auth.

When both `api_keys` and `jwt` are configured, a request passes if **either**
check succeeds.

| Field | Type | Description |
|-------|------|-------------|
| `api_keys` | []string | Accepted API keys. Matched against the `X-API-Key` request header using constant-time comparison. |

#### server.auth.jwt

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `jwks_url` | string | yes | URL to the JWKS endpoint for token verification |
| `issuer` | string | no | Expected `iss` claim. Validated if set. |
| `audience` | string | no | Expected `aud` claim. Validated if set. |

```yaml
server:
  port: 8080
  log_format: json
  auth:
    api_keys: ["${GATEWAY_KEY}"]
    jwt:
      jwks_url: "https://auth.example.com/.well-known/jwks.json"
      issuer: "https://auth.example.com"
      audience: "restitch"
```

---

## admin

Admin API server configuration. The admin API runs on a separate port from
the data plane.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable the admin API server |
| `port` | int | `9090` | Admin API listen port |
| `bind` | string | `"127.0.0.1"` | Admin API bind address. Use `0.0.0.0` only in a trusted network |
| `api_key` | string | `""` | Required. All admin endpoints (including `OPTIONS` preflights) reject requests without a matching `X-Admin-Key` header. With no key set, every request is rejected |
| `request_log_size` | int | `500` | Number of entries in the request ring buffer |

```yaml
admin:
  enabled: true
  port: 9090
  api_key: "${ADMIN_KEY}"
  request_log_size: 1000
```

### admin.storage

Persistent storage backend for request logs and statistics. When omitted,
data is kept in memory only and lost on restart.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `type` | string | `"memory"` | Storage backend: `memory` or `sqlite`. `turso` is rejected (not supported by this build) |
| `url` | string | `""` | Database URL (required for `sqlite`) |
| `auth_token` | string | `""` | Reserved; unused by the sqlite backend |
| `retention` | duration | `"168h"` | How long to retain request records |

```yaml
admin:
  storage:
    type: sqlite
    url: "file:restitch.db"
    retention: 72h
```

---

## upstreams

A map of named upstream services. Each key is the upstream name referenced by
steps.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | *required* | Base URL of the upstream service |
| `timeout` | duration | `"30s"` | Default per-step timeout for requests to this upstream |
| `health_path` | string | `""` | Health check path. Empty string sends `HEAD` to the base URL. |
| `max_response_bytes` | int | `10485760` | Maximum response body size (bytes). Default 10 MiB. |

### upstreams.*.transport

Fine-grained HTTP transport settings. All optional.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `dial_timeout` | duration | `"5s"` | TCP dial timeout |
| `tls_handshake_timeout` | duration | `"5s"` | TLS handshake timeout |
| `response_header_timeout` | duration | `"10s"` | Time to wait for response headers |
| `max_idle_conns_per_host` | int | `100` | Maximum idle connections per host |
| `insecure_skip_verify` | bool | `false` | Skip TLS certificate verification (not recommended for production). Logs a warning at startup when enabled. |

### upstreams.*.auth

Upstream authentication. Specify exactly one strategy per upstream. Omit for
unauthenticated upstreams.

**header** — inject a static header:

```yaml
auth:
  header:
    name: "X-API-Key"
    value: "${API_KEY}"
```

**basic** — HTTP Basic Auth:

```yaml
auth:
  basic:
    username: "${USER}"
    password: "${PASS}"
```

**passthrough** — forward the client's `Authorization` header. Only
passthrough upstreams receive the client credential; other upstreams never
see it.

```yaml
auth:
  passthrough: {}
```

**oauth2** — client credentials grant with automatic token refresh (bounded
to 10s):

```yaml
auth:
  oauth2:
    token_url: "https://auth.example.com/oauth/token"
    client_id: "${CLIENT_ID}"
    client_secret: "${CLIENT_SECRET}"
    scopes: ["read", "write"]
```

### upstreams.*.retry

Retry configuration. Optional. When set at the upstream level, it applies to
all steps using this upstream unless the step provides its own `retry` block.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_attempts` | int | — | Total attempts including the first request |
| `interval` | duration | — | Initial backoff interval |
| `max_backoff` | duration | — | Maximum backoff duration |
| `backoff_on` | []int | — | HTTP status codes that trigger a retry (and network errors) |
| `drop_on` | []int | `[]` | HTTP status codes that cause an immediate failure (no retry) |
| `retry_non_idempotent` | bool | `false` | Allow retries for non-idempotent methods (POST, PUT, PATCH, DELETE) |

Retries use exponential backoff with jitter. If the upstream returns a
`Retry-After` header, that value is honored instead of the calculated backoff.

```yaml
retry:
  max_attempts: 3
  interval: 250ms
  max_backoff: 5s
  backoff_on: [429, 502, 503, 504]
```

### upstreams.*.circuit_breaker

Circuit breaker configuration. Optional. One breaker per upstream.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_failures` | int | — | Consecutive failures (status >= 500 or network error) to trip the breaker |
| `interval` | duration | — | Rolling window for failure counter reset |
| `timeout` | duration | — | Duration the breaker stays open before transitioning to half-open |

States: **closed** (normal) → **open** (all requests fail fast) → **half-open**
(one probe allowed; success closes, failure re-opens). Client cancellations
(`context.Canceled`) do not count as failures.

```yaml
circuit_breaker:
  max_failures: 5
  interval: 60s
  timeout: 10s
```

---

## compositions

A map of named compositions. Each composition binds an inbound HTTP route to
a DAG of upstream steps and a response template.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `path` | string | *required* | Route pattern. Supports Go 1.22 `http.ServeMux` path parameters: `/api/users/{id}`. |
| `method` | string | *required* | HTTP method to match (`GET`, `POST`, etc.) |
| `public` | bool | `false` | When `true`, skip inbound auth for this route |
| `rate_limit` | object | — | Per-composition rate limiter. Same fields as `server.rate_limit`. |
| `max_request_bytes` | int | `1048576` | Maximum request body size in bytes (default 1 MiB). Requests exceeding this are rejected with 413. |
| `request_schema` | object | — | JSON Schema for request body validation. Invalid requests are rejected with 400. |

### compositions.*.steps

An ordered list of upstream requests. Steps in the same DAG wave (no mutual
dependencies) execute in parallel.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | *required* | Unique step identifier within the composition |
| `upstream` | string | *required* | Name of an upstream defined in the `upstreams` map |
| `path` | string | *required* | URL path appended to the upstream base URL. Supports `{{ }}` expressions. Path parameter values are automatically URL-escaped. |
| `method` | string | composition method | HTTP method for this step |
| `headers` | map[string]string | `{}` | Additional request headers. Values support `{{ }}` expressions. |
| `body` | string | `""` | Request body template for POST/PUT. Supports `{{ }}` expressions. |
| `depends_on` | []string | `[]` | Explicit step dependencies. Merged with dependencies inferred from expressions. |
| `optional` | bool | `false` | When `true`, failure degrades to a partial response instead of failing the composition |
| `timeout` | duration | upstream timeout | Per-step timeout, overrides the upstream default |
| `cache` | object | — | Response cache config. GET only. See below. |
| `coalesce` | bool | `false` | Deduplicate concurrent identical GET requests via singleflight |
| `retry` | object | — | Per-step retry config. Same shape as `upstreams.*.retry`. Overrides the upstream retry. |
| `error_rules` | []object | `[]` | Status-to-body replacement rules. See below. |

#### steps.*.cache

| Field | Type | Description |
|-------|------|-------------|
| `ttl` | duration | Cache lifetime. Only GET requests are cached. |

```yaml
cache:
  ttl: 30s
```

#### steps.*.error_rules

Replace the step result body when the upstream returns specific status codes.

| Field | Type | Description |
|-------|------|-------------|
| `statuses` | []int | HTTP status codes to match (e.g., `[404, 410]`) |
| `body` | any | Replacement body value (object, array, string, `null`, etc.) |

```yaml
error_rules:
  - statuses: [404]
    body: null
```

### compositions.*.response

Defines the composition response returned to the client.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `status` | int or string | `200` | Response status code. Can be an expression: `"{{ steps.user.status }}"`. |
| `content_type` | string | `"application/json"` | Response `Content-Type` header |
| `body` | object | — | Response body template. A nested map where string values can contain `{{ }}` expressions referencing step results. |

```yaml
response:
  status: 200
  content_type: application/json
  body:
    user: "{{ steps.user.body }}"
    orders: "{{ steps.orders.body }}"
```

---

## Complete Example

```yaml
server:
  port: 8080
  tls_cert: "/etc/restitch/tls/cert.pem"
  tls_key: "/etc/restitch/tls/key.pem"
  read_timeout: 10s
  write_timeout: 30s
  shutdown_timeout: 30s
  log_format: json
  log_level: info
  auth:
    api_keys: ["${GATEWAY_KEY}"]
    jwt:
      jwks_url: "https://auth.example.com/.well-known/jwks.json"
      issuer: "https://auth.example.com"
      audience: "restitch"

admin:
  enabled: true
  port: 9090
  api_key: "${ADMIN_KEY}"
  request_log_size: 1000

upstreams:
  users-api:
    url: "https://users.internal"
    timeout: 10s
    health_path: "/health"
    max_response_bytes: 10485760
    transport:
      dial_timeout: 5s
      tls_handshake_timeout: 5s
      response_header_timeout: 10s
      max_idle_conns_per_host: 100
    auth:
      header:
        name: "X-API-Key"
        value: "${USERS_API_KEY}"
    retry:
      max_attempts: 3
      interval: 250ms
      max_backoff: 5s
      backoff_on: [429, 502, 503, 504]
    circuit_breaker:
      max_failures: 5
      interval: 60s
      timeout: 10s

  orders-api:
    url: "https://orders.internal"
    timeout: 5s
    auth:
      passthrough: {}

compositions:
  user-dashboard:
    path: "/api/users/{id}/dashboard"
    method: GET
    steps:
      - name: user
        upstream: users-api
        path: "/users/{{ req.params.id }}"
        headers:
          X-Tenant: "{{ req.headers['X-Tenant'] }}"
        timeout: 5s
        cache:
          ttl: 30s
        coalesce: true
        error_rules:
          - statuses: [404]
            body: null
      - name: orders
        upstream: orders-api
        path: "/orders?userId={{ steps.user.body.id }}"
        optional: true
    response:
      status: 200
      content_type: application/json
      body:
        user: "{{ steps.user.body }}"
        orders: "{{ steps.orders.body }}"
```
