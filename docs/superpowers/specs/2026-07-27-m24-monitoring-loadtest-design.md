# M24 — Production Monitoring & Load Testing

**Date:** 2026-07-27
**Milestone:** M24 (PLAN.md §"M24 — Production Monitoring & Load Testing (extends M14, M15)")
**Status:** design approved, awaiting spec review

---

## Context

M24 is one of the two remaining unbuilt milestones. `scripts/check-ledger.sh`
exits 0 at 100/100 green; M1–M19 plus M20–M23 all carry gate evidence. Only
M24 and M25 remain, and both gate scripts are still the 14-line placeholders
that self-fail.

### Environment facts this design depends on

Verified on the development machine at design time:

| Fact | Value |
|------|-------|
| `k6` | v2.1.0, installed natively |
| Docker daemon | running |
| `promtool` | **not** installed on host; ships inside `prom/prometheus` image |
| `deploy/` directory | does not exist |
| Prometheus rules anywhere in repo | none |
| `.github/workflows/ci.yml` jobs | `go`, `studio`, `docker` — no k6, no release-branch job |
| Existing loadtest assets | `tests/loadtest/m23_fanout.js` only |

### Metrics the rules will be built on

All exist today in `internal/observability/metrics.go`:

| Metric | Labels |
|--------|--------|
| `restitch_requests_total` | `composition`, `method`, `status` |
| `restitch_request_duration_seconds` (histogram, `DefBuckets`) | `composition` |
| `restitch_partial_responses_total` | `composition` |
| `restitch_registry_polls_total` | `result` |
| `restitch_registry_last_success_timestamp` | — |

`status` is the full numeric code as a string
(`observability.StatusStr` → `strconv.Itoa`, `metrics.go:140`), so `status=~"5.."`
is a valid selector.

`/metrics` and `/health` are registered **outside** `requireKey` on the admin
server (`internal/admin/server.go:125-130`), so Prometheus can scrape even when
`admin.api_key` is set. The scrape config needs no secrets.

### Pre-existing drift noted, not fixed here

1. **PLAN.md status table** (lines 14–34) lists M23 as DONE but omits M20, M21
   and M22 entirely, despite all three having green gates. Fixed as a separate
   `docs:` commit before the M24 expansion begins — see "Out of scope" below.
2. **Registry store is SQLite-only.** PLAN T20.2 says "SQLite (dev) / Postgres
   (prod)", but `go.mod` carries only `modernc.org/sqlite` and there is no
   `postgres` reference in `internal/registry/`. The deploy stack therefore uses
   SQLite on a named volume. Adding a Postgres service would require code that
   does not exist; that is M20 scope, not M24.
3. **`examples/docker-compose/docker-compose.yml` is broken.** It sets
   `command: ["/restitch-studio"]` while the Dockerfile declares
   `ENTRYPOINT ["/restitch"]`, which resolves to `/restitch /restitch-studio`.
   Reported, not fixed inside M24 — it deserves its own commit.

---

## Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Rule correctness proven by `promtool test rules` (hermetic) **plus** one live compose smoke | Unit tests turn "alert fires" into a deterministic sub-second assertion with no clock-watching; the smoke proves the real server loads the rules and reaches the gateway. Near-zero MANUAL lines. |
| D2 | k6 runs in CI on release branches with **hard** thresholds, but a **scaled** profile | A wall-clock P95 threshold at 1000 RPS on a shared 2-core runner is a flake source. Scaling the load and setting the threshold from measured headroom keeps the build-failing signal honest. |
| D3 | `deploy/` runs the gateway in **registry mode** against Studio | The only place in the repo exercising the M20+M21 centralized-config loop as an operator would deploy it. `examples/docker-compose/` stays the simple file-mode quickstart. |
| D4 | **No Alertmanager** in the stack | A demo Alertmanager whose receiver discards every notification proves nothing and adds a service to keep healthy in the gate. `prometheus.yml` carries a commented `alerting:` block marking the wiring point. |
| D5 | Approach A — hermetic phases first, one integration smoke last | Strongest proof (promtool) in the cheapest phase; Docker flake confined to a single teardown-able step. |

