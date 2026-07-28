# Restitch Plan Remediation Tracker

Audit findings from `scripts/verify.sh all` (2026-07-13) that need fixing.
Each R<n> entry links to the failing task, evidence, and fix plan.
A later PASS ledger row for the original task ID closes the item.

| ID | Task | Symptom | Evidence | Fix Plan | Status |
|----|------|---------|----------|----------|--------|
| R1 | M1.gate | `make ci` fails: golangci-lint built with Go 1.24, project targets 1.25.6 | evidence/2026-07-13-M1-bba6fae7.log | Update golangci-lint: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` | **FIXED** — env updated + lint errors fixed in be7a6e44 |
| R2 | T9.1, M9.gate | Inbound auth not wired in `cmd/restitch/run.go` — `NewHandler(compiled, nil)` always passes nil authenticator. Unit tests pass (test the handler directly) but the binary never applies auth. Live smoke: request without API key returns 200 instead of 401 | evidence/2026-07-13-M9-bba6fae7.log | Wire `inbound.New()` in run.go when `gwcfg.Server.Auth` is configured; pass authenticator to `NewHandler` and `BuildPipeline`. Also add an E2E golden spec testing auth | **FIXED** — 6f91f433 |
| R3 | M10.gate | SIGHUP log grep timing: `h_assert_log` uses `grep -E` (ERE) but pattern used `\|` (BRE alternation). On macOS BSD grep, `\|` in ERE is a literal backslash+pipe, not alternation | evidence/2026-07-13-M10-bba6fae7.log | Gate script fixed in 8afdda7d (changed `\|` to `|` in ERE patterns) | **FIXED** — 8afdda7d |
| R4 | M12.gate | Studio binary uses `-gateway-admin-url` flag, gate script passed `-admin` | evidence/2026-07-13-M12-bba6fae7.log | Gate script fixed in 01fc4a2e. Re-run confirmed PASS | **FIXED** — re-run at 8afdda7d |
| R5 | M14.gate | CI reports failure on this branch (hasn't been pushed with lint fixes) | evidence/2026-07-13-M14-bba6fae7.log | Push branch to trigger CI with fixed lint. Depends on R1 | OPEN — push needed |
| R6 | T18.5, M18.gate | All T18.x task checks pass. Live OTLP sink test fails: Python HTTP server doesn't bind when backgrounded in this shell env | evidence/2026-07-13-M18-8afdda7d.log | ENV issue — Python background HTTP server doesn't start in sandbox. Gate script itself is correct (01fc4a2e). Needs manual re-run in a terminal | ENV — not a codebase bug |
