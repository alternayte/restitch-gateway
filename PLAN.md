# Restitch — Master Implementation Plan

This document is the single source of truth for taking Restitch from its current
state (v1.0 MVP, audited 2026-07-08) to the complete, feature-finished product:
a production-grade REST API composition gateway plus the Restitch Studio web UI.

## 0.1 Completion Status (audited 2026-07-09)

All milestones M1–M19 (excluding deferred WASM M16) have been
implemented and verified against the codebase.

| Milestone | Tasks | Status |
|-----------|-------|--------|
| M1 — Foundation, tooling, hygiene | T1.1–T1.6 | DONE |
| M2 — Expression/template engine rewrite | T2.1–T2.4 | DONE |
| M3 — Executor semantics, partial-response | T3.1–T3.4 | DONE |
| M4 — Router replacement, path parameters | T4.1–T4.3 | DONE |
| M5 — Upstream clients, auth, config | T5.1–T5.6 | DONE |
| M6 — Resilience: retry, breaker, coalesce | T6.1–T6.3 | DONE |
| M7 — Per-step response caching | T7.1 | DONE |
| M8 — Observability: metrics, admin server | T8.1–T8.3 | DONE |
| M9 — Inbound authentication | T9.1 | DONE |
| M10 — Hot reload + pipeline swap | T10.1 | DONE |
| M11 — CLI subcommands | T11.1–T11.3 | DONE |
| M12 — Studio backend | T12.1 | DONE |
| M13 — Studio frontend | T13.1–T13.8 | DONE |
| M14 — E2E test harness, hardening | T14.1–T14.3 | DONE |
| M15 — Docs, examples, packaging | T15.1–T15.4 | DONE |
| M16 — WASM plugins | — | DEFERRED |
| M16 — Production Hardening (addendum) | T16.1–T16.10 | DONE |
| M17 — Rate Limiting & Validation | T17.1–T17.5 | DONE |
| M18 — OpenTelemetry Tracing | T18.1–T18.5 | DONE |
| M19 — CI & Test Hardening | T19.1–T19.5 | DONE |
| M20 — Config Registry & Centralized Management | T20.1–T20.6 | DONE |
| M21 — Gateway Registry Polling | T21.1–T21.5 | DONE |
| M22 — Dev Mode Orchestrator | T22.1–T22.5 | DONE |
| M23 — Upstream HTTP Client Optimization | T23.1–T23.3 | DONE |

Known drift from plan (see Addendum A1): Pipeline not moved to
`internal/server/` (A1.7), extra admin endpoints and deps not in
original schema (A1.1–A1.6). These are documentation-only items.

All critical production gaps (A2.1–A2.4) and important production
gaps (A3.1, A3.2, A3.4, A3.6, A3.8, A3.9) fixed in M16. CI gaps
(A1.9 e2e job, A1.10 continue-on-error) fixed in M19.

## 0. How to use this plan (read this first, executor)

- You see ONLY this file and the code. Everything you need is inline. Do not
  invent scope; do not skip acceptance checks.
- Work milestone by milestone, in order (M1 → M15). Within a milestone, tasks
  are numbered `T<milestone>.<n>` and list their dependencies explicitly. A task
  with no listed dependency depends only on the previous milestone's gate.
- After EVERY task: run `go build ./... && go vet ./...` (must be clean) and the
  task's acceptance check. After every milestone: run the milestone's
  **Verification gate** exactly as written and compare against the expected
  results before moving on.
- Commit after each task with message `feat(M<m>): <task title>` (or `fix:`/
  `test:`/`docs:` as appropriate). Never commit a failing build.
- When a task says "delete", delete — do not comment out or keep fallbacks.
- Code style: match the existing codebase (Go stdlib style, table-driven tests,
  `internal/` packages). Keep doc comments factual and short; do not copy the
  old code's habit of citing "CONTEXT.md/RESEARCH.md" in comments.
- The module path is `github.com/restitch/restitch-gateway`. Go version in
  go.mod is `1.25.6` (Go ≥ 1.22 semantics available, e.g. `http.ServeMux`
  method+wildcard patterns, `slices`, `maps` packages).
- All new third-party dependencies are pre-decided in §3.3. Do not add others.
- If a referenced line number has drifted slightly, locate the code by the
  quoted identifier/function name instead.

## 1. Project context

### 1.1 What Restitch is

