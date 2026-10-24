# Handover

Updated: 2026-10-24, session 5 (closed; v2.0.0 released)

## Where we are

Every chunk C01-C31 is done and merged to main (PRs #34-#42). v2.0.0 is released from main: the release workflow publishes ghcr.io/alternayte/restitch-gateway:v2.0.0 and :latest and attaches four binaries. The README shows Studio with a flow GIF and screenshots.

## Prove it

Run: `docker run -d --rm --name restitch-pg-test -e POSTGRES_PASSWORD=test -e POSTGRES_DB=restitch_test -p 55432:5432 postgres:17-alpine`, then `RESTITCH_TEST_DATABASE_URL="postgres://postgres:test@localhost:55432/restitch_test?sslmode=disable" make ci && make e2e && (cd studio && npm run test) && make deploy-lint`
Expect: golangci-lint "0 issues.", every Go package ok (internal/studio runs its postgres subtests), `make e2e` ok, Studio: 5 files, 11 tests passed, `make deploy-lint` 4 resources valid. Seen at the C26 commit on 2026-10-24. The Postgres container is started with --rm, so after `docker stop` run the `docker run` command again; a refused connection in the postgres subtests means the container is missing, not a code fault.

## Active chunk

None. The plan has no open chunk.

## Next action

Ask the user what comes next. Open items, none of them in the plan:
- The spec's manual check: a gateway with two env vars and Studio with three on Postgres serve traffic on one container platform from docs/deployment.md. Only the user can run it.
- Studio's upstream table counts a step failure as an upstream error, so upstream 5xx responses (passed through as step data by design) show 0% errors there, while `restitch_upstream_requests_total{status_class="5xx"}` counts them.
- Jaeger is not in deploy/; the gateway still exports OTLP traces when OTEL_EXPORTER_OTLP_ENDPOINT is set.

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

- sluice's UI is AGPL-3.0-only and Restitch is Apache-2.0. The ported files are the user's own code; the user decides whether that needs a note.

## Environment

- Branch: work branches come off `main`; `v2-control-plane` merged as PR #34. C27 is on `c27-docs`.
- Setup: `make studio`
- Postgres for tests: the docker command under Prove it. CI runs a postgres:17-alpine service in the go job.
- Docker in this session could not read the macOS keychain (`docker compose build` failed on credentials). Workaround: DOCKER_CONFIG pointing at a copy of ~/.docker/config.json without credsStore, plus symlinks to cli-plugins, contexts and buildx, after `docker pull` of the base images. Docker Desktop's disk filled once; the user allowed a builder and container prune.
- Reference app for UI patterns: `../sluice/ui/src`. Auth library: `../auth-all` (read `docs/guides/huma.md`, `roles.md`, `bootstrap.md`).
