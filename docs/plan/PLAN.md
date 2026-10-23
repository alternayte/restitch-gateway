# Plan: Studio control plane (v2)

Source: `docs/specs/studio-control-plane.md` (all sections)
Test command: `make ci && make e2e && (cd studio && npm run test)`
Setup command: `make studio`; Postgres tests also need `RESTITCH_TEST_DATABASE_URL` (added in C05)

## Chunks

### Gateway runtime
- [x] C01 — `restitch gateway` runs standalone from RESTITCH_CONFIG under the /v2 module
- [x] C02 — A v1 file with server: or admin: is refused with the replacement env var named
- [x] C03 — An artifact with a non-public composition and no auth policy is refused
- [x] C04 — The gateway has no admin API; probes on the data port, metrics on the ops port

### Studio core
- [x] C05 — `restitch studio` signs in a bootstrap owner and refuses anonymous API calls
- [x] C06 — An editor saves resources in an environment draft, and a stale save gets 409
- [x] C07 — Publish validates the draft, returns a diff and writes an artifact version
- [x] C08 — Rollback and promote write new versions and drafts without changing history
- [x] C09 — A gateway token pulls its environment's artifact version with an ETag
- [x] C10 — `restitch gateway` in managed mode pulls, applies and survives a Studio outage
- [x] C11 — Studio lists each gateway instance with its applied artifact version
- [x] C12 — Gateways push one-minute aggregates, and Studio keeps them for the retention period
- [x] C13 — Gateways push sampled requests with redacted headers and no bodies
- [x] C14 — `restitch publish` and YAML import and export use the publish path
- [x] C15 — `restitch dev` runs both roles in one process on SQLite

### Studio UI
- [x] C16 — The new UI shell signs in and renders environments through the generated client
- [x] C17 — Studio serves a JSON Schema of the artifact that covers every config field
- [x] C18 — A resource form generated from the schema saves a resource, with a YAML tab
- [ ] C19 — The step graph widget edits depends_on
- [x] C20 — Expression fields show server lint errors as the user types
- [ ] C21 — The secret picker inserts ${VAR} names and the environment lists required names
- [x] C22 — The publish screen shows the diff, history, rollback and promote
- [x] C23 — Owners manage gateway tokens and users in settings
- [x] C24 — Environment dashboards show aggregates, sampled requests and instances
- [ ] C25 — The command palette and keyboard shortcuts reach every page and action

### Delivery
- [ ] C26 — One image runs every role, with Compose and Kubernetes files linted in CI
- [ ] C27 — The docs describe v2: container platforms, configuration, Studio, CHANGELOG

---

### C01 — `restitch gateway` runs standalone from RESTITCH_CONFIG under the /v2 module

- **Status:** done
- **Depends on:** none
- **Files:** `go.mod`, `cmd/restitch/main.go`, `cmd/restitch/run.go`, `internal/gwconfig/env.go`, `cmd/restitch/run_test.go`
- **Goal:** `restitch gateway` reads every flag and its RESTITCH_ env twin with flag over env over default, and serves the file named by RESTITCH_CONFIG with reload on change and SIGHUP.
- **First failing test:** `cmd/restitch/run_test.go` — "TestGatewayFlagOverEnvOverDefault"
- **Done when:** the test command passes, `go list -m` prints `github.com/alternayte/restitch-gateway/v2`, and `restitch run` no longer exists.
- **Spec:** Decisions on one binary, the naming rule, standalone mode, v2.0.0.
- **Notes:** The env name is RESTITCH_ plus the flag name in upper snake case; derive it in one function, never by hand. Rewrite every import path in the same commit. RESTITCH_CONFIG and RESTITCH_STUDIO_URL together exit 2 (the second flag lands in C10; reserve the check here).
  Done: `gwconfig.EnvName` and `gwconfig.BindEnv` own the rule; `parseGatewayFlags` in `cmd/restitch/run.go` is testable. `-cert`/`-key` became `-tls-cert`/`-tls-key`. Bare `restitch` prints usage; the Dockerfile CMD is `gateway`. Admin env vars still go through `ApplyEnvOverrides` until C04. The registry-url check exits 2 and moves to the studio-url flag in C10.

---

### C02 — A v1 file with server: or admin: is refused with the replacement env var named