Restitch is a REST API composition gateway that replaces hand-written
Backend-for-Frontend (BFF) services. Platform teams declare "compositions" in
YAML: each composition is an HTTP route whose handler executes a DAG of
upstream REST calls ("steps"), with the [expr-lang](https://expr-lang.org)
expression language (`{{ ... }}` templates) wiring request data and earlier
step results into later steps and into a response template. Steps in the same
DAG wave run in parallel. Per-upstream auth (header / basic / passthrough /
OAuth2 client-credentials) is applied transparently. Failures of `optional`
steps degrade gracefully into partial responses.

Two deliverables:
- **`restitch`** (exists, needs completion) — the Go data-plane gateway.
- **`restitch-studio`** (new) — control-plane web UI: dashboard, composition
  DAG visualization, request explorer, config validation, visual composition
  builder. React + TypeScript + Tailwind CSS v4 + shadcn/ui, served by a small
  Go binary that embeds the built SPA and proxies to the gateway's admin API.

### 1.2 Current package map (before this plan)

```
cmd/restitch/main.go            flag parsing, wiring, dual HTTP/HTTPS serve
internal/client/                shared http.Client (pooling), DrainAndClose
internal/auth/                  Strategy interface + header/basic/passthrough/oauth2
internal/config/env.go          ${VAR} expansion with (flawed) validation
internal/observability/         ULID request IDs + middleware
internal/server/                hand-rolled router, middleware, health, tls, shutdown
internal/composition/           config structs, YAML parser, expr compile,
                                DAG builder, executor, step HTTP calls,
                                response templating, HTTP handler
```

### 1.3 Confirmed defects this plan fixes (audit 2026-07-08)

Referenced by ID from tasks below.

| ID | Defect | Where |
|----|--------|-------|
| D1 | Optional-step failure → 500. Response templates referencing a failed optional step error out (`cannot fetch body from <nil>`), so the flagship partial-response feature never works. | `internal/composition/response.go`, `step.go` |
| D2 | Inferred dependencies don't skip. `checkDependenciesFailed` reads only explicit `depends_on`; dependencies inferred from expressions are computed in `BuildDAG` then discarded (`ExecutionPlan` keeps only waves). Dependent steps run against nil and fail; if required, the whole request 502s. | `internal/composition/executor.go:266`, `dag.go` |
| D3 | Docs/engine mismatch: README uses `request.*`, engine defines only `req.*`. Every README example fails at runtime. | README.md, `internal/composition/expr.go` |
| D4 | "Compiled at parse time" is false: template strings (`parser.go` `compileTemplateString` TODO) and all response-body expressions (`response.go` `evaluateExpressionString`) call `expr.Compile` on every request. `CompiledResponse.BodyExprs` is built and never read. | `internal/composition/parser.go`, `response.go`, `step.go` |
| D5 | Step/upstream timeouts > 30s silently capped by shared `http.Client.Timeout` (30s). | `internal/client/client.go:57`, `step.go:137` |
| D6 | OAuth2 token refresh uses startup context + `http.DefaultClient`, no timeout; a hung IdP blocks all requests to that upstream forever behind singleflight. | `internal/auth/oauth2.go` |
| D7 | Env expansion: validation only matches `${VAR}` but `os.ExpandEnv` also expands `$VAR`, silently mangling secrets containing `$`. No escape for literal `$`. | `internal/config/env.go` |
| D8 | Client `Authorization` header is propagated to EVERY upstream (even unauthenticated third parties) — credential exfiltration. | `internal/composition/step.go` `propagateHeaders` |
| D9 | `go vet` fails: `context.WithTimeout` cancel discarded in `ShutdownContext`. | `internal/server/shutdown.go:97` |
| D10 | Router holds RWMutex across entire handler execution; middleware chain rebuilt per request; no path params; O(n) matching; HEAD → 405. Routing logic duplicated again in `composition.Handler.matchComposition`. | `internal/server/router.go`, `composition/handler.go` |
| D11 | `os.Stat(configFile)` treats ANY error (e.g. EACCES) as "no config" and boots a gateway with zero compositions. | `cmd/restitch/main.go:54` |
| D12 | `-log-format=json` produces MIXED output: slog default (text) + hand-rolled JSON middleware. Request ID missing from executor/step logs. | `cmd/restitch/main.go`, `internal/server/middleware.go` |
| D13 | No URL escaping of interpolated path values (`?id=1/../admin` rewrites the upstream path); string-interpolated JSON bodies break on quotes. | `internal/composition/step.go` `interpolateTemplate` |
| D14 | Unbounded `io.ReadAll` of upstream responses (memory exhaustion). | `internal/composition/step.go` |
| D15 | Dead code: `NoneStrategy`, `WaitForShutdown`, `TLSConfig` struct, `/ping` route, `trimSpaces` (reimplements `strings.TrimSpace`), duplicated shutdown paths, `oklog/ulid` marked `// indirect`. | various |
| D16 | `/health/upstreams` unauthenticated fan-out (amplification + internal URL disclosure); health checks bypass upstream auth so secured upstreams read "unhealthy"; `ready` set before port bind. | `internal/server/health.go`, `server.go` |
| D17 | `writeError` leaks internal error text to clients. | `internal/composition/handler.go` |
| D18 | Transport gaps: HTTP/2 silently disabled (custom TLSClientConfig w/o `ForceAttemptHTTP2`), no proxy support, no dial/TLS-handshake timeouts, `Transport()` unchecked type assertion. | `internal/client/client.go` |
| D19 | `req.body` unimplemented (TODO); only first query value visible; no `req.params` (no path params). | `internal/composition/step.go` `buildRequestEnv` |
| D20 | `internal/server` and `cmd/restitch` have zero tests; both headline bugs (D1, D2) untested; no E2E tests. | tests |

### 1.4 What already works (do not regress)

- YAML parse + startup validation of step names, upstream references, cycles.
- Wave-based parallel execution (Kahn's algorithm levels) with per-step
  timeout hierarchy (step > upstream > 30s default).
- Auth strategies as `http.RoundTripper` wrappers; OAuth2 initial fetch
  fail-fast; passthrough 401 contract (`WWW-Authenticate: Bearer`).
- Error rules (`error_rules: [{statuses: [404], body: ...}]`) replacing step
  bodies; `_errors` array; `X-Partial-Response` header (contract kept, see
  §2.4 for its successor).
- Graceful shutdown with `/ready` flip; TLS termination; ULID request IDs.
- The existing test suite passes: keep it passing except where a task
  explicitly says a behavior changes (update those tests in the same task).

## 2. Target end state

### 2.1 Feature list (complete product)

Gateway:
1. Correct composition semantics: partial responses that actually work (D1),
   inferred-dependency skipping (D2), documented env with `request` + `req`
   aliases (D3), single compile-at-startup expression engine (D4, D13).
2. Path parameters in composition routes (`/api/users/{id}` → `req.params.id`)
   via Go 1.22 `http.ServeMux`.
3. Request body access (`req.body`) for JSON POST/PUT compositions.
4. Per-upstream HTTP clients with full transport config (timeouts, pools,
   HTTP/2, proxy) built once at startup; auth-scoped `Authorization`
   propagation (D8).
5. Resilience: per-upstream/per-step retries (status-code-driven policy,
   `Retry-After`, idempotent-only by default), per-upstream circuit breaker,
   per-step request coalescing (singleflight), per-step response caching (TTL).
6. Inbound auth: gateway-level API-key and JWT (JWKS) validation with
   per-composition `public: true` opt-out.
7. Observability: uniform slog JSON logging with request ID on every line,
   Prometheus `/metrics`, access log with gateway-vs-upstream latency split,
   auth-aware cached upstream health.
8. Ops: `restitch check` (validate + boot smoke test), `restitch import
   openapi` (scaffold YAML from an OpenAPI 3 spec), `RESTITCH_*` env
   overrides, hot config reload (SIGHUP + file watch + admin endpoint) with
   validate-then-atomic-swap, admin API on a separate port.
9. Packaging: Makefile, Dockerfile, docker-compose demo, GitHub Actions CI,
   golangci-lint, LICENSE (Apache-2.0), examples/, rewritten README + docs/.

Studio:
10. `restitch-studio` binary serving an embedded React SPA: Dashboard,
    Compositions (list + DAG graph + YAML), Requests explorer (ring buffer +
    step waterfall), Config (viewer + validate), visual composition Builder
    (form → YAML). Tailwind v4 + shadcn/ui.

Deferred (M16, design note only — no implementation tasks): WASM plugins.

### 2.2 Target package map (after this plan)

```
cmd/restitch/                 main.go (subcommand dispatch), run.go, check.go,
                              version.go, importcmd.go
cmd/restitch-studio/          main.go (embedded SPA + admin proxy)
cmd/mockupstream/             dev/demo mock upstream server
internal/gwconfig/            root config: file schema (server/admin/telemetry/
                              upstreams/compositions), env overrides, strict
                              ${VAR} expansion, JSON-schema-less struct
                              validation, config hash
internal/composition/         parser, template engine (rewritten), dag,
                              executor, response, handler, errors
internal/upstream/            compiled upstream: transport build, retry RT,
                              circuit breaker RT, cache, coalescing, health
internal/auth/                strategies (kept, fixed)
internal/inbound/             inbound auth middleware (api key, JWT/JWKS)
internal/server/              server, mux registration, middleware, tls, shutdown
internal/admin/               admin API server + request ring buffer + stats
internal/observability/       requestid, logging (slog setup + ctx handler),
                              metrics (prometheus)
internal/testenv/             in-process E2E harness
studio/                       React app (Vite + TS + Tailwind v4 + shadcn/ui)
tests/specs/*.json            golden E2E specs run against the real binary
examples/                     example configs + docker-compose demo
docs/                         topic docs
```

`internal/client` and `internal/config` are absorbed into `internal/upstream`
and `internal/gwconfig` respectively and deleted by the end of M5. The old
`internal/server/router.go` is deleted in M4.

### 2.3 Final configuration schema (normative reference)

Tasks refer to this schema. Field names are exact. Durations are Go strings
(`"5s"`, `"250ms"`). `${VAR}` / `${VAR:default}` expansion applies to all
string values; `$$` escapes a literal `$` (see T5.6).

```yaml
# restitch.yaml — full schema
server:                      # all optional; flags/env override (see §2.5)
  port: 8080
  tls_port: 8443
  tls_cert: ""               # path; TLS enabled iff cert+key set
  tls_key: ""
  read_timeout: 10s
  write_timeout: 30s
  shutdown_timeout: 30s
  log_format: json           # json | text
  log_level: info            # debug | info | warn | error
  auth:                      # inbound auth (M9); omitted = no inbound auth
    api_keys: ["${GATEWAY_KEY}"]   # any match on X-API-Key header passes
    jwt:
      jwks_url: "https://issuer/.well-known/jwks.json"
      issuer: "https://issuer"      # optional; validated if set
      audience: "restitch"          # optional; validated if set

admin:                       # admin API + /metrics (M8/M10); omitted = enabled on 9090 without auth
  enabled: true
  port: 9090
  api_key: "${ADMIN_KEY}"    # optional; if set, X-Admin-Key required
  request_log_size: 500      # ring buffer entries

upstreams:
  users-api:
    url: "https://users.internal"
    timeout: 10s             # default per-step timeout for this upstream
    health_path: "/health"   # "" = HEAD to base URL
    max_response_bytes: 10485760   # per-response read cap; default 10 MiB
    transport:               # all optional (M5)
      dial_timeout: 5s
      tls_handshake_timeout: 5s
      response_header_timeout: 10s
      max_idle_conns_per_host: 100
      insecure_skip_verify: false
    auth:                    # unchanged from v1.0 (one of):
      header: {name: "X-API-Key", value: "${API_KEY}"}
      basic: {username: "${U}", password: "${P}"}
      passthrough: {}
      oauth2: {token_url: "...", client_id: "${ID}", client_secret: "${S}", scopes: [a, b]}
    retry:                   # optional (M6)
      max_attempts: 3        # total attempts including the first
      interval: 250ms        # initial backoff
      max_backoff: 5s
      backoff_on: [429, 502, 503, 504]   # retry these statuses (and network errors)
      drop_on: []            # give up immediately on these statuses
      retry_non_idempotent: false        # POST etc. retried only if true
    circuit_breaker:         # optional (M6)
      max_failures: 5        # consecutive failures to trip
      interval: 60s          # rolling reset of counters
      timeout: 10s           # open → half-open after this

compositions:
  user-dashboard:
    path: "/api/users/{id}/dashboard"   # ServeMux pattern; params via req.params
    method: GET
    public: false            # true = skip inbound auth for this route (M9)
    steps:
      - name: user
        upstream: users-api
        path: "/users/{{ req.params.id }}"
        method: GET
        headers: {X-Tenant: "{{ req.headers['X-Tenant'] }}"}
        body: ""             # request body template (POST/PUT)
        depends_on: []       # explicit deps; merged with inferred
        optional: false
        timeout: 5s
        cache: {ttl: 30s}    # optional per-step response cache (M7), GET only
        coalesce: true       # optional in-flight dedup (M6), GET only
        retry: {...}         # same shape as upstream retry; overrides it
        error_rules:
          - statuses: [404]
            body: null
    response:
      status: 200            # int or "{{ expr }}"
      content_type: application/json
      body:
        user: "{{ steps.user.body }}"
```

### 2.4 Expression environment (normative)

Available in every `{{ ... }}`:

| Variable | Type | Notes |
|----------|------|-------|
| `req.method` | string | incoming method |
| `req.path` | string | incoming URL path |
| `req.params` | map[string]string | path parameters from the route pattern |
| `req.query` | map[string]string | first value per key |
| `req.query_all` | map[string][]string | all values |
| `req.headers` | map[string]string | canonical-cased keys, first value |
| `req.body` | any | parsed JSON body (nil if absent/not JSON) |
| `request` | same object as `req` | alias, fixes D3 |
| `steps.<name>.status` | int | |
| `steps.<name>.headers` | map[string]string | |
| `steps.<name>.body` | any | parsed JSON, or raw string if non-JSON |
| `steps.<name>` | nil | when that step failed or was skipped |

Failure semantics (fixes D1):
- A step that failed or was skipped is `nil` in `steps`.
- Step-level expressions never see nil dependencies: dependents of a
  failed/skipped step are themselves skipped (fixes D2).
- Response-template expressions: evaluation is attempted; if it errors AND the
  expression's compile-time step-dependency set intersects the failed/skipped
  set, the result is `null` (for whole-string expressions) or `""` (inside
  string interpolation). Errors with no failed dependency are real template
  bugs → 500. Users can still write `steps.x?.body?.points ?? 0` for explicit
  defaults.
- Response envelope on any step failure/skip: HTTP 200, headers
  `X-Restitch-Complete: false` and `X-Partial-Response: true` (legacy alias,
  kept), body gains `_errors: [{step, message, status}]` where `status` is
  `"failed"` or `"skipped"` and `message` is sanitized (`"timeout"` /
  `"upstream error"` / `"dependency_failed"`).
- All steps succeeded → `X-Restitch-Complete: true`, no `_errors`.

### 2.5 Config precedence and env overrides

`flags > RESTITCH_* env > YAML file > defaults`. Env override names map to
`server.*` and `admin.*` fields only (not upstreams/compositions):
`RESTITCH_PORT`, `RESTITCH_TLS_PORT`, `RESTITCH_TLS_CERT`, `RESTITCH_TLS_KEY`,
`RESTITCH_LOG_FORMAT`, `RESTITCH_LOG_LEVEL`, `RESTITCH_ADMIN_PORT`,
`RESTITCH_ADMIN_ENABLED`, `RESTITCH_ADMIN_API_KEY`, `RESTITCH_CONFIG`
(config file path).

## 3. Global conventions and decisions record

### 3.1 Decisions (made now; do not re-litigate)

| # | Decision | Rationale |
|---|----------|-----------|
| K1 | Router = stdlib `http.ServeMux` (Go 1.22 patterns). | Path params, method matching, 405/Allow, HEAD-for-GET all built in; deletes ~350 LOC of buggy custom code (D10). Third-party routers add nothing we need. |
| K2 | Rewrite the template engine as pre-parsed segment lists compiled once at startup; runtime never calls `expr.Compile`. | D4/D13 root fix; three divergent evaluation paths were the bug factory. |
| K3 | Nil-handling rule of §2.4 (attempt eval; null only when a failed dep explains the error). | Preserves `??`/`?.` user defaults while making the default experience non-fatal. Deterministic and simple to implement. |
| K4 | `Authorization` is owned by the auth layer: only `passthrough` upstreams receive it, read from request context. | Fixes D8 exfiltration; keeps strategies self-contained. |
| K5 | One `*http.Client` per upstream, built at compile time, `Timeout: 0` (deadlines via per-step context). RoundTripper decorator chain: metrics → retry → breaker → auth → base transport. | Fixes D5/D6/D18; pattern proven in Cosmo router (`core/transport.go`) and KrakenD backend factory. |
| K6 | Retry policy is status-code-driven (`backoff_on`/`drop_on`) honoring `Retry-After`, idempotent-only by default. | Benthos `internal/httpclient` + Cosmo `retrytransport` pattern; richer than a bare count. |
| K7 | Circuit breaker: `sony/gobreaker/v2`, one breaker per upstream. | Widely used, tiny API; per-upstream granularity matches KrakenD/Cosmo. |
| K8 | Hot reload: validate + build the complete new pipeline aside; on failure keep the old one; `atomic.Pointer` swap of the handler. Triggers: SIGHUP, debounced fsnotify, `POST /admin/api/reload`. | Cosmo `SwapGraphServer` pattern: a bad config can never take down a healthy gateway. |
| K9 | Admin API is a separate `http.Server` on its own port (default 9090); `/metrics` lives there, not on the data port. | Keeps the data plane surface minimal; fixes D16 exposure. |
| K10 | Inbound auth: static API keys and/or JWT via JWKS (`golang-jwt/jwt/v5` + `MicahParks/keyfunc/v3`). | Standard, small deps; per-composition `public: true` opt-out. |
| K11 | Studio = separate binary `restitch-studio` embedding the built SPA via `go:embed`, proxying `/api/*` to the gateway admin API. | Matches PROJECT.md's two-binary decision; avoids CORS; gateway stays UI-free. |
| K12 | Studio stack: Vite + React 18 + TypeScript + Tailwind CSS **v4** + shadcn/ui; DAG via `@xyflow/react`; YAML via `js-yaml`; editor via CodeMirror 6. | User-specified stack; libraries are the de-facto standards for each job. |
| K13 | Config editing in Studio is validate-and-download only; config deployment stays git-driven. Reload is triggered explicitly. | A web UI writing prod config files is an ops foot-gun; validation + reload covers the real workflow. |
| K14 | License: Apache-2.0. | Open-source → commercial path per PROJECT.md; patent grant matters for infra. |
| K15 | Caching is per-step, in-memory, TTL-based, GET-only, keyed by method+URL+auth identity hash. No Redis/distributed cache. | Single-binary simplicity; CDN remains the answer for big caching. |
| K16 | OpenAPI import generates a YAML scaffold (upstream + one composition per selected operation) for humans to edit; it does not auto-serve specs. | Keeps the feature honest: composition design is the human's job. |
| K17 | Keep wave-based (level-synchronized) DAG execution rather than per-node scheduling. | Simpler; latency cost only when waves are unbalanced; revisit post-v2 if profiles demand. |
| K18 | Legacy YAML without a `server:` block, and `request.*`/`req.*` both, remain valid. `X-Partial-Response` kept as alias of `X-Restitch-Complete: false`. | Zero-breakage upgrade for existing users. |

### 3.2 Error taxonomy (client-visible)

| Condition | Status | Body |
|-----------|--------|------|
| No matching route | 404 | `{"error":"not found"}` |
| Method mismatch | 405 + `Allow` | ServeMux default |
| Rate limit exceeded | 429 + `Retry-After: 1` | `{"error":"rate limit exceeded"}` |
| Request body too large | 413 | `{"error":"request body too large"}` |
| Request body fails JSON Schema | 400 | `{"error":"request validation failed","details":[...]}` |
| Inbound auth missing/invalid | 401 + `WWW-Authenticate` | `{"error":"unauthorized"}` |
| Passthrough upstream, client sent no `Authorization` | 401 + `WWW-Authenticate: Bearer` | `{"error":"authorization header required"}` |
| Required step failed (network, 5xx after retries, breaker open, auth failure) | 502 | `{"error":"upstream error","step":"<name>"}` |
| Required step timeout | 504 | `{"error":"upstream timeout","step":"<name>"}` |
| Response template bug (eval error, no failed dep) | 500 | `{"error":"internal error"}` |
| Optional failures only | 200 + `X-Restitch-Complete: false` | template output + `_errors` |

Never include Go error strings, URLs, or env var names in client bodies (D17).
Full detail goes to logs only.

### 3.3 Approved new dependencies

Go: `github.com/prometheus/client_golang`, `github.com/sony/gobreaker/v2`,
`github.com/fsnotify/fsnotify`, `github.com/golang-jwt/jwt/v5`,
`github.com/MicahParks/keyfunc/v3`, `github.com/getkin/kin-openapi`,
`modernc.org/sqlite`, `golang.org/x/time/rate`,
`github.com/santhosh-tekuri/jsonschema/v6`,
`go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/sdk`,
`go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`.
Already present: `expr-lang/expr`, `google/uuid`, `oklog/ulid/v2`,
`golang.org/x/sync`, `golang.org/x/oauth2`, `gopkg.in/yaml.v3`.

Studio (npm): `react`, `react-dom`, `react-router-dom`, `tailwindcss` (v4),
`@tailwindcss/vite`, shadcn/ui (CLI-managed), `@xyflow/react`, `js-yaml`,
`@uiw/react-codemirror`, `@codemirror/lang-yaml`, `lucide-react`,
`recharts`, dev: `vite`, `typescript`, `@vitejs/plugin-react`, `vitest`,
`@testing-library/react`, `jsdom`.

---

# Milestone M1 — Foundation, tooling, hygiene [DONE]

Goal: a clean, lintable, CI-guarded baseline; kill dead code and the wiring
bugs that don't require redesign. No behavior redesign here.

### T1.1 Fix `go vet` failure and consolidate shutdown (D9, D15)
- Files: `internal/server/shutdown.go`, `cmd/restitch/main.go`.
- Delete `WaitForShutdown()` and `ShutdownContext()` entirely. Keep
  `WaitForShutdownSignal()` but move `signal.Notify` OUT of the goroutine
  (register before `go func()` so no signal is dropped). Remove the
  `fmt.Println("shutdown complete")` from `Shutdown()` (keep the one in main's
  flow, converted to `slog.Info`).
- In `cmd/restitch/main.go` replace `srv.Shutdown(srv.ShutdownContext())` with:
  ```go
  ctx, cancel := context.WithTimeout(context.Background(), server.ShutdownTimeout)
  defer cancel()
  if err := srv.Shutdown(ctx); err != nil { ... }
  ```
- Accept: `go vet ./...` exits 0. `grep -rn "ShutdownContext\|WaitForShutdown(" internal cmd` → only `WaitForShutdownSignal` remains.

### T1.2 Delete dead code (D15)
- Delete: `NoneStrategy`/`NewNoneStrategy` in `internal/auth/auth.go` (and any
  tests referencing them); unused `TLSConfig` struct in
  `internal/server/tls.go`; the `/ping` route in `cmd/restitch/main.go`;
  `trimSpaces` in `internal/composition/parser.go` (replace call sites with
  `strings.TrimSpace`).
- Run `go mod tidy` (fixes the `oklog/ulid // indirect` mislabel).
- Accept: `go build ./... && go test ./...` green; `grep -rn "NoneStrategy\|trimSpaces" internal` → empty.

### T1.3 Unified slog logging (D12, part 1)
- New file `internal/observability/logging.go`:
  ```go
  // Setup configures the global slog default.
  // format: "json"|"text"; level: "debug"|"info"|"warn"|"error".
  func Setup(format, level string) error
  // ContextHandler wraps a slog.Handler and injects request_id from ctx.
  type ContextHandler struct{ slog.Handler }
  func (h ContextHandler) Handle(ctx context.Context, r slog.Record) error
  ```
  `Setup` builds `slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})`
  (or TextHandler), wraps it in `ContextHandler`, `slog.SetDefault`.
  `ContextHandler.Handle` calls `observability.GetRequestID(ctx)`; if non-empty,
  `r.AddAttrs(slog.String("request_id", id))`.
- Call `observability.Setup(*logFormat, *logLevel)` first thing in main (add a
  `-log-level` flag, default `info`). Delete the hand-rolled JSON encoding in
  `internal/server/middleware.go` `NewLoggingMiddleware`: reimplement it as a
  thin middleware that calls `slog.InfoContext(r.Context(), "request", ...)`
  with fields `method, path, status, duration_ms, remote_addr, user_agent`.
  Keep the `responseWriter` status capture but add pass-through `Flush()`
  (implement `http.Flusher` when the wrapped writer supports it).
- IMPORTANT: every `slog.Info/Warn/Error/Debug` call in
  `internal/composition/{executor,handler,step}.go` becomes the `Context`
  variant (`slog.InfoContext(ctx, ...)`) so request IDs appear (full sweep —
  grep for `slog.` in that package). Where a function lacks `ctx`, thread it.
  Remove the duplicate "composition complete" log from `executor.Execute`
  (handler logs it).
- Accept: `go run ./cmd/restitch -log-format=json 2>&1 | head -5` — every line
  is valid JSON (pipe through `python3 -c 'import json,sys; [json.loads(l) for l in sys.stdin]'`).
  A request to a composition logs step lines containing `"request_id"`.

### T1.4 Config-file stat correctness (D11)
- `cmd/restitch/main.go`: replace the `os.Stat` check with:
  ```go
  cfg, err := composition.LoadConfigFile(*configFile)
  if errors.Is(err, fs.ErrNotExist) { slog.Warn("no composition config found", ...) }
  else if err != nil { slog.Error(...); os.Exit(1) }
  ```
  (`LoadConfigFile` already wraps the ReadFile error; ensure it wraps with
  `%w` so `errors.Is` works — it does.)
- Accept: `touch /tmp/x.yaml && chmod 000 /tmp/x.yaml && go run ./cmd/restitch -config /tmp/x.yaml`
  exits non-zero with a permission error (not a silent boot).

### T1.5 Makefile, lint config, LICENSE, mock upstream
- `Makefile` targets: `build` (`go build -o bin/restitch ./cmd/restitch`),
  `test` (`go test ./...`), `race` (`go test -race ./...`), `vet`, `lint`
  (`golangci-lint run`), `run`, `ci` (vet+lint+race).
- `.golangci.yml`: enable `govet, errcheck, staticcheck, unused, ineffassign,
  misspell, unconvert`; disable nothing else exotic. Exclude `studio/`.
- `LICENSE`: Apache-2.0, copyright "Restitch Authors".
- `cmd/mockupstream/main.go` (~90 LOC): flags `-port` (default 8081). Routes:
  `GET /users/{id}` → `{"id":<id>,"name":"user-<id>","active":true}`;
  `GET /orders` → echo `userId` query into `[{"id":1,"userId":...,"total":9.5}]`;
  `GET /slow?ms=N` → sleeps N ms then `{"ok":true}`;
  `GET /status/{code}` → responds with that status, body `{"status":<code>}`;
  `ANY /echo` → JSON `{method, path, query, headers, body}`. All
  `Content-Type: application/json`. Used by examples, manual gates, and specs.
- Accept: `make ci` passes locally (install golangci-lint if absent:
  `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`).
  `go run ./cmd/mockupstream & curl -s localhost:8081/users/7` returns the JSON.

### T1.6 GitHub Actions CI
- `.github/workflows/ci.yml`: on push/PR. Jobs: (1) `go`: setup-go from
  go.mod, `go vet ./...`, golangci-lint-action, `go test -race ./...`,
  `go build ./...`; (2) `studio` (added in M13; create the job now with an
  `if: hashFiles('studio/package.json') != ''` guard running
  `npm ci && npm run build` in `studio/`).
- Accept: `git push` → workflow file is valid (run `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml'))"`).

### M1 Verification gate
```bash
make ci                         # vet + lint + race all green
go run ./cmd/mockupstream &     # then:
go run ./cmd/restitch -config restitch.yaml -log-format=json 2>&1 | head -3
# → all-JSON log lines, no text-format lines mixed in
```

---

# Milestone M2 — Expression/template engine rewrite [DONE]

Goal: one evaluation core, everything compiled at startup, nil-safe, escaped.
Fixes D1 (template half), D3, D4, D13, D19. This milestone touches only
`internal/composition/`.

Design (implement exactly):

```go
// internal/composition/template.go

// Template is a parsed "{{ expr }}"-bearing string, compiled once.
type Template struct {
    Raw      string
    Segments []Segment     // alternating literals and expressions
    Deps     []string      // union of step names referenced by all segments
    IsSingle bool          // whole string is exactly one expression
}
type Segment struct {
    Literal string         // set when Program == nil
    Program *vm.Program    // compiled expr
    Expr    string         // original expression text (for errors)
}

type EscapeMode int
const (
    EscapeNone EscapeMode = iota  // headers, response strings
    EscapePath                    // url.PathEscape per interpolated value
    EscapeQuery                   // url.QueryEscape per interpolated value
    EscapeJSON                    // JSON-encode the value (bodies)
)

// CompileTemplate parses raw, compiles every {{...}} against env.
func CompileTemplate(raw string, env map[string]any) (*Template, error)

// EvalString renders the template as a string, escaping each evaluated
// expression per mode. Literal segments are never escaped.
func (t *Template) EvalString(env map[string]any, mode EscapeMode) (string, error)

// EvalValue: for IsSingle templates returns the raw evaluated value
// (preserving type); otherwise same as EvalString(env, EscapeNone).
func (t *Template) EvalValue(env map[string]any) (any, error)
```

Parsing: reuse `exprPattern` regex to split into segments; expression text is
`strings.TrimSpace` of the capture. Compile each with the existing
`CompileExpression` (keep `expr.AllowUndefinedVariables()`). `Deps` comes from
the existing `ExtractDependencies` per segment (dedup). `IsSingle` is true iff
after trimming, the string starts with `{{`, ends with `}}`, and contains
exactly one expression segment and no other non-empty literal.

Path escaping rule: when a step path template is compiled, split `Raw` at the
first `?`. Segments falling before it evaluate with `EscapePath`, after it
with `EscapeQuery`. Implement by compiling the path as TWO Templates
(`PathPart`, `QueryPart`) in `CompiledStep`; join at eval time with `?` when
QueryPart is non-nil.

### T2.1 Implement `template.go` + tests
- New files: `internal/composition/template.go`, `template_test.go`.
- Tests (table-driven) must cover: pure literal; single expression preserving
  type (`{{ 1 + 1 }}` → int 2); multi-expression interpolation; extra/odd
  whitespace inside braces (`{{  x }}`, `{{x }}`); value containing `{{` (no
  re-substitution — segment-based rendering makes this impossible by
  construction, assert it); PathEscape (`a/b` → `a%2Fb`); QueryEscape;
  EscapeJSON (string with quotes → valid JSON literal); Deps extraction.
- Accept: `go test ./internal/composition/ -run TestTemplate -v` green.

### T2.2 Runtime environment: params, body, aliases (D3, D19)
- Rewrite `buildRequestEnv` in `step.go` (move to new file `env.go`):
  ```go
  func buildRequestEnv(req *http.Request, params map[string]string,
      body any, stepResults map[string]*StepResult) map[string]any
  ```
  Populates per §2.4: `req.method/path/params/query/query_all/headers/body`,
  sets `env["request"] = env["req"]` (same map — alias), and `steps` (nil for
  failed/skipped). `params` comes from the router (M4 wires `r.PathValue`;
  until then pass empty map). Request body: read in the handler ONCE with
  `io.LimitReader(r.Body, 1<<20)` (1 MiB), JSON-parse when Content-Type
  contains `json`; pass `nil` otherwise. Headers map uses
  `http.CanonicalHeaderKey` keys AND expose the raw `textproto` lookup —
  concretely: build the map with canonical keys, and in
  `BuildBaseEnvironment` document that `req.headers['X-Request-Id']` needs
  canonical casing (README task M15 documents this).
- Update `BuildBaseEnvironment` (compile-time env) to declare the same shape
  including `params`, `query_all`, `body: nil`, and the `request` alias.
- Accept: new test `TestBuildRequestEnv_Aliases` asserting
  `env["request"]` and `env["req"]` are the same map and `req.params` /
  `req.body` populated.

### T2.3 Compile everything at startup; delete runtime compilation (D4)
- `parser.go`: `CompiledStep` becomes:
  ```go
  type CompiledStep struct {
      Step      *Step
      PathPart  *Template   // before '?'
      QueryPart *Template   // after '?', nil if none
      BodyTmpl  *Template   // nil if no body
      Headers   map[string]*Template  // ALL headers compiled (literals too)
      Deps      []string    // resolved inferred+explicit deps (T3.1 consumes)
      Optional  bool
      ErrorRules []ErrorRule
  }
  ```
  Delete `compileTemplateString`, `CompiledExpr` usage for steps, and the
  parser TODO. `compileStep` compiles path (split at first `?`), body,
  every header value via `CompileTemplate`, and sets
  `Deps = MergeDependencies(step.DependsOn, union of all templates' Deps)`.
- `CompiledResponse` becomes:
  ```go
  type CompiledResponse struct {
      StatusTmpl  *Template              // nil if static int
      Body        *CompiledBodyNode      // compiled tree mirroring YAML body
      ContentType string
  }
  type CompiledBodyNode struct {
      Tmpl     *Template                    // leaf string with exprs
      Literal  any                          // leaf non-template value
      Map      map[string]*CompiledBodyNode
      List     []*CompiledBodyNode
  }
  ```
  `compileResponse` walks the YAML body once and builds this tree. Delete
  `BodyExprs`/`BodyTemplate`/`compileBodyExpressions`.
- `step.go`: `evaluatePath/evaluateBody/evaluateHeader/interpolateTemplate`
  are deleted. `ExecuteStepWithTimeout` uses:
  `path := step.PathPart.EvalString(env, EscapePath)`; if QueryPart non-nil,
  `path += "?" + step.QueryPart.EvalString(env, EscapeQuery)`;
  body via `BodyTmpl.EvalValue` then `json.Marshal` when not `IsSingle`... —
  precisely: if `BodyTmpl.IsSingle`, marshal the evaluated value to JSON;
  otherwise `EvalString(env, EscapeJSON)` and use the string bytes as-is.
  Set `Content-Type: application/json` on step requests with a body unless a
  step header overrides it.
- `response.go`: `BuildResponse` walks `CompiledBodyNode` evaluating leaf
  templates via `EvalValue`, applying the K3 nil rule: on eval error, if
  `len(intersect(tmpl.Deps, failedSteps)) > 0` → `nil` (or `""` for non-single
  string templates); else return the error. `BuildResponse` signature gains
  `failedSteps map[string]bool`. Delete `evaluateTemplate`/
  `evaluateExpressionString`.
- Grep-proof: `grep -rn "expr.Compile\|CompileExpression(" internal/composition --include='*.go' | grep -v _test | grep -v template.go` → empty
  (runtime never compiles).
- Update existing tests that called deleted helpers; behavior-equivalent
  cases move onto the new API.
- Accept: full package tests green; the D1 repro (added as a permanent test in
  T3.3) compiles.

### T2.4 Response size cap (D14)
- In `ExecuteStepWithTimeout`, replace `io.ReadAll(resp.Body)` with
  `io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))`; if
  `len(raw) > maxBytes` treat as step failure `fmt.Errorf("response exceeds %d bytes", maxBytes)`.
  `maxBytes` comes from `upstream.MaxResponseBytes` (add field to `Upstream`
  YAML struct, default `10 * 1024 * 1024` applied in `validateAndApplyDefaults`).
- Accept: unit test with an httptest server returning > cap → step fails,
  optional step degrades.

### M2 Verification gate
```bash
go test ./internal/composition/ -count=1 -race     # green
grep -rn "TODO" internal/composition/               # empty
# escaping smoke (mockupstream running on 8081):
cat > /tmp/m2.yaml <<'EOF'
upstreams: {mock: {url: "http://localhost:8081"}}
compositions:
  echo:
    path: "/t"
    steps: [{name: e, upstream: mock, path: "/users/{{ req.query.id }}"}]
    response: {body: {u: "{{ steps.e.body }}"}}
EOF
go run ./cmd/restitch -config /tmp/m2.yaml &
curl -s 'localhost:8080/t?id=1%2F..%2Fadmin' | python3 -m json.tool
# → upstream saw the id PATH-ESCAPED (mockupstream /users/{id} returns id
#   containing the literal "1/../admin" as one segment name, not a traversal)
```

---

# Milestone M3 — Executor semantics and the partial-response contract [DONE]

Goal: fix D1/D2 end-to-end; make degradation observable and correct.
Touches `internal/composition/{dag,executor,handler,errors}.go`.

### T3.1 Resolved dependencies live in the plan (D2)
- `dag.go`: `ExecutionPlan` gains `Deps map[string][]string` (step → resolved
  deps, inferred+explicit). `BuildDAG` populates it from `CompiledStep.Deps`
  (T2.3) instead of re-analyzing expressions (delete `analyzeDependencies` —
  the parser already did the work).
- `executor.go`: `checkDependenciesFailed` takes the plan:
  ```go
  func checkDependenciesFailed(deps []string, results map[string]*StepResult) bool
  ```
  called with `plan.Deps[stepName]`. A dep with `exists && result == nil` OR
  (present in results as nil) means failed/skipped; a dep NOT YET in results
  cannot happen (waves guarantee ordering) — if it does, treat as failed and
  log at error level (invariant breach).
- Accept: permanent regression test `TestExecutor_InferredDependencySkipped`:
  step `a` (optional) fails; step `b` references `steps.a.body.id` in its path
  with NO `depends_on`; assert b's upstream is never called (use httptest hit
  counter), b's timing status is `"skipped"`, composition returns without
  error.

### T3.2 Step status model and error details
- `executor.go`: introduce
  ```go
  type StepStatus string
  const (StepSuccess StepStatus = "success"; StepFailed = "failed"; StepSkipped = "skipped")
  ```
  `StepTiming.Status` uses it. `stepError` gains `skipped bool`.
  `errors.go`: `StepErrorDetail` gains `Status string` json field (`"failed"`
  or `"skipped"`); `BuildErrorsArray` maps `dependency_failed` → status
  `"skipped"`, message `"dependency_failed"`. Timeout stays `"timeout"`,
  others `"upstream error"`.
- Skipped-step semantics: a skipped step whose `Optional` is FALSE is still a
  composition-level failure ONLY IF its failed dependency was required (which
  already failed the composition). Since required-failure fail-fasts the wave
  loop, any skip that happens implies the failed dep was optional → skipped
  steps never fail the composition regardless of their own Optional flag.
  Record this rule as a comment on the skip branch.
- Accept: `_errors` in a partial response contains
  `{"step":"b","message":"dependency_failed","status":"skipped"}` (extend the
  T3.1 test through `BuildResponse`).

### T3.3 Completeness headers + permanent D1 regression test
- `handler.go` `ServeHTTP`: after `BuildResponse` succeeds set
  `X-Restitch-Complete: "true"|"false"`; keep `X-Partial-Response: true` when
  incomplete (K18). Pass `failedSteps` (built from `result.Steps` nil entries)
  into `BuildResponse` (T2.3 signature).
- Add permanent test `TestHandler_PartialResponse_OptionalStepInTemplate`
  reproducing D1 exactly: optional step to a dead upstream
  (`http://127.0.0.1:1`), response template references
  `{{ steps.loyalty.body.points }}` — assert HTTP 200,
  `X-Restitch-Complete: false`, body has `"points": null` and `_errors` with
  the failed step. THIS IS THE HEADLINE FIX — it must pass.
- Accept: that test green; existing handler tests updated for the new header.

### T3.4 Error taxonomy at the handler (D17, §3.2)
- `handler.go`: `writeError` never emits `err.Error()`. Map: timeout
  (`errors.Is(err, context.DeadlineExceeded)` anywhere in the chain) → 504
  `{"error":"upstream timeout","step":name}`; passthrough-missing-auth → 401
  (existing); everything else from Execute → 502
  `{"error":"upstream error","step":name}`. To carry the step name, change
  `Executor.Execute`'s required-failure return to a typed error:
  ```go
  type RequiredStepError struct{ Step string; Err error }
  func (e *RequiredStepError) Error() string; func (e *RequiredStepError) Unwrap() error
  ```
  Template-eval errors (from BuildResponse) → 500 `{"error":"internal error"}`,
  full detail logged with `slog.ErrorContext`.
- Client-cancellation: if `r.Context().Err() != nil` when Execute returns an
  error, log at `Info` ("client canceled") and write nothing further.
- Accept: unit tests for each mapping (dead upstream → 502; `?ms=` slow
  upstream with 50ms step timeout → 504 with step name; broken template with
  healthy steps → 500 generic body).

### M3 Verification gate
```bash
go test ./internal/composition/ -count=1 -race
# Manual (mockupstream on 8081):
cat > /tmp/m3.yaml <<'EOF'
upstreams:
  mock: {url: "http://localhost:8081"}
  dead: {url: "http://127.0.0.1:1"}
compositions:
  partial:
    path: "/p"
    steps:
      - {name: user, upstream: mock, path: "/users/1"}
      - {name: loyalty, upstream: dead, path: "/x", optional: true}
      - {name: bonus, upstream: mock, path: "/users/{{ steps.loyalty.body.id }}", optional: true}
    response:
      body: {user: "{{ steps.user.body }}", points: "{{ steps.loyalty.body.points }}"}
EOF
go run ./cmd/restitch -config /tmp/m3.yaml &
curl -si localhost:8080/p
# Expected: HTTP/1.1 200; X-Restitch-Complete: false; body JSON with
# user populated, "points": null, _errors listing loyalty (failed) and
# bonus (skipped, message dependency_failed). Logs show bonus status=skipped
# and NO request to mockupstream for bonus.
```

---

# Milestone M4 — Router replacement and path parameters [DONE]

Goal: delete the hand-rolled router (D10); routes use `http.ServeMux` Go 1.22
patterns; compositions get `{param}` path parameters.

### T4.1 Replace `internal/server/router.go` with a ServeMux-based registry
- Rewrite `internal/server/router.go` (keep the file name; delete old content):
  ```go
  type Router struct {
      mux         *http.ServeMux
      middlewares []func(http.Handler) http.Handler
      handler     http.Handler   // built once by Finalize
  }
  func NewRouter() *Router
  func (r *Router) Use(mw func(http.Handler) http.Handler)          // pre-Finalize only
  func (r *Router) Handle(method, pattern string, h http.HandlerFunc) // registers "METHOD pattern"
  func (r *Router) Finalize()                                        // wraps mux in middlewares, once
  func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request)
  ```
  `Handle("GET", "/api/users/{id}", h)` → `mux.Handle("GET /api/users/{id}", h)`;
  empty method registers the bare pattern. `Finalize` composes the middleware
  chain ONCE (reverse order, same as before); `ServeHTTP` panics with a clear
  message if `Finalize` wasn't called (catch wiring mistakes early). Call
  `srv.Router().Finalize()` in main after all registrations. Delete
  `allowedMethods`, the mutex-across-dispatch, and prefix-matching (ServeMux
  handles `/prefix/` natively; 405 + `Allow` and HEAD-for-GET are stdlib
  behavior).
- Add `internal/server/router_test.go`: method routing, 405 with Allow header,
  HEAD served by GET handler, `{id}` wildcard reachable via `r.PathValue("id")`,
  middleware order (record slice), Finalize-once semantics.
- Accept: `go test ./internal/server/ -run TestRouter -v` green.

### T4.2 Composition handler: one closure per route; delete duplicate matching
- `internal/composition/handler.go`: delete `routes` map, `matchComposition`,
  `routeKey`. `RegisterRoutes` becomes:
  ```go
  func (h *Handler) RegisterRoutes(router *server.Router) {
      for name, comp := range h.config.Config.Compositions {
          name := name
          router.Handle(comp.Method, comp.Path, func(w http.ResponseWriter, r *http.Request) {
              h.serveComposition(w, r, name)
          })
      }
  }
  ```
  `serveComposition` is the old `ServeHTTP` body minus matching. Extract path
  params: parse `comp.Path` once at registration for `{name}` segments
  (regexp `\{([a-zA-Z_][a-zA-Z0-9_]*)\}`), store `paramNames []string` per
  composition; in the handler build `params[p] = r.PathValue(p)`.
- Validate at parse time (`validateAndApplyDefaults`): composition `path` must
  start with `/`; `{param}` names must be valid identifiers; reject `{...}`
  duplicates; reject paths that ServeMux would panic on (register into a
  throwaway `http.ServeMux` inside a `func() (err error) { defer recover }`
  during CompileConfig — this IS the validation).
- Pass `params` + parsed request body into `buildRequestEnv` (T2.2 signature).
  Read+parse the body once in `serveComposition` before Execute; make it
  available to executor via a new field on a small `RequestData` struct passed
  through instead of `*http.Request`:
  ```go
  type RequestData struct {
      Method, Path string
      Params  map[string]string
      Query   url.Values
      Headers http.Header
      Body    any
      Authorization string          // consumed by M5 passthrough
  }
  func NewRequestData(r *http.Request, params map[string]string, body any) *RequestData
  ```
  `Executor.Execute(ctx, name, rd *RequestData)` and `buildRequestEnv(rd, results)`
  replace the `*http.Request` plumbing (executor must not depend on the live
  request object).
- Accept: composition with path `/api/users/{id}` serves
  `curl localhost:8080/api/users/42` and `{{ req.params.id }}` renders `42`
  (add handler test).

### T4.3 Health/readiness fixes (part of D16)
- `internal/server/server.go`: set `ready` true only AFTER `net.Listen`
  succeeds — split `ListenAndServe` into explicit `net.Listen` +
  `httpServer.Serve(ln)`; flip `SetReady(true)` between them.
- `/health/upstreams` moves to the admin server in M8 (leave a note; no change
  yet). `/health` keeps liveness-only JSON but REMOVE memory stats and
  version from the public payload (return `{"status":"ok"}`); detailed variant
  moves to admin in M8.
- Add `internal/server/health_test.go` + `server_test.go` basics (ready
  before/after listen using a random port).
- Accept: `go test ./internal/server/ -count=1` green (package now has tests —
  closes part of D20).

### M4 Verification gate
```bash
go test ./... -count=1 -race
cat > /tmp/m4.yaml <<'EOF'
upstreams: {mock: {url: "http://localhost:8081"}}
compositions:
  user:
    path: "/api/users/{id}"
    steps: [{name: u, upstream: mock, path: "/users/{{ req.params.id }}"}]
    response: {body: {user: "{{ steps.u.body }}"}}
EOF
go run ./cmd/restitch -config /tmp/m4.yaml &
curl -s localhost:8080/api/users/42 | python3 -m json.tool   # user-42 payload
curl -si -X POST localhost:8080/api/users/42 | head -1        # 405
curl -sI localhost:8080/api/users/42 | head -1                # HEAD → 200
```

---

# Milestone M5 — Upstream clients, auth ownership, config hygiene [DONE]

Goal: one client per upstream built at startup; `Authorization` scoped to
passthrough only; OAuth2 refresh bounded; strict env expansion. Fixes D5, D6,
D7, D8, D18. Creates `internal/upstream/`, deletes `internal/client/`.

### T5.1 `internal/upstream` package: per-upstream client construction
- New `internal/upstream/client.go`:
  ```go
  // TransportConfig mirrors the YAML upstream.transport block (§2.3).
  type TransportConfig struct {
      DialTimeout, TLSHandshakeTimeout, ResponseHeaderTimeout time.Duration
      MaxIdleConnsPerHost int
      InsecureSkipVerify  bool
  }
  // BuildTransport returns a hardened *http.Transport.
  func BuildTransport(tc TransportConfig) *http.Transport
  ```
  Defaults (zero-values replaced): DialTimeout 5s, TLSHandshakeTimeout 5s,
  ResponseHeaderTimeout 0 (unset), MaxIdleConnsPerHost 100. Transport must
  set: `Proxy: http.ProxyFromEnvironment`, `ForceAttemptHTTP2: true`,
  `DialContext: (&net.Dialer{Timeout: dial, KeepAlive: 30*time.Second}).DialContext`,
  `MaxIdleConns: 100`, `IdleConnTimeout: 90*time.Second`,
  `TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: ...}`.
  (fixes D18).
- `Client` per upstream:
  ```go
  type Upstream struct {
      Name    string
      BaseURL string
      Client  *http.Client   // Timeout: 0 — deadlines come from step context (fixes D5)
      MaxResponseBytes int64
      Timeout time.Duration  // default step timeout
      HealthPath string
  }
  // Build assembles the RoundTripper chain (outermost first):
  //   metrics (M8) → retry (M6) → breaker (M6) → auth → transport
  // Each layer is nil-skipped when unconfigured (zero cost).
  func Build(name string, cfg UpstreamYAML, authStrategy auth.Strategy) (*Upstream, error)
  ```
  For M5 the chain is just auth → transport; T6.x and T8.x insert their
  layers inside `Build` behind config checks.
- Move `DrainAndClose` here (`internal/upstream/drain.go`); update the one
  call site in `step.go`. Delete `internal/client/` entirely and its tests
  (port the transport-default assertions into `internal/upstream/client_test.go`).
- `composition/parser.go`: `CompiledUpstream` now embeds `*upstream.Upstream`
  (drop its own Timeout/Auth juggling); `CompileConfig` calls `upstream.Build`.
  `ExecuteStepWithTimeout` drops the `baseClient` parameter and per-request
  client construction (delete the `httpClient := baseClient; if upstream.Auth
  != nil {...}` block) — it uses `upstream.Client` directly.
  `Executor`/`NewHandler` lose their `*http.Client` parameter; update
  `cmd/restitch/main.go` accordingly.
- Accept: `go build ./...`; `grep -rn "internal/client" --include='*.go' .` →
  empty; existing auth integration tests still pass.

### T5.2 Authorization scoping (D8)
- `composition/step.go` `propagateHeaders`: REMOVE `"Authorization"` from the
  propagated list. Propagated headers stay: `X-Request-ID`,
  `X-Correlation-ID`, `traceparent`, `Accept`, `Accept-Language`.
- Passthrough redesign (`internal/auth/passthrough.go`): the strategy reads
  the client's Authorization from the request CONTEXT, not the outgoing
  headers:
  ```go
  type ctxKey struct{}
  // WithClientAuthorization stores the inbound Authorization header value.
  func WithClientAuthorization(ctx context.Context, value string) context.Context
  func clientAuthorization(ctx context.Context) string
  ```
  `passthroughRoundTripper.RoundTrip`: `v := clientAuthorization(req.Context())`;
  empty → `ErrMissingAuthHeader` (unchanged contract); else clone request,
  `Set("Authorization", v)`.
- `composition/handler.go` `serveComposition`: after building `RequestData`,
  `ctx = auth.WithClientAuthorization(ctx, r.Header.Get("Authorization"))`
  before Execute. (`RequestData.Authorization` field from T4.2 is therefore
  unnecessary — remove it.)
- Tests: update passthrough tests to the context flow; ADD the negative test:
  a composition with one passthrough upstream and one no-auth upstream —
  assert the no-auth upstream's received request has NO Authorization header
  while the passthrough one does (use two httptest servers capturing headers).
- Accept: that negative test green (this is the D8 regression guard).

### T5.3 OAuth2 refresh hardening (D6)
- `internal/auth/oauth2.go` `NewOAuth2Strategy`: build a dedicated refresh
  client and bind it to the token source's context:
  ```go
  refreshClient := &http.Client{Timeout: 10 * time.Second}
  tsCtx := context.WithValue(context.Background(), oauth2.HTTPClient, refreshClient)
  baseSource := ccConfig.TokenSource(tsCtx)
  ```
  (golang.org/x/oauth2 uses `oauth2.HTTPClient` from the context for all
  refresh calls; the 10s client timeout bounds a hung IdP.)
- Delete the redundant `singleflight.Group` (ReuseTokenSourceWithExpiry
  already serializes refresh internally); simplify `oauth2RoundTripper` to
  `rt.tokenSource.Token()` directly.
- Add test: token endpoint that hangs 30s; expired token; a request through
  the strategy fails within ~10s (assert elapsed < 15s), not forever.
- Accept: `go test ./internal/auth/ -run TestOAuth2 -count=1` green and total
  auth package test time < 30s.

### T5.4 Upstream health honors auth + caching (rest of D16)
- Move upstream health checking into `internal/upstream/health.go`:
  ```go
  type HealthStatus struct { Status string; LatencyMS float64; CheckedAt time.Time; Error string }
  // Checker probes each upstream through ITS OWN client (auth applied),
  // caches results for ttl (default 10s), single-flights concurrent probes.
  type Checker struct{ ... }
  func NewChecker(ups map[string]*Upstream, ttl time.Duration) *Checker
  func (c *Checker) Check(ctx context.Context) map[string]HealthStatus
  ```
  Probe: `GET <url><health_path>` (or `HEAD <url>` when health_path empty)
  with 5s timeout via the upstream's `Client` (breaker/retry layers will wrap
  it — acceptable). 2xx/3xx = healthy; 401/403 = healthy-but-unauthorized?
  NO — with auth applied these should be 2xx; treat ≥400 as unhealthy and
  include the status code in `Error` (`"status 401"`).
