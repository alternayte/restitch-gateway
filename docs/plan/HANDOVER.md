# Handover

Updated: 2026-10-24, session 5 (closed; every chunk done)

## Where we are

Every chunk C01-C27 is done. C01-C24, C26 and C27 are merged to main (PRs #34-#37); C25 is PR #38, merged when its CI passed. The v2 control plane described in `docs/specs/studio-control-plane.md` is built: Studio with environments, drafts, publish, rollback, promote, gateway tokens, users, dashboards, the step graph, the secret picker and the command palette; managed and standalone gateways; one image; Compose and Kubernetes files; docs for v2.

## Prove it

Run: `docker run -d --rm --name restitch-pg-test -e POSTGRES_PASSWORD=test -e POSTGRES_DB=restitch_test -p 55432:5432 postgres:17-alpine`, then `RESTITCH_TEST_DATABASE_URL="postgres://postgres:test@localhost:55432/restitch_test?sslmode=disable" make ci && make e2e && (cd studio && npm run test) && make deploy-lint`
Expect: golangci-lint "0 issues.", every Go package ok (internal/studio runs its postgres subtests), `make e2e` ok, Studio: 5 files, 11 tests passed, `make deploy-lint` 4 resources valid. Seen at the C26 commit on 2026-10-24. The Postgres container is started with --rm, so after `docker stop` run the `docker run` command again; a refused connection in the postgres subtests means the container is missing, not a code fault.

## Active chunk

None. The plan has no open chunk.

## Next action

Ask the user what comes next. Open items, none of them in the plan:
- The spec's manual check: a gateway with two env vars and Studio with three on Postgres serve traffic on one container platform from docs/deployment.md. Only the user can run it.
- Release v2.0.0: no tag is cut. `.github/workflows/release.yml` publishes the image on a v* tag.
- The step graph does not draw dependencies from {{ steps.X }} references; that needs an API that returns the dependencies the gateway computes.
- Metrics keep the v1 names `restitch_registry_*`; renaming breaks dashboards.
- The secret picker shows on every free string field, step names and methods included.
- Jaeger was dropped from deploy/; the gateway still exports OTLP traces when OTEL_EXPORTER_OTLP_ENDPOINT is set.

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
