# M21 — Gateway Registry Polling Design

**Date:** 2026-07-15
**Milestone:** M21
**Status:** Approved
**Depends on:** M20 (Config Registry), M10 (Hot Reload)

## Summary

Adds a registry polling mode to the gateway as an alternative to file-watching.
The gateway periodically fetches the config bundle from Studio's registry
endpoint (`GET /api/v1/registry/bundle`), using ETag-based change detection and
exponential backoff on errors. When running in registry mode, the gateway no
longer watches a local file — it polls the registry on an interval and
hot-swaps the pipeline when the bundle changes.

## Design Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | CLI: `--registry-url` flag on `run`, mutually exclusive with `--config` | Industry standard (Apollo Router, Cosmo, Hive). No separate subcommand needed. |
| D2 | Stdlib exponential backoff (~30 LOC) | `cenkalti/backoff` not in approved deps (§3.3). Simple enough to implement. |
| D3 | New package `internal/hotreload/` | Clean separation from file-watch code in `cmd/restitch/reload.go`. Testable independently. |
| D4 | Approach B: refactor reload to accept bytes | Both file mode and registry mode share the same `reloadFromBytes` compile-swap-close path. No disk writes in registry mode. |
| D5 | Server-side 304 via `If-None-Match` | M20's bundle endpoint already supports this. More efficient than downloading the full bundle every poll. |
| D6 | Prometheus metrics for polling health | Counter, histogram, and last-success gauge. Same pattern as existing M8 metrics. |

## Package Structure

### `internal/hotreload/client.go` — Registry HTTP Client

```go
type ClientOption func(*RegistryClient)

func WithAdminKey(key string) ClientOption
func WithHTTPClient(c *http.Client) ClientOption

type RegistryClient struct {
    url        string
    httpClient *http.Client // default: 15s timeout
    adminKey   string       // optional, sent as X-Admin-Key
}

type FetchResult struct {
    YAML             []byte
    ETag             string
    CompositionCount int
    NotModified      bool // true when server returned 304
}

func NewRegistryClient(url string, opts ...ClientOption) *RegistryClient
func (c *RegistryClient) Fetch(ctx context.Context, lastETag string) (*FetchResult, error)
```

**Behavior:**

- Calls `GET <url>/api/v1/registry/bundle`
- Sends `If-None-Match: <lastETag>` when `lastETag` is non-empty
- On 304: returns `FetchResult{NotModified: true}`
- On 200: parses JSON response, extracts `yaml_content`, `etag`, `composition_count`
- On non-200/304: returns error with status code
- Stateless — caller manages the ETag

### `internal/hotreload/poller.go` — Polling Engine

```go
type Poller struct {
    client    *RegistryClient
    interval  time.Duration
    reloadFn  func(yaml []byte) (string, error)
    triggerCh chan struct{} // buffered size 1
    status    atomic.Pointer[PollStatus]
    metrics   *observability.Metrics // nilable
}

type PollStatus struct {
    LastPollTime      time.Time
    LastSuccessTime   time.Time
    LastETag          string
    CompositionCount  int
    LastError         string
    ErrorType         string // "fetch", "invalid_config", "reload", or ""
    ConsecutiveErrors int    // resets on success
}

func NewPoller(client *RegistryClient, interval time.Duration, reloadFn func([]byte) (string, error), metrics *observability.Metrics) *Poller
func (p *Poller) Run(ctx context.Context) error // blocking, returns on ctx cancel
func (p *Poller) Trigger()                       // non-blocking send on triggerCh
func (p *Poller) Status() PollStatus             // atomic read for admin endpoint
```

### `internal/hotreload/backoff.go` — Stdlib Backoff

```go
// backoffDuration returns min(base * 2^attempt, max) with ±20% jitter.
func backoffDuration(attempt int, base, max time.Duration) time.Duration
```

## Polling Engine Behavior

### Normal Loop

1. Fetch bundle with last known ETag
2. On 304 (not modified): increment `not_modified` counter, sleep `poll-interval`, repeat
3. On 200: call `reloadFn(yamlBytes)` → on success update ETag + status, reset backoff → sleep `poll-interval`
4. On error: classify, increment backoff counter, sleep backoff duration, repeat
5. On context cancel: return cleanly

### Error Classification

| Error type | Examples | Behavior |
|-----------|----------|----------|
| `fetch` | Network error, DNS, connection refused, non-200/304 HTTP | Transient — backoff and retry. Gateway keeps serving last known config. |
| `invalid_config` | Registry returned YAML that fails compile | Persistent — backoff and retry (registry might fix the config). Log error. |
| `reload` | Pipeline swap failed | Persistent — backoff and retry. Log error. |

### Backoff Policy

- Base: `poll-interval` (default 10s)
- Max: 5 minutes
- Formula: `min(base * 2^attempt, max)` with ±20% jitter
- Resets to zero on any successful poll (including 304)
- Progression: ~10s → ~20s → ~40s → ~80s → ~160s → 300s (cap)

### Startup Behavior

On first boot in registry mode, the poller does a **blocking** initial fetch
before the gateway starts serving. If this fails, the gateway exits with a
clear error: `"failed to fetch initial config from registry at <url>: <error>"`.
No serving with zero config — matches how `--config` mode exits if the file is
missing. After the initial load succeeds, all subsequent failures are non-fatal
(keep serving last good config).

### SIGHUP

Sends on `triggerCh` (buffered size 1, non-blocking send). The poll loop
selects on `triggerCh` alongside the interval timer — a trigger skips the
sleep and does an immediate fetch, bypassing any active backoff.

