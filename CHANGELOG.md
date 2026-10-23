# Changelog

## v2.0.0 — Unreleased

Studio becomes a multi-environment control plane, and gateways pull
published artifact versions from it. One binary and one image serve every
role. No v1 compatibility layer and no v1 config converter exist. See
[docs/configuration.md](docs/configuration.md#moving-from-a-v1-file) for the
move from a v1 file.

### Breaking Changes

- **Module path** is `github.com/alternayte/restitch-gateway/v2`. Change
  every import path.
- **`server:` and `admin:` are refused.** A config file is now an artifact:
  `auth`, `rate_limit`, `block_private_upstreams`, `upstreams` and
  `compositions` only. The load error names the replacement of each key, for
  example `server.port moved to RESTITCH_PORT`. `server.auth` and
  `server.rate_limit` moved to the top-level `auth` and `rate_limit` keys.
  `admin.request_log_size` and `admin.storage` are removed.
- **Process settings are flags and env vars only.** Each flag has one env
  var: `RESTITCH_` plus the flag name in upper snake case, with flag over
  env over default. `-cert` and `-key` are now `-tls-cert` and `-tls-key`
  (`RESTITCH_TLS_CERT`, `RESTITCH_TLS_KEY`). The Studio env vars
  `STUDIO_*` are replaced by `RESTITCH_*` names.
- **The gateway admin API is removed**, with its port 9090, `X-Admin-Key`,
  the request ring buffer, the stats endpoints, `POST /admin/api/reload`,
  `POST /admin/api/validate` and gateway-side storage. `/health` and `/ready`
  are on the data port. `/metrics` is on the optional ops port
  (`RESTITCH_OPS_PORT`, off by default). Studio shows usage, instances and
  sampled requests.
- **Registry mode is replaced by managed mode.** `-registry-url` and
  `-registry-key` are removed. A managed gateway sets `RESTITCH_STUDIO_URL`
  and `RESTITCH_GATEWAY_TOKEN`, and pulls the current artifact version of
  one environment. The config registry, its bundle and `/api/v1/configs` are
  removed. `RESTITCH_CONFIG` and `RESTITCH_STUDIO_URL` together exit with
  status 2.
- **Pull metrics are renamed for Studio.** `restitch_registry_polls_total`
  is now `restitch_studio_pulls_total`,
  `restitch_registry_poll_duration_seconds` is now
  `restitch_studio_pull_duration_seconds`, and
  `restitch_registry_last_success_timestamp` is now
  `restitch_studio_last_pull_success_timestamp`. Labels and meaning do not
  change. Update dashboards and alert rules that query the old names.
- **A composition that is not public needs an auth policy.** Publish,
  `restitch check` and the gateway refuse an artifact with a composition
  that is not `public: true` when the artifact has no top-level `auth`
  block. v1 served such a route with no auth.
- **`/health` and `/ready` are reserved.** A composition cannot use them.
- **One image with no default command.** The image holds only `/restitch`.
  The container command picks the role: `gateway` or `studio`. The separate
  `restitch-studio` binary and its `--entrypoint` override are removed.
- **CLI**: `restitch run` and the bare `restitch -config ...` form are
  removed. Use `restitch gateway`. A bare `restitch` prints usage and exits
  with status 2. `restitch dev` no longer supervises two child processes,
  and `--gateway-args` and `--studio-args` are removed.
- **Studio storage**: Studio uses Postgres when deployed, and SQLite on local
  disk for one node. The v1 registry and browser-preference data are not
  migrated.

### Features

- **Environments, drafts and resources**: each environment owns one draft
  and its artifact versions. Resources are typed (upstream, composition,
  auth policy, rate limit, settings), with names unique per environment. A
  stale save gets 409 and a reload prompt.
- **Publish**: validates the whole draft with the gateway validator, shows a
  diff and writes an immutable, numbered artifact version with author and
  message. Rollback publishes an old version again as a new version.
  Promote copies a version into the draft of another environment.
- **Gateway tokens**: one environment per token, shown once, stored hashed,
  revocable. A token can only pull the artifact and push telemetry.
- **Managed gateways**: pull with ETag, retry with backoff when Studio is
  down, keep the last good version in memory, and answer `/ready` with 503
  until the first version. Each instance sends a heartbeat, one-minute usage
  aggregates and sampled requests with redacted headers and no bodies.
- **Users and roles**: local users, OIDC sign-in, the roles viewer, editor
  and owner, a bootstrap owner from env, and API keys for CI.
- **`restitch publish -env NAME FILE`**: imports a file into a draft and
  publishes it from CI with an API key.
- **`restitch dev`**: Studio and a managed gateway in one process on SQLite,
  with no env vars.
- **Studio UI**: rebuilt on TanStack Router and Query with a generated API
  client, forms derived from the artifact JSON Schema, a YAML view, server
  lint of expressions, an environment overview with usage charts,
  instances and sampled requests, and user and token management.
- **Deployment**: a generic container-platform guide
  ([docs/deployment.md](docs/deployment.md)), a Docker Compose stack and
  plain Kubernetes manifests in `deploy/`, linted in CI.

### Bug Fixes

- `request_schema` validation can no longer be bypassed. A non-empty body
  with a missing or non-JSON Content-Type returns 415, an empty or `null`
  body is validated as JSON `null`, and a body read error returns 400
  instead of executing the composition. JSON detection now parses the media
  type (`application/json` or `+json`) instead of matching the substring
  "json".
- The per-step cache and coalesce key is derived from the outgoing upstream
  request (method, escaped URL, headers including forwarded
  `Accept`/`Accept-Language`, body, inbound `Authorization`). Path params
  that differ only in `/` placement and requests with different forwarded
  headers no longer share an entry. Coalesced callers no longer share a
  response that carries `Set-Cookie` or `Cache-Control: private`/`no-store`.
- Gateway shutdown runs in a fixed order: servers drain, then the poller and
  the file watcher stop, then the last telemetry push runs with its own
  `RESTITCH_SHUTDOWN_TIMEOUT` deadline.

## v1.2.0 — 2026-09-02

First public release. Everything below this entry — the composition-engine
rewrite, the Studio, milestones M17-M25, and the open-source hardening pass —
shipped together in this tag. The hardening review is recorded in the repo's
plan ledger.

### Hardening (Security)

- **Studio registry API authenticated** (C1): every `/api/v1/configs*` and
  `/api/v1/registry/bundle` handler requires `X-Admin-Key`, compared in
  constant time. With no key configured every request is rejected. Gateway
  pollers send the matching key via `-registry-key`.
- **Studio binds loopback by default** (C2): new `-bind` flag (env
  `STUDIO_BIND`); a non-loopback bind logs a warning.
- **Admin API key is required** (C3): `admin.enabled` is now honored, the
  empty-key default rejects every admin request, the admin server binds
  `127.0.0.1` by default (new `admin.bind` / `RESTITCH_ADMIN_BIND`), and
  preflight `OPTIONS` also requires the key (C4).
- **Module path renamed** (H9): `github.com/alternayte/restitch-gateway`.
- Registry bundle and Studio JSON bodies are size-bounded (H4, H5); malformed
  JSON bodies are rejected with 400 (H11).
- Step cache keys include evaluated step headers; responses with `Set-Cookie`
  or `Cache-Control: private|no-store` are not cached (H1).
- Cross-origin registry requests are rejected (M11); the Studio proxy strips
  gateway CORS headers (M12); client-supplied request IDs cannot overwrite
  stored records (M5); retries refuse to re-send non-rewindable bodies (M1);
  per-key rate-limit tables are bounded LRU (M3); JWKS refreshes use a
  time-bounded client (M4); request-record writes go through a bounded async
  queue (M6); literal `response.status` integers are honored and validated
  (M7); session cookies honor `X-Forwarded-Proto` for `Secure` (M8); the
  oauth2 validate-only strategy fails cleanly instead of panicking (M10).

### Hardening (Containers and operations)

- Docker image runs as the distroless non-root user, built on `node:24`
  (H7, M16); `.dockerignore` keeps the build context small.
- Real HTTP container healthchecks via `restitch healthcheck <url>` and
  `restitch-studio -healthcheck <url>` (M17).
- The docker-compose quickstart works again: `/mockupstream` is in the image
  and services select their binary with `entrypoint` (H12).

### Hardening (Verification integrity)

- Four gate scripts no longer record PASS vacuously (H8).
- The CI coverage gate uses `awk` instead of `bc` (H6); actions are pinned to
  commit SHAs with Dependabot (M20); jobs declare permissions and timeouts
  (M23); coverage and test results are uploaded (M24); the e2e job runs with
  `-race` and a timeout (M22); `--passWithNoTests` is gone (M18).
- Frontend: TypeScript `strict` + `noUncheckedIndexedAccess` (M13), a
  top-level error boundary (M14), and fetch error states with retry (M15).

### Rewrite overview

Complete rewrite of the composition engine and addition of Restitch Studio
(this section was drafted under the heading "v2.0.0" before the release was tagged v1.2.0 to match the module path).

### Features

- **Template engine**: Segment-based rendering compiled once at startup. No
  runtime `expr.Compile` calls. Path, query, and JSON escaping built in.
- **Expression environment**: `req.method`, `req.path`, `req.params`,
  `req.query`, `req.query_all`, `req.headers`, `req.body`, `req.auth`.
  `request` alias for `req` (backwards compatible).
- **Partial responses (D1 fix)**: Optional step failures produce HTTP 200 with
  `X-Restitch-Complete: false`, `_errors` array, and `null` for failed step
  references in templates.
- **Inferred dependency skipping (D2 fix)**: Steps whose inferred dependencies
  failed are skipped automatically.
- **Path parameters**: `{id}` in composition paths via Go 1.22 `http.ServeMux`.
- **Per-upstream clients**: One `*http.Client` per upstream built at startup
  with full transport config (HTTP/2, proxy, dial/TLS timeouts).
- **Authorization scoping (D8 fix)**: Only passthrough upstreams receive the
  client's `Authorization` header, via context — no credential leak.
- **Resilience**: Per-upstream retries (status-code-driven, exponential backoff,
  `Retry-After`), circuit breaker (`sony/gobreaker`), per-step response caching
  (TTL, sharded map), request coalescing (singleflight).
- **Inbound authentication**: Gateway-level API keys and JWT/JWKS validation
  with per-composition `public: true` opt-out.
- **Observability**: Prometheus metrics on admin port, structured JSON logging
  with request ID on every line, per-step timing.
- **Admin API**: Separate server on port 9090 with composition info, request
  ring buffer, stats, config validation, and reload endpoints.
- **Hot reload**: Validate-then-atomic-swap via `atomic.Pointer`. Triggers:
  SIGHUP, fsnotify file watch, `POST /admin/api/reload`.
- **CLI subcommands**: `restitch run`, `restitch check` (config validation with
  wave printout), `restitch version`.
- **Restitch Studio**: React SPA with Dashboard, Requests, and Config pages.
  Embedded in `restitch-studio` binary, proxies to gateway admin API.
- **Response size cap (D14)**: `max_response_bytes` per upstream (default 10 MiB).
- **Strict env expansion (D7)**: `${VAR}`, `${VAR:default}`, `$$` for literal
  `$`. Bare `$` is an error.
- **OAuth2 hardening (D6)**: Dedicated 10s-timeout refresh client.

### Breaking Changes

- Removed `/health/upstreams` from the data port (moved to admin API at
  `GET /admin/api/upstreams`).
- Removed `/ping` endpoint.
- `/health` returns `{"status":"ok"}` only (version/memory/uptime moved to
  admin API).
- `internal/client` package deleted (absorbed into `internal/upstream`).

### Bug Fixes

- D1: Optional step failure no longer causes 500 in response templates.
- D2: Inferred dependencies are now checked (not just explicit `depends_on`).
- D3: `request.*` and `req.*` both work; README examples use `req.*`.
- D4: All expressions compiled at startup, never at request time.
- D5: Step/upstream timeouts no longer capped by shared client timeout.
- D6: OAuth2 refresh bounded to 10s; hung IdP cannot block forever.
- D7: Strict `${VAR}` expansion; bare `$` rejected at startup.
- D8: `Authorization` header scoped to passthrough upstreams only.
- D9: `go vet` clean (context.WithTimeout cancel no longer discarded).
- D10: Hand-rolled router replaced with `http.ServeMux`.
- D11: Config file permission errors no longer silently ignored.
- D12: Uniform slog JSON logging with request ID on every line.
- D13: Path/query values escaped; JSON bodies properly encoded.
- D14: Unbounded `io.ReadAll` replaced with `io.LimitReader`.
- D15: Dead code deleted (NoneStrategy, TLSConfig struct, trimSpaces, etc.).
- D16: `/ready` set after `net.Listen`; health checks use per-upstream client.
- D17: `writeError` never leaks internal error text to clients.
- D18: HTTP/2 enabled, proxy support, dial/TLS timeouts configured.
- D19: `req.body` and `req.params` implemented.
- D20: Server package and handler now have tests; E2E test harness added.
