# Restitch

## What this is
Restitch is a REST API composition gateway: each composition maps one inbound route to a dependency graph of upstream calls and renders one response.
Restitch Studio is the web UI and config registry that gateways poll in registry mode.

## Run
- `make build` builds `bin/restitch`, stamped with the git version.
- `make run` runs the gateway against `restitch.yaml`.
- `make studio` builds the frontend into `cmd/restitch-studio/dist`, which the Studio binary embeds.
- `make build-all` builds both binaries, and builds the frontend first when its assets are missing.
- `bin/restitch dev` runs the gateway and Studio as child processes; it finds `bin/restitch-studio` next to itself.
- `make docker` builds the one image; its entrypoint is the gateway, so Studio needs an entrypoint override.
- `cd studio && npm run dev` runs the frontend alone with Vite.

## Test
- `make ci` is the gate: vet, golangci-lint and the race test suite.
- `make e2e` runs the golden-spec suite in `tests/`, which compiles only with the e2e build tag.
- CI also fails on any `gofmt -l .` output and on coverage below the threshold in `.github/workflows/ci.yml`.
- `cd studio && npm run test` runs the frontend tests with vitest.
- `go build ./...` fails on a fresh clone until `make studio` has produced the embedded dist directory.

## Stack rules
- The gate is `make ci` plus `make e2e`. The repo has no justfile.
- The Studio frontend runs on Node and npm, not Bun.
- Storage is SQLite. Schema changes are goose migrations in `internal/registry/migrations`.

## Domain words
- composition: one inbound route, method plus path, with the steps and the response template that serve it.
- step: one upstream call inside a composition; steps form a graph through depends_on and template references.
- upstream: a named backend with a base URL and its auth, transport, retry and breaker settings.
- partial response: a response that carries the successful steps plus error details for failed optional steps.
- registry: the versioned store of config records inside Studio that gateways poll in registry mode.
- bundle: the merged YAML of the active version of every registry config, served to polling gateways.
- environment: a named deployment target in Studio that owns one draft and its artifact versions. Avoid: variant, target, stage, namespace.
- draft: the editable set of resources for one environment. Avoid: workspace, working copy.
- resource: one typed config item in a draft: an upstream, a composition, the auth policy or a rate-limit policy. Avoid: entity, config record, fragment.
- publish: validate a draft and write a new artifact version. Avoid: deploy, activate, apply.
- artifact version: an immutable, numbered, validated snapshot of an environment's config that gateways pull. Avoid: bundle, release, config version, snapshot.
- promote: copy an artifact version into another environment's draft. Avoid: deploy, copy.
- gateway token: a credential for one environment that pulls artifacts and pushes telemetry. Avoid: registry key, admin key, CDN key.
- standalone mode: a gateway that runs from one file set by RESTITCH_CONFIG, with no Studio. Avoid: file mode, local mode.
