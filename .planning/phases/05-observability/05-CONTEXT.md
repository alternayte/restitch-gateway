# Phase 5: Observability - Context

**Gathered:** 2026-02-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Structured logging and debugging capabilities for understanding request flow and identifying issues. Includes request/step logging with timing, request ID tracing, and upstream health checks. Metrics export and distributed tracing integration are separate concerns.

</domain>

<decisions>
## Implementation Decisions

### Log format & fields
- JSON structured logging with snake_case field names (request_id, status_code, duration_ms)
- Request/response bodies logged at debug level only (off by default)
- Composition name and step name included in every log line for context
- Log levels: Claude's discretion based on standard practices

### Step timing display
- Both individual step logs AND summary in request completion log
- Millisecond precision (duration_ms: 142)
- Wave number included in step logs (wave: 1) to show DAG execution order explicitly
- Slow step detection: Claude's discretion on threshold-based warnings

### Request tracing
- ULID format for request IDs (26 chars, time-sortable, collision-safe)
- Honor incoming X-Request-ID header if present, generate ULID if not
- Forward request ID to upstreams with configurable header name per upstream (default: X-Request-ID)
- Return request ID to clients in X-Request-ID response header

### Health check depth
- Separate endpoints: /health for gateway, /health/upstreams for upstream connectivity
- Upstream check method: HEAD request to base URL by default, configurable health path per upstream
- Response includes status, latency, and last check timestamp per upstream
- /ready remains independent of upstream health (gateway resilience via Phase 4 partial responses)

### Claude's Discretion
- Log level hierarchy and defaults
- Whether to implement slow step threshold warnings
- Upstream health check interval/caching

</decisions>

<specifics>
## Specific Ideas

- ULID chosen over UUID for time-sortability in log grep/analysis
- Upstream health independent from /ready to avoid cascading outages (aligns with Phase 4 graceful degradation philosophy)
- Configurable trace header name supports diverse upstream conventions

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 05-observability*
*Context gathered: 2026-02-03*
