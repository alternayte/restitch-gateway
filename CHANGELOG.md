# Changelog

## v2.0.0

Complete rewrite of the composition engine and addition of Restitch Studio.

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