- **Status:** done
- **Depends on:** C01
- **Files:** `internal/gwconfig/config.go`, `internal/gwconfig/config_test.go`, `cmd/restitch/run.go`
- **Goal:** Loading a file with any key under server: or admin: fails with a message such as "server.port moved to RESTITCH_PORT".
- **First failing test:** `internal/gwconfig/config_test.go` — "TestLoadRejectsV1ServerBlock"
- **Done when:** the test command passes and the message names the env var for every v1 key that still has one; keys with no successor say "removed in v2".
- **Spec:** Decision on v2.0.0.
- **Notes:** Process settings come from flags and env only after this chunk. Decode the artifact strictly (unknown keys fail) with yaml KnownFields.
  Done: `gwconfig.checkTopLevelKeys` refuses server:, admin: and unknown top-level keys; auth and rate_limit are top-level artifact keys on `gwconfig.File`. New flags -read-timeout, -write-timeout, -shutdown-timeout feed `server.Config`. Admin settings are RESTITCH_ADMIN_* env only, and admin storage is memory until C04 deletes it. Nested keys are not strict yet: C03 owns that.

---

### C03 — An artifact with a non-public composition and no auth policy is refused

- **Status:** done
- **Depends on:** C02
- **Files:** `internal/composition/config.go`, `internal/composition/parser.go`, `internal/composition/validate.go`, `internal/composition/validate_test.go`, `cmd/restitch/check.go`
- **Goal:** One function validates an artifact: auth policy, rate-limit policies, reserved paths and the existing compile, and both gateway load and `restitch check` call it.
- **First failing test:** `internal/composition/validate_test.go` — "TestValidateRejectsNonPublicWithoutAuthPolicy"
- **Done when:** the test command passes, a composition on /health or /ready is refused, and `handler.go` has no path that serves a non-public route without an authenticator.
- **Spec:** Decisions on the artifact contents, the fail-open route, one validator, reserved probe paths.
- **Notes:** auth and rate_limit are already top-level keys on `gwconfig.File` (C02); move them into the composition config so one type owns the artifact. `restitch check` today runs only `composition.ParseConfig` and prints "Syntax OK" for a v1 file; it must call the shared validator.
  Done: `composition.Config` owns auth (inbound.InboundAuthConfig) and rate_limit (ratelimit.Config); `gwconfig.File` holds only process settings and the hash. `composition.ParseConfig` runs `gwconfig.CheckArtifactKeys`, decodes with KnownFields, and refuses a non-public composition without auth and the /health and /ready paths. `composition.Validate` wraps parse plus compile. The handler answers 503 for a non-public route with no authenticator. Every test fixture and example that relied on the old open default now says public: true. testenv builds its authenticator from the artifact's auth block. ${VAR} checks without expansion are still open: C07 needs them for Studio-side validation. ${VAR} checks name syntax only; expansion happens on the gateway after validation of names.

---

### C04 — The gateway has no admin API; probes on the data port, metrics on the ops port

- **Status:** done
- **Depends on:** C01
- **Files:** `cmd/restitch/run.go`, `cmd/restitch/shutdown.go`, `internal/admin/`, `internal/server/server.go`, `cmd/restitch/run_test.go`
- **Goal:** The gateway opens only the data port, with /health and /ready, plus RESTITCH_OPS_PORT for /metrics when set.
- **First failing test:** `cmd/restitch/run_test.go` — "TestGatewayOpensNoAdminListener"
- **Done when:** the test command passes and `internal/admin` keeps only the accumulator and recorder that C12 and C13 export.
- **Spec:** Decision on removing the admin API and gateway-side storage.
- **Notes:** Delete the admin server, SQL and memory storage, async store and their tests. Keep the ordered shutdown in `shutdown.go`; it loses the store steps.
  Done: `internal/admin` keeps `accumulator.go`, `bucket.go` (Bucket, StepBucket, StepSample) and `recorder.go` (MultiRecorder feeds the accumulator only); run.go wires no recorder until C12. `gwconfig.File` has no Admin block; v1 admin.* keys say "removed in v2: the gateway has no admin API". `-ops-port` (RESTITCH_OPS_PORT, 0 = off) serves only /metrics. `gatewayShutdown` runs servers, then background work, then an optional `final` step with its own deadline: C12 puts the last telemetry push there. The upstream health checker went with the admin API; C12 brings upstream health back as pushed telemetry. The old `cmd/restitch-studio` dashboards now fail against a v2 gateway: C24 replaces them. testenv records request records in its own `RecordLog`.

---

### C05 — `restitch studio` signs in a bootstrap owner and refuses anonymous API calls

