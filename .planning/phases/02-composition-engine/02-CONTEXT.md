# Phase 2: Composition Engine - Context

**Gathered:** 2026-02-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Users can define multi-step compositions in YAML that fetch data from multiple APIs in parallel and merge responses. All steps are required in Phase 2 — optional steps and partial responses are Phase 4.

</domain>

<decisions>
## Implementation Decisions

### YAML Structure
- Single config file for all compositions (not file-per-composition)
- Named upstreams defined once, referenced by name in steps (not inline URLs)
- Smart defaults — GET if no method specified, empty headers if not specified
- Dependencies: inferred from Expr usage by default, explicit `depends_on` available for edge cases

### Expression Language
- Use expr-lang/expr library natively — support whatever syntax it provides
- Expressions can access: req.path, req.query, req.headers, req.body (for POST/PUT)
- Step results include full response: status, headers, and body (steps.user.status, steps.user.body, etc.)
- Array operations: whatever expr-lang/expr supports natively (filter, map, etc.)

### Execution Behavior
- Fail fast on required step failure — cancel remaining steps immediately
- Upstream error passthrough — return upstream's status code and body directly
- Default 30-second timeout for upstream calls (per-step configuration in Phase 4)
- Auto-propagate headers: X-Request-ID, X-Correlation-ID, traceparent, Accept, Accept-Language
- Generate UUID for X-Request-ID if client doesn't provide one
- Logs show step start/complete events at normal level, full DAG plan at debug level
- Response can include optional timing header for developers

### Response Merging
- Template object approach — YAML structure mirrors response structure with Expr placeholders
- Status code configurable via `status:` field (static value or Expr like `steps.create.status`)
- Content-Type configurable via `content_type:` field, defaults to application/json
- Response headers NOT configurable (gateway sets standard headers only)

### Claude's Discretion
- Exact YAML schema field names and nesting
- Error message formatting for invalid expressions
- How to handle non-JSON upstream responses
- Logging format details

</decisions>

<specifics>
## Specific Ideas

- "Similar to GraphQL federation partial responses" — but that's Phase 4
- Named upstreams pattern inspired by reverse proxy configs (nginx upstream blocks)
- Template object approach chosen for clarity: "what you see is what you get"

</specifics>

<deferred>
## Deferred Ideas

- Optional steps and partial responses — Phase 4 (confirmed during discussion)
- Per-step timeout configuration — Phase 4
- Custom response headers — out of scope

</deferred>

---

*Phase: 02-composition-engine*
*Context gathered: 2026-02-03*