- Delete `internal/server/health.go`'s `UpstreamHealthHandler`,
  `UpstreamInfo`, `checkUpstreamHealth` and the main.go `upstreamInfos`
  bridge (D15/PROJECT's "UpstreamInfo bridge type" is obsolete once health
  lives beside upstream). The HTTP endpoint reappears on the admin server in
  M8 backed by `Checker`.
- Accept: `internal/upstream/health_test.go` — cached result within TTL (hit
  counter == 1 after two Check calls), auth header present on probe (httptest
  capture).

### T5.5 Root config struct + `internal/gwconfig` (schema §2.3, precedence §2.5)
- New package `internal/gwconfig`:
  ```go
  type File struct {
      Server       ServerConfig            `yaml:"server"`
      Admin        AdminConfig             `yaml:"admin"`
      Upstreams    map[string]UpstreamYAML `yaml:"upstreams"`
      Compositions map[string]Composition  `yaml:"compositions"`
  }
  func Load(path string) (*File, error)      // read → expand env (T5.6) → unmarshal → defaults → Validate
  func (f *File) Validate() error            // joined errors (errors.Join) for ALL problems at once
  func (f *File) Hash() string               // sha256 hex of canonical yaml re-marshal (config fingerprint)
  func ApplyEnvOverrides(f *File)            // RESTITCH_* per §2.5
  ```
  Move `Upstream`/`Composition`/`Step`/`ErrorRule`/`ResponseTemplate` YAML
  structs from `internal/composition/config.go` into `gwconfig` (composition
  imports gwconfig; break the old import of auth config by moving
  `auth.Config` YAML struct references along — `gwconfig` may import
  `internal/auth` for the auth config types; keep the dependency direction
  gwconfig → auth, composition → gwconfig).
  `ServerConfig`/`AdminConfig` fields exactly per §2.3 with defaults applied
  in `Load`. Duration fields: keep `time.Duration` with yaml.v3 — yaml.v3
  does NOT parse `10s` into time.Duration natively; add a `Duration` wrapper
  type with `UnmarshalYAML` calling `time.ParseDuration` (accept bare
  integers as nanoseconds is NOT allowed — string only; error otherwise) and
  use it for every duration field in the YAML structs (including existing
  `Upstream.Timeout` — note the current code compiles because tests pass
  `10s`? yaml.v3 fails on that for time.Duration; verify and migrate all).
  `Validate` collects: unknown upstream refs, duplicate step names, path
  syntax, retry/breaker/cache field ranges (max_attempts ≥ 1, ttl > 0, ports
  1-65535), auth mutual exclusivity (delegate to auth.Config.Validate).
