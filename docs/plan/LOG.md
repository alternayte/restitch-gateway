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
