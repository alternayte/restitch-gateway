---
phase: 05-observability
plan: 03
subsystem: health
tags: [health-check, upstream-status, monitoring, observability]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: HTTP server with routing, health endpoints
  - phase: 02-composition
    provides: CompiledConfig with upstream definitions
provides:
  - /health/upstreams endpoint for monitoring upstream connectivity
  - HealthPath config field for custom health check paths
  - UpstreamInfo type to avoid import cycles
affects: [deployment-monitoring, alerting, observability]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Concurrent health checks with sync.WaitGroup"
    - "UpstreamInfo bridge type to avoid import cycles"
    - "HEAD request by default, GET with health_path"

key-files:
  created: []
  modified:
    - internal/server/health.go
    - internal/composition/config.go
    - cmd/restitch/main.go

key-decisions:
  - "UpstreamInfo type avoids composition/server import cycle"
  - "HEAD request by default for minimal overhead"
  - "GET request when health_path configured (assumes health endpoint)"
  - "2xx/3xx = healthy, 4xx/5xx = unhealthy"
  - "10 second timeout for all upstream health checks"
  - "/ready independent of upstream health (per CONTEXT.md)"

patterns-established:
  - "Bridge types for avoiding import cycles"
  - "Concurrent upstream checks with mutex-protected results"

# Metrics
duration: 4min
completed: 2026-02-03
---

# Phase 5 Plan 03: Upstream Health Endpoint Summary

**/health/upstreams endpoint with per-upstream status, latency, and configurable health paths**

## Performance

- **Duration:** 4 min
- **Started:** 2026-02-03T11:20:07Z
- **Completed:** 2026-02-03T11:24:22Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- /health/upstreams endpoint returns all upstream connectivity status
- Each upstream shows URL, status (healthy/unhealthy), latency_ms, last_check, and error
- Overall status "healthy" or "degraded" based on upstream health
- Configurable health_path per upstream (default: HEAD to base URL)
- /ready remains independent of upstream health

## Task Commits

Each task was committed atomically:

1. **Task 1: Upstream config health path and checker** - `bec963b` (feat)
2. **Task 2: Upstream health endpoint and integration** - `3a1b5b8` (feat)

## Files Created/Modified
- `internal/composition/config.go` - Added HealthPath field to Upstream struct
- `internal/server/health.go` - Added UpstreamHealthResponse, UpstreamStatus, UpstreamInfo types; checkUpstreamHealth function; UpstreamHealthHandler
- `cmd/restitch/main.go` - Register /health/upstreams endpoint, build UpstreamInfo map from config

## Decisions Made
- **UpstreamInfo bridge type:** Created UpstreamInfo in server package to pass upstream data without importing composition (avoids import cycle since composition/handler.go imports server)
- **HEAD by default:** Uses HEAD request to base URL for minimal overhead when no health_path configured
- **GET with health_path:** Uses GET when health_path configured, assuming health endpoints return body content
- **2xx/3xx healthy:** Status codes < 400 are considered healthy
- **10s timeout:** All health checks have 10 second context timeout
- **Always 200 response:** Returns HTTP 200 even if upstreams are unhealthy (for monitoring tool compatibility)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Import cycle between server and composition packages**
- **Found during:** Task 1 (Adding health check types)
- **Issue:** Plan suggested importing composition.Upstream directly, but composition/handler.go already imports server, creating a cycle
- **Fix:** Created UpstreamInfo bridge type in server package; main.go builds UpstreamInfo map from CompiledConfig
- **Files modified:** internal/server/health.go, cmd/restitch/main.go
- **Verification:** Build passes, no import cycle errors
- **Committed in:** bec963b (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Auto-fix necessary for compilation. Bridge type pattern is clean and maintains separation of concerns.

## Issues Encountered
None beyond the auto-fixed import cycle.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Upstream health monitoring complete
- Phase 5 observability features partially complete (01-02-03 done)
- Ready for remaining observability plans

---
*Phase: 05-observability*
*Plan: 03*
*Completed: 2026-02-03*