- **Status:** done
- **Depends on:** C01
- **Files:** `internal/studio/server.go`, `internal/studio/store.go`, `internal/studio/server_test.go`, `cmd/restitch/studio.go`, `.github/workflows/ci.yml`
- **Goal:** `restitch studio` serves a huma v2 API with auth-all mounted through humaauth; the bootstrap owner signs in, an anonymous call gets 401, a cross-site POST gets 403.
- **First failing test:** `internal/studio/server_test.go` — "TestAnonymousAPICallReturns401"
- **Done when:** the test command passes on SQLite and on Postgres, and `openapi.yaml` is generated and committed with a test that fails on drift.
- **Spec:** Decisions on sign-in, storage, the Studio API.
- **Notes:** This chunk carries the highest risk: do it early. Roles viewer, editor, owner through the auth-all roles plugin. RESTITCH_DATABASE_URL picks the store. Add a Postgres service to CI and read RESTITCH_TEST_DATABASE_URL in tests. Open an auth-all issue for any gap instead of working around it.
  Done: `internal/studio` (server.go, store.go, me.go, migrations/) and `cmd/restitch/studio.go`. The OpenAPI document is `api/openapi.yaml`; refresh it with `go test ./internal/studio -run TestOpenAPIUpToDate -update`. Studio routes are huma operations under /api/v1 behind `auth.RequireAuth`; auth-all lives under /api/auth. auth-all migrates its own tables with `Auth.Migrate`; Studio tables use goose from `internal/studio/migrations`. Sign-in uses the store-backed rate limiter with strict rate limiting. Public sign-up is unrouted and dropped from the document until auth-all#17 ships. `GET/PUT /api/v1/me/preferences` stores UI preferences per user; it replaces the browser-ID sessions when the old Studio goes in C24.

---

### C06 — An editor saves resources in an environment draft, and a stale save gets 409

- **Status:** done
- **Depends on:** C05
- **Files:** `internal/studio/draft.go`, `internal/studio/draft_test.go`, `internal/studio/migrations/`, `internal/studio/server.go`
- **Goal:** Owners create environments; editors create, update and delete typed resources in a draft; each save carries a base revision.
- **First failing test:** `internal/studio/draft_test.go` — "TestStaleSaveReturns409"
- **Done when:** the test command passes, a duplicate name in one environment fails on the database constraint, and a viewer gets 403 on save.
- **Spec:** Decisions on typed resources, the shared draft, base revision.
- **Notes:** A resource row holds environment, kind, name, revision and the body as JSON. Save validates the single resource with the C03 rules that apply to it.
  Done: `internal/studio/draft.go`. Kinds: upstream, composition, auth, rate_limit, settings (block_private_upstreams); the last three are singletons named "default". Bodies are JSON in `studio_resources.body`; save decodes them strictly into the artifact types, and upstream/auth/rate_limit run through `composition.ParseConfig` as one-resource artifacts. base_revision 0 creates (INSERT ON CONFLICT DO NOTHING), any other value updates WHERE revision matches; DELETE takes ?base_revision. huma reads path params only from fields on the input struct, not from an embedded struct.

---

### C07 — Publish validates the draft, returns a diff and writes an artifact version

- **Status:** done
- **Depends on:** C06, C03
- **Files:** `internal/studio/publish.go`, `internal/studio/publish_test.go`, `internal/studio/server.go`
- **Goal:** A publish preview returns the diff against the current version; publish runs the C03 validator on the whole draft and writes an immutable numbered version with author and message.
- **First failing test:** `internal/studio/publish_test.go` — "TestPublishRefusesInvalidDraft"
- **Done when:** the test command passes and two publishes produce versions 1 and 2 with the artifact YAML stored verbatim.
- **Spec:** Decision on publish.
- **Notes:** The stored artifact is the YAML format that standalone mode reads. Author comes from the session, never from the request body.
  Done: `internal/studio/publish.go`. `assembleArtifact` renders the draft deterministically; `validateArtifact` expands every ${VAR} to "x" with `gwconfig.ExpandWith`, then runs `composition.Validate`. Versions live in `studio_artifact_versions` (PK environment, number); the current version is the highest number. Preview returns valid, errors, yaml and a unified diff (go-difflib).

---

### C08 — Rollback and promote write new versions and drafts without changing history

- **Status:** done
- **Depends on:** C07
- **Files:** `internal/studio/publish.go`, `internal/studio/publish_test.go`
- **Goal:** Rollback to version N writes version N+k with N's content; promote copies a version into another environment's draft and returns the diff.
- **First failing test:** `internal/studio/publish_test.go` — "TestRollbackAppendsVersion"
- **Done when:** the test command passes and no update or delete statement touches the versions table.
- **Spec:** Decision on rollback and promote.
- **Notes:** Done: rollback re-validates the old YAML and appends it with message "Roll back to version N"; the draft does not change. Promote parses the version with `artifactResources` (the inverse of `assembleArtifact`, reused by C14 import) and `replaceDraft` swaps the whole draft in one transaction, counting revisions up from the old rows so stale tabs still get 409. `TestNoCodeRewritesArtifactVersions` fails if any non-test file updates or deletes a version row.

