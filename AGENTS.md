# Restitch

## What this is
Restitch is a REST API composition gateway: each composition maps one inbound route to a dependency graph of upstream calls and renders one response.
Restitch Studio is the control plane: people edit drafts per environment and publish artifact versions, which managed gateways pull.

## Run
- `make build` builds `bin/restitch`, stamped with the git version.
- `make run` runs `restitch gateway` in standalone mode against `restitch.yaml`.
- `make studio` builds the frontend into `internal/studio/dist`, which `bin/restitch` embeds.
- `make build-all` builds the frontend when `internal/studio/dist` is missing, then `bin/restitch`.
- `bin/restitch dev` runs Studio on 127.0.0.1:3080 and a managed gateway on :8080 in one process on SQLite.
- `make docker` builds the one image; it has no default command, so each container names its role (`gateway`, `studio`).
- `cd studio && npm run dev` runs the frontend alone with Vite.

## Test
- `make ci` is the gate: vet, golangci-lint and the race test suite.
- `make e2e` runs the golden-spec suite in `tests/`, which compiles only with the e2e build tag.
- CI also fails on any `gofmt -l .` output and on coverage below the threshold in `.github/workflows/ci.yml`.
- `cd studio && npm run test` runs the frontend tests with vitest.
- `go build ./...` fails on a fresh clone until `make studio` has produced the embedded dist directory.
- Studio tests run against Postgres only when `RESTITCH_TEST_DATABASE_URL` is set; CI sets it, so a local run without it skips the Postgres subtests.
- `api/openapi.yaml` is generated: refresh it with `go test ./internal/studio -run TestOpenAPIUpToDate -update`, then `cd studio && npm run gen:api` for the client.

## Stack rules
- The gate is `make ci` plus `make e2e`, and `make deploy-lint` after changes under `deploy/`. The repo has no justfile.
- The Studio frontend runs on Node and npm, not Bun.
- Studio storage is Postgres when deployed and SQLite on local disk only. Schema changes are goose migrations in `internal/studio/migrations`; auth-all migrates its own tables.

## Domain words
- composition: one inbound route, method plus path, with the steps and the response template that serve it.
- step: one upstream call inside a composition; steps form a graph through depends_on and template references.
- upstream: a named backend with a base URL and its auth, transport, retry and breaker settings.
- partial response: a response that carries the successful steps plus error details for failed optional steps.
- environment: a named deployment target in Studio that owns one draft and its artifact versions. Avoid: variant, target, stage, namespace.
- draft: the editable set of resources for one environment. Avoid: workspace, working copy.
- resource: one typed config item in a draft: an upstream, a composition, the auth policy, the rate-limit policy or the settings. Avoid: entity, config record, fragment.
- publish: validate a draft and write a new artifact version. Avoid: deploy, activate, apply.
- artifact version: an immutable, numbered, validated snapshot of an environment's config that gateways pull. Avoid: bundle, release, config version, snapshot.
- promote: copy an artifact version into another environment's draft. Avoid: deploy, copy.
- gateway token: a credential for one environment that pulls artifacts and pushes telemetry. Avoid: registry key, admin key, CDN key.
- standalone mode: a gateway that runs from one file set by RESTITCH_CONFIG, with no Studio. Avoid: file mode, local mode.
- managed mode: a gateway that pulls its environment's current artifact version from Studio with RESTITCH_STUDIO_URL and RESTITCH_GATEWAY_TOKEN. Avoid: registry mode.
