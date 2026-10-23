# Handover

Updated: 2026-10-23, session 1 (closed)

## Where we are

C01 is done: the module is `/v2`, `restitch gateway` replaced `restitch run`, and every gateway flag reads its RESTITCH_ env twin. The rest of the code is v1.2.0 plus three review fixes (schema bypass, cache key, shutdown order).

## Prove it

Run: `make ci && make e2e && (cd studio && npm run test)`
Expect: `make ci` exits 0 with "0 issues." from golangci-lint and every Go package ok; `make e2e` ok; Studio: 9 files, 28 tests passed. Seen after commit 7778ebf.

## Active chunk

None. C01 is done. Unblocked next: C02, C04, C05.

## Next action

Start C05 (highest risk): write "TestAnonymousAPICallReturns401" in `internal/studio/server_test.go`, with auth-all mounted through `github.com/alternayte/auth-all/humaauth` on huma v2. Read `../auth-all/docs/guides/huma.md` and `../auth-all/docs/guides/bootstrap.md` first.

## Decisions

- Every design decision lives in `docs/specs/studio-control-plane.md`. Do not reopen them; the user settled each one in a grill on 2026-10-23.
- Chunks cut by behaviour; the gateway runtime comes first, so Studio builds against the v2 artifact format.
- The old milestone process in the project `CLAUDE.md` (root `PLAN.md`, ledger, `scripts/verify.sh` gates) does not apply to this plan. The gate is the test command above. The user confirmed this on 2026-10-23.
- auth-all is the user's own library. Open an issue on github.com/alternayte/auth-all for any gap. Do not work around it in Restitch.

## Open questions

- The form library for C18 is open. Assumption: try `@rjsf/core` with a shadcn theme first; drop it for a small hand-written renderer over the C17 schema if it cannot render the custom widgets. Record the choice here.
- `cmd/mockupstream` is in the image today. Assumption: tests need it, the image does not (C26).

## Environment

- Branch: `v2-control-plane`, cut from `fix/review-5-6-7`; neither is merged to `main`
- Setup: `make studio`
- Reference app for UI patterns: `../sluice/ui/src`. Auth library: `../auth-all`.
