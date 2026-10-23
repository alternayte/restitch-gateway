# Configuration Reference

Restitch has two kinds of configuration:

- **Process settings**: ports, timeouts, logging, the Studio database, the
  gateway token. Each process setting is a command-line flag and an
  environment variable. A YAML file never holds a process setting.
- **The artifact**: upstreams, compositions, the auth policy, the
  gateway-wide rate limit and `block_private_upstreams`. Studio stores the
  artifact as resources and publishes it as artifact versions. A gateway in
  standalone mode reads the artifact from one YAML file.

## Process settings

### The naming rule

Every flag has one environment variable. The name is `RESTITCH_` plus the
flag name in upper snake case. For example, `-gateway-token` is
`RESTITCH_GATEWAY_TOKEN`.

Precedence, highest first:

```
command-line flag  >  RESTITCH_ environment variable  >  built-in default
```

An empty environment variable counts as unset. A value that the flag cannot
parse stops the process with an error that names the variable.

### Environment variables

The Commands column names the `restitch` subcommands that read the variable.
A duration uses Go syntax: `500ms`, `10s`, `1m30s`.

| Variable | Commands | Default | Description |
|----------|----------|---------|-------------|
| `RESTITCH_PORT` | gateway, studio | gateway `8080`, studio `3080` | HTTP port. The gateway serves compositions, `/health` and `/ready` on this port. |
| `RESTITCH_TLS_PORT` | gateway | `8443` | HTTPS port. The gateway listens on it only when `RESTITCH_TLS_CERT` and `RESTITCH_TLS_KEY` are both set. |
| `RESTITCH_TLS_CERT` | gateway | empty | Path to the TLS certificate file. |
| `RESTITCH_TLS_KEY` | gateway | empty | Path to the TLS private key file. |
| `RESTITCH_READ_TIMEOUT` | gateway | `30s` | HTTP read timeout. |
| `RESTITCH_WRITE_TIMEOUT` | gateway | `30s` | HTTP write timeout. |
| `RESTITCH_SHUTDOWN_TIMEOUT` | gateway, studio | `30s` | Deadline to drain in-flight requests after SIGTERM or SIGINT. The gateway uses the same deadline again for its last telemetry push. |
| `RESTITCH_LOG_FORMAT` | gateway, studio | `json` | Log format: `json` or `text`. |
| `RESTITCH_LOG_LEVEL` | gateway, studio | `info` | Minimum log level: `debug`, `info`, `warn` or `error`. |
| `RESTITCH_CONFIG` | gateway | `restitch.yaml` | Standalone mode: path to the artifact file. Do not set it together with `RESTITCH_STUDIO_URL`. |
| `RESTITCH_STUDIO_URL` | gateway, publish | empty | URL of Studio. For `gateway`, setting it selects managed mode. For `publish`, it is required. |
| `RESTITCH_GATEWAY_TOKEN` | gateway | empty | Managed mode: the gateway token of the environment. Required when `RESTITCH_STUDIO_URL` is set. |
| `RESTITCH_POLL_INTERVAL` | gateway | `10s` | Managed mode: time between two pulls of the current artifact version. |
| `RESTITCH_OPS_PORT` | gateway | `0` | Port that serves `/metrics`. `0` turns the ops port off. |
| `RESTITCH_BIND` | studio | empty | Bind address. Empty means every interface. |
| `RESTITCH_DATABASE_URL` | studio | none, required | Studio database: a `postgres://` URL, or `sqlite:` plus a file path on local disk. |
| `RESTITCH_PUBLIC_URL` | studio | `http://localhost:<port>` | URL that people use to reach Studio. Studio uses it for sign-in redirects and the origin check. An `https://` URL makes the session cookie `Secure`. |
| `RESTITCH_BOOTSTRAP_EMAIL` | studio | empty | Email of the first owner. Studio creates this user at start when no user exists. Set it together with `RESTITCH_BOOTSTRAP_PASSWORD`. |
| `RESTITCH_BOOTSTRAP_PASSWORD` | studio | empty | Password of the first owner. |
| `RESTITCH_OIDC_ISSUER` | studio | empty | OIDC issuer URL for single sign-on. Optional. |
| `RESTITCH_OIDC_CLIENT_ID` | studio | empty | OIDC client ID. Required when `RESTITCH_OIDC_ISSUER` is set. |
| `RESTITCH_OIDC_CLIENT_SECRET` | studio | empty | OIDC client secret. Required when `RESTITCH_OIDC_ISSUER` is set. |
| `RESTITCH_API_KEY` | publish | empty | Studio API key of an editor or an owner. Required. |
| `RESTITCH_ENV` | publish | empty | Name of the environment to publish to. Required. |
| `RESTITCH_M` | publish | `Published with restitch publish` | Message of the new artifact version. |

