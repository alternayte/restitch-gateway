# Observability

A gateway exposes Prometheus metrics, OpenTelemetry traces and structured
logs. A managed gateway also sends heartbeats, usage aggregates and sampled
requests to Studio.

## Prometheus Metrics

The gateway serves `GET /metrics` on the ops port. The ops port is off by
default. Set `RESTITCH_OPS_PORT`, for example to `9090`, to turn it on. The
data port does not serve metrics.

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
| `restitch_telemetry_dropped_total` | counter | `kind` | Telemetry a managed gateway dropped after one failed retry to push it to Studio: `usage` (one-minute aggregates) or `requests` (sampled requests). Request handling never waits on a push. |
| `restitch_studio_pulls_total` | counter | `result` | Managed mode: pulls from Studio by result (`success`, `not_modified`, `error`) |
| `restitch_studio_pull_duration_seconds` | histogram | — | Managed mode: duration of a pull from Studio |
| `restitch_studio_last_pull_success_timestamp` | gauge | — | Managed mode: Unix time of the last successful pull |

### Scrape Configuration

```yaml
# prometheus.yml
scrape_configs:
  - job_name: restitch
    static_configs:
      - targets: ['restitch-gateway:9090']   # the RESTITCH_OPS_PORT of each gateway
```

`deploy/prometheus/` has a complete configuration with recording and alert
rules.

---

## Probes

The data port serves both probes.

| Endpoint | Answer |
|----------|--------|
| `GET /health` | 200 while the process runs. |
| `GET /ready` | 503 until the gateway applies its first artifact, then 200. |

A composition cannot use these paths.

---

## Studio telemetry

A managed gateway sends three kinds of telemetry to Studio with its gateway
token:

| Kind | Interval | Content |
|------|----------|---------|
| Heartbeat | 15s | Instance ID, binary version, applied artifact version, result of the last pull. |
| Usage | 1 minute | Request, error and latency aggregates per composition, step and upstream. |
| Requests | 10s | Every failed request and a capped sample of successful requests, with step timings and redacted headers. Never a body. |

Studio sums the data of every instance of an environment. The **Overview**
page of the environment shows it. Studio keeps usage for 7 days by default.
An owner changes the retention and the sample cap per environment. See
[studio.md](studio.md#overview-and-telemetry).

A push that fails twice is dropped and counted in
`restitch_telemetry_dropped_total`. A standalone gateway sends no
telemetry.

---

## Distributed Tracing

Restitch supports OpenTelemetry (OTel) distributed tracing via the OTLP HTTP
exporter. When enabled, the gateway creates spans for each composition
execution and each upstream step call.

### Enabling

Set the `OTEL_EXPORTER_OTLP_ENDPOINT` environment variable to the address
of your OTel collector:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318 restitch gateway
```

When the variable is unset or empty, tracing is disabled (zero overhead).
The exporter also reads the other standard `OTEL_EXPORTER_OTLP_*`
variables, for example `OTEL_EXPORTER_OTLP_HEADERS`.

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

---

## Logging

Restitch uses Go's `slog` for structured logging. Set the format and the
level with flags or environment variables:

```bash
restitch gateway -log-format=json -log-level=debug
RESTITCH_LOG_FORMAT=json RESTITCH_LOG_LEVEL=debug restitch gateway
```

| Variable | Values | Default |
|----------|--------|---------|
| `RESTITCH_LOG_FORMAT` | `json`, `text` | `json` |
| `RESTITCH_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |

`restitch studio` reads the same two variables.


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
