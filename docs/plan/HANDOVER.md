# Handover

Updated: 2026-10-23, session 3

## Where we are

C01-C13 are done. Studio has environments, typed draft resources with 409 on stale saves, publish with a diff, numbered append-only versions, rollback, promote, and per-environment gateway tokens. A gateway in managed mode (RESTITCH_STUDIO_URL + RESTITCH_GATEWAY_TOKEN) pulls its environment's current version, stays /ready 503 until the first one, and survives a Studio outage. The gateway has no admin API; /metrics is on the optional ops port. Each managed gateway sends a heartbeat every 15s, and Studio lists instances per environment with the version each applied and a stale flag. Managed gateways also push one-minute usage aggregates and sampled requests (redacted, no bodies); Studio sums instances and applies per-environment retention and sample caps.

## Prove it

Run: `docker run -d --rm --name restitch-pg-test -e POSTGRES_PASSWORD=test -e POSTGRES_DB=restitch_test -p 55432:5432 postgres:17-alpine`, then `RESTITCH_TEST_DATABASE_URL="postgres://postgres:test@localhost:55432/restitch_test?sslmode=disable" make ci && make e2e && (cd studio && npm run test)`
Expect: golangci-lint "0 issues.", every Go package ok (internal/studio runs its postgres subtests), `make e2e` ok, Studio: 9 files, 28 tests passed. Seen at commit 28c29f3.

## Active chunk

None. Unblocked next: C14 (CLI publish and import/export), C15 (dev mode), C16 (UI shell), C17 (schema).

## Next action

Start C14: write "TestExportThenImportRoundTrips" in `internal/studio/importexport_test.go`. Import replaces the draft through `artifactResources` and `replaceDraft` in `internal/studio/publish.go`; `restitch publish` calls import then publish with an auth-all API key (Authorization: Bearer). Check what `cmd/restitch/importcmd.go` does first.

## Decisions

- Every design decision lives in `docs/specs/studio-control-plane.md`. Do not reopen them; the user settled each one in a grill on 2026-10-23.
- The old milestone process in the project `CLAUDE.md` (root `PLAN.md`, ledger, `scripts/verify.sh` gates) does not apply to this plan. The gate is the test command above. The user confirmed this on 2026-10-23. Do not edit `scripts/gates/`; those scripts still describe v1 and go away with the old process.
- auth-all is the user's own library. Open an issue on github.com/alternayte/auth-all for any gap. Do not work around it in Restitch.
- auth-all has no option to disable public sign-up: issue https://github.com/alternayte/auth-all/issues/17. Until it ships, `internal/studio/server.go` routes POST /api/auth/sign-up/email to 404 and drops the operation from the OpenAPI document. Replace both with the new option when it lands.
- auth-all migrates its own tables (`Auth.Migrate`); Studio's tables are goose migrations. The two sets never share a version table.
- `api/openapi.yaml` is generated and committed; `TestOpenAPIUpToDate` fails on drift. Refresh with `go test ./internal/studio -run TestOpenAPIUpToDate -update`.
- Bare `restitch` prints usage and exits 2, so every container names its role.
- Studio validates unexpanded YAML by expanding every ${VAR} to "x" (`gwconfig.ExpandWith` in `validateArtifact`). A ${VAR} belongs in a string field; a URL writes its scheme out (https://${HOST}). Defaults in ${VAR:default} are ignored in Studio validation.
- Gateway routes live under /gateway/v1 outside /api/v1, with a bearer gateway token (rgt_...). A gateway token on a user route gets 401, not 403: auth-all rejects it before any role check.
- huma reads path parameters only from fields declared on the input struct itself, never from an embedded struct.
- Fixtures and examples that relied on the old open default now say public: true. A test that needs auth declares a top-level auth block; testenv builds the authenticator from it.

## Open questions

- The form library for C18 is open. Assumption: try `@rjsf/core` with a shadcn theme first; drop it for a small hand-written renderer over the C17 schema if it cannot render the custom widgets. Record the choice here.
- `cmd/mockupstream` is in the image today. Assumption: tests need it, the image does not (C26).
- README, docs, `deploy/` and `examples/registry` still show v1 server:/admin: blocks and the removed -registry-url/-registry-key flags. C26 and C27 rewrite them; do not patch them piecemeal.
- The old `cmd/restitch-studio` binary, `internal/registry` and `internal/devmode` still build and pass their tests, but their dashboards fail against a v2 gateway. C15, C24 and C26 delete them.

## Environment

- Branch: `v2-control-plane`, cut from `fix/review-5-6-7`; neither is merged to `main` or pushed.
- Setup: `make studio`
- Postgres for tests: the docker command under Prove it. CI runs a postgres:17-alpine service in the go job.
- Reference app for UI patterns: `../sluice/ui/src`. Auth library: `../auth-all` (read `docs/guides/huma.md`, `roles.md`, `bootstrap.md`).
