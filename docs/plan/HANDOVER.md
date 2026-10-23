# Handover

Updated: 2026-10-23, session 1

## Where we are

The spec is cut into 27 chunks in `docs/plan/PLAN.md`. No chunk has started. The code is v1.2.0 plus three review fixes (schema bypass, cache key, shutdown order) on branch `fix/review-5-6-7`.

## Prove it

Run: `make ci && make e2e && (cd studio && npm run test)`
Expect: `make ci` fails in lint with 6 staticcheck SA5011 issues in `internal/registry/store_test.go` and `internal/registry/validator_test.go` (local golangci-lint 2.12.2). `go test -race ./...` passes when run alone. `make e2e` passes. Studio: 9 files, 28 tests passed.

## Active chunk

None. Start with C01, then C05: C05 carries the highest risk (auth-all through humaauth on huma v2, SQLite and Postgres).

## Next action

Create branch `v2-control-plane` from `fix/review-5-6-7`, then write "TestGatewayFlagOverEnvOverDefault" in `cmd/restitch/run_test.go` for C01.

## Decisions

- Every design decision lives in `docs/specs/studio-control-plane.md`. Do not reopen them; the user settled each one in a grill on 2026-10-23.
- Chunks cut by behaviour; the gateway runtime comes first, so Studio builds against the v2 artifact format.
- The old milestone process in the project `CLAUDE.md` (root `PLAN.md`, ledger, `scripts/verify.sh` gates) does not apply to this plan. The gate is the test command above. The user confirmed this on 2026-10-23.
- auth-all is the user's own library. Open an issue on github.com/alternayte/auth-all for any gap. Do not work around it in Restitch.

## Open questions

- The 6 staticcheck SA5011 issues come from a newer local linter than CI. Assumption: leave them; C09 and C10 delete `internal/registry`. Fix them earlier only if CI fails on them.
- The form library for C18 is open. Assumption: try `@rjsf/core` with a shadcn theme first; drop it for a small hand-written renderer over the C17 schema if it cannot render the custom widgets. Record the choice here.
- `cmd/mockupstream` is in the image today. Assumption: tests need it, the image does not (C26).

## Environment

- Branch: `fix/review-5-6-7` (not yet merged to `main`)
- Setup: `make studio`
- Reference app for UI patterns: `../sluice/ui/src`. Auth library: `../auth-all`.