- `cmd/restitch/main.go` (will be reshaped again in M11; for now):
  flags override `File.Server` after `ApplyEnvOverrides`. Wire
  `read_timeout/write_timeout/shutdown_timeout` into `server.Config`.
- Existing `composition.ParseConfig` shrinks to: `gwconfig.Load` + compile.
  Keep a thin `composition.LoadConfigFile` delegating to gwconfig so tests
  keep working, or update tests — prefer updating tests to gwconfig.
- Accept: `internal/gwconfig` tests: full-schema YAML round-trip; `10s`
  duration parse; multi-error validation returns BOTH errors for a config
  with two problems; `RESTITCH_PORT=9999` override wins over file, flag wins
  over env.

### T5.6 Strict env expansion (D7)
- Rewrite `internal/config/env.go` as `internal/gwconfig/env.go` (delete the
  old package after):
  ```go
  // ExpandEnvStrict expands ${VAR} and ${VAR:default}. "$$" → literal "$".
  // A "$" not part of "$$" or "${...}" is an error. ${VAR} with VAR unset
  // and no default is an error listing the variable name.
  func ExpandEnvStrict(s string) (string, error)
  ```
  Implement by manual scan (no os.ExpandEnv): iterate runes; on `$`: next is
  `$` → emit one `$`; next is `{` → read to `}` (error if EOF), split on
  first `:` for default; anything else → `fmt.Errorf("invalid '$' at offset %d: use $$ for a literal dollar or ${VAR} syntax", i)`.
  Applied to the RAW file bytes in `gwconfig.Load` BEFORE yaml.Unmarshal
  (whole-file expansion — same model as Cosmo/Benthos), so all values get
  it uniformly. Auth strategies then receive already-expanded values: delete
  `ExpandEnvWithValidation` calls inside `internal/auth/*.go` builders (and
  the tests that set env vars move to gwconfig tests).
- Accept: table tests: `pa$$word` → `pa$word`; `pa$sword` → error; `${MISSING}`
  → error naming MISSING; `${MISSING:fallback}` → `fallback`; `${SET}` →
  value. Whole-file: a config with `value: "${API_KEY}"` fails Load when
  unset — at startup, not first request.

### M5 Verification gate
```bash
go test ./... -count=1 -race
go vet ./...
grep -rn "internal/client\|internal/config\b" --include='*.go' cmd internal | grep -v gwconfig   # empty
# Authorization scoping smoke (two-terminal):
go run ./cmd/mockupstream &
cat > /tmp/m5.yaml <<'EOF'
upstreams: {mock: {url: "http://localhost:8081"}}
compositions:
  e: {path: "/e", steps: [{name: x, upstream: mock, path: "/echo"}], response: {body: {h: "{{ steps.x.body.headers }}"}}}
EOF
go run ./cmd/restitch -config /tmp/m5.yaml &
curl -s -H 'Authorization: Bearer SECRET' localhost:8080/e | grep -c SECRET   # → 0
```

---

# Milestone M6 — Resilience: retries, circuit breaker, coalescing [DONE]

All three are RoundTripper/step-level layers in `internal/upstream`, inserted
by `upstream.Build` per K5, active only when configured.

### T6.1 Retry RoundTripper (K6)
- `internal/upstream/retry.go`:
  ```go
  type RetryConfig struct {
      MaxAttempts int; Interval, MaxBackoff gwconfig.Duration
      BackoffOn, DropOn []int; RetryNonIdempotent bool
  }
  func newRetryTripper(next http.RoundTripper, cfg RetryConfig, upstreamName string) http.RoundTripper
  ```
  Behavior (implement exactly):
  1. Attempt the request. Network error → retryable. Response status in
     `DropOn` → return response immediately (no retry). Status in `BackoffOn`
     → retryable. 2xx/anything else → return response.
  2. Retry only if method is idempotent (GET/HEAD/OPTIONS/PUT/DELETE) or
     `RetryNonIdempotent`. Requests with a body are retryable only when
     `req.GetBody != nil` (rebuild body via GetBody each attempt; step.go
     builds requests from a `bytes.Reader`, so `http.NewRequestWithContext`
     sets GetBody automatically — assert in a test).
  3. Before retrying: fully drain+close the previous response body
     (connection reuse), then sleep `min(Interval * 2^(attempt-1), MaxBackoff)`
     with ±20% jitter (`rand.Float64()`), UNLESS the response carried
     `Retry-After` (seconds integer or HTTP-date) — then sleep that, capped
     at MaxBackoff. Abort immediately if `req.Context().Done()`.
  4. Stop after MaxAttempts total attempts; return the last response/error.
  Defaults when the `retry:` block exists but fields are empty: MaxAttempts 3,
  Interval 250ms, MaxBackoff 5s, BackoffOn [429,502,503,504].
- Per-step override: `Step.Retry *RetryConfig` in gwconfig; effective config =
  step's if set else upstream's else nil. Since the tripper lives in the
  upstream client, implement step overrides via context:
  `upstream.WithRetryOverride(ctx, cfg)` read by the tripper (nil = use its
  own). `step.go` sets it when the step has a retry block.
- Tests (`retry_test.go`, httptest with scripted status sequences):
  503,503,200 → success in 3 attempts; 400 with BackoffOn=[503] → 1 attempt;
  DropOn=[503] → 1 attempt; `Retry-After: 1` honored (assert elapsed ≥ 1s,
  use small MaxBackoff to keep tests fast); POST not retried by default;
  context cancellation aborts the sleep promptly.
- Accept: tests green; a step-level `retry:` beats upstream-level (test).

### T6.2 Circuit breaker (K7)
- `go get github.com/sony/gobreaker/v2`.
- `internal/upstream/breaker.go`:
  ```go
  type BreakerConfig struct { MaxFailures int; Interval, Timeout gwconfig.Duration }
  func newBreakerTripper(next http.RoundTripper, cfg BreakerConfig, name string) http.RoundTripper
  ```
  `gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{
    Name: name, Interval, Timeout,
    ReadyToTrip: func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= uint32(cfg.MaxFailures) },
    IsSuccessful: func(err error) bool { return err == nil },
    OnStateChange: log via slog + set metrics gauge (M8 hook: expose a
      package-level `var OnBreakerStateChange func(name, state string)` set by
      the metrics package; nil-check before call),
  })`. Failure definition: transport error OR status ≥ 500 (wrap: if
  resp.StatusCode >= 500 return resp with a sentinel error
  `errBreakerCountedStatus` that the tripper unwraps back to (resp, nil) for
  the caller — the breaker must count 5xx as failures but callers still get
  the response). Client-canceled contexts (`errors.Is(err, context.Canceled)`)
  are NOT failures (KrakenD's subtlety — don't trip on client hangups).
  Open state → return `fmt.Errorf("upstream %s: circuit open: %w", name, gobreaker.ErrOpenState)`
  without calling next; handler taxonomy maps it to 502 (already generic).
- Breaker sits INSIDE retry (retry → breaker → auth): a retry attempt against
  an open breaker fails fast and `BackoffOn` doesn't apply to breaker errors
  (network-error path: retryable — that's fine, attempts are cheap when open;
  add note in code).
