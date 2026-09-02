# Observability

Restitch exposes Prometheus metrics, structured logging, an admin API with a
request ring buffer, and per-composition statistics.

## Prometheus Metrics

Metrics are served on the admin port (default 9090) at `GET /metrics`.

### Metric Families

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `restitch_requests_total` | counter | `composition`, `method`, `status` | Total composition requests handled |
| `restitch_request_duration_seconds` | histogram | `composition` | End-to-end composition request latency |
| `restitch_partial_responses_total` | counter | `composition` | Responses where at least one optional step failed |
| `restitch_step_duration_seconds` | histogram | `composition`, `step`, `upstream`, `status` | Per-step upstream call latency |
| `restitch_upstream_requests_total` | counter | `upstream`, `status_class` | Total upstream HTTP requests (`2xx`, `3xx`, `4xx`, `5xx`, `error`) |
| `restitch_retries_total` | counter | `upstream` | Total retry attempts (not counting the initial request) |
| `restitch_breaker_state` | gauge | `upstream` | Circuit breaker state: 0 = closed, 1 = half-open, 2 = open |
| `restitch_cache_hits_total` | counter | `composition`, `step` | Step cache hits |
| `restitch_cache_misses_total` | counter | `composition`, `step` | Step cache misses |
| `restitch_coalesced_total` | counter | `composition`, `step` | Requests that shared a coalesced upstream call |
| `restitch_registry_polls_total` | counter | `result` | Registry polls by result (`success`, `not_modified`, `error`) |
| `restitch_registry_poll_duration_seconds` | histogram | — | Registry poll duration |
| `restitch_registry_last_success_timestamp` | gauge | — | Unix time of the last successful registry poll |

### Scrape Configuration

```yaml
# prometheus.yml
scrape_configs:
  - job_name: restitch
    static_configs:
      - targets: ['restitch:9090']
```

---

## Distributed Tracing

Restitch supports OpenTelemetry (OTel) distributed tracing via the OTLP HTTP
exporter. When enabled, the gateway creates spans for each composition
execution and each upstream step call.

### Enabling

Set the `OTEL_EXPORTER_OTLP_ENDPOINT` environment variable to the address
of your OTel collector:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318 restitch run
```

When the variable is unset or empty, tracing is disabled (zero overhead).

### Span Structure

Each incoming request produces a root span for the composition, with child
spans for each step:

```
composition: user-dashboard (root)
  ├── step: user (upstream: users-api)
  └── step: orders (upstream: orders-api)
