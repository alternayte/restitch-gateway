# Handover

Updated: 2026-10-23, session 2 (closed)

## Where we are

C01, C02, C03 and C05 are done. The gateway runs as `restitch gateway` under the `/v2` module, reads process settings only from flags and RESTITCH_ env vars, refuses v1 server:/admin: blocks, and refuses any non-public composition when the artifact has no top-level auth block. `restitch studio` runs huma v2 with auth-all sign-in (roles viewer, editor, owner; bootstrap owner; store-backed rate limits) on SQLite or Postgres. The old `cmd/restitch-studio` binary and the registry still exist and still work.

## Prove it

Run: `docker run -d --rm --name restitch-pg-test -e POSTGRES_PASSWORD=test -e POSTGRES_DB=restitch_test -p 55432:5432 postgres:17-alpine`, then `RESTITCH_TEST_DATABASE_URL="postgres://postgres:test@localhost:55432/restitch_test?sslmode=disable" make ci && make e2e && (cd studio && npm run test)`
Expect: golangci-lint "0 issues.", every Go package ok (internal/studio runs its postgres subtests), `make e2e` ok, Studio: 9 files, 28 tests passed. Seen at commit cee1e5b.

## Active chunk

None. Unblocked next: C04 (depends C01), C06 (depends C05), C17 (depends C03, C05).

## Next action

Start C06: write "TestStaleSaveReturns409" in `internal/studio/draft_test.go`. Follow the patterns in `internal/studio/me.go` (huma.Register with humaauth.RequireRole, `s.db.rebind` for placeholders) and add the tables as a goose migration in `internal/studio/migrations/`.

## Decisions

- Every design decision lives in `docs/specs/studio-control-plane.md`. Do not reopen them; the user settled each one in a grill on 2026-10-23.
- The old milestone process in the project `CLAUDE.md` (root `PLAN.md`, ledger, `scripts/verify.sh` gates) does not apply to this plan. The gate is the test command above. The user confirmed this on 2026-10-23. Do not edit `scripts/gates/`; those scripts still describe v1 and go away with the old process.
- auth-all is the user's own library. Open an issue on github.com/alternayte/auth-all for any gap. Do not work around it in Restitch.
- auth-all has no option to disable public sign-up: issue https://github.com/alternayte/auth-all/issues/17. Until it ships, `internal/studio/server.go` routes POST /api/auth/sign-up/email to 404 and drops the operation from the OpenAPI document. Replace both with the new option when it lands.
- auth-all migrates its own tables (`Auth.Migrate`); Studio's tables are goose migrations. The two sets never share a version table.
- `api/openapi.yaml` is generated and committed; `TestOpenAPIUpToDate` fails on drift. Refresh with `go test ./internal/studio -run TestOpenAPIUpToDate -update`.
- Bare `restitch` prints usage and exits 2, so every container names its role.
- Fixtures and examples that relied on the old open default now say public: true. A test that needs auth declares a top-level auth block; testenv builds the authenticator from it.

## Open questions

- The form library for C18 is open. Assumption: try `@rjsf/core` with a shadcn theme first; drop it for a small hand-written renderer over the C17 schema if it cannot render the custom widgets. Record the choice here.
- `cmd/mockupstream` is in the image today. Assumption: tests need it, the image does not (C26).
- Studio-side validation (C07) runs on unexpanded YAML, but `composition.Validate` expects env-expanded YAML. Assumption: C07 adds a check that validates ${VAR} name syntax and substitutes typed placeholders before Validate. Decide the placeholder rule in C07 and record it here.
- README, docs and `deploy/` still show v1 server:/admin: blocks. C27 and C26 rewrite them; do not patch them piecemeal.

## Environment

- Branch: `v2-control-plane`, cut from `fix/review-5-6-7`; neither is merged to `main` or pushed.
- Setup: `make studio`
- Postgres for tests: the docker command under Prove it. CI runs a postgres:17-alpine service in the go job.
- Reference app for UI patterns: `../sluice/ui/src`. Auth library: `../auth-all` (read `docs/guides/huma.md`, `roles.md`, `bootstrap.md`).