- Tests: 5 consecutive 503s trip it (6th request errs without hitting the
  server — hit counter); after `Timeout` a half-open probe goes through;
  canceled contexts don't count toward failures.
- Accept: tests green.

### T6.3 Request coalescing (`coalesce: true`)
- `internal/upstream/coalesce.go`: step-level, in `step.go` execution path
  (not a RoundTripper — the key needs the final URL):
  ```go
  // package-level per Executor: 
  type Coalescer struct{ g singleflight.Group }
  // Do executes fn once per (method, url, authKey) among concurrent callers.
  // Result is shared: the StepResult is cloned per caller (bodies are
  // decoded values; share is safe because step results are never mutated —
  // enforce with a comment on StepResult).
  func (c *Coalescer) Do(key string, fn func() (*StepResult, error)) (*StepResult, error)
  ```
  Key: `method + " " + finalURL + " " + sha256(clientAuthorization(ctx))[:16]`
  (auth identity prevents cross-user data leaks through coalescing — REQUIRED).
  Only applied when `step.Coalesce && method == GET`. `Executor` owns one
  `Coalescer`; `executeStepWithErrorHandling` routes through it.
- Tests: two concurrent executions of the same composition (barrier via
  channel) → upstream hit counter == 1; different Authorization values → 2.
- Accept: tests green.

### M6 Verification gate
```bash
go test ./internal/upstream/ ./internal/composition/ -count=1 -race
# Manual breaker demo:
cat > /tmp/m6.yaml <<'EOF'
upstreams:
  flaky:
    url: "http://localhost:8081"
    retry: {max_attempts: 2, backoff_on: [503]}
    circuit_breaker: {max_failures: 3, timeout: 5s}
compositions:
  f: {path: "/f", steps: [{name: s, upstream: flaky, path: "/status/503"}], response: {body: {r: "{{ steps.s.status }}"}}}
EOF
go run ./cmd/mockupstream & go run ./cmd/restitch -config /tmp/m6.yaml &
for i in $(seq 1 6); do curl -s -o /dev/null -w '%{http_code} ' localhost:8080/f; done; echo
# Expected: first responses 200 (5xx passthrough is step "success" with status 503
# rendered), then after 3 counted failures... NOTE: 503 passthrough is NOT a step
# error, so the breaker counts it at transport level only — expect log lines
# "circuit breaker 'flaky' state change: closed -> open" and subsequent 502s.
```

---

# Milestone M7 — Per-step response caching [DONE]

### T7.1 TTL cache
- `internal/upstream/cache.go`:
  ```go
  type StepCache struct{ ... } // sharded map[string]entry{result *composition-agnostic payload, expires time.Time}, RWMutex per shard (16 shards)
  func NewStepCache() *StepCache
  func (c *StepCache) Get(key string) (*CachedResponse, bool)
  func (c *StepCache) Set(key string, v *CachedResponse, ttl time.Duration)
  type CachedResponse struct { Status int; Headers http.Header; Body []byte }
  ```
  Janitor goroutine (started by NewStepCache, stopped via `Close()`)
  sweeps expired entries every 30s. No size cap for v1 — but count entries in
  a gauge (M8) and document the tradeoff in code.
- Wiring: `Step.Cache *CacheConfig{TTL gwconfig.Duration}` in gwconfig
  (validated ttl > 0, GET-only — reject cache on non-GET at Validate).
  In `executeStepWithErrorHandling` (same layer as coalescing; order:
  cache check → coalesce → execute → cache fill): key = same identity key as
  coalescing (method+URL+auth hash). Cache only stores status < 500 responses
  with `ErrorRuleMatched == false`. StepResult is rebuilt from CachedResponse
  (re-parse body JSON — cheap, avoids sharing mutable maps).
- Executor owns one StepCache; Close it on... the executor has no lifecycle —
  give `Executor` a `Close()` called from server shutdown path in main
  (and by hot-reload swap in M10 when discarding an old pipeline).
- Tests: second request within TTL → 1 upstream hit; after expiry → 2;
  different auth → 2; POST step with cache config → Validate error.
- Accept: tests green.

### M7 Verification gate
```bash
go test ./internal/upstream/ ./internal/gwconfig/ -count=1
# Manual: config with cache: {ttl: 30s} on a mockupstream step;
# two curls; mockupstream logs show one upstream request.
```

---

# Milestone M8 — Observability: metrics, access log, admin server [DONE]

### T8.1 Prometheus metrics
- `go get github.com/prometheus/client_golang`.
- `internal/observability/metrics.go` — a `Metrics` struct with promauto
  collectors on a private registry (`prometheus.NewRegistry`):
  | metric | type | labels |
  |---|---|---|
  | `restitch_requests_total` | counter | composition, method, status |
  | `restitch_request_duration_seconds` | histogram (default buckets) | composition |
  | `restitch_partial_responses_total` | counter | composition |
  | `restitch_step_duration_seconds` | histogram | composition, step, upstream, status |
  | `restitch_upstream_requests_total` | counter | upstream, status_class (2xx/3xx/4xx/5xx/error) |
  | `restitch_retries_total` | counter | upstream |
  | `restitch_breaker_state` | gauge (0 closed,1 half,2 open) | upstream |
  | `restitch_cache_hits_total` / `_misses_total` | counter | composition, step |
  | `restitch_coalesced_total` | counter | composition, step |
  ```go
  func NewMetrics() *Metrics
  func (m *Metrics) Handler() http.Handler   // promhttp.HandlerFor(registry, ...)
  ```
- Instrumentation points: handler (requests_total, duration, partial);
  executor timing loop (step_duration); `internal/upstream` metrics
  RoundTripper (upstream_requests_total) as outermost chain layer — Build
  takes a `*observability.Metrics` (nil = no-op layer); retry tripper
  increments retries; breaker OnStateChange sets gauge; cache/coalesce
  counters at their call sites. Pass `*Metrics` from main through
  `NewHandler`/`upstream.Build`.
- Accept: unit test scraping the registry after one in-process request
  (`testutil.CollectAndCount` or read the Handler output) shows
  `restitch_requests_total` == 1 with correct labels.

### T8.2 Access log with latency split
- Extend the T1.3 logging middleware fields with `bytes_written` and, via a
  context carrier set by the executor, `upstream_ms` (sum of step wall time
  of the slowest chain ≈ use `CompositionResult` total: simplest correct
  metric = time spent inside `executor.Execute`; record it in handler and
  pass to middleware via response header trick? NO — cleaner: the middleware
  logs `duration_ms` only; the handler's existing "composition complete" log
  carries `step_timings` + `slowest_step`. ADD `gateway_overhead_ms` =
  handler total minus Execute total to the handler log. Skip plumbing into
  middleware.)
- Accept: handler log line contains `gateway_overhead_ms` (test via
  `slog` capture handler or just visual in gate).

### T8.3 Admin server (K9) + request ring buffer
- New package `internal/admin`:
  ```go
  type Server struct{ ... }
  type Deps struct {
      Version, ConfigPath string
      ConfigHash func() string
      Compositions func() []CompositionInfo      // from current CompiledConfig (hot-reload safe: closures)
      Upstreams   func(ctx context.Context) []UpstreamInfo   // includes health via upstream.Checker
      Validate    func(yamlBytes []byte) []string             // returns human messages, empty = valid
      Reload      func() (newHash string, err error)          // wired in M10; until then returns error "hot reload not enabled"
      Requests    *RingBuffer
      Stats       *Stats
      Metrics     http.Handler
  }
  func New(cfg gwconfig.AdminConfig, deps Deps) *Server
  func (s *Server) Start() error; func (s *Server) Shutdown(ctx) error
  ```
  Endpoints (JSON; if `cfg.APIKey != ""` every request must send
  `X-Admin-Key` matching, else 401):
  - `GET /admin/api/info` → `{version, uptime_seconds, config_hash, config_path, compositions, upstreams}` (counts)
  - `GET /admin/api/compositions` → `[{name, path, method, public, steps:[{name, upstream, method, optional, timeout_ms, depends_on}], waves:[[..]]}]`
  - `GET /admin/api/compositions/{name}` → single (404 JSON if unknown)
  - `GET /admin/api/upstreams` → `[{name, url, auth_type, timeout_ms, health:{status, latency_ms, checked_at, error}}]`
  - `POST /admin/api/validate` (body: raw YAML) → `{valid: bool, errors: []string}`
  - `POST /admin/api/reload` → `{ok, config_hash}` or 400 `{ok:false, errors}`
  - `GET /admin/api/requests?limit=100` → newest-first ring entries
  - `GET /admin/api/stats` → `{total_requests, total_errors, partial_responses, per_composition: {name: {count, errors, avg_ms, p95_ms}}}`
  - `GET /metrics` → T8.1 handler
  - `GET /health` → `{"status":"ok"}` (admin liveness)
  CORS: allow `*` for GET/POST with the header (studio proxies anyway;
  permissive CORS here is acceptable because the API is key-protected and
  non-public by deployment).
- `RingBuffer` (`internal/admin/ring.go`): fixed-size slice + mutex + write
  index; entry:
  ```go
  type RequestRecord struct {
      ID string; Time time.Time; Composition, Method, Path string
      Status int; DurationMS float64; Partial bool
      Steps []StepRecord // {Name, Status string, Wave int, DurationMS float64, HTTPStatus int}
  }
  ```
  `Stats` (`internal/admin/stats.go`): per-composition count/errors/rolling
  latency reservoir (keep it simple: ring of last 512 durations per
  composition for avg/p95). Both are fed from `composition.Handler` via a
  narrow interface to avoid an import cycle:
  ```go
  // in composition package:
  type Recorder interface{ Record(rec admin-shaped struct) }
  ```
  — define the record struct in a tiny leaf package `internal/reqlog` used by
  both admin and composition (structs only, no logic imports).
- Wire in main: start admin server when `admin.enabled` (default true).
  `/health/upstreams` on the DATA port is removed (its content now at
  `GET /admin/api/upstreams`); public `/health` and `/ready` stay on the data
  port.
- Accept: `internal/admin` httptest tests for: api-key 401; info; requests
  ring rollover (size 3, write 5, read 3 newest); validate happy+sad; stats
  aggregates. Handler test asserting a served request appears in the ring.

### M8 Verification gate
```bash
go test ./... -count=1 -race
go run ./cmd/mockupstream & go run ./cmd/restitch -config /tmp/m4.yaml &
curl -s localhost:8080/api/users/1 >/dev/null
curl -s localhost:9090/metrics | grep restitch_requests_total   # count 1, labels present
curl -s localhost:9090/admin/api/requests | python3 -m json.tool # the request with step records
curl -s localhost:9090/admin/api/compositions | python3 -m json.tool
curl -si localhost:8080/health/upstreams | head -1              # 404 (moved)
```

---

# Milestone M9 — Inbound authentication [DONE]

Gateway-level auth for composition routes (K10). `/health`, `/ready` stay
public; admin has its own key (M8).

### T9.1 `internal/inbound` middleware
- Config: `server.auth` per §2.3. Validation: at least one of
  `api_keys`/`jwt` present when the block exists; `jwks_url` required inside
  `jwt`.
- `go get github.com/golang-jwt/jwt/v5 github.com/MicahParks/keyfunc/v3`.
- `internal/inbound/inbound.go`:
  ```go
  type Authenticator struct{ ... }
  // New builds the authenticator; when cfg.JWT != nil it constructs
  // keyfunc.NewDefaultCtx(ctx, []string{cfg.JWT.JWKSURL}) (background refresh
  // built in). Fail startup if the JWKS is unreachable.
  func New(ctx context.Context, cfg gwconfig.InboundAuthConfig) (*Authenticator, error)
  // Middleware wraps composition routes. public paths bypass (see wiring).
  func (a *Authenticator) Middleware(next http.Handler) http.Handler
  ```
  Request passes if EITHER: `X-API-Key` matches any configured key
  (constant-time compare, `crypto/subtle`), OR `Authorization: Bearer <jwt>`
  verifies (`jwt.Parse` with keyfunc; validate `iss` when configured via
  `jwt.WithIssuer`, `aud` via `jwt.WithAudience`, and expiry by default).
  Failure → 401 `{"error":"unauthorized"}` + `WWW-Authenticate: Bearer`
  (JSON per §3.2). On success, stash claims:
  `inbound.WithClaims(ctx, claims jwt.MapClaims)` — and expose them to
  expressions as `req.auth` (map) — add to §2.4 env in `buildRequestEnv`
  (nil when no JWT).
- Wiring: per-composition `public: true` (gwconfig field on Composition).
  Since middleware wraps everything, implement bypass in
  `composition.Handler.RegisterRoutes`: wrap each route's handler
  individually — `if !comp.Public && authenticator != nil { h = authenticator.Middleware(h) }`.
  Pass the authenticator into `NewHandler` (nilable).
- Tests: no auth configured → open; api key wrong/right; expired JWT rejected
  (mint tokens in-test with a generated RSA key + a local httptest JWKS
  endpoint serving the public key — see keyfunc docs pattern); `public: true`
  route open; claims visible via `{{ req.auth.sub }}` in a template test.
- Accept: `go test ./internal/inbound/ ./internal/composition/ -count=1` green.

### M9 Verification gate
```bash
go test ./... -count=1 -race
# manual: config with server.auth.api_keys: ["k1"]:
# curl → 401; curl -H 'X-API-Key: k1' → 200; /health → 200 without key.
```

---

# Milestone M10 — Hot reload + pipeline swap [DONE]

Validate-then-swap per K8. A bad config NEVER takes down the running gateway.

### T10.1 Swappable pipeline
- New `internal/server/pipeline.go` (or in main package if cleaner — choose
  `cmd/restitch/pipeline.go` since it composes everything; decision: put it
  in `cmd/restitch/pipeline.go`):
  ```go
  // Pipeline is everything derived from one config file version.
  type Pipeline struct {
      Hash     string
      File     *gwconfig.File
      Compiled *composition.CompiledConfig
      Handler  http.Handler        // fully built data-plane mux (composition routes + health), middlewares applied
      Executor *composition.Executor
  }
  // BuildPipeline loads, validates, compiles, and constructs the mux.
  // MUST have no side effects on failure (no listeners, no goroutines
  // beyond upstream health checker/caches owned by the Pipeline).
  func BuildPipeline(ctx context.Context, path string, deps PipelineDeps) (*Pipeline, error)
  func (p *Pipeline) Close()   // stops cache janitor, health checker
  ```
  The data `http.Server.Handler` becomes a thin `atomic.Pointer[Pipeline]`
  dispatcher:
  ```go
  type Swapper struct{ ptr atomic.Pointer[Pipeline] }
  func (s *Swapper) ServeHTTP(w, r) { s.ptr.Load().Handler.ServeHTTP(w, r) }
  func (s *Swapper) Swap(p *Pipeline) *Pipeline // returns old
  ```
  Note: server-level middlewares (request ID, logging, recovery — ADD a
  recovery middleware now: catch panics → 500 + slog.Error with stack)
  wrap the Swapper once; per-pipeline mux contains routes + inbound auth.
  In-flight requests hold the old Pipeline pointer — delay `old.Close()` by
  `time.AfterFunc(30*time.Second, old.Close)` after swap (drain window;
  document the tradeoff).
- Reload triggers (`cmd/restitch/reload.go`):
  1. `SIGHUP` (signal.Notify loop);
  2. fsnotify watcher on the config file, debounced 500ms (collect events,
     fire once after quiet period); watch the DIRECTORY of the file and
     filter by name (editors replace files, inode changes — Cosmo/Benthos
     pattern; re-add watch if needed);
  3. `POST /admin/api/reload` → wired to the same `reload()` func via
     `admin.Deps.Reload`.
  `reload()`: `BuildPipeline`; on error slog.Error + return err (admin
  surfaces messages; SIGHUP/watch just log) — old pipeline untouched. On
  success: swap, schedule old Close, slog.Info with old/new hash. Reloads are
  serialized with a mutex; identical hash → no-op ("config unchanged").
- `go get github.com/fsnotify/fsnotify`.
- Tests (in `cmd/restitch` — package main tests are fine):
  `TestBuildPipeline_InvalidConfigLeavesNothingRunning` (bad YAML → error, no
  goroutine leak — use `goleak`? NOT approved dep; instead assert Close-less
  error return), `TestSwapper` (concurrent Serve during Swap — race detector
  covers it), reload-with-bad-config keeps serving old routes (in-process:
  build v1 pipeline, write bad file, reload err != nil, request still 200).
- Accept: tests green under `-race`.

### M10 Verification gate
```bash
go test ./cmd/restitch/ -count=1 -race
# Manual:
go run ./cmd/mockupstream & go run ./cmd/restitch -config /tmp/m4.yaml &
curl -s localhost:8080/api/users/1 >/dev/null                     # 200
echo 'garbage: [' >> /tmp/m4.yaml
curl -s -X POST localhost:9090/admin/api/reload | python3 -m json.tool   # ok:false + errors
curl -so /dev/null -w '%{http_code}\n' localhost:8080/api/users/1        # STILL 200
git checkout -- /tmp/m4.yaml 2>/dev/null || <restore file>; kill %2 # restore + SIGHUP path: kill -HUP <pid> logs "config reloaded"
```

