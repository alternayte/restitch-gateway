# Changelog

## v2.0.0

Complete rewrite from v1.0 MVP to production-grade gateway + Studio UI.

### Features

- **Template engine**: Pre-compiled segment-based rendering with path/query/JSON escaping
- **Partial responses**: `X-Restitch-Complete` header, nil-safe template evaluation (D1 fix)
- **Path parameters**: `{param}` in composition routes via Go 1.22 `http.ServeMux`
- **Request body**: `req.body` available in expressions for POST/PUT compositions
- **Per-upstream clients**: Hardened transports with HTTP/2, proxy, dial/TLS timeouts
- **Authorization scoping**: `Authorization` header only propagated to passthrough upstreams (D8 fix)
- **Resilience**: Retry with backoff/jitter/Retry-After, circuit breaker (gobreaker), request coalescing, per-step TTL caching
- **Inbound auth**: Gateway-level API key and JWT/JWKS validation with per-composition `public: true`
- **Observability**: Prometheus metrics, admin API on separate port, request ring buffer, stats
- **Hot reload**: SIGHUP, fsnotify, `POST /admin/api/reload` with validate-then-swap
- **CLI**: `restitch run|check|version` subcommands
- **Studio**: React + Tailwind v4 SPA with Dashboard, Requests, Config pages
- **Strict env expansion**: `${VAR}`, `${VAR:default}`, `$$` escape, error on bare `$` (D7 fix)

### Bug fixes

- D1: Optional step failure no longer causes 500 — partial responses work correctly
- D2: Inferred dependencies from expressions are now tracked and skipped properly
- D3: `request.*` alias works alongside `req.*`
- D4: All expressions compiled at startup, zero runtime compilation
- D5: Per-step timeouts via context, not shared client timeout
- D6: OAuth2 refresh bounded to 10s with dedicated client
- D7: Strict `${VAR}` expansion, reject bare `$`
- D8: Authorization header scoped to passthrough upstreams only
- D9: `go vet` clean — context leak in ShutdownContext fixed
- D10: Hand-rolled router replaced with `http.ServeMux`
- D11: Config file permission errors no longer silently ignored
- D12: Unified slog JSON logging with request ID injection
- D13: Path/query values properly escaped in upstream requests
- D14: Response size cap via `io.LimitReader` (default 10 MiB)
- D15: Dead code removed (NoneStrategy, TLSConfig, /ping, trimSpaces)
- D16: Ready set after listen; upstream health moved to admin port
- D17: Error responses never leak internal error text
- D18: HTTP/2, proxy support, dial/TLS timeouts in upstream transport
- D19: `req.body`, `req.params`, full query values implemented
- D20: Server and composition packages now have comprehensive tests

### Breaking changes

- `/health/upstreams` removed from data port (now at `GET /admin/api/upstreams` on port 9090)
- `/ping` route removed
- `server.auth` block format for inbound authentication (new feature, not a migration)
