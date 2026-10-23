# Session log

Append only. Newest entry at the bottom. Never edit an older entry.

---

## 2026-10-23 — session 1

- **Chunks:** none started; 27 chunks planned.
- **Did:** adversarial review of hardening-doc.md; fixes for request_schema bypass, cache key and shutdown order merged on `fix/review-5-6-7`; grill; spec `docs/specs/studio-control-plane.md`; AGENTS.md written with init-agents; domain words accepted.
- **Decided:** the Apollo/Hive control-plane model, the Kong split for artifact contents, auth-all for sign-in, Postgres for deployed Studio, v2 with no compatibility layer, generic container-platform docs instead of vendor recipes.
- **Blocked:** none.

---

## 2026-10-23 — session 1, continued

- **Chunks:** C01 done.
- **Did:** module path `/v2`; `restitch gateway` replaces `restitch run`; `gwconfig.EnvName` and `gwconfig.BindEnv` bind every flag to RESTITCH_ plus its name; `-cert`/`-key` renamed `-tls-cert`/`-tls-key`; compose files, CI load test and Dockerfile CMD call `gateway`.
- **Decided:** bare `restitch` prints usage and exits 2, because a default role hides which role a container runs. `TestApplyEnvOverrides` now covers admin env only, because server env moved to `BindEnv`.
- **Blocked:** none. The SA5011 lint issues seen earlier did not recur on a clean `make ci`.

---

## 2026-10-23 — session 2

- **Chunks:** C05, C02, C03 done.
- **Did:** `internal/studio` with auth-all on huma v2 (sign-in, roles, bootstrap owner, API keys plugin, OIDC option, store rate limiter, per-user preferences) and `restitch studio`; CI Postgres service. Loader refuses server:/admin: with the replacement env var; auth and rate_limit became top-level artifact keys owned by `composition.Config`; strict decoding; one validator; the fail-open route is refused at parse and fails closed in the handler; reserved probe paths.
- **Decided:** C02 moved auth to the top level in the same change, because removing server: first would leave gateways with no auth between chunks. Read and write timeouts became flags because the YAML values were parsed but never used. Studio binds every interface by default, because it now authenticates every API call.
- **Blocked:** none. auth-all#17 (disable public sign-up) is open; Studio routes the path to 404 meanwhile.

---

## 2026-10-23 — session 3

- **Chunks:** C06, C07, C08, C09, C04, C10 done.
- **Did:** draft resources (`internal/studio/draft.go`), publish/versions/rollback/promote (`publish.go`), gateway tokens and the pull endpoint (`gatewayapi.go`), removal of the gateway admin API and storage, the ops port, managed mode with readiness gated on the first pull.
- **Decided:** a gateway token on a user route answers 401 (it is no user credential), not 403 as the plan said; the plan row now says 401. The managed-mode end-to-end test lives in `cmd/restitch` as a helper-process test, because it needs `gatewayCmd` and an in-process Studio. `composition.MaxArtifactBytes` is the one size limit for publish and pull.
- **Blocked:** none.
- **Chunks (continued):** C11 done: heartbeats and the instances list. Session closed near the context budget the user set.

---

## 2026-10-23 — session 3, continued

- **Chunks:** C12, C13 done.
- **Did:** usage push and query (`internal/studio/usage.go`, `internal/hotreload/usage.go`), sampled requests with redaction (`internal/reqlog`, `internal/studio/requests.go`, `internal/hotreload/requests.go`), environment settings (GET/PATCH /api/v1/environments/{env}).
- **Decided:** `restitch_telemetry_dropped_total{kind}` replaced the request-records metric, which lost its user when C04 removed the store. Upstream health is the per-upstream error count from pushed step metrics, plus breaker states; v2 has no active health probe. The gateway caps successful samples at 60 a minute and Studio applies the environment cap on top.
- **Blocked:** none.