---

# Milestone M11 — CLI subcommands [DONE]

Shape: `restitch [run|check|version|import] ...`. Bare `restitch -config x`
(no subcommand) must keep working == `run` (K18 compatibility).

### T11.1 Subcommand dispatch
- Restructure `cmd/restitch/`: `main.go` dispatches on `os.Args[1]`:
  known subcommand → strip it and run that command's `flag.FlagSet`;
  unknown/absent or starts with `-` → `run` with full args. Files: `run.go`
  (current main body: flags `-config -port -tls-port -cert -key -log-format
  -log-level`), `check.go`, `version.go`, `importcmd.go`.
- `version.go`: `var version = "dev"` package var set via
  `-ldflags "-X main.version=..."` (Makefile `build` passes
  `$(shell git describe --tags --always)`); `restitch version` prints
  `restitch <version> (<go version>)`. Replace `server.Version` usages;
  pass version into admin Deps.
- Accept: `go run ./cmd/restitch version` prints; `go run ./cmd/restitch
  -config restitch.yaml` still runs (back-compat); `restitch nonsense` exits
  2 with usage listing subcommands.

### T11.2 `restitch check`
- `check.go`: flags `-config` (default restitch.yaml), `-q` (quiet).
  Steps: (1) `gwconfig.Load` (env-expansion + validation, joined errors);
  (2) `composition.CompileConfig` with auth building SKIPPED for network
  calls — add `composition.CompileOptions{SkipAuthInit bool}` so check
  doesn't need live credentials/IdP: when set, OAuth2 strategy construction
  validates fields + env expansion only (no initial token fetch — refactor
  `NewOAuth2Strategy(ctx, cfg, opts...)` with `WithoutInitialFetch()`);
  (3) boot smoke: `BuildPipeline` into an `httptest.NewServer` for < 1s and
  GET `/health` (catches ServeMux pattern conflicts — KrakenD `-t` pattern);
  (4) print per-composition summary: name, path, step count, wave layout
  (`user → [orders loyalty] → merge`).
  Output: human lines + final `Syntax OK` / errors; exit 0/1. `-q`: errors only.
- Accept: `go run ./cmd/restitch check -config restitch.yaml` → `Syntax OK`
  with wave printout; a config referencing a missing upstream → exit 1
  listing the exact composition/step; missing env var → exit 1 naming it.

### T11.3 `restitch import openapi`
- `go get github.com/getkin/kin-openapi`.
- `importcmd.go`: usage
  `restitch import openapi <spec.(json|yaml)> --upstream NAME [--base-url URL] [--ops opId1,opId2] [-o out.yaml]`.
  Behavior: parse via `openapi3.NewLoader().LoadFromFile`; resolve base URL
  (flag > first `servers[0].url`); emit YAML:
  - one upstream entry `NAME: {url: <base>}`;
  - one composition per selected operation (default: ALL operations with an
    `operationId`; `--ops` filters): name = operationId kebab-cased; path =
    the OpenAPI path with `{param}` kept verbatim (ServeMux-compatible);
    method from the spec; single step `main` calling the same path with each
    `{param}` replaced by `{{ req.params.<param> }}` and required query
    params appended as `?q={{ req.query.q }}`; response
    `body: {result: "{{ steps.main.body }}"}`.
  Print to stdout or `-o` file, plus a stderr note: "scaffold only — review
  auth, timeouts, and response shaping". No auth inference.
- Tests: golden test with a small petstore-like spec fixture under
  `cmd/restitch/testdata/petstore.yaml` → compare generated YAML to
  `testdata/petstore_expected.yaml`; generated YAML must pass
  `gwconfig.Load` + `CompileConfig(SkipAuthInit)`.
- Accept: golden test green; `restitch import openapi testdata/petstore.yaml
  --upstream pets | restitch check -config /dev/stdin` — if /dev/stdin is
  awkward, write to a temp file in the test instead; manual gate uses `-o`.

### M11 Verification gate
```bash
go test ./cmd/... -count=1
go build -o bin/restitch ./cmd/restitch
./bin/restitch version
./bin/restitch check -config restitch.yaml            # Syntax OK + waves
./bin/restitch import openapi cmd/restitch/testdata/petstore.yaml --upstream pets -o /tmp/pets.yaml
./bin/restitch check -config /tmp/pets.yaml           # Syntax OK
```

---

# Milestone M12 — Studio backend: `restitch-studio` binary [DONE]

### T12.1 The studio server
- `cmd/restitch-studio/main.go`:
  ```go
  //go:embed all:dist
  var distFS embed.FS   // dist lives at cmd/restitch-studio/dist (copied by studio build, see T13.8)
  ```
  Flags/env: `-port` (default 3080, env `STUDIO_PORT`), `-gateway-admin-url`
  (default `http://localhost:9090`, env `STUDIO_GATEWAY_ADMIN_URL`),
  `-admin-key` (env `STUDIO_ADMIN_KEY`, optional).
  Routes:
  - `/api/` and `/metrics` → `httputil.NewSingleHostReverseProxy` to the
    admin URL, REWRITING `/api/*` → `/admin/api/*` and attaching
    `X-Admin-Key` when configured (proxy `Director`/`Rewrite` func). 15s
    `Transport` timeout via `ResponseHeaderTimeout`.
  - everything else → SPA file server over `distFS` with an index.html
    fallback for client-side routes: if the requested file doesn't exist in
    dist, serve `dist/index.html` (standard SPA pattern; implement with
    `fs.Sub(distFS, "dist")` + a wrapper handler that stats first).
  Startup log: studio URL + proxied gateway URL. Graceful shutdown same
  pattern as the gateway.
- Keep the binary buildable BEFORE the React app exists: commit a placeholder
  `cmd/restitch-studio/dist/index.html` ("Restitch Studio — run `make studio`
  to build the UI") so `go build ./...` never breaks. `.gitignore` must NOT
  exclude that placeholder but SHOULD exclude the rest of generated dist
  content — simplest rule: commit dist/index.html placeholder now; after
  T13.8 the built dist is committed on release builds only; add
  `cmd/restitch-studio/dist/*` to .gitignore with exception
  `!cmd/restitch-studio/dist/index.html`. Document in Makefile comments.
- Tests: httptest for the proxy rewrite (`/api/info` hits a fake admin server
  at `/admin/api/info` with the key header) and SPA fallback (`/compositions`
  → index.html bytes; `/assets/x.js` missing → index.html; present file →
  file).
- Accept: `go build ./cmd/restitch-studio && go test ./cmd/restitch-studio/`
  green; `go run ./cmd/restitch-studio` serves the placeholder at :3080.

### M12 Verification gate
```bash
go test ./cmd/restitch-studio/ -count=1
go run ./cmd/restitch -config restitch.yaml &      # admin on 9090
go run ./cmd/restitch-studio &                     # 3080
curl -s localhost:3080/api/info | python3 -m json.tool    # proxied gateway info
curl -s localhost:3080/ | grep -i studio                   # placeholder page
```

---

# Milestone M13 — Studio frontend (React + Tailwind v4 + shadcn/ui) [DONE]

All work under `studio/`. Node ≥ 20 assumed. The Studio talks ONLY to
same-origin `/api/*` (the M12 proxy) — no CORS handling in the app.

### T13.1 Scaffold Vite + React + TS + Tailwind v4 + shadcn/ui
Run exactly (Tailwind v4 differs from v3 — no tailwind.config.js needed,
CSS-first config, dedicated Vite plugin):
```bash
npm create vite@latest studio -- --template react-ts
cd studio
npm install
npm install tailwindcss @tailwindcss/vite
npm install react-router-dom @xyflow/react js-yaml @uiw/react-codemirror @codemirror/lang-yaml lucide-react
npm install -D @types/js-yaml vitest @testing-library/react jsdom @vitejs/plugin-react
```
- `vite.config.ts`:
  ```ts
  import path from "path"
  import react from "@vitejs/plugin-react"
  import tailwindcss from "@tailwindcss/vite"
  import { defineConfig } from "vite"
  export default defineConfig({
    plugins: [react(), tailwindcss()],
    resolve: { alias: { "@": path.resolve(__dirname, "./src") } },
    server: { proxy: { "/api": "http://localhost:3080" } },  // dev against studio binary
    build: { outDir: "../cmd/restitch-studio/dist", emptyOutDir: true },
  })
  ```
- `src/index.css` (replace entirely): first line `@import "tailwindcss";`
  (Tailwind v4 style — do NOT create tailwind.config.js or use
  `@tailwind base` directives).
- `tsconfig.json` + `tsconfig.app.json`: add
  `"baseUrl": ".", "paths": {"@/*": ["./src/*"]}` (shadcn requirement).
- shadcn/ui init (answers: style *New York*, base color *zinc*, CSS variables
  *yes*):
  ```bash
  npx shadcn@latest init
  npx shadcn@latest add button card table badge tabs dialog input label select textarea separator tooltip sonner skeleton
  ```
  (shadcn's CLI detects Tailwind v4 and writes `@theme` blocks into
  index.css; if it asks for a components.json target, accept defaults with
  alias `@/components`.)
- Delete Vite demo content (`App.css`, logo assets, counter).
- Accept: `npm run build` succeeds and populates
  `cmd/restitch-studio/dist/` (index.html + assets); `npm run dev` renders a
  shadcn Button on a blank page (temporary smoke, removed next task).

### T13.2 App shell, routing, API client
- `src/lib/api.ts` — typed client, mirrors §T8.3 responses:
  ```ts
  export interface Info { version: string; uptime_seconds: number; config_hash: string; config_path: string; compositions: number; upstreams: number }
  export interface StepInfo { name: string; upstream: string; method: string; optional: boolean; timeout_ms: number; depends_on: string[] }
  export interface CompositionInfo { name: string; path: string; method: string; public: boolean; steps: StepInfo[]; waves: string[][] }
  export interface UpstreamInfo { name: string; url: string; auth_type: string; timeout_ms: number; health: { status: string; latency_ms: number; checked_at: string; error?: string } }
  export interface StepRecord { name: string; status: "success"|"failed"|"skipped"; wave: number; duration_ms: number; http_status: number }
  export interface RequestRecord { id: string; time: string; composition: string; method: string; path: string; status: number; duration_ms: number; partial: boolean; steps: StepRecord[] }
  export interface Stats { total_requests: number; total_errors: number; partial_responses: number; per_composition: Record<string, { count: number; errors: number; avg_ms: number; p95_ms: number }> }
  export interface ValidateResult { valid: boolean; errors: string[] }
  export const api = {
    info: () => get<Info>("/api/info"),
    compositions: () => get<CompositionInfo[]>("/api/compositions"),
    composition: (name: string) => get<CompositionInfo>(`/api/compositions/${name}`),
    upstreams: () => get<UpstreamInfo[]>("/api/upstreams"),
    requests: (limit = 100) => get<RequestRecord[]>(`/api/requests?limit=${limit}`),
    stats: () => get<Stats>("/api/stats"),
    validate: (yaml: string) => post<ValidateResult>("/api/validate", yaml),
    reload: () => post<{ ok: boolean; config_hash?: string; errors?: string[] }>("/api/reload", ""),
  }
  ```
  `get`/`post` throw on non-2xx with the response text; callers surface via
  sonner toasts.
- `src/App.tsx`: `BrowserRouter` + a sidebar layout (shadcn styles, lucide
  icons): nav items Dashboard `/`, Compositions `/compositions`,
  Requests `/requests`, Builder `/builder`, Config `/config`. Header shows
  gateway version + config hash (from `api.info`, poll 30s) and a "Reload
  config" button (confirm dialog → `api.reload` → toast result).
- `src/hooks/usePoll.ts`: `usePoll<T>(fn: () => Promise<T>, ms: number)` —
  interval + visibilitychange pause, returns `{data, error, refresh}`. Used
  by all pages (no react-query; keep deps minimal).
- Accept: `npm run build` clean; dev server shows the shell with empty pages.

### T13.3 Dashboard page (`src/pages/Dashboard.tsx`)
- Top row: four shadcn `Card` stat tiles from `usePoll(api.stats, 5000)`:
  Total requests, Error rate (`total_errors/total_requests`, "—" when 0
  requests), Partial responses, Compositions (from info). 
- Below: per-composition `Table`: name, requests, errors, avg ms, p95 ms;
  row click → composition detail. Upstream health strip: one `Badge` per
  upstream (green `healthy` / red `unhealthy` with tooltip showing error),
  from `usePoll(api.upstreams, 10000)`.
- Empty state (no traffic yet): friendly card "No requests yet — point
  traffic at the gateway".
- Accept: with gateway+mockupstream running and a few curls, tiles and table
  populate live (manual); `npm run build` clean.

### T13.4 Compositions pages
- `src/pages/Compositions.tsx`: table of compositions (name, method+path,
  steps count, public badge). Row → `/compositions/:name`.
- `src/pages/CompositionDetail.tsx`: three `Tabs`:
  1. **Graph** — React Flow (`@xyflow/react`) DAG: one node per step laid out
     by wave (x = wave index * 240, y = index-in-wave * 100), edges from
     `depends_on`. Node renders step name, upstream, method badge, optional
     badge. Include `<Controls/>` and `<Background/>`; `fitView`. No custom
     interactivity beyond pan/zoom (nodes not editable here).
  2. **Steps** — table with all StepInfo fields.
  3. **Route** — method, path, public flag, and a ready-to-copy
     `curl` line (`curl http://<host>:8080<path>` with `{param}` placeholders
     highlighted).
- Accept: for the M3 gate config, the graph shows `user`/`loyalty` in wave 1
  and `bonus` in wave 2 with an edge loyalty→bonus (manual); build clean.

### T13.5 Requests explorer
- `src/pages/Requests.tsx`: `usePoll(api.requests, 3000)`. Table: time
  (relative), composition, method+path, status (badge: 2xx green, 4xx amber,
  5xx red), duration, partial badge. Row expands (chevron) to a **step
  waterfall**: pure CSS/div bars — for each step a row with name, a bar whose
  left offset = sum of previous waves' max duration (approximation from wave
  numbers) and width ∝ duration_ms, colored by status
  (success=primary, failed=destructive, skipped=muted); text label
  `wave N · X ms · HTTP 200`. Tooltip on truncation. Limit select (50/100/500).
- Accept: after curling the M3 partial composition, the row shows
  partial=true and the waterfall shows loyalty failed + bonus skipped
  (manual); build clean.