### Rules the commands enforce

- `gateway` exits with status 2 when `RESTITCH_CONFIG` and
  `RESTITCH_STUDIO_URL` are both set.
- `gateway` exits with status 2 when `RESTITCH_STUDIO_URL` is set and
  `RESTITCH_GATEWAY_TOKEN` is not.
- `studio` exits with status 2 when `RESTITCH_DATABASE_URL` is not set.
- `studio` exits with status 2 when only one of `RESTITCH_BOOTSTRAP_EMAIL` and
  `RESTITCH_BOOTSTRAP_PASSWORD` is set.
- `studio` exits with status 2 when `RESTITCH_OIDC_ISSUER` is set without the
  client ID and the client secret.

### Tracing variables

The gateway uses the standard OpenTelemetry variables. They have no
`RESTITCH_` name.

| Variable | Description |
|----------|-------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP HTTP endpoint, for example `http://otel-collector:4318`. Tracing is on only when this variable is set. |
| `OTEL_EXPORTER_OTLP_HEADERS` | Headers that the exporter sends, for example an API key of the tracing backend. |

The exporter also reads the other standard `OTEL_EXPORTER_OTLP_*` variables.
See [observability.md](observability.md#distributed-tracing).

### Flags without an environment variable

`restitch dev`, `restitch check` and `restitch import openapi` read flags
only.

| Command | Flag | Default | Description |
|---------|------|---------|-------------|
| `dev` | `-port` | `8080` | Gateway port. |
| `dev` | `-studio-port` | `3080` | Studio port. Studio binds `127.0.0.1` only. |
| `dev` | `-data-dir` | `.restitch` | Directory of the SQLite database. |
| `check` | `-config` | `restitch.yaml` | Artifact file to validate. |
| `check` | `-q` | `false` | Print errors only. |
| `import openapi` | `-upstream` | `api` | Name of the upstream to generate. |
| `import openapi` | `-base-url` | `servers[0]` of the spec | Base URL of the upstream. |
| `import openapi` | `-ops` | all | Comma-separated operation IDs to include. |
| `import openapi` | `-o` | stdout | Output file. |

### Moving from a v1 file

v2 refuses a file with a `server:` or an `admin:` block. The error names the
replacement of each key:

| v1 key | v2 replacement |
|--------|----------------|
| `server.port`, `server.tls_port`, `server.tls_cert`, `server.tls_key` | `RESTITCH_PORT`, `RESTITCH_TLS_PORT`, `RESTITCH_TLS_CERT`, `RESTITCH_TLS_KEY` |
| `server.read_timeout`, `server.write_timeout`, `server.shutdown_timeout` | `RESTITCH_READ_TIMEOUT`, `RESTITCH_WRITE_TIMEOUT`, `RESTITCH_SHUTDOWN_TIMEOUT` |
| `server.log_format`, `server.log_level` | `RESTITCH_LOG_FORMAT`, `RESTITCH_LOG_LEVEL` |
| `server.auth` | the top-level `auth` key |
| `server.rate_limit` | the top-level `rate_limit` key |
| `admin.*` | removed. The gateway has no admin API. Studio shows usage, instances and sampled requests. |

---

## The artifact

An artifact is one YAML document with these top-level keys. Any other
top-level key is an error.

| Key | Studio resource | Description |
|-----|-----------------|-------------|
| `auth` | auth policy | Inbound authentication. Required when a composition is not public. |
| `rate_limit` | rate limit | Gateway-wide rate limit, applied before routing. |
| `block_private_upstreams` | settings | Refuse upstream URLs whose host is a literal private, loopback or link-local IP address. |
| `upstreams` | upstream, one per name | Named backends. |
| `compositions` | composition, one per name | Inbound routes and their steps. |

One validator checks the artifact in every place: `restitch gateway` at load
and reload, `restitch check`, a resource save in Studio, publish, import and
`restitch publish`.

### Environment variable expansion

A gateway expands variables in the artifact before it parses it. Use this
for secrets: the secret stays on the gateway host and never reaches Studio.

| Syntax | Behavior |
|--------|----------|
| `${VAR}` | The value of `VAR`. An error when `VAR` is unset or empty. |
| `${VAR:default}` | The value of `VAR`, or `default` when `VAR` is unset or empty. |
| `$$` | A literal `$`. |

A `$` that is not part of `$$` or `${...}` is an error.

Put a `${VAR}` in a string field. Write the scheme of a URL out:
`https://${USERS_HOST}`, not `${USERS_URL}`. Studio validates an artifact
with every variable replaced by `x`, so it ignores defaults.

### Duration format

Duration fields use Go duration strings: a sequence of decimal numbers, each
with a unit. The units are `ns`, `us` or `µs`, `ms`, `s`, `m` and `h`.

Examples: `"5s"`, `"250ms"`, `"1m30s"`.

---

## auth

The inbound auth policy. It applies to every composition that does not set
`public: true`. A composition that is not public needs this block: publish,
`restitch check` and the gateway refuse the artifact without it.

When `api_keys` and `jwt` are both set, a request passes when one of the
two checks succeeds.

| Field | Type | Description |
|-------|------|-------------|
| `api_keys` | []string | Accepted API keys. The gateway compares the `X-API-Key` request header in constant time. |
| `jwt` | object | JWT validation, see below. |

### auth.jwt

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `jwks_url` | string | yes | URL of the JWKS endpoint. |
| `issuer` | string | no | Expected `iss` claim. Checked when set. |
| `audience` | string | no | Expected `aud` claim. Checked when set. |

```yaml
auth:
  api_keys: ["${GATEWAY_KEY}"]
  jwt:
    jwks_url: "https://auth.example.com/.well-known/jwks.json"
    issuer: "https://auth.example.com"
    audience: "restitch"
```

---

## rate_limit

The gateway-wide rate limiter. It applies to every request before routing.
A composition can also have its own `rate_limit` with the same fields. A
request must pass both.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `requests_per_second` | float | — | Sustained request rate. |
| `burst` | int | — | Maximum burst above the sustained rate. |
| `key` | string | `"ip"` | Rate limit key: `ip`, `header:<name>` or `api-key`. |

```yaml
rate_limit:
  requests_per_second: 1000
  burst: 50
  key: "ip"
```

---

## block_private_upstreams

When `true`, the artifact is invalid if an upstream URL has a literal
private, loopback or link-local IP address as its host. The check does not
resolve host names.

```yaml
block_private_upstreams: true
```

---

## upstreams

A map of named upstreams. Steps refer to an upstream by its key.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | *required* | Base URL of the upstream. |
| `timeout` | duration | `"30s"` | Default timeout of a step that calls this upstream. |
| `health_path` | string | `""` | Health check path. An empty string sends `HEAD` to the base URL. |
| `max_response_bytes` | int | `10485760` | Maximum response body size in bytes (10 MiB). |

### upstreams.*.transport

HTTP transport settings. All fields are optional.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `dial_timeout` | duration | `"5s"` | TCP dial timeout. |
| `tls_handshake_timeout` | duration | `"5s"` | TLS handshake timeout. |
| `response_header_timeout` | duration | `"10s"` | Time to wait for the response headers. |
| `max_idle_conns_per_host` | int | `100` | Maximum idle connections per host. |
| `insecure_skip_verify` | bool | `false` | Skip TLS certificate verification. Do not use it in production. The gateway logs a warning at start when it is on. |

### upstreams.*.auth

Upstream authentication. Set one strategy per upstream. Omit the block for an
upstream without authentication.

**header**: send a static header.

```yaml
auth:
  header:
    name: "X-API-Key"
    value: "${API_KEY}"
```

**basic**: HTTP Basic Auth.

```yaml
auth:
  basic:
    username: "${USER}"
    password: "${PASS}"
```

**passthrough**: forward the `Authorization` header of the client. Only
passthrough upstreams receive the client credential.

```yaml
auth:
  passthrough: {}
```

**oauth2**: client credentials grant. The gateway refreshes the token. A
token request has a 10s limit.

```yaml
auth:
  oauth2:
    token_url: "https://auth.example.com/oauth/token"
    client_id: "${CLIENT_ID}"
    client_secret: "${CLIENT_SECRET}"
    scopes: ["read", "write"]
```

### upstreams.*.retry

Retry settings. Optional. They apply to every step that calls this upstream,
unless the step has its own `retry` block.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_attempts` | int | — | Total attempts, the first request included. |
| `interval` | duration | — | First backoff interval. |
| `max_backoff` | duration | — | Maximum backoff. |
| `backoff_on` | []int | — | HTTP status codes that cause a retry. Network errors also cause a retry. |
| `drop_on` | []int | `[]` | HTTP status codes that fail at once, with no retry. |
| `retry_non_idempotent` | bool | `false` | Allow retries of POST, PUT, PATCH and DELETE. |

Retries use exponential backoff with jitter. When the upstream sends a
`Retry-After` header, the gateway uses that value.

```yaml
retry:
  max_attempts: 3
  interval: 250ms
  max_backoff: 5s
  backoff_on: [429, 502, 503, 504]
```

### upstreams.*.circuit_breaker

Circuit breaker settings. Optional. Each upstream has one breaker.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_failures` | int | — | Consecutive failures that open the breaker. A failure is a status of 500 or more, or a network error. |
| `interval` | duration | — | Window after which the failure count resets. |
| `timeout` | duration | — | Time the breaker stays open before it goes half-open. |

States: **closed** (normal), **open** (every request fails at once) and
**half-open** (one probe request; a success closes the breaker, a failure
opens it again). A client cancellation is not a failure.

```yaml
circuit_breaker:
  max_failures: 5
  interval: 60s
  timeout: 10s
```

---

## compositions

A map of named compositions. Each composition maps one inbound route to a
graph of steps and a response template.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `path` | string | *required* | Route pattern with Go `http.ServeMux` path parameters, for example `/api/users/{id}`. `/health` and `/ready` are reserved for the probes. |
| `method` | string | *required* | HTTP method to match (`GET`, `POST` and so on). |
| `public` | bool | `false` | When `true`, the route skips inbound auth. When `false`, the artifact needs an `auth` block. |
| `rate_limit` | object | — | Rate limit of this composition. Same fields as the top-level `rate_limit`. |
| `max_request_bytes` | int | `1048576` | Maximum request body size in bytes (1 MiB). A larger request gets 413. |
| `request_schema` | object | — | JSON Schema of the request body. An invalid body gets 400. A non-empty body must have a JSON Content-Type (`application/json` or a `+json` type), or it gets 415. An empty body or a literal `null` is validated as JSON `null`, so a `type: object` schema refuses it. |

### compositions.*.steps

A list of upstream calls. Steps in the same wave have no dependency on each
other and run in parallel.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | *required* | Step name, unique in the composition. |
| `upstream` | string | *required* | Key of an upstream in `upstreams`. |
| `path` | string | *required* | Path after the upstream base URL. Supports `{{ }}` expressions. The gateway URL-escapes path parameter values. |
| `method` | string | composition method | HTTP method of this step. |
| `headers` | map[string]string | `{}` | Extra request headers. Values support `{{ }}` expressions. |
| `body` | string | `""` | Request body template for POST and PUT. Supports `{{ }}` expressions. |
| `depends_on` | []string | `[]` | Explicit step dependencies. The gateway adds the dependencies that the expressions refer to. |
| `optional` | bool | `false` | When `true`, a failure gives a partial response instead of a failed composition. |
| `timeout` | duration | upstream timeout | Timeout of this step. |
| `cache` | object | — | Response cache. GET only. See below. |
| `coalesce` | bool | `false` | Share one upstream call between concurrent identical GET requests. |
| `retry` | object | — | Retry settings of this step. Same fields as `upstreams.*.retry`. |
| `error_rules` | []object | `[]` | Replace the body for specified status codes. See below. |

#### steps.*.cache

| Field | Type | Description |
|-------|------|-------------|
| `ttl` | duration | Cache lifetime. The gateway caches GET requests only. |

```yaml
cache:
  ttl: 30s
```

#### steps.*.error_rules

Replace the step result body when the upstream returns one of the status
codes.

| Field | Type | Description |
|-------|------|-------------|
| `statuses` | []int | HTTP status codes to match, for example `[404, 410]`. |
| `body` | any | Replacement body: an object, an array, a string, `null` and so on. |

```yaml
error_rules:
  - statuses: [404]
    body: null
```

### compositions.*.response

The response that the client receives.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `status` | int or string | `200` | Response status code, or an expression such as `"{{ steps.user.status }}"`. |
| `content_type` | string | `"application/json"` | `Content-Type` of the response. |
| `body` | object | — | Response body template. String values can hold `{{ }}` expressions that refer to step results. |

```yaml
response:
  status: 200
  content_type: application/json
  body:
    user: "{{ steps.user.body }}"
    orders: "{{ steps.orders.body }}"
```

---

## Complete example

```yaml
auth:
  api_keys: ["${GATEWAY_KEY}"]
  jwt:
    jwks_url: "https://auth.example.com/.well-known/jwks.json"
    issuer: "https://auth.example.com"
    audience: "restitch"

rate_limit:
  requests_per_second: 1000
  burst: 50
  key: "ip"

block_private_upstreams: true

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