---

## Architecture

### File layout

```
deploy/
  docker-compose.yml          # gateway (registry mode) + studio + prometheus + jaeger
  .env.example                # ports, image tag, admin key, OTEL endpoint
  README.md                   # bring-up, teardown, per-service purpose, threshold tuning
  prometheus/
    prometheus.yml            # scrape jobs + commented alerting: block
    rules/
      recording.yml           # T24.1
      alerts.yml              # T24.2
      recording_test.yml      # promtool unit tests
      alerts_test.yml         # promtool unit tests
tests/loadtest/
  m24_baseline.js             # T24.3, env-parameterised
scripts/gates/
  m24.sh                      # replaces placeholder; four phases
.github/workflows/
  ci.yml                      # T24.3 — new release-branch `loadtest` job
```

Unit tests live beside the rules because `promtool test rules` resolves
`rule_files` relative to the test file. `prometheus.yml` lists rule files
**explicitly** rather than globbing, so the `_test.yml` files are never loaded
by the running server.

### Service topology

The gateway runs `--registry-url=http://studio:3080`; Studio owns configuration
and persists its SQLite registry to a named volume. Prometheus scrapes the
gateway admin port and Studio. Jaeger receives OTLP from both.

**Port collisions, both real and both resolved in `.env.example`:**

- The gateway admin port is `9090` (`internal/gwconfig/config.go:165`), which is
  also Prometheus's conventional port. On the host they cannot both bind 9090,
  so Prometheus maps to **9091** and the gateway admin keeps 9090. Inside the
  compose network both keep native ports, so no config rewriting.
- The deploy stack uses `entrypoint: ["/restitch-studio"]`, not `command:`,
  avoiding the defect in `examples/docker-compose/`.

### Harness reuse

`scripts/lib/harness.sh` already provides `h_require_tool`,
`h_start_mockupstream`, `h_start_gateway`, `h_wait_for_port`, `h_config`,
`h_task`, `h_pass`/`h_fail`/`h_manual`, and an `h_cleanup` trap. The M24 gate
composes these and introduces **no new harness primitives**. Per CLAUDE.md,
`scripts/lib/` is not modified.

---

## T24.1 — Recording rules

`deploy/prometheus/rules/recording.yml`, `level:metric:operation` naming,
30s evaluation interval:

```yaml
groups:
  - name: restitch_composition
    interval: 30s
    rules:
      - record: composition:restitch_requests:rate5m
        expr: sum by (composition) (rate(restitch_requests_total[5m]))

      - record: composition:restitch_requests_errors:rate5m
        expr: sum by (composition) (rate(restitch_requests_total{status=~"5.."}[5m]))

      - record: composition:restitch_requests:error_ratio5m
        expr: |
          composition:restitch_requests_errors:rate5m
            / on (composition)
          composition:restitch_requests:rate5m

      - record: composition:restitch_partial_responses:rate5m
        expr: sum by (composition) (rate(restitch_partial_responses_total[5m]))

      - record: composition:restitch_request_duration_seconds:p50
        expr: histogram_quantile(0.50, sum by (composition, le) (rate(restitch_request_duration_seconds_bucket[5m])))
      - record: composition:restitch_request_duration_seconds:p95
        expr: histogram_quantile(0.95, sum by (composition, le) (rate(restitch_request_duration_seconds_bucket[5m])))
      - record: composition:restitch_request_duration_seconds:p99
        expr: histogram_quantile(0.99, sum by (composition, le) (rate(restitch_request_duration_seconds_bucket[5m])))
```

### Two deliberate choices

**Partial responses are recorded separately and NOT folded into
`error_ratio5m`.** PLAN.md says "error rate by composition", read here as 5xx.
But a partial response is the gateway's signature degraded state and returns a
success status, so a pure 5xx ratio would show a composition as perfectly
healthy while half its steps fail. Recording it separately costs one rule and
makes the dashboard honest. Folding it in would make the T24.2 error-rate alert
fire on a condition operators routinely accept.

