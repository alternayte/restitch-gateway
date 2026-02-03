# Requirements: Restitch

**Defined:** 2026-02-03
**Core Value:** Frontend teams can compose data from multiple backend services without writing, deploying, or maintaining BFF code.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Composition Engine

- [ ] **COMP-01**: Gateway parses YAML configuration defining compositions
- [ ] **COMP-02**: Compositions define steps with upstream service calls
- [ ] **COMP-03**: Steps can declare dependencies on other steps
- [ ] **COMP-04**: Gateway builds DAG from step dependencies
- [ ] **COMP-05**: Independent steps execute in parallel via goroutines
- [ ] **COMP-06**: Dependent steps wait for their dependencies to complete
- [ ] **COMP-07**: Expr language evaluates dynamic values in step paths
- [ ] **COMP-08**: Expr language evaluates dynamic values in step params
- [ ] **COMP-09**: Expr language evaluates dynamic values in response body
- [ ] **COMP-10**: Steps can access results from completed dependency steps
- [ ] **COMP-11**: Response body merges/reshapes data from multiple steps

### Upstream Authentication

- [ ] **AUTH-01**: Upstream auth strategy: header (static API key/token injection)
- [ ] **AUTH-02**: Upstream auth strategy: basic (username/password)
- [ ] **AUTH-03**: Upstream auth strategy: passthrough (forward caller's auth header)
- [ ] **AUTH-04**: Upstream auth strategy: oauth2_client_credentials
- [ ] **AUTH-05**: OAuth2 tokens are cached and auto-refreshed before expiry
- [ ] **AUTH-06**: Each upstream can have its own auth configuration

### Error Handling

- [ ] **ERR-01**: Compositions define error matching rules on step status codes
- [ ] **ERR-02**: Matched errors return configured status code and body
- [ ] **ERR-03**: Steps can be marked as optional (non-blocking on failure)
- [ ] **ERR-04**: Optional step failures return partial response with remaining data
- [ ] **ERR-05**: Upstream timeouts are configurable per step

### Gateway Core

- [ ] **GATE-01**: Gateway routes requests by path pattern to compositions
- [ ] **GATE-02**: Gateway routes requests by HTTP method
- [ ] **GATE-03**: Gateway terminates TLS (HTTPS support)
- [ ] **GATE-04**: Gateway exposes /health endpoint for liveness checks
- [ ] **GATE-05**: Gateway exposes /ready endpoint for readiness checks
- [ ] **GATE-06**: Gateway drains connections gracefully on shutdown signal

### Observability

- [ ] **OBS-01**: Structured JSON logging for all requests
- [ ] **OBS-02**: Logs include request ID, method, path, status, duration
- [ ] **OBS-03**: Per-step timing logged (which step took how long)
- [ ] **OBS-04**: DAG execution order logged for debugging

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Caching

- **CACHE-01**: Response caching with configurable TTL per step
- **CACHE-02**: Cache key derivation from request parameters
- **CACHE-03**: Cache invalidation via API or TTL

### Advanced Resilience

- **RES-01**: Circuit breaker per upstream service
- **RES-02**: Request coalescing for duplicate in-flight requests
- **RES-03**: Retry policies with exponential backoff

### Configuration

- **CFG-01**: Hot-reload configuration without restart
- **CFG-02**: Configuration validation on load with helpful errors
- **CFG-03**: OpenAPI spec import to auto-generate upstream definitions

### Studio (Control Plane)

- **STU-01**: Web UI for viewing compositions
- **STU-02**: Visual DAG editor for creating compositions
- **STU-03**: Latency waterfall visualization
- **STU-04**: Configuration audit log with rollback
- **STU-05**: Real-time metrics dashboard

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| WASM plugins | Complexity; hardcode what's needed for v1 |
| Inbound JWT validation | Behind VPN/mesh for internal use; simple API key sufficient |
| GraphQL support | REST-only gateway; GraphQL has Apollo Federation |
| Rate limiting | Use upstream rate limits or external solution for v1 |
| Load balancing | Single upstream per service; use service mesh for v1 |
| Request transformation | Expr covers field mapping; full body transformation deferred |
| WebSocket support | REST-only for v1 |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| COMP-01 | TBD | Pending |
| COMP-02 | TBD | Pending |
| COMP-03 | TBD | Pending |
| COMP-04 | TBD | Pending |
| COMP-05 | TBD | Pending |
| COMP-06 | TBD | Pending |
| COMP-07 | TBD | Pending |
| COMP-08 | TBD | Pending |
| COMP-09 | TBD | Pending |
| COMP-10 | TBD | Pending |
| COMP-11 | TBD | Pending |
| AUTH-01 | TBD | Pending |
| AUTH-02 | TBD | Pending |
| AUTH-03 | TBD | Pending |
| AUTH-04 | TBD | Pending |
| AUTH-05 | TBD | Pending |
| AUTH-06 | TBD | Pending |
| ERR-01 | TBD | Pending |
| ERR-02 | TBD | Pending |
| ERR-03 | TBD | Pending |
| ERR-04 | TBD | Pending |
| ERR-05 | TBD | Pending |
| GATE-01 | TBD | Pending |
| GATE-02 | TBD | Pending |
| GATE-03 | TBD | Pending |
| GATE-04 | TBD | Pending |
| GATE-05 | TBD | Pending |
| GATE-06 | TBD | Pending |
| OBS-01 | TBD | Pending |
| OBS-02 | TBD | Pending |
| OBS-03 | TBD | Pending |
| OBS-04 | TBD | Pending |

**Coverage:**
- v1 requirements: 32 total
- Mapped to phases: 0
- Unmapped: 32 (will be mapped during roadmap creation)

---
*Requirements defined: 2026-02-03*
*Last updated: 2026-02-03 after initial definition*