---

### C09 — A gateway token pulls its environment's artifact version with an ETag

- **Status:** done
- **Depends on:** C07
- **Files:** `internal/studio/gatewayapi.go`, `internal/studio/tokens.go`, `internal/studio/gatewayapi_test.go`
- **Goal:** Owners create and revoke gateway tokens per environment; a token pulls the current version and each immutable version URL, with ETag and 304.
- **First failing test:** `internal/studio/gatewayapi_test.go` — "TestTokenCannotPullOtherEnvironment"
- **Done when:** the test command passes, a revoked token gets 401, and a gateway token gets 401 on every user API route (auth-all rejects it before any role check; a gateway token is no user credential).
- **Spec:** Decisions on the gateway token and immutable version URLs.
- **Notes:** Store the token hashed; return it once. Many tokens per environment. Body size limit matches the gateway's pull limit.
  Done: `internal/studio/gatewayapi.go`. Tokens are rgt_ plus 32 random bytes, stored as SHA-256 in `studio_gateway_tokens`; owners issue, list and revoke them under /api/v1/environments/{env}/gateway-tokens. Gateways call GET /gateway/v1/artifact (current, Cache-Control no-cache) and GET /gateway/v1/artifact/versions/{n} (immutable) with Authorization: Bearer; the body is the artifact YAML, the ETag is "<env>-<n>", the version is in the Restitch-Artifact-Version header, 404 means no published version. `composition.MaxArtifactBytes` bounds both publish and the gateway read.

---

### C10 — `restitch gateway` in managed mode pulls, applies and survives a Studio outage

- **Status:** done
- **Depends on:** C09, C04
- **Files:** `internal/hotreload/client.go`, `internal/hotreload/poller.go`, `cmd/restitch/run.go`, `internal/server/health.go`, `internal/hotreload/poller_test.go`
- **Goal:** With RESTITCH_STUDIO_URL and RESTITCH_GATEWAY_TOKEN the gateway pulls and applies versions; while Studio is unreachable /health is 200 and /ready is 503 until the first version.
- **First failing test:** `internal/hotreload/poller_test.go` — "TestReadyStays503UntilFirstArtifact"
- **Done when:** the test command passes and an e2e spec in `tests/` starts a gateway before Studio, then serves traffic after a publish.
- **Spec:** Decision on cold start.
- **Notes:** No disk cache. Keep the last good version in memory on a failed pull. The old bundle endpoint and registry key are gone.
  Done: `hotreload.Client` pulls /gateway/v1/artifact with the gateway token (ErrNoVersion on 404, ErrTokenRejected on 401/403); `Poller.OnApplied` turns /ready on through `server.SetConfigured`. `gatewayCmd` starts every mode with a probes-only pipeline and applies every config through one `reloadFromBytes`. Flags -studio-url and -gateway-token replace -registry-url and -registry-key. The end-to-end check is `TestManagedGatewaySurvivesStudioOutage` in `cmd/restitch/managed_test.go`, not in `tests/`: it runs the gateway as a helper process against an in-process Studio. `deploy/` and `examples/registry` still pass the removed registry flags: C26 rewrites them. The old `cmd/restitch-studio` and `internal/registry` stay until C24/C26 delete them.

---

### C11 — Studio lists each gateway instance with its applied artifact version

- **Status:** done
- **Depends on:** C10
- **Files:** `internal/hotreload/heartbeat.go`, `internal/studio/instances.go`, `internal/studio/instances_test.go`
- **Goal:** Each gateway posts a heartbeat with instance ID, binary version, applied version and last pull result; Studio lists instances per environment and marks silent ones stale.
- **First failing test:** `internal/studio/instances_test.go` — "TestHeartbeatRecordsAppliedVersion"
- **Done when:** the test command passes and two gateways in one environment appear as two instances.
- **Spec:** Decision on the heartbeat.
- **Notes:** Done: POST /gateway/v1/heartbeat upserts `studio_gateway_instances`; GET /api/v1/environments/{env}/instances lists them with current_version and stale (no heartbeat for `staleAfter`, one minute). The gateway sends `hotreload.Heartbeat` every 15s and at once after each applied version (`hotreload.RunHeartbeat`); the instance ID is the host name plus a random suffix. `Server.now` is the clock, so tests can age instances. The heartbeat carries the last pull error, so a stale config shows in Studio.

---

### C12 — Gateways push one-minute aggregates, and Studio keeps them for the retention period