**`error_ratio5m` has no `or vector(0)` fallback.** A composition that has served
zero 5xx produces no series from the numerator, so the ratio is absent rather
than zero. Absent means nothing to alert on. A zero-fill would manufacture
series for every composition forever and defeat `absent()`-style checks.

---

## T24.2 — Alert rules

`deploy/prometheus/rules/alerts.yml`, built on the recorded series above:

| Alert | Expression | `for` | Severity |
|-------|-----------|-------|----------|
| `RestitchHighP95Latency` | `composition:restitch_request_duration_seconds:p95 > 1` | 5m | warning |
| `RestitchHighErrorRate` | `composition:restitch_requests:error_ratio5m > 0.05` | 5m | critical |
| `RestitchConfigReloadFailing` | `rate(restitch_registry_polls_total{result!="success"}[5m]) > 0` | 10m | warning |
| `RestitchGatewayDown` | `up{job="restitch-gateway"} == 0` | 2m | critical |

**On "configurable, default 1s":** Prometheus rule files have no variable
substitution. The threshold is a literal in `alerts.yml` with a comment marking
it as the tuning knob, documented in `deploy/README.md`. Building a templating
layer nothing else in the repo uses is not justified.

**`RestitchConfigReloadFailing` uses a 10m `for` deliberately.** M21's poller
retries with exponential backoff, so a single transient fetch failure is normal
self-healing behaviour. A 5m `for` would page on something the system is
designed to absorb.

---

## Unit tests — the actual proof of "alert fires"

Each alert gets a **positive and a negative** case in
`deploy/prometheus/rules/alerts_test.yml`:

```yaml
tests:
  - interval: 1m
    input_series:
      - series: 'restitch_requests_total{composition="orders",method="GET",status="500"}'
        values: '0+6x10'          # 6 errors/min
      - series: 'restitch_requests_total{composition="orders",method="GET",status="200"}'
        values: '0+54x10'         # 54 ok/min → 10% error ratio
    alert_rule_test:
      - eval_time: 10m
        alertname: RestitchHighErrorRate
        exp_alerts:
          - exp_labels: {severity: critical, composition: orders}
```

The positive cases are load-bearing: they catch a recording rule renamed in
`recording.yml` but not in `alerts.yml`, which produces an alert that silently
never fires. A negative case alone would still pass against that defect.

---

## T24.3 — Load test and CI job

### `tests/loadtest/m24_baseline.js`

PLAN.md phrases the target as "~1000 RPS (50 VUs)". Those are different things
in k6: 50 VUs in an open loop produce whatever throughput the system allows, so
if the gateway slows down the offered load drops with it and the test stops
measuring what it was built to measure. The script uses `constant-arrival-rate`,
holding request rate fixed and treating VUs as a pool:

```js
export const options = {
  scenarios: {
    compositions: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RATE || 1000),
      timeUnit: '1s',
      duration: __ENV.DURATION || '60s',
      preAllocatedVUs: Number(__ENV.VUS || 50),
      maxVUs: Number(__ENV.MAX_VUS || 200),
    },
    // studio_api is added only when STUDIO_URL is set — see below.
  },
  thresholds: {
    'http_req_duration{scenario:compositions}': [`p(95)<${__ENV.P95_MS || 1000}`],
    'http_req_failed{scenario:compositions}': [`rate<${__ENV.ERR_RATE || 0.01}`],
  },
};
```

**Studio deliberately gets a low rate — a documented deviation.** PLAN.md says
"exercise gateway compositions and Studio API at ~1000 RPS". Studio is the
control plane (config CRUD, registry bundles); driving it at 1000 RPS would
measure SQLite write contention rather than anything an operator experiences,
and would dominate the failure signal. 5 req/s proves the control plane stays
responsive while the data plane is saturated, which is the real question.

**The studio scenario is conditional.** `constant-arrival-rate` rejects
`rate: 0`, so the scenario cannot be disabled by setting it to zero. Instead the
script builds `options.scenarios` at load time and adds `studio_api` **only when
`__ENV.STUDIO_URL` is set**:

