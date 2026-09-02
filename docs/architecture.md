# Architecture

Two processes: the gateway (data plane) and Studio (control plane). They
share no memory; Studio talks to the gateway's admin API over HTTP.

## Request lifecycle

1. A client request hits the gateway data port.
2. Middleware assigns a request ID, logs the request, and applies the global
   rate limiter when configured.
3. The composition router matches the path. Inbound auth runs unless the
   composition is `public: true`; per-composition rate limits apply.
4. The executor runs the composition's step DAG wave by wave. Steps in a
   wave run in parallel. Each step has its own upstream client with retry,
   circuit breaker, optional response cache, and optional coalescing.
   Templated path, query, body, and header values evaluate against the
   request environment; a step failure marks dependents skipped.
5. The response template evaluates against step results and optional
   `_errors`, and the merged response is written.
6. Request recording (ring buffer, stats, durable storage) happens after the
   response is written.

## Configuration sources

- **File mode**: the gateway reads `restitch.yaml` and watches it for
  changes (SIGHUP, file watch, or `POST /admin/api/reload` also trigger a
  reload). Reloads validate-then-swap atomically.
- **Registry mode**: the gateway polls Studio's config registry
  (`restitch run -registry-url http://studio:3080 -registry-key <key>`).
  Studio serves active configs as one YAML bundle with an ETag; unchanged
  bundles cost a 304. A failing poll keeps the last known-good config.

## Trust boundaries

- The **admin API** (gateway) requires `admin.api_key`. Without a key every
  request is rejected, including preflights. It binds loopback by default.
  `GET /metrics` and `GET /health` are the only open endpoints.
- The **Studio registry API** requires the key configured via
  `-registry-key`; the gateway sends the same value as `X-Admin-Key`.
  Studio binds loopback by default.
- The **browser preferences API** is cookie-bound only. It stores UI state,
  never credentials.
- Studio's proxy attaches the admin key server-side; the browser never sees
  it, and gateway CORS headers are stripped at the Studio boundary.
- `${VAR}` expansion in configs runs in the gateway's process environment:
  a config (file or registry bundle) is as trusted as the machine's
  environment.

## Design notes

- Expressions compile once at startup, never at request time.
- All upstream and request bodies are read through `io.LimitReader`; JSON
  schema validation guards composition request bodies.
- The per-step cache keys include the evaluated step headers; responses with
  `Set-Cookie` or a private `Cache-Control` are never cached.
- The pipeline swap uses an `atomic.Pointer`; reload never blocks requests.
