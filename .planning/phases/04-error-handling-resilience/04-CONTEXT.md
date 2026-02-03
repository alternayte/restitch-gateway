# Phase 4: Error Handling & Resilience - Context

**Gathered:** 2026-02-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Gateway handles upstream failures gracefully and returns partial data when optional steps fail. Users receive partial responses with clear signaling when some upstreams fail, making the gateway more reliable than direct API calls. Error matching rules allow configured responses for specific error conditions.

</domain>

<decisions>
## Implementation Decisions

### Partial response signaling
- HTTP 200 with `X-Partial-Response: true` header when any step fails
- Top-level `_errors` field in response body with failure details
- Each error entry contains: step name + error message (not status codes or timing)
- Example: `{ user: {...}, _errors: [{ step: "inventory", message: "upstream timeout" }] }`

### Error matching behavior
- Error matching rules replace the failed step's slot with the configured body value
- Final HTTP status code is always 200 (composition succeeded, partial data is valid)
- If a step with dependents fails, dependent steps are skipped and marked as `dependency_failed` in `_errors`
- Skipped dependents cascade (entire dependency chain skipped on root failure)

### Timeout hierarchy
- Both global composition timeout AND per-step timeouts
- Default step timeout: 30 seconds if not configured
- Timeouts configured at upstream level with step-level override
- Timeout treated as distinct error type (not 504), appears as `"message": "timeout"` in `_errors`

### Optional step semantics
- Failed optional steps return `null` for `steps.X` in expressions
- No automatic retries on timeout — fail immediately
- Steps required by default, must be explicitly marked optional

### Claude's Discretion
- Error matching rule syntax (status code list vs ranges vs wildcards)
- `optional: true` vs `required: false` syntax choice
- Whether dependents of optional steps inherit optional status
- Global composition timeout default value

</decisions>

<specifics>
## Specific Ideas

- Partial response signaling combines header + body for both programmatic detection and detailed info
- Keep it simple: no retries in this phase (fail immediately on timeout)
- Cascading skip for dependents is predictable and debuggable

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 04-error-handling-resilience*
*Context gathered: 2026-02-03*