```

Span attributes include composition name, step name, upstream name, HTTP
method, status code, and whether the step was optional.

### Context Propagation

Trace context is propagated to upstream services via W3C `traceparent`
headers. If the incoming request already carries a `traceparent`, the
gateway continues that trace rather than starting a new one.

### Request Records

The `trace_id` field is included in request records returned by the admin
API (`GET /admin/api/requests`) when tracing is active, making it easy to
correlate gateway requests with downstream traces.

---

## Admin API

The admin API runs on a separate port (default 9090) from the data plane,
bound to loopback by default (`admin.bind`). The `admin.api_key` is
required: every request without a matching `X-Admin-Key` is rejected,
including `OPTIONS` preflights. `GET /metrics` and `GET /health` stay open.
POST endpoints (`/admin/api/reload`, `/admin/api/validate`) are rate-limited
at 10 requests per second with a burst of 5, per IP.

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/admin/api/info` | Gateway info (version, uptime, config hash, composition/upstream counts) |
| `GET` | `/admin/api/compositions` | List all compositions with steps and wave layout |
| `GET` | `/admin/api/compositions/{name}` | Single composition detail (404 if unknown) |
| `GET` | `/admin/api/upstreams` | List upstreams with health status |
| `GET` | `/admin/api/requests?limit=100` | Recent requests from the ring buffer (newest first) |
| `GET` | `/admin/api/requests/{id}` | One stored request record with step detail (durable storage only) |
| `GET` | `/admin/api/stats` | Per-composition statistics (counts, error rates, latency percentiles) |
| `GET` | `/admin/api/stats/timeseries` | Bucketed request/error/partial counts over a range |
| `GET` | `/admin/api/stats/steps` | Per-step aggregates for one composition |
| `GET` | `/admin/api/registry/status` | Registry poll status (registry mode only): last poll, ETag, count, error state |
| `POST` | `/admin/api/validate` | Validate raw YAML config (body = YAML text) |
| `POST` | `/admin/api/reload` | Trigger hot config reload |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/health` | Admin server liveness (`{"status":"ok"}`) |

### Response Formats

**GET /admin/api/info**

```json
{
  "version": "v2.0.0",
  "uptime_seconds": 3600,
  "config_hash": "a1b2c3d4...",
  "config_path": "/etc/restitch/restitch.yaml",
  "compositions": 5,
  "upstreams": 3
}
```

**GET /admin/api/compositions**

```json
[
  {
    "name": "user-dashboard",
    "path": "/api/users/{id}/dashboard",
    "method": "GET",
    "public": false,
    "steps": [
      {
        "name": "user",
        "upstream": "users-api",
        "method": "GET",
        "optional": false,
        "timeout_ms": 5000,
        "depends_on": []
      }
    ],
    "waves": [["user", "loyalty"], ["merge"]]
  }
]
```

**GET /admin/api/upstreams**

```json
[
  {
    "name": "users-api",
    "url": "https://users.internal",
    "auth_type": "header",
    "timeout_ms": 10000,
    "health": {
      "status": "healthy",
      "latency_ms": 12.5,
      "checked_at": "2026-07-09T10:00:00Z",
      "error": ""
    }
  }
]
```

**GET /admin/api/requests**

```json
[
  {
    "id": "01JXXXXXXXXXX",
    "time": "2026-07-09T10:00:00Z",
    "composition": "user-dashboard",
    "method": "GET",
    "path": "/api/users/42/dashboard",
    "status": 200,
    "duration_ms": 45.2,
    "partial": false,
    "steps": [
      {
        "name": "user",
        "status": "success",
        "wave": 0,
        "duration_ms": 12.3,
        "http_status": 200
      }
    ]
  }
]
```

**GET /admin/api/stats**

```json
{
  "total_requests": 1500,
  "total_errors": 12,
  "partial_responses": 45,
  "per_composition": {
    "user-dashboard": {
      "count": 800,
      "errors": 5,
      "avg_ms": 42.5,
      "p95_ms": 120.0
    }
  }
}
```

**POST /admin/api/validate**

Request body: raw YAML config text.

```json
{"valid": true, "errors": []}
```

```json
{"valid": false, "errors": ["composition 'x': unknown upstream 'y'"]}
```

**POST /admin/api/reload**

```json
{"ok": true, "config_hash": "a1b2c3d4..."}
```

```json
{"ok": false, "errors": ["invalid YAML: ..."]}
```

---

## Request Ring Buffer

The admin server maintains a fixed-size ring buffer of recent requests
(default 500 entries, configured via `admin.request_log_size`). Each entry
includes the composition name, timing, status, partial flag, and per-step
records with wave number, duration, and HTTP status.

The ring buffer is useful for debugging recent requests without external log
infrastructure. Oldest entries are overwritten as new requests arrive.

---

## Logging

Restitch uses Go's `slog` for structured logging.

### Configuration

```yaml
server:
  log_format: json    # json | text
  log_level: info     # debug | info | warn | error
```

Or via environment / flags:

```bash
restitch run -log-format=json -log-level=debug
RESTITCH_LOG_FORMAT=json RESTITCH_LOG_LEVEL=debug restitch run
```

### Log Fields

Every log line includes `request_id` when processing a request. The request
ID is generated (ULID) per incoming request and propagated to all step
executions.

**Access log** (per request):

| Field | Description |
|-------|-------------|
| `method` | HTTP method |
| `path` | Request path |
| `status` | Response status code |
| `duration_ms` | Total request duration |
| `remote_addr` | Client address |
| `user_agent` | Client user agent |
| `request_id` | Unique request identifier |

**Composition handler log** (per composition execution):

| Field | Description |
|-------|-------------|
| `composition` | Composition name |
| `step_timings` | Per-step timing details |
| `slowest_step` | Name of the slowest step |
| `gateway_overhead_ms` | Handler time minus executor time |
| `request_id` | Request identifier |

**Step log** (per upstream call):

| Field | Description |
|-------|-------------|
| `step` | Step name |
| `upstream` | Upstream name |
| `status` | Step status (success/failed/skipped) |
| `duration_ms` | Step execution time |
| `http_status` | Upstream HTTP response code |
| `request_id` | Request identifier |