- **Status:** done
- **Depends on:** C11
- **Files:** `internal/admin/accumulator.go`, `internal/hotreload/telemetry.go`, `internal/studio/usage.go`, `internal/studio/usage_test.go`
- **Goal:** Gateways push per-minute counts, errors, latency histograms, cache hits, breaker state and upstream health per composition, step and upstream; Studio sums replicas and deletes data past the environment's retention.
- **First failing test:** `internal/studio/usage_test.go` — "TestAggregatesSumAcrossInstances"
- **Done when:** the test command passes and the retention default is 7 days.
- **Spec:** Decision on usage push.
- **Notes:** A failed push retries the next interval and then drops with a counter metric; it never blocks requests.
  Done: managed gateways record into `admin.Accumulator` (now with step cache hits) and `hotreload.UsagePusher` posts each minute to POST /gateway/v1/usage with breaker states; a failed window rides along once, then counts in `restitch_telemetry_dropped_total{kind="usage"}` (it replaced the old request-records metric). Shutdown's final step pushes the last minute. Studio stores rows per instance/minute/composition in `studio_usage`, deletes rows older than `studio_environments.usage_retention_days` (default 7) on each push, and GET /api/v1/environments/{env}/usage sums instances per minute with a histogram p95, per-step totals, and per-upstream requests, errors and live breaker states. Upstream health is the per-upstream error count; there is no active health probe in v2.

---

### C13 — Gateways push sampled requests with redacted headers and no bodies

- **Status:** done
- **Depends on:** C12
- **Files:** `internal/admin/recorder.go`, `internal/hotreload/telemetry.go`, `internal/studio/requests.go`, `internal/studio/requests_test.go`
- **Goal:** Every failed request and a capped sample of successes reach Studio with step timings and redacted headers.
- **First failing test:** `internal/studio/requests_test.go` — "TestSampleNeverCarriesBodyOrAuthorization"
- **Done when:** the test command passes and the success sample cap is a per-environment setting.
- **Notes:** Done: `reqlog.Record` gains Headers (from `reqlog.RedactHeaders`); step URLs pass through `reqlog.RedactURL` (query values and user info become [redacted]); Studio runs `Record.Redact` again at ingest. The gateway's `hotreload.RequestSampler` keeps every failed or partial request and up to 60 successes a minute, queues at most 1000, pushes every 10s to POST /gateway/v1/requests and on shutdown, and counts drops in `restitch_telemetry_dropped_total{kind="requests"}`. Studio keeps failures and up to `request_samples_per_minute` (default 10) successes per minute in `studio_requests`, deleted after the usage retention. GET/PATCH /api/v1/environments/{env} reads and changes both settings (PATCH is owner-only). GET /api/v1/environments/{env}/requests lists samples.
- **Spec:** Decision on sampled requests.

---

### C14 — `restitch publish` and YAML import and export use the publish path

- **Status:** done
- **Depends on:** C08
- **Files:** `cmd/restitch/publish.go`, `internal/studio/importexport.go`, `internal/studio/importexport_test.go`, `cmd/restitch/importcmd.go`
- **Goal:** `restitch publish --env NAME FILE` replaces the draft from the file and publishes with an API key; Studio exports any version as the same YAML.
- **First failing test:** `internal/studio/importexport_test.go` — "TestExportThenImportRoundTrips"
- **Done when:** the test command passes and an invalid file exits non-zero with the validator message.
- **Spec:** Decision on CLI publish.
- **Notes:** Check what `restitch import` does today before you touch it; keep it only if it still has a job.
  Done: GET /api/v1/environments/{env}/export returns the draft YAML; POST .../import validates the whole file with `validateArtifact`, then `replaceDraft`s it, and a refused file leaves the draft unchanged. `restitch publish -env NAME [-m MSG] FILE` (RESTITCH_STUDIO_URL, RESTITCH_API_KEY) runs import then publish with an auth-all API key as a bearer token, prints the diff and the new version, and exits 1 with the validator message. API keys carry the rak_ prefix. `restitch import openapi` still scaffolds a config from an OpenAPI spec, and now emits auth.api_keys: [${GATEWAY_API_KEY}] so its output validates.

---

### C15 — `restitch dev` runs both roles in one process on SQLite

- **Status:** done
- **Depends on:** C10
- **Files:** `cmd/restitch/dev.go`, `cmd/restitch/dev_test.go`, `internal/devmode/`
- **Goal:** `restitch dev` starts Studio and a gateway in one process with a dev environment, a bootstrap user and a wired token, with no env vars.
- **First failing test:** `cmd/restitch/dev_test.go` — "TestDevServesPublishedComposition"
- **Done when:** the test command passes and `internal/devmode` no longer spawns child processes.
- **Spec:** Decision on dev mode.
- **Notes:** SQLite under `.restitch/` in the working directory. Print the sign-in URL and credentials on start.
  Done: `cmd/restitch/dev.go` starts `studio.Server` on 127.0.0.1 (default port 3080, SQLite in -data-dir, default .restitch), signs in dev@restitch.local / restitch-dev-password, and runs `gatewayCmd` in the same process as a managed gateway of environment "dev" with a 1s poll. `Server.PrepareDevEnvironment` creates the environment and issues a fresh token on each start, revoking the previous dev token. `internal/devmode` is deleted. Flags: -port, -studio-port, -data-dir.