```js
if (__ENV.STUDIO_URL) {
  options.scenarios.studio_api = {
    executor: 'constant-arrival-rate',
    rate: Number(__ENV.STUDIO_RATE || 5),
    timeUnit: '1s',
    duration: __ENV.DURATION || '60s',
    preAllocatedVUs: 2,
    maxVUs: 10,
  };
}
```

This matters because the gate and the CI job have different topologies: the gate
starts Studio via the harness's `h_start_studio` and sets `STUDIO_URL`, while a
gateway-only run simply omits it. Without the guard, any run without Studio
would fail on connection errors in a scenario that is not the subject of the
test.

### Two profiles

| | RATE | DURATION | P95_MS | ERR_RATE |
|---|------|----------|--------|----------|
| Gate (`scripts/gates/m24.sh`) | 1000 | 60s | 1000 | 0.01 |
| CI (release branches) | 200 | 30s | measured × 2, rounded up to nearest 50ms | 0.01 |

The gate profile drives mockupstream + Studio + gateway (`h_start_mockupstream`,
`h_start_studio`, `h_start_gateway`) with `STUDIO_URL` set. The CI job runs the
gateway-only topology and leaves `STUDIO_URL` unset.

**The CI `P95_MS` value is set in a separate, later task**, after the job runs on
a real runner and produces numbers, with the observed P95 pasted into the commit
as evidence. The formula is fixed in advance — observed P95 × 2, rounded up to
the nearest 50ms — so the value is derived mechanically from the evidence rather
than chosen to make the build pass. Guessing it now yields either a threshold so
loose it catches nothing or so tight it flakes immediately.

Error rate stays at 1% in both profiles — it is not load-dependent.

### CI job

No release-branch convention exists yet (`ci.yml` triggers on `push:` for all
branches and PRs to `main`). This adds:

```yaml
  loadtest:
    if: startsWith(github.ref, 'refs/heads/release/') || startsWith(github.ref, 'refs/tags/v')
    runs-on: ubuntu-latest
    # build binaries → start mockupstream + gateway → setup-k6 → run reduced profile
```

`workflow_dispatch` is added to the workflow so the job can be triggered by hand
without cutting a release branch. Otherwise it is unrunnable until the first
`release/*` branch exists, and an untested CI job is not a delivered CI job.

The job runs binaries directly rather than compose: it measures gateway
throughput, and inserting Docker networking between k6 and the gateway would
measure the bridge as much as the code.

---

## T24.4 — Deploy stack

`deploy/docker-compose.yml` with four services:

| Service | Notes |
|---------|-------|
| `studio` | `entrypoint: ["/restitch-studio"]`; SQLite registry on a named volume |
| `gateway` | `--registry-url=http://studio:3080`; admin+metrics on 9090 |
| `prometheus` | pinned image tag; binds `deploy/prometheus/` read-only; host port 9091 |
| `jaeger` | OTLP receiver for gateway and studio traces |

`.env.example` carries host ports, image tag, admin API key, and OTEL endpoint.
`deploy/README.md` documents bring-up, teardown, each service's purpose, and
the P95 threshold tuning knob.

---

## Gate — `scripts/gates/m24.sh`

Replaces the placeholder. Follows `m23.sh` conventions (`h_task <ID>` per PLAN
task, `h_pass`/`h_fail` assertions, per-task evidence).

```
h_task M24.gate     preflight: h_require_tool docker, docker info, h_require_tool k6
h_task T24.1        promtool check rules + test rules → recording.yml
h_task T24.2        promtool check rules + test rules → alerts.yml
h_task T24.3        build binaries → mockupstream + gateway → k6 gate profile → assert summary
h_task T24.4        docker compose up → assert rules loaded + gateway scraped → down
h_task M24.unit     aggregate promtool unit-test counts
h_task M24.gate     final roll-up
```

`promtool` runs as
`docker run --rm -v deploy/prometheus:/p prom/prometheus:${PROMETHEUS_VERSION} promtool …`.