### T13.6 Config page
- `src/pages/Config.tsx`: CodeMirror YAML editor
  (`@uiw/react-codemirror` + `@codemirror/lang-yaml`, theme follows
  `prefers-color-scheme`). Buttons: **Validate** (POST buffer →
  `api.validate`; render errors as a destructive `Card` list or green "Valid
  ✓"), **Download** (Blob download `restitch.yaml`), **Load current** —
  NOTE: the admin API does not expose raw config content (secrets); instead
  `Load current` fetches `/api/compositions` + `/api/upstreams` and
  regenerates equivalent YAML via js-yaml with a banner "regenerated from
  runtime state — secrets and comments not included". Editor starts with a
  commented skeleton template. (K13: no write-back.)
- Accept: pasting the M3 gate config validates ✓; deleting a required field
  shows the gateway's real error strings; build clean.

### T13.7 Builder page (visual composition builder)
- `src/pages/Builder.tsx` + `src/lib/builder.ts`. Local-state form (no
  server writes): 
  - Composition meta: name, path, method (`Select`), public (`Switch`? use
    checkbox `Input` if switch not added — add `npx shadcn@latest add switch checkbox`).
  - Upstreams: rows of {name, url} (add/remove).
  - Steps: cards with {name, upstream (Select from above), method, path,
    optional, timeout, depends_on (multi via comma input)}; add/remove;
    reorder not needed (order irrelevant to the DAG).
  - Response: textarea for body template YAML fragment.
  - Live preview pane (right half): generated YAML via
    `buildYaml(state): string` (js-yaml `dump` of the §2.3 shape; unit-test
    this function with vitest — given a two-step state, output parses back
    (js-yaml `load`) and contains expected keys).
  - Buttons: Validate (send generated YAML to `api.validate`), Copy, Download.
  - A small inferred-DAG preview: reuse the wave-layout logic client-side —
    `src/lib/dag.ts` `computeWaves(steps: {name, depends_on, path, headers?, body?}[]): string[][]`
    that ALSO infers deps by scanning template strings for
    `steps.<name>` with regex `/steps\.([A-Za-z_][A-Za-z0-9_]*)/g` (mirror of
    gateway inference; unit-test with the M3 example → 2 waves).
- Accept: vitest green (`npm run test` — add `"test": "vitest run"` script);
  building the M3 example in the form yields YAML that the Config page
  validates ✓ (manual); build clean.

### T13.8 Build integration + CI
- Makefile: `studio` target = `cd studio && npm ci && npm run build`;
  `build-all` = `studio` + `go build` both binaries (gateway build does not
  require studio). Root README gains a Studio section (M15 rewrites fully).
- Ensure `.github/workflows/ci.yml` studio job (T1.6 guard) now activates:
  `npm ci && npm run test && npm run build` with working-directory `studio`,
  Node 20, cache npm.
- `studio/README.md`: dev workflow (`go run ./cmd/restitch` +
  `go run ./cmd/restitch-studio` + `npm run dev`), build, page map.
- Accept: `make build-all` from clean checkout produces `bin/restitch` and a
  studio-embedded `bin/restitch-studio` (add both to Makefile); running
  `bin/restitch-studio` serves the real UI at :3080.

### M13 Verification gate
```bash
cd studio && npm run test && npm run build && cd ..
make build-all
go run ./cmd/mockupstream & ./bin/restitch -config /tmp/m3.yaml & ./bin/restitch-studio &
curl -s localhost:8080/p >/dev/null   # generate traffic
# Open http://localhost:3080 — manual checklist:
#  Dashboard tiles populate; composition "partial" graph shows 2 waves;
#  Requests row expands to waterfall with failed+skipped steps;
#  Config page validates the M3 yaml; Builder generates valid YAML.
```

---

# Milestone M14 — End-to-end test harness and hardening [DONE]

Two complementary harnesses (patterns: Tyk `StartTest`, KrakenD
`tests/integration.go`, Cosmo `testenv`).

### T14.1 `internal/testenv` — in-process harness
- ```go
  // Run boots a full gateway pipeline in-process with scripted upstreams.
  type UpstreamSpec struct {
      // Handler wins if set; otherwise Script maps "METHOD /path" → response.
      Handler http.Handler
      Script  map[string]ScriptedResponse   // ScriptedResponse{Status int; Body string; Headers map[string]string; DelayMS int}
      CloseOnStart bool                      // simulate a down upstream
  }
  type Config struct {
      YAML      string                       // config with ${UPSTREAM_<name>} placeholders for URLs
      Upstreams map[string]UpstreamSpec
  }
  type Env struct {
      GatewayURL, AdminURL string
      Hits func(upstream string) int         // per-upstream request counter
      Client *http.Client
  }
  func Run(t *testing.T, cfg Config, fn func(t *testing.T, env *Env))
  ```
  Implementation: start one `httptest.Server` per upstream (counting hits via
  atomic counter middleware; `CloseOnStart` → start then Close immediately so
  the port refuses); substitute `${UPSTREAM_x}` in the YAML with each URL
  (plain `strings.ReplaceAll` — do this BEFORE gwconfig env expansion by
  using a distinct placeholder syntax `@@UPSTREAM_x@@` to avoid colliding
  with `${}` — decision: use `@@UPSTREAM_x@@`); write YAML to `t.TempDir()`;
  `BuildPipeline`; serve via `httptest.NewServer(swapper-or-handler)`; admin
  server on a real ephemeral port. `t.Cleanup` closes everything.
  This requires `BuildPipeline` (T10.1) to be importable: MOVE it from
  `cmd/restitch/pipeline.go` to `internal/server/pipeline.go` (adjust M10 if
  executed in order — final home: `internal/server`; `cmd/restitch` calls it).
- Port the manual gate scenarios into `internal/testenv/e2e_test.go` (or a
  sibling `tests/e2e` package importing testenv):
  E2E-1 happy path (2-step chain, assert merged body + `X-Restitch-Complete:
  true`); E2E-2 partial (M3 scenario: assert 200, complete=false, points
  null, `_errors` with failed+skipped, bonus upstream hits == 0);
  E2E-3 required failure → 502 taxonomy body; E2E-4 timeout → 504 (script
  DelayMS > step timeout); E2E-5 passthrough + no-auth-upstream Authorization
  scoping; E2E-6 retry (script: 503,200 → success, hits == 2); E2E-7 cache
  (2 requests, hits == 1); E2E-8 inbound API key (401/200); E2E-9 path
  params; E2E-10 admin: request appears in ring, metrics counter increments;
  E2E-11 hot reload (rewrite config file, POST reload, new route serves, old
  route 404s).
- Accept: `go test ./internal/testenv/... -count=1 -race` green, < 60s total.

### T14.2 Golden binary specs (`tests/specs`)
- `tests/binary_test.go` (build-tagged `//go:build e2e`): compiles the real
  binary (`go build -o <tmp>/restitch ./cmd/restitch` via `os/exec` in
  TestMain), starts `cmd/mockupstream` in-process (import its handler — 
  refactor mockupstream so `mockupstream.Handler() http.Handler` is exported
  from `internal/mockupstream` and `cmd/mockupstream` is a thin main),
  starts the binary with `tests/fixtures/gateway.yaml` (upstream URL injected
  via env — the fixture uses `${MOCK_URL}`), waits for `/health`, then
  executes every `tests/specs/*.json`:
  ```json
  {"name": "partial response", 
   "in": {"method": "GET", "path": "/p", "headers": {}},
   "out": {"status": 200,
            "headers": {"X-Restitch-Complete": "false"},
            "body_contains": ["\"points\": null", "dependency_failed"]}}
  ```
  Runner asserts status, each header, and each `body_contains` substring
  (on the pretty-printed body). Write ≥ 6 specs mirroring E2E-1/2/3/4/8/9.
- Makefile: `e2e` target = `go test -tags e2e ./tests/ -count=1 -v`; CI runs
  it as a third job.
- Accept: `make e2e` green locally and in CI.

### T14.3 Golden DAG snapshots + coverage floor
- `internal/composition/dag_golden_test.go`: for each YAML in
  `internal/composition/testdata/dags/*.yaml` (write 4 fixtures: linear
  chain, diamond, all-parallel, explicit depends_on mix), compile and
  marshal `{waves, deps}` to JSON; compare to sibling `.golden.json`
  (regenerate with `-update` flag convention: `go test -run TestDAGGolden
  -update`).
- CI: add `go test -coverprofile` for `internal/...` and fail under **70%**
  total (`go tool cover -func=cover.out | tail -1` awk check in the
  workflow).
- Accept: golden tests green; CI coverage step passes (if under 70%, add
  tests to the weakest package until it passes — likely `internal/admin` or
  `internal/server`).

### M14 Verification gate
```bash
go test ./... -count=1 -race
make e2e
# CI green on all three jobs (go, studio, e2e) + coverage ≥ 70%.
```

---

# Milestone M15 — Docs, examples, packaging [DONE]

### T15.1 README rewrite (D3 final closure)
- Rewrite `README.md` top-to-bottom against the ACTUAL engine:
  every example must use variables from §2.4 (`req.*` — mention `request.*`
  works as an alias); quick start uses `cmd/mockupstream` instead of external
  APIs (offline-friendly); sections: Why → Quick start → Configuration
  reference (§2.3 condensed with links to docs/) → Expression language (§2.4
  table + nil semantics + escaping guarantees) → Partial responses contract →
  Auth (inbound + upstream) → Resilience (retry/breaker/cache/coalesce) →
  Observability (metrics table, admin API) → CLI (`run/check/version/import`)
  → Hot reload → Studio (screenshot placeholder + `make studio`) → Deployment
  (Docker) → Development (Makefile targets).
- MANDATORY check: extract every YAML block from the README into temp files
  and run `restitch check` on each complete-config block (blocks that are
  fragments must be marked ` ```yaml (fragment)` and are exempt). Add
  `tests/readme_test.go` (tag e2e) doing exactly this automatically — parse
  README for ```yaml fences, skip `(fragment)`, run gwconfig.Load +
  CompileConfig(SkipAuthInit). README examples can never rot again.
- Accept: `make e2e` includes the README test, green.

### T15.2 docs/ + examples/
- `docs/`: `configuration.md` (full §2.3 with every field, default, and
  validation rule), `expressions.md` (§2.4 + expr-lang link + 15 worked
  examples incl. `?.`/`??`), `partial-responses.md` (contract + `_errors`
  schema), `resilience.md`, `observability.md` (metric names table, admin
  endpoints table), `studio.md`, `deployment.md`.
- `examples/`:
  - `examples/quickstart/restitch.yaml` (mockupstream-based, 2 compositions);
  - `examples/auth/restitch.yaml` (all four upstream auth types + inbound
    api key, env-var driven);
  - `examples/resilience/restitch.yaml` (retry+breaker+cache on a flaky
    route via mockupstream `/status/503`);
  - `examples/docker-compose/`: compose file running `restitch`,
    `restitch-studio`, `mockupstream` (all from the Dockerfile below,
    mockupstream via `go run` stage or its own target) + README.
- Add `tests/examples_test.go` (tag e2e): every `examples/**/restitch.yaml`
  passes `gwconfig.Load` + compile with SkipAuthInit (set required env vars
  to dummies inside the test).
- Accept: examples test green.

### T15.3 Dockerfile + release wiring
- `Dockerfile` (multi-stage): stage 1 `node:20-alpine` builds `studio/`;
  stage 2 `golang:1.25-alpine` builds both binaries (copying studio dist into
  `cmd/restitch-studio/dist` first), `-ldflags "-s -w -X main.version=$VERSION"`;
  stage 3 `gcr.io/distroless/static-debian12`: both binaries, `EXPOSE 8080
  8443 9090 3080`, `ENTRYPOINT ["/restitch"]` (studio image users override
  entrypoint; document in docs/deployment.md). Build arg `VERSION=dev`.
  CGO_DISABLED note: set `CGO_ENABLED=0`.
- `.dockerignore`: `.git`, `.planning`, `studio/node_modules`, `bin`.
- Makefile: `docker` target (`docker build -t restitch:dev .`).
- CI: add a job building the Docker image (no push) on PRs to catch
  Dockerfile rot.
- Accept: `docker build .` succeeds (if docker unavailable in the exec
  environment, the CI job is the acceptance check — mark the task done only
  when CI passes).

### T15.4 Project docs refresh
- Update `.planning/PROJECT.md` "Current State" and add a `CHANGELOG.md`
  (`## v2.0.0` summarizing this plan's delivery, listing breaking changes:
  none for config (K18), removed `/health/upstreams` from data port, removed
  `/ping`).
- Accept: files exist; `restitch check -config examples/quickstart/restitch.yaml`
  passes from a clean checkout per README instructions.

---

# Milestone M16 — DEFERRED: WASM plugins (design note only — no tasks)

Not implemented in this plan (user decision 2026-07-08: mention, defer).
When picked up: follow Cosmo's module pattern for the interface shape
(typed hooks `OnRequest`, `OnStepRequest`, `OnStepResponse`, `OnResponse`)
with plugins compiled to WASM and executed via a runtime such as wazero
(pure-Go, no CGO). Config: `plugins: [{name, path, config: {...}}]` at
top level; per-composition enablement. The K5 RoundTripper chain and the K8
pipeline swap were designed so a plugin layer can be inserted without
restructuring. Do not start this without a fresh design review.

---

# Final verification (run after M15; the definition of done)

```bash
# 1. Static
go vet ./...                                # exit 0, no output
golangci-lint run                           # exit 0
grep -rn "TODO\|FIXME" internal cmd --include='*.go'   # empty

# 2. Tests
go test ./... -count=1 -race                # all ok, no FAIL lines
make e2e                                    # binary specs + README + examples green
cd studio && npm run test && npm run build && cd .. # vitest green, dist built

# 3. Build artifacts
make build-all                              # bin/restitch, bin/restitch-studio
./bin/restitch version                      # prints version
./bin/restitch check -config examples/quickstart/restitch.yaml   # "Syntax OK"

# 4. Live smoke (three processes)
go run ./cmd/mockupstream &
./bin/restitch -config examples/quickstart/restitch.yaml &
./bin/restitch-studio &
curl -si localhost:8080/api/user-posts?id=1 | grep -E "200|X-Restitch-Complete: true"
curl -s  localhost:9090/metrics | grep -c '^restitch_'      # ≥ 8 metric families
curl -s  localhost:9090/admin/api/requests | python3 -m json.tool | grep composition
curl -s  localhost:3080/api/info | python3 -m json.tool     # studio proxy works
kill -HUP %2 && sleep 1                                     # logs "config unchanged"

# 5. CI: all four jobs green (go, e2e, studio, docker) with coverage ≥ 70%.
```

Expected end state: every command above passes exactly as described. If any
deviates, the corresponding milestone's gate has regressed — fix before
declaring completion.

---

# Addendum: Production Readiness Audit (2026-07-09)

Conducted after completing M1–M15 + Studio Observability upgrade. Findings
organized into: plan-vs-codebase drift, critical production gaps, competitive
feature gaps, and proposed new milestones.

## A1. Plan-vs-Codebase Drift

Items implemented but not reflected in PLAN.md sections 2.2, 2.3, or 3.3.
These are documentation-only fixes to keep the plan as single source of truth.

| # | Drift | Where to fix |
|---|-------|-------------|
| A1.1 | `admin.storage` config (`type`, `url`, `auth_token`, `retention`) exists in code but not in §2.3 config schema | Add to §2.3 under `admin:` |
| A1.2 | `modernc.org/sqlite` dep added (SQLite storage backend) — not in §3.3 approved deps | Add to §3.3 Go deps |
| A1.3 | `recharts` npm dep added (dashboard charts) — not in §3.3 approved deps | Add to §3.3 npm deps |
| A1.4 | Three admin API endpoints added (`/stats/timeseries`, `/stats/steps`, `/requests/{id}`) — not in T8.3 | Add to T8.3 endpoint table |
| A1.5 | `internal/reqlog` package not in §2.2 target package map | Add to §2.2 |
| A1.6 | Studio Metrics tab, 7 chart components, 3 waterfall components, 1 filter component — not in T13.3–T13.5 | Add to M13 or create M13b |
| A1.7 | `Pipeline` remains in `cmd/restitch/pipeline.go` — T14.1 says move to `internal/server/` | Update T14.1 or move the file |
| A1.8 | E2E-10 (admin integration) replaced by ResponseSizeCap — undocumented substitution | Document in T14.1 |
| A1.9 | CI has no `e2e` job (T14.2 calls for one) | Add to CI or update T14.2 |
| A1.10 | CI studio test uses `continue-on-error: true` — test failures silently ignored | Remove `continue-on-error` |

## A2. Critical Production Gaps

Issues that must be fixed before any production deployment.

### A2.1 Admin server missing HTTP timeouts

`internal/admin/server.go` creates `http.Server` without `ReadTimeout`,
`WriteTimeout`, `ReadHeaderTimeout`, or `IdleTimeout`. Vulnerable to
slowloris attacks and connection exhaustion. The data-plane server sets
these correctly (30s/30s/120s).

**Fix:** Add to `New()`:
```go
s.httpServer = &http.Server{
    Addr:              fmt.Sprintf(":%d", cfg.Port),
    Handler:           corsMiddleware(mux),
    ReadTimeout:       30 * time.Second,
    WriteTimeout:      30 * time.Second,
    ReadHeaderTimeout: 10 * time.Second,
    IdleTimeout:       120 * time.Second,
}
```

### A2.2 Main gateway missing ReadHeaderTimeout

`internal/server/server.go` sets ReadTimeout and WriteTimeout but not
`ReadHeaderTimeout`. Without it, a client can hold connections indefinitely
by sending headers slowly. Known Go security concern (gosec G112).

**Fix:** Add `ReadHeaderTimeout: 10 * time.Second` to both httpServer and
httpsServer in `internal/server/server.go`.

### A2.3 Admin CORS allows all origins

`Access-Control-Allow-Origin: *` on the admin API means any website can
call `POST /admin/api/reload` via a browser. The API key in `X-Admin-Key`
is explicitly allowed in `Access-Control-Allow-Headers`, so the preflight
check doesn't protect it.

**Fix:** When `api_key` is set, restrict CORS to the Studio origin only
(or make CORS origins configurable). When no key is set (dev mode), `*`
is acceptable.

### A2.4 SQL QueryRequests fetches entire table

`internal/admin/sql_storage.go` `QueryRequests` runs `SELECT data FROM
request_log ORDER BY timestamp DESC` with no `LIMIT` clause, then filters
in Go. As the table grows, this is a full table scan loading all data into
memory.

**Fix:** Push `LIMIT`, `composition`, and `timestamp` filters down to SQL
WHERE clauses. Filter `status` and `duration` in Go only (they're in the
JSON blob).

## A3. Important Production Gaps

Should fix before v1 — operational risks under load or during incidents.

| # | Gap | Severity | Fix |
|---|-----|----------|-----|
| A3.1 | Shutdown does not flush accumulator — up to 60s of metrics lost on restart | Important | Call `acc.Flush()` + write buckets before `store.Close()` in shutdown path |
| A3.2 | No upstream URL validation at config time — empty/malformed URLs fail only at request time | Important | Add `url.Parse()` check in `validateAndApplyDefaults()` |
| A3.3 | `AllowUndefinedVariables()` in expression engine — typos like `steps.usre.body` compile silently, return nil at runtime | Important | Remove the flag (breaking change for existing configs with intentionally undefined vars) or add a strict-mode config option |
| A3.4 | `handleValidate` uses `r.Body.Read(buf[:])` once — may get partial reads on large bodies, error discarded | Important | Use `io.ReadAll(io.LimitReader(r.Body, 1<<20))` |
| A3.5 | No rate limiting on admin API — `POST /admin/api/reload` can be hammered causing resource exhaustion | Important | Add simple token-bucket limiter on mutation endpoints |
| A3.6 | `MultiRecorder.Record` uses `context.Background()` for storage writes — not cancellable during shutdown | Important | Use a context with timeout (e.g. 5s) |
| A3.7 | Accumulator latency slices unbounded between 60s flushes — 100k req/min = 100k float64s per composition in memory | Important | Cap at e.g. 10k samples with reservoir sampling, or flush more frequently under load |
| A3.8 | `InsecureSkipVerify` configurable with no warning log | Important | Add `slog.Warn` when TLS verification is disabled for an upstream |
| A3.9 | No `admin.storage` validation in `gwconfig.Validate()` — invalid storage type/URL passes silently | Important | Add validation for storage config fields |
| A3.10 | No frontend tests for any chart, waterfall, filter, or page components (0/17 new components tested) | Important | Add at minimum smoke-render tests for key components |

## A4. Competitive Feature Gaps

Features that production API gateways (KrakenD, Tyk, Kong, APISIX) offer
that Restitch does not. Ordered by user-impact.

Sources: [KrakenD](https://www.krakend.io/), [Tyk](https://tyk.io/),
[API Gateway Comparison](https://www.moesif.com/blog/technical/api-gateways/How-to-Choose-The-Right-API-Gateway-For-Your-Platform-Comparison-Of-Kong-Tyk-Apigee-And-Alternatives/),
[Open Source Gateways 2026](https://daily.dev/blog/top-6-open-source-api-gateway-frameworks/),
[APISIX Comparison](https://api7.ai/learning-center/api-gateway-guide/api-gateway-comparison-apisix-kong-traefik-krakend-tyk)

### Tier 1 — Expected by users (competitive table stakes)

| Feature | KrakenD | Tyk | Kong | Restitch | Priority |
|---------|---------|-----|------|----------|----------|
| **Rate limiting** (per-client, per-endpoint) | Token bucket, multi-layer | Built-in, per-key/per-endpoint | Plugin (rate-limiting) | Missing | High |
| **Request/response payload size limits** | Configurable per-endpoint | Configurable | Plugin | Hard-coded 1MiB, not configurable | High |
| **Request schema validation** (JSON Schema) | Built-in | Built-in | Plugin | Missing | Medium |
| **OpenTelemetry / distributed tracing** | Plugin | Built-in | Plugin | Header propagation only, no span generation | Medium |
| **Response transformation** (field filtering, renaming) | Built-in (flatmap, allow/deny lists) | Built-in | Plugin | Template-only (powerful but no declarative filtering) | Low |
| **IP allow/deny lists** | Built-in | Built-in | Plugin | Missing | Medium |

### Tier 2 — Differentiators (not expected but valuable)

| Feature | Who has it | Restitch | Notes |
|---------|-----------|----------|-------|
| **GraphQL composition** | Tyk (Universal Data Graph), Apollo | N/A | REST-only is Restitch's niche — not a gap |
| **gRPC support** | Kong, Tyk, APISIX | N/A | REST-only scope |
| **WebSocket proxying** | Kong, APISIX | N/A | Not in scope |
| **Plugin/WASM extensibility** | KrakenD, Kong, APISIX | Deferred (M16) | Already noted |
| **Developer portal** | Tyk, Kong, Apigee | Missing | Enterprise feature — v2+ |
| **Canary/traffic splitting** | Kong, Tyk | Missing | v2+ |
| **Request body passthrough** (non-JSON) | Most | Missing | Only JSON POST bodies parsed |

### Tier 3 — Restitch competitive advantages

| Feature | vs. Competition |
|---------|----------------|
| **Declarative DAG composition** | Unique — KrakenD aggregates but doesn't support multi-wave DAGs with dependency inference |
| **Expression-based wiring** | More flexible than KrakenD's flatmap; comparable to Tyk's Universal Data Graph but for REST |
| **Studio UI with execution overlay** | No competitor offers a built-in DAG visualization with live trace overlay |
| **Partial responses** | Built-in with `optional` steps — most gateways fail the whole request |
| **Config-as-code with hot reload** | On par with KrakenD; better than Kong (requires DB) |

## A5. Proposed New Milestones

### M16 — Production Hardening (pre-v1 release) [DONE]

Priority: **Must complete before any production deployment.**

| Task | Description | Files | Status |
|------|-------------|-------|--------|
| T16.1 | Add HTTP timeouts to admin server (A2.1) | `internal/admin/server.go` | DONE |
| T16.2 | Add `ReadHeaderTimeout` to gateway server (A2.2) | `internal/server/server.go` | DONE |
| T16.3 | Restrict admin CORS when API key is set (A2.3) | `internal/admin/server.go` | DONE |
| T16.4 | Push SQL filters to QueryRequests query (A2.4) | `internal/admin/sql_storage.go` | DONE |
| T16.5 | Flush accumulator on shutdown (A3.1) | `cmd/restitch/run.go` | DONE |
| T16.6 | Validate upstream URLs at config time (A3.2) | `internal/composition/parser.go` | DONE |
| T16.7 | Fix `handleValidate` body read (A3.4) | `internal/admin/server.go` | DONE |
| T16.8 | Validate `admin.storage` config (A3.9) | `internal/gwconfig/config.go` | DONE |
| T16.9 | Warn when `InsecureSkipVerify` is enabled (A3.8) | `internal/upstream/client.go` | DONE |
| T16.10 | Update PLAN.md for drift items (A1.1–A1.10) | `PLAN.md` | DONE |

Verification: `go build ./... && go vet ./... && go test ./... -count=1`
green; admin server withstands `slowhttptest` without leaking connections.

### M17 — Rate Limiting & Request Validation [DONE]

Priority: **High — competitive table stakes.**

| Task | Description |
|------|-------------|
| T17.1 | Per-composition token-bucket rate limiter with configurable burst, rate, and key extraction (client IP / API key / header). Config under `compositions.*.rate_limit: { requests_per_second: 100, burst: 10, key: "header:X-Client-ID" }`. Use `golang.org/x/time/rate`. |
| T17.2 | Global gateway rate limiter (admin config). Reject with 429 + `Retry-After` header. |
| T17.3 | Configurable request body size limit per composition (`max_request_bytes`, default 1MiB). Return 413 when exceeded. |
| T17.4 | Optional JSON Schema validation for request body (`request_schema` field on composition). Validate before executing steps. Reject with 400 + structured error. |
| T17.5 | Admin API rate limiter — simple per-IP limit on mutation endpoints (`/reload`, `/validate`). |

### M18 — Observability: OpenTelemetry Tracing [DONE]

Priority: **Medium — important for production debugging at scale.**

| Task | Description |
|------|-------------|
| T18.1 | Add `go.opentelemetry.io/otel` SDK. Create/continue server spans at the gateway for each composition request. |
| T18.2 | Create child spans per step execution. Attach step name, upstream, wave, status as span attributes. |
| T18.3 | Propagate `traceparent`/`tracestate` to upstream requests (already partially done — formalize). |
| T18.4 | Configure exporter via env vars (`OTEL_EXPORTER_OTLP_ENDPOINT`, standard OTel env config). |
| T18.5 | Add trace ID to Studio request records. Show trace ID in the request explorer with a link to external trace viewer. |

### M19 — CI & Test Hardening [DONE]

Priority: **Medium — prevents regressions.**

| Task | Description |
|------|-------------|
| T19.1 | Add `e2e` CI job (T14.2 — currently missing). Run binary specs + E2E Go tests. |
| T19.2 | Remove `continue-on-error: true` from studio CI test step. |
| T19.3 | Add smoke-render tests for key Studio components (Dashboard, Requests, CompositionDetail). |
| T19.4 | Add E2E test for admin integration path: data-plane request → ring buffer → admin API response → time-series storage. |
| T19.5 | Add expression-typo detection: config warning when a `{{ steps.X.body }}` reference doesn't match any step name. |

---

# Future Milestones — Candidates from `experimental/v1.1-v1.2`

The `experimental/v1.1-v1.2` branch (preserved on remote) contains prior
implementations of features not yet on the main development branch. The code
below is reference material — not copy-paste-ready — but captures proven
designs worth adopting. Milestones are numbered M20+ to avoid collisions.

## M20 — Config Registry & Centralized Management

Priority: **High — enables Studio-driven config workflow.**

Replaces file-based config distribution with a database-backed registry.
Compositions are created/edited/versioned through the Studio API, validated
by running them through the gateway parser before saving.

| Task | Description |
|------|-------------|
| T20.1 | Config registry domain types: `Config`, `ConfigVersion`, `CreateConfigInput`, `UpdateConfigInput`. ULID-based IDs, semantic versioning per config, `active` flag for the live version. |
| T20.2 | Registry store with CRUD operations: create, list, get, update, delete, activate a version, diff between versions. Store in SQLite (dev) / Postgres (prod) via the existing `db` package. |
| T20.3 | Validation layer: parse YAML → compile composition → build a handler to verify correctness before persisting. Return structured validation errors on failure. |
| T20.4 | YAML bundle generation: `GET /api/v1/registry/bundle` returns all active configs as a single YAML document. SHA-256 ETag for change detection. |
| T20.5 | CRUD API endpoints under `/api/v1/configs`: create, list, get, update, delete, activate, diff. Huma-registered with OpenAPI docs. |
| T20.6 | Database migration for registry schema (configs table, config_versions table). |

Reference: `experimental/v1.1-v1.2` → `internal/studio/registry/`, `internal/studio/api/configs.go`, `internal/studio/api/configs_polling.go`, `internal/studio/db/migrations/`

### M20 Verification gate

```
curl -X POST localhost:8090/api/v1/configs -d '{"name":"test","yaml":"..."}' → 201
curl localhost:8090/api/v1/configs → list with version history
curl localhost:8090/api/v1/registry/bundle → YAML bundle with ETag header
# Repeat bundle request with If-None-Match → 304
# Invalid YAML → 400 with validation errors
```

---

## M21 — Gateway Registry Polling (extends M10)

Priority: **High — closes the centralized config loop.**

Adds a registry polling mode to the gateway as an alternative to file-watching.
The gateway periodically fetches the config bundle from Studio's registry
endpoint, using ETag-based change detection and exponential backoff on errors.

| Task | Description |
|------|-------------|
| T21.1 | Registry HTTP client: fetch `GET /api/v1/registry/bundle` with `If-None-Match` ETag support. Return `FetchResult` with YAML bytes, ETag, composition count, and change flag. |
| T21.2 | Polling engine with exponential backoff (cenkalti/backoff). Classify errors: `ErrorTypeFetch` (network), `ErrorTypeInvalidConfig` (bad YAML), `ErrorTypeReload` (swap failed). Backoff on transient errors, stop on persistent config errors. |
| T21.3 | Wire registry mode into gateway CLI: `--config-source=registry --registry-url=http://studio:8090`. Mutually exclusive with `--config` file mode. |
| T21.4 | Status endpoint: expose last poll time, ETag, composition count, error state via the admin API `/admin/api/registry/status`. |
| T21.5 | SIGHUP triggers immediate poll (skip backoff timer). |

Reference: `experimental/v1.1-v1.2` → `internal/hotreload/poller.go`, `internal/hotreload/registry_client.go`, `internal/hotreload/signals.go`, `cmd/restitch/cmd/gateway.go`

### M21 Verification gate

```
# Start gateway in registry mode pointing at Studio
restitch gateway --config-source=registry --registry-url=http://localhost:8090
# Verify it loads compositions from Studio bundle
curl localhost:8081/admin/api/registry/status → last_poll, etag, count
# Update a config in Studio → gateway picks it up within poll interval
# Kill Studio → gateway retries with backoff, keeps serving last known config
# Send SIGHUP → immediate poll
```

---

## M22 — Dev Mode Orchestrator (extends M11)

Priority: **Medium — developer experience.**

`restitch dev` runs gateway + studio together with process management,
auto-restart on crash, and colored log output for local development.

| Task | Description |
|------|-------------|
| T22.1 | `ProcessManager`: spawn and supervise child processes with configurable restart policy using exponential backoff (initial 1s, max 30s). Reset backoff after process is stable for a configurable duration. |
| T22.2 | `PrefixWriter`: wrap each child's stdout/stderr with a colored prefix (`[gateway]` blue, `[studio]` green) using `fatih/color`. Thread-safe via mutex. |
| T22.3 | Health-check waiting: `WaitForHealth(url, timeout)` polls a health endpoint before considering a process ready. |
| T22.4 | `restitch dev` CLI subcommand: fixed ports (gateway 8080, admin 8081, studio 8090), passes `ExtraEnv` to child processes (e.g. OTEL config), graceful shutdown on SIGINT/SIGTERM. |
| T22.5 | Support `--gateway-args` and `--studio-args` for passing through flags to child processes. |

Reference: `experimental/v1.1-v1.2` → `internal/devmode/manager.go`, `internal/devmode/writer.go`, `cmd/restitch/cmd/dev.go`

### M22 Verification gate

```
restitch dev
# Both gateway and studio start, logs are color-prefixed
# Kill gateway process → auto-restarts within backoff interval
# Ctrl+C → both processes shut down cleanly
```

---

## M23 — Upstream HTTP Client Optimization (extends M5)

Priority: **Medium — performance under load.**

| Task | Description |
|------|-------------|
| T23.1 | Set `MaxIdleConnsPerHost: 100` on the upstream HTTP transport (Go default is 2, causing 4-5x latency penalty under concurrent fan-out). Enforce TLS 1.2 minimum. |
| T23.2 | Add `DrainAndClose` helper: drain response body before closing to return connections to the pool instead of destroying them. Use in all upstream response handling paths. |
| T23.3 | Make `MaxIdleConnsPerHost` configurable per-upstream in composition YAML: `upstreams.*.pool: { max_idle: 100 }`. |

Reference: `experimental/v1.1-v1.2` → `internal/client/client.go`

### M23 Verification gate

```
# Load test with k6: fan-out composition hitting 5 upstreams
# Before: observe connection churn in netstat, P95 latency > 200ms
# After: stable idle connections, P95 latency < 50ms
```

---

## M24 — Production Monitoring & Load Testing (extends M14, M15)

Priority: **Medium — operational readiness.**

| Task | Description |
|------|-------------|
| T24.1 | Prometheus recording rules: pre-compute dashboard queries (composition request rate, P50/P95/P99 latency, error rate by composition). |
| T24.2 | Prometheus alert rules: P95 latency > threshold (configurable, default 1s), error rate > 5% sustained 5m, config reload failures, gateway down. |
| T24.3 | k6 load test script: exercise gateway compositions and Studio API at ~1000 RPS (50 VUs). Thresholds: P95 < 1s, error rate < 1%. Run as part of CI on release branches. |
| T24.4 | Docker Compose production stack: gateway, studio, Prometheus, Jaeger, with `deploy/` directory containing all configs, `.env.example`, and a `docker-compose.yml`. |

Reference: `experimental/v1.1-v1.2` → `deploy/prometheus/`, `test/loadtest.js`, `deploy/docker-compose.yml`

### M24 Verification gate

```
docker compose -f deploy/docker-compose.yml up -d
# Prometheus loads rules without error
# Trigger alert condition → alert fires
# k6 run test/loadtest.js → all thresholds pass
```

---

## M25 — Browser Session & User Preferences (extends M12)

Priority: **Low — quality-of-life for Studio users.**

| Task | Description |
|------|-------------|
| T25.1 | Browser session middleware: set `restitch_browser_id` cookie (256-bit random, 1-year expiry, `HttpOnly`, `SameSite=Strict`). Store sessions in DB. No login required. |
| T25.2 | Preferences CRUD API: `GET/PUT /api/v1/preferences` scoped to browser session. Store pinned compositions, sidebar collapsed state, default time range. |
| T25.3 | Database migration for browser_sessions table. |
| T25.4 | Wire preferences into Studio frontend: pin button on composition cards, persist sidebar state, restore on reload. |

Reference: `experimental/v1.1-v1.2` → `internal/studio/session/session.go`, `internal/studio/api/preferences.go`

### M25 Verification gate

```
# Open Studio in browser → cookie is set
curl -b cookies.txt localhost:8090/api/v1/preferences → empty prefs
curl -X PUT -b cookies.txt localhost:8090/api/v1/preferences -d '{"pinned":["comp-1"]}' → 200
# Refresh browser → pinned compositions persist
# Different browser → independent preferences
```

