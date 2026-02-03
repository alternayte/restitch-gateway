---
phase: 03-upstream-authentication
plan: 01
subsystem: auth
tags: [env-vars, strategy-pattern, round-tripper, yaml-config]

# Dependency graph
requires:
  - phase: 02-composition-engine
    provides: "Config parsing, Upstream struct, YAML schema"
provides:
  - "Environment variable expansion with fail-fast validation"
  - "Auth Strategy interface using RoundTripper pattern"
  - "Config types for header, basic, passthrough, oauth2 strategies"
  - "Per-upstream auth configuration in composition config"
affects: [03-02-header-auth, 03-03-basic-auth, 03-04-passthrough, 03-05-oauth2]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Strategy interface with RoundTripper for auth injection"
    - "Environment variable validation before expansion (fail-fast)"
    - "Mutual exclusivity validation for auth config"

key-files:
  created:
    - internal/config/env.go
    - internal/config/env_test.go
    - internal/auth/auth.go
  modified:
    - internal/composition/config.go

key-decisions:
  - "ExpandEnvWithValidation validates ALL ${VAR} references before any expansion"
  - "Empty env var treated as error (not just missing)"
  - "NoneStrategy as default passthrough (base RoundTripper unchanged)"
  - "Config.Validate() enforces exactly one strategy per upstream"

patterns-established:
  - "Strategy interface: RoundTripper(base) http.RoundTripper"
  - "Env var syntax: ${VAR_NAME} with alphanumeric and underscore"
  - "Fail-fast validation: startup errors not runtime errors"

# Metrics
duration: 2min
completed: 2026-02-03
---

# Phase 03 Plan 01: Authentication Foundation Summary

**Environment variable expansion with fail-fast validation, Strategy interface using RoundTripper pattern, and per-upstream auth config schema in YAML**

## Performance

- **Duration:** 2 min
- **Started:** 2026-02-03T08:54:11Z
- **Completed:** 2026-02-03T08:56:03Z
- **Tasks:** 3
- **Files modified:** 4

## Accomplishments
- ExpandEnvWithValidation function validates ${VAR} references exist and are non-empty before expansion
- Strategy interface defined with RoundTripper pattern for auth injection
- Config types for all four auth strategies (header, basic, passthrough, oauth2)
- Upstream struct extended with optional Auth field for per-upstream configuration

## Task Commits

Each task was committed atomically:

1. **Task 1: Create environment variable expansion with validation** - `d941770` (feat)
2. **Task 2: Create auth strategy interface and config types** - `45a3189` (feat)
3. **Task 3: Add auth configuration to upstream config schema** - `07cea95` (feat)

## Files Created/Modified
- `internal/config/env.go` - ExpandEnvWithValidation function for ${VAR} expansion with validation
- `internal/config/env_test.go` - Comprehensive tests for env var expansion edge cases
- `internal/auth/auth.go` - Strategy interface, Config struct, all strategy config types, NoneStrategy
- `internal/composition/config.go` - Added Auth field to Upstream struct

## Decisions Made
- **Validate before expand:** ExpandEnvWithValidation validates all ${VAR} references exist AND are non-empty before calling os.ExpandEnv (per RESEARCH.md Pitfall 1)
- **Empty vars are errors:** Empty environment variables treated as errors, not just missing ones (security)
- **Mutual exclusivity:** Config.Validate() returns error if multiple auth strategies defined per upstream
- **NoneStrategy as default:** Base RoundTripper passthrough when no auth configured
- **Strategy pattern:** Each auth type returns http.RoundTripper that wraps base transport

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None - all tasks completed without issues.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Strategy interface ready for concrete implementations (header, basic, passthrough, oauth2)
- Env var expansion ready to use in auth config parsing
- Upstream.Auth field ready to parse auth blocks from YAML
- Next plans can implement individual strategies using the patterns established here

---
*Phase: 03-upstream-authentication*
*Completed: 2026-02-03*