**The version has a single source of truth.** `PROMETHEUS_VERSION` is defined in
`deploy/.env.example`; both `deploy/docker-compose.yml` and the gate read it from
there. A gate that validated rules with a different Prometheus than the one that
loads them would prove nothing about the deployed stack, and two independently
maintained version literals would drift apart silently. The gate `h_fail`s if the
variable is unset rather than falling back to `latest`.

### Negative controls

The M23 gate history (`bb9c8f04` closing mutation gaps, `251ec993` adding a
loop-closer and a magnitude floor) sets the standard: a gate must fail when the
feature is absent. M24's weak spots and guards:

| Weak spot | Guard |
|-----------|-------|
| `promtool test rules` passes vacuously on an empty/missing test file | Parse reported unit-test count; `h_fail` if zero or below expected |
| Prometheus starts but loads no rules | Assert `/api/v1/rules` returns groups **and** a rule count matching the files, not just HTTP 200 |
| Prometheus runs but never reaches the gateway | Query `up{job="restitch-gateway"} == 1`, not merely that Prometheus is healthy |
| k6 runs but drives nothing | Assert `http_reqs` ≥ 95% of `rate × duration` from the exported summary |
| Alert references a renamed recording rule | Positive-case unit test per alert |

### Failure handling

Compose teardown (`docker compose down -v`) goes in the existing `h_cleanup`
trap. The M22 ledger already records a run interrupted before `h_finish` wrote
its rows; orphaned containers and volumes would make the next run fail
confusingly. Host ports come from gate-set `.env` values rather than defaults,
so a gate run cannot collide with a stack already running locally.

The first `docker compose build` runs `npm ci` and two Go builds and takes
minutes. The gate logs this before starting rather than appearing to hang.

### Testing

**M24 adds no Go code** — rules YAML, compose, a k6 script, CI YAML, and the
gate. `make ci` is untouched, and the promtool unit tests *are* this milestone's
test suite. That is precisely why the `M24.unit` count assertion above is
required: without it, the milestone's only tests could silently be no tests.

`M24.unit` will appear as an UNKNOWN-ID in `check-ledger.sh`, exactly as
`M20.unit`–`M23.unit` already do. Expected, not a regression.

---

## Task breakdown

Per CLAUDE.md rule 2, the gate script is the first task and needs user approval
before feature work begins.

| Task | Description |
|------|-------------|
| T24.0 | Replace `scripts/gates/m24.sh` placeholder with the real gate. **User approval required before proceeding.** |
| T24.1 | `recording.yml` + `recording_test.yml` |
| T24.2 | `alerts.yml` + `alerts_test.yml` |
| T24.3a | `tests/loadtest/m24_baseline.js` + gate profile wiring |
| T24.3b | CI `loadtest` job; set `P95_MS` from measured baseline, evidence in commit |
| T24.4 | `deploy/` stack: compose, prometheus.yml, `.env.example`, README |

**Plan tasks vs ledger IDs.** PLAN.md defines four tasks, T24.1–T24.4, and those
are the only task IDs `check-ledger.sh` knows. T24.0, T24.3a and T24.3b are
plan-level subdivisions for execution order only. The gate emits a single
`h_task T24.3` covering 3a and 3b, and T24.0 produces no task row of its own —
the gate script's own correctness is what every other row depends on. Ledger
rows for M24 are therefore exactly: `T24.1`, `T24.2`, `T24.3`, `T24.4`,
`M24.unit`, `M24.gate`. (`M24.unit` is a known UNKNOWN-ID, matching
`M20.unit`–`M23.unit`.)

---

## Out of scope

- **PLAN.md status-table drift** (missing M20/M21/M22 rows): separate `docs:`
  commit *before* the M24 expansion, so rule 2 still holds and the fix is not
  entangled with a milestone that could fail.
- **`examples/docker-compose/` entrypoint defect**: reported, fixed separately.
- **Postgres support for the registry**: M20 scope; requires code that does not
  exist.
- **Alertmanager**: per D4.
- **M25**: gets its own brainstorm, spec, and plan after M24's gate is green.
