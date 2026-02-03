# Phase 3: Upstream Authentication - Context

**Gathered:** 2026-02-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Configure authentication strategies for upstream services without writing code. Users define auth in YAML configuration, and the gateway handles credential injection, token management, and header forwarding transparently. Supports header (static API key), basic auth, passthrough (forward client auth), and OAuth2 client credentials.

</domain>

<decisions>
## Implementation Decisions

### Credential storage
- Environment variables only — no file paths, no vault integration
- Config syntax uses `${VAR_NAME}` (shell-style, matches Docker/K8s conventions)
- Secrets resolved at startup only — env vars read once, changes require restart
- Missing env var referenced in config causes gateway startup failure (fail-fast)

### Passthrough behavior
- Forward only the standard `Authorization` header (not configurable header names)
- Forward header verbatim — "Bearer xyz" goes to upstream as "Bearer xyz" (no stripping/re-adding)
- Auth strategies are mutually exclusive per upstream — either passthrough OR static/oauth, not both

### OAuth2 token lifecycle
- Honor token's `expires_in` value for cache duration
- Refresh 30 seconds before expiry (fixed buffer, not percentage)
- Single-flight refresh — first request triggers refresh, concurrent requests wait for same result

### Auth failure responses
- Upstream 401/403 responses pass through verbatim to client
- Gateway auth failures (token fetch fails, network error) return 502 Bad Gateway
- No auth strategy details exposed in error responses (hide internals)
- Full details logged for debugging (token endpoint, error response, but never secrets)

### Claude's Discretion
- Whether to use lazy or background token refresh (tradeoff: simplicity vs latency)
- Passthrough behavior when client sends no Authorization header (security best practice)
- Internal cache implementation (map with mutex, sync.Map, etc.)

</decisions>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 03-upstream-authentication*
*Context gathered: 2026-02-03*
