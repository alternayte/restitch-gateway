# Restitch Plan Remediation Tracker

Audit findings from `scripts/verify.sh all` (2026-07-13) that need fixing.
Each R<n> entry links to the failing task, evidence, and fix plan.
A later PASS ledger row for the original task ID closes the item.

| ID | Task | Symptom | Evidence | Fix Plan | Status |
|----|------|---------|----------|----------|--------|
| R1 | M1.gate | `make ci` fails: golangci-lint built with Go 1.24, project targets 1.25.6 | evidence/2026-07-13-M1-bba6fae7.log | Update golangci-lint: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` | ENV — not a codebase bug |
| R2 | T9.1, M9.gate | Inbound auth not wired in `cmd/restitch/run.go` — `NewHandler(compiled, nil)` always passes nil authenticator. Unit tests pass (test the handler directly) but the binary never applies auth. Live smoke: request without API key returns 200 instead of 401 | evidence/2026-07-13-M9-bba6fae7.log | Wire `inbound.New()` in run.go when `gwcfg.Server.Auth` is configured; pass authenticator to `NewHandler` and `BuildPipeline`. Also add an E2E golden spec testing auth | **FIXED** — 6f91f433 |
| R3 | M10.gate | SIGHUP log grep timing: `h_assert_log` ran before log flushed to disk. Evidence shows "config reloaded" IS in the log — grep just missed it | evidence/2026-07-13-M10-bba6fae7.log | Gate script fixed in 01fc4a2e (increased sleep, broadened pattern). Re-run to confirm | GATE FIX — re-run needed |
| R4 | M12.gate | Studio binary uses `-gateway-admin-url` flag, gate script passed `-admin` | evidence/2026-07-13-M12-bba6fae7.log | Gate script fixed in 01fc4a2e. Re-run to confirm | GATE FIX — re-run needed |
| R5 | M14.gate | CI reports failure on this branch (likely hasn't been pushed with latest changes or golangci-lint issue propagates) | evidence/2026-07-13-M14-bba6fae7.log | Push branch + fix CI. Depends on R1 | ENV — push + fix lint |
| R6 | T18.5, M18.gate | Gate grepped `internal/admin/` for trace_id but it lives in `internal/reqlog/`. Also OTLP env vars not exported to child process | evidence/2026-07-13-M18-bba6fae7.log | Gate script fixed in 01fc4a2e. Re-run to confirm | GATE FIX — re-run needed |