## CLI Wiring

### Flags on `run` subcommand (`cmd/restitch/run.go`)

| Flag | Type | Default | Env var | Description |
|------|------|---------|---------|-------------|
| `--config` | string | `restitch.yaml` | `RESTITCH_CONFIG` | Config file path (existing) |
| `--registry-url` | string | — | `RESTITCH_REGISTRY_URL` | Registry URL for polling mode |
| `--registry-key` | string | — | `RESTITCH_REGISTRY_KEY` | Admin API key for registry auth (optional) |
| `--poll-interval` | duration | `10s` | `RESTITCH_POLL_INTERVAL` | Registry poll interval |

**Mutual exclusivity:** If both `--config` and `--registry-url` are provided,
exit with error: `"--config and --registry-url are mutually exclusive"`. If
neither is provided, default to `--config restitch.yaml` (backwards compatible).

### Reload Refactor

Extract the body of the existing `doReload` closure into a shared function:

```go
func reloadFromBytes(yamlBytes []byte, deps reloadDeps) (string, error)
```

- **File mode:** `reloader.reload()` reads the file, calls `reloadFromBytes(bytes, deps)`
- **Registry mode:** poller's `reloadFn` is wired to `reloadFromBytes`

Both modes share the same compile → hash check → swap → schedule-old-close path.

### Mode-Specific Startup in `run.go`

```go
if registryURL != "" {
    // Registry mode:
    // 1. Build RegistryClient
    // 2. Build Poller with reloadFromBytes as reloadFn
    // 3. Blocking initial fetch (exit on failure)
    // 4. Start data server
    // 5. Run poller in goroutine
    // 6. SIGHUP calls poller.Trigger()
    // 7. Admin Reload dep calls poller.Trigger()
    // fsnotify is NOT started
} else {
    // File mode: existing behavior (fsnotify + SIGHUP → file reload)
}
```

## Admin Status Endpoint

### `GET /admin/api/registry/status`

Only registered when the gateway is running in registry mode. Not registered
in file mode (returns 404 from ServeMux).

**Response (healthy):**
```json
{
  "mode": "registry",
  "registry_url": "http://studio:8090",
  "poll_interval_seconds": 10,
  "last_poll": "2026-07-15T14:30:00Z",
  "last_success": "2026-07-15T14:30:00Z",
  "etag": "a1b2c3d4e5f6",
  "composition_count": 5,
  "error": null,
  "error_type": null,
  "consecutive_errors": 0
}
```

**Response (errored):**
```json
{
  "mode": "registry",
  "registry_url": "http://studio:8090",
  "poll_interval_seconds": 10,
  "last_poll": "2026-07-15T14:30:12Z",
  "last_success": "2026-07-15T14:30:00Z",
  "etag": "a1b2c3d4e5f6",
  "composition_count": 5,
  "error": "connection refused",
  "error_type": "fetch",
  "consecutive_errors": 3
}
```

**Wiring:** Add `RegistryStatus func() *hotreload.PollStatus` field to
`admin.Deps` (nilable). When non-nil, register the endpoint. The poller
exposes `Status()` via its atomic pointer — lock-free reads from the admin
handler.

## Prometheus Metrics

Added to `observability.Metrics`, only registered when running in registry mode:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `restitch_registry_polls_total` | counter | `result` (success, not_modified, error) | Total poll attempts |
| `restitch_registry_poll_duration_seconds` | histogram | — | Time per poll (fetch + reload) |
| `restitch_registry_last_success_timestamp` | gauge | — | Unix timestamp of last successful poll |

The poller receives `*Metrics` (nilable) and increments at appropriate points.
In file mode these metrics do not exist in the registry (no phantom zeros).

## Testing Strategy

### Unit Tests (`internal/hotreload/`)

**`client_test.go`:**
- 200 with YAML + ETag → correct FetchResult
- 304 on matching `If-None-Match` → `NotModified: true`
- 500 → error
- Connection refused → error
- Admin key header sent when configured

**`poller_test.go`:**
- Happy path: polls, gets YAML, calls reloadFn, status updates
- 304 no-op: ETag unchanged, reloadFn not called, counter incremented as `not_modified`
- Backoff: simulate 3 consecutive errors, assert sleep durations increase
- Trigger: send on triggerCh, assert immediate poll (no interval wait)
- Error classification: network error → `fetch`, reloadFn returns error → `reload`
- Context cancel → Run returns cleanly

**`backoff_test.go`:**
- Table-driven: assert capped at max, jitter within ±20%

### Integration Test (`internal/testenv/`)

**E2E-registry:**
- Start Studio with a registry config → start gateway in registry mode → verify
  gateway serves the composition
- Update config in Studio → wait for poll → verify new route serves, old 404s
- Kill Studio → gateway keeps serving last known config, admin status shows error

### CLI Tests (`cmd/restitch/`)

- Both `--config` and `--registry-url` provided → exit code 1 with error
- Registry mode with unreachable URL → exit code 1

## Verification Gate

Note: PLAN.md's gate uses `restitch gateway --config-source=registry`. This
design uses `restitch run --registry-url` instead (same `run` subcommand,
no `--config-source` indirection). The gate script for M21 will be written
to match this design.

```bash
# Start gateway in registry mode pointing at Studio
restitch run --registry-url http://localhost:8090
# Verify it loads compositions from Studio bundle
curl localhost:9090/admin/api/registry/status → last_poll, etag, count
# Update a config in Studio → gateway picks it up within poll interval
# Kill Studio → gateway retries with backoff, keeps serving last known config
# Send SIGHUP → immediate poll
```