---

### C16 — The new UI shell signs in and renders environments through the generated client

- **Status:** done
- **Depends on:** C06
- **Files:** `studio/package.json`, `studio/openapi-ts.config.ts`, `studio/src/routes/`, `studio/src/app/`, `studio/src/components/data-state.tsx`
- **Goal:** The UI uses TanStack Router and Query with a hey-api client generated from `openapi.yaml`; sign-in, sign-out, the environment list, light and dark themes, DataState and toasts work.
- **First failing test:** `studio/src/routes/login.test.tsx` — "redirects to sign-in without a session"
- **Done when:** the test command passes and `npm run build` succeeds with no react-router-dom dependency.
- **Spec:** Decision on the Studio API and UI stack.
- **Notes:** Copy patterns from `../sluice/ui/src` (data-state, toast, confirm-dialog, theme, app-shell); keep the Restitch accent. shadcn on Radix replaces base-ui. Delete pages as their replacements land, not before.
  Done: `studio/` is a new Vite + TanStack Router/Query app; the client is generated into `studio/src/api` by `npm run gen:api` from `api/openapi.yaml`. Ported from sluice: index.css tokens, components/ui, DataState, ConfirmDialog, theme, toasts, app and root shells, sign-in and change-password; the command palette, shortcuts and AI drawer were left out (C25 adds the palette). `api-client.ts` maps both error shapes (auth-all {error:{code,message}} and huma problem details) onto ApiError. The build lands in `internal/studio/dist` and `internal/studio/spa.go` serves it on / with an index.html fallback; /api/* that matches nothing answers 404. The old v1 pages could not run against v2, so they went now instead of one by one; their chart, waterfall and DAG components are in git at 751221b (`studio/src/components`, `studio/src/lib/dag.ts`) for C19 and C24. `cmd/restitch-studio`, `internal/registry` and `internal/session` are deleted (pulled forward from C26) because the old binary embedded the old build. Verified in a browser against `restitch dev`: sign-in, the environment list, create with a toast, light and dark themes, 390px width.

---

### C17 — Studio serves a JSON Schema of the artifact that covers every config field

- **Status:** done
- **Depends on:** C03, C05
- **Files:** `internal/composition/schema.go`, `internal/composition/schema_test.go`, `internal/studio/server.go`
- **Goal:** A JSON Schema generated from the Go artifact types describes every resource kind and field, and Studio serves it.
- **First failing test:** `internal/composition/schema_test.go` — "TestSchemaCoversEveryYAMLField"
- **Done when:** the test command passes and the test walks the Go types by reflection, so a new field without schema coverage fails it.
- **Spec:** Decision on forms from JSON Schema.
- **Notes:** Done: `composition.ArtifactSchema` uses github.com/invopop/jsonschema with yaml tags, package-qualified definition names (`composition.Upstream`, `auth.Config`, `ratelimit.Config`), durations as strings with a pattern, additionalProperties false like the strict decoder, and no required fields. `TestSchemaAcceptsTheShippedExamples` validates every example artifact against it. GET /api/v1/schema returns the artifact schema, the $defs name of each resource kind's body, and the settings schema.

---

### C18 — A resource form generated from the schema saves a resource, with a YAML tab

- **Status:** done
- **Depends on:** C16, C17
- **Files:** `studio/src/features/resources/`, `studio/src/routes/environments.$env.tsx`
- **Goal:** The draft page lists resources in a TanStack Table; each resource opens a schema-driven form and a YAML tab on the same object; a stale save shows a reload prompt.
- **First failing test:** `studio/src/features/resources/resource-form.test.tsx` — "saves an upstream created through the form"
- **Done when:** the test command passes and request_schema edits in a CodeMirror JSON editor.
- **Spec:** Decisions on forms and the shared object.
- **Notes:** The form library is open; see HANDOVER Open questions.
  Done: a hand-written renderer (`studio/src/features/resources/schema-form.tsx`) over GET /api/v1/schema, not rjsf: rjsf's theme layer clashes with the ported sluice components and the schema uses few shapes. Shapes: $ref and object sections (added and removed explicitly; an added empty section stays), string maps (headers), scalar arrays as comma lists, object arrays as repeatable cards, schema `true` and open objects as YAML/JSON CodeMirror fields, durations with the schema pattern checked inline. `CodeField` uses sluice's CodeMirror theme (`code-theme.ts`) and follows the light/dark class. Routes: /environments/$env (draft list with a New resource dialog) and /environments/$env/resources/$kind/$name (Form and YAML tabs on one object, Save with base_revision, Delete with confirm, a Reload banner on 409). A 404 on GET means a new resource (base_revision 0). Verified in a browser against `restitch dev`: an upstream and a composition with a step and a YAML response body saved through the forms and read back from the API unchanged. Wording to polish in C25: array items read "Steps 1" and "Add steps".

---

### C19 — The step graph widget edits depends_on

- **Status:** todo
- **Depends on:** C18
- **Files:** `studio/src/features/resources/step-graph.tsx`, `studio/src/lib/dag.ts`, `studio/src/features/resources/step-graph.test.tsx`
- **Goal:** Connecting two step nodes adds depends_on; removing the edge removes it; a cycle is refused.
- **First failing test:** `studio/src/features/resources/step-graph.test.tsx` — "refuses an edge that makes a cycle"
- **Done when:** the test command passes.
- **Notes:** Reuse @xyflow/react and `studio/src/lib/dag.ts`.

---

### C20 — Expression fields show server lint errors as the user types

- **Status:** done
- **Depends on:** C18
- **Files:** `internal/studio/lint.go`, `internal/studio/lint_test.go`, `studio/src/features/resources/expression-field.tsx`
- **Goal:** Expression and template fields post to a lint endpoint after typing stops and show errors inline.
- **Known gap to close:** `composition.Validate` accepts an expression with an unknown root (seen: `{{ request.path.id }}`; the root is `req`). Make compile reject unknown roots, so publish refuses it too, not only the lint.
- **First failing test:** `internal/studio/lint_test.go` — "TestLintReportsUnknownStepReference"
- **Done when:** the test command passes.
- **Notes:** The lint endpoint calls the same compile code as C03.
  Done: the gap was not an unknown root (request is a valid alias of req) but a member of a scalar: req.path is a string. `internal/composition/references.go` walks each compiled expression and checks dotted names against the executor's environment shape: req/request fields, no member on req.method, req.path or steps.<name>.status, steps.<name> fields, and unknown bare names. Template compilation runs it, so publish, `restitch check` and gateway load refuse those expressions. `composition.LintTemplate` also checks that steps.<name> names a step (compilation leaves that to the DAG check, whose wording existing tests pin). POST /api/v1/lint {template, steps} returns errors; text fields that hold {{ are linted 400ms after typing stops, against the composition's step names (`studio/src/features/resources/lint.tsx`). The check caught the same wrong fixture in `internal/studio/publish_test.go`.

---

### C21 — The secret picker inserts ${VAR} names and the environment lists required names

- **Status:** todo
- **Depends on:** C18
- **Files:** `internal/studio/secrets.go`, `internal/studio/secrets_test.go`, `studio/src/features/resources/secret-picker.tsx`
- **Goal:** Studio lists the ${VAR} names a draft and each version need; string fields offer a picker over those names.
- **First failing test:** `internal/studio/secrets_test.go` — "TestRequiredNamesListsEveryReference"
- **Done when:** the test command passes and no secret value is stored in Studio.

---

### C22 — The publish screen shows the diff, history, rollback and promote

- **Status:** done
- **Depends on:** C08, C16
- **Files:** `studio/src/features/publish/`, `studio/src/routes/environments.$env.versions.tsx`
- **Goal:** An editor previews the diff, enters a message and publishes; the history lists versions; rollback and promote ask for confirmation.
- **Notes:** Done: /environments/$env/versions (`studio/src/features/publish/VersionsPage.tsx`) with Review changes (preview diff through sluice's DiffView, validation errors block Publish, an unchanged draft says so), a required message, history with View YAML and Roll back behind ConfirmDialog, and a Promote dialog that picks environment and version and shows the draft diff. The draft page links to it. Verified in a browser against `restitch dev`: publish v1, edit, publish v2 (the gateway served it), roll back (v3 = v1, the gateway followed). Found while verifying: the poller backed off exponentially while no version existed, so a first publish could take minutes to arrive; it now keeps the poll interval for that case (`TestPollerKeepsIntervalWhileNoVersionExists`). Also found: validation accepts an unknown expression root (`request.path.id` instead of `req.params.id`), which fails only at request time; C20 must catch it.
- **First failing test:** `studio/src/features/publish/publish.test.tsx` — "shows validation errors and blocks publish"
- **Done when:** the test command passes.

---

### C23 — Owners manage gateway tokens and users in settings

- **Status:** done
- **Depends on:** C09, C16
- **Files:** `studio/src/features/settings/`, `studio/src/routes/settings.*.tsx`
- **Goal:** Owners create a gateway token and see it once, revoke tokens, and invite, change and disable users; non-owners do not see these actions.
- **Notes:** Done: gateway tokens live under the environment (/environments/$env/tokens, linked from the draft page for owners), because a token belongs to one environment; users are /settings/users (Settings nav, owner only). Users: add with a role and a generated temporary password shown once, role select, reset password, disable behind confirm, enable. The issue-token response schema now lists every field (`GatewayToken` is exported so huma includes the embedded fields). `restitch dev` deletes its previous token instead of revoking it, so the list does not fill with dead dev tokens. Test setup gained a native dialog stub and Testing Library cleanup. Verified in a browser: token shown once with copy, temporary password shown once.
- **First failing test:** `studio/src/features/settings/tokens.test.tsx` — "shows a new token once"
- **Done when:** the test command passes.

---

### C24 — Environment dashboards show aggregates, sampled requests and instances

- **Status:** done
- **Depends on:** C13, C16
- **Files:** `studio/src/features/dashboard/`, `studio/src/components/charts/`, `studio/src/components/waterfall/`
- **Goal:** Each environment shows usage charts, a sampled request list with the waterfall view, and the instances table.
- **First failing test:** `studio/src/features/dashboard/dashboard.test.tsx` — "shows the empty state before any gateway reports"
- **Done when:** the test command passes and the old Dashboard, Requests, Builder and Config pages are deleted.
- **Notes:** Reuse the chart and waterfall components; feed them from the new usage and request endpoints.
  Done: the environment landing page (/environments/$env) is the overview (`studio/src/features/dashboard/OverviewPage.tsx`): range switch, KPI tiles, requests-per-minute and p95 charts (sluice's lazy Recharts wrapper with data tables, `studio/src/components/charts`), upstreams with breaker states, instances with version lag, stale flag and last pull error, and sampled requests that expand into a step waterfall. An environment layout route adds sub-navigation (Overview, Draft at /environments/$env/draft, Publish and history, Gateway tokens for owners). The v1 pages were already deleted in C16; the v1 chart components were not reused because sluice's are accessible and themed. Found while verifying with live traffic: buckets were stamped with the flush time, so requests showed one minute late (the accumulator now stamps the window start), and instances of earlier dev runs never left the list (instances silent for a day are now hidden).

---

### C25 — The command palette and keyboard shortcuts reach every page and action

- **Status:** todo
- **Depends on:** C22, C23, C24
- **Files:** `studio/src/app/command-palette.tsx`, `studio/src/app/keyboard.tsx`, `studio/src/app/command-palette.test.tsx`
- **Goal:** Cmd+K finds pages, environments and resources; ? shows shortcuts; g plus a letter jumps to pages.
- **First failing test:** `studio/src/app/command-palette.test.tsx` — "finds a resource by name"
- **Done when:** the test command passes.
- **Notes:** Port from `../sluice/ui/src/app/`.

---

### C26 — One image runs every role, with Compose and Kubernetes files linted in CI

- **Status:** todo
- **Depends on:** C15
- **Files:** `Dockerfile`, `deploy/docker-compose.yml`, `deploy/kubernetes/`, `.github/workflows/ci.yml`, `Makefile`
- **Goal:** The image has entrypoint `/restitch`; Compose and plain manifests run Studio on Postgres and two gateways.
- **First failing test:** a CI step — `docker compose -f deploy/docker-compose.yml config` and `kubeconform deploy/kubernetes`
- **Done when:** the test command passes, both lint steps pass in CI, and `docker compose up` serves a published composition.
- **Notes:** Delete `cmd/restitch-studio`, `cmd/mockupstream` from the image if only tests need it, and the registry examples. No committed keys; Compose generates or demands them.

---

### C27 — The docs describe v2: container platforms, configuration, Studio, CHANGELOG

- **Status:** todo
- **Depends on:** C26
- **Files:** `docs/deployment.md`, `docs/configuration.md`, `docs/studio.md`, `README.md`, `CHANGELOG.md`
- **Goal:** A reader deploys Studio and gateways to any image-and-env-vars platform from the generic guide, and every env var in the code appears once in the docs.
- **First failing test:** `tests/readme_test.go` — "TestDocsListEveryEnvVar"
- **Done when:** the test command passes and AGENTS.md lines for registry and bundle are replaced by an accepted agents-audit item.
- **Notes:** The test reads the env names from the C01 derivation function, so docs cannot drift.
