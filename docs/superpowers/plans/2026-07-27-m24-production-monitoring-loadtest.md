# M24 — Production Monitoring & Load Testing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship Prometheus recording and alert rules with executable unit tests, a parameterised k6 load test wired into both the gate and CI, and a `deploy/` compose stack running the gateway in registry mode alongside Prometheus and Jaeger.

**Architecture:** M24 adds **no Go code**. It is rules YAML, a compose stack, a k6 script, a CI job, and the gate that proves them. Rule *logic* is proven hermetically by `promtool test rules` (synthetic series in, exact alert assertions out); rule *loading* is proven by one live compose smoke that queries Prometheus's `/api/v1/rules` and asserts it is scraping the gateway. The gate composes existing `scripts/lib/harness.sh` helpers and introduces no new primitives.

**Tech Stack:** Prometheus v3.13.1 (`prom/prometheus`, verified present in registry), Jaeger 1.70.0 (`jaegertracing/all-in-one`, verified present), k6 v2.1.0 at `/opt/homebrew/bin/k6`, Docker 27.3.1 with Compose v2.30.3, Python 3.12.3, bash gate harness.

**Spec:** `docs/superpowers/specs/2026-07-27-m24-monitoring-loadtest-design.md`

## Global Constraints

- Go module: `github.com/restitch/restitch-gateway`, Go 1.25.6. **M24 changes no Go files.**
- **NEVER** edit `scripts/verify.sh`, `scripts/check-ledger.sh`, `scripts/lib/`, or any gate script other than `scripts/gates/m24.sh`. `m24.sh` composes only existing harness helpers.
- **NEVER** edit or delete existing `LEDGER.md` rows. Append only. A FAIL is superseded by a later PASS row, never removed.
- **NEVER** weaken, skip, or delete a test to make a gate pass.
- After every task: run its Accept command, paste the **real output** into the commit message, and append one `LEDGER.md` row.
- Commit prefixes: `feat(M24):`, `test:`, `docs:`, `chore:`. The gate script commit **must** start with `gate:` and requires explicit user approval first.
- PLAN.md's M24 section has no `Accept:` lines (verified — the milestone is a bare task table, PLAN.md:2325-2345). Accept commands below are derived from the approved spec and encoded in `scripts/gates/m24.sh`. This matches the M23 precedent.
- **Ledger IDs for M24 are exactly:** `T24.1`, `T24.2`, `T24.3`, `T24.4`, `M24.unit`, `M24.gate`. `h_finish` writes one row per `h_task` name, so **no `h_task` may use any other ID.** T24.0/T24.3a/T24.3b in this plan are execution-order subdivisions only and must never appear as `h_task` names.
- `PROMETHEUS_VERSION` has one source of truth: `deploy/.env.example`. Both `deploy/docker-compose.yml` and `scripts/gates/m24.sh` read it from there. Never hardcode a second copy; never fall back to `latest`.
- Docker is **hard-required** by the gate. There is no skip path — a missing daemon is a `h_fail`, not a `h_skip`.

---

## File Structure

**Create:**
- `scripts/gates/m24.sh` — replaces the placeholder; the real M24 gate. Four phases.
- `deploy/.env.example` — image version pins and host ports. Read by compose *and* the gate.
- `deploy/prometheus/prometheus.yml` — scrape jobs, explicit `rule_files` list, commented `alerting:` block.
- `deploy/prometheus/rules/recording.yml` — T24.1 recording rules.
- `deploy/prometheus/rules/recording_test.yml` — promtool unit tests for the above.
- `deploy/prometheus/rules/alerts.yml` — T24.2 alert rules.
- `deploy/prometheus/rules/alerts_test.yml` — promtool unit tests for the above.
- `deploy/docker-compose.yml` — gateway (registry mode) + studio + prometheus + jaeger.
- `deploy/README.md` — bring-up, teardown, seeding, threshold tuning.
- `tests/loadtest/m24_baseline.js` — env-parameterised k6 scenario with `handleSummary`.

**Modify:**
- `PLAN.md:14-34` — status table: add missing M20/M21/M22 rows (Task 0), then M24 (Task 7).
- `.github/workflows/ci.yml` — add `workflow_dispatch` trigger and a `loadtest` job (Task 6).
- `docs/plan-progress/LEDGER.md` — append-only rows after each task.

**Never touched:** `scripts/lib/harness.sh`, `scripts/verify.sh`, `scripts/check-ledger.sh`, all other `scripts/gates/*.sh`, `examples/docker-compose/`, any `.go` file.

---

## Verified Environment Facts

These were confirmed at planning time. If any is false at execution time, **stop and report** rather than adapting silently.

| Fact | Value |
|------|-------|
| `k6` | v2.1.0 at `/opt/homebrew/bin/k6` |
| `docker` | 27.3.1, daemon running |
| `docker compose` | v2.30.3-desktop.1 |
| `promtool` | NOT on host — only via `docker run --entrypoint=/bin/promtool prom/prometheus` |
| `prom/prometheus:v3.13.1` | tag exists (`docker manifest inspect` succeeded) |
| `jaegertracing/all-in-one:1.70.0` | tag exists (`docker manifest inspect` succeeded) |
| Gateway metrics endpoint | `/metrics` on the **admin** port, outside `requireKey` (`internal/admin/server.go:129`) |
| Gateway admin port default | 9090 (`internal/gwconfig/config.go:165`) |
| Admin env overrides | `RESTITCH_ADMIN_ENABLED`, `RESTITCH_ADMIN_PORT`, `RESTITCH_ADMIN_API_KEY` (`internal/gwconfig/config.go:252-263`) |
| Registry mode flags | `-registry-url`, `-registry-key`, `-poll-interval` (`cmd/restitch/run.go:36-38`); mutually exclusive with `-config` |
| Studio flags | `-port` (3080), `-gateway-admin-url`, `-admin-key`, `-db-path` (`./studio.db`), `-no-migrate` |
| Studio list endpoint | `GET /api/v1/configs` (`cmd/restitch-studio/api.go:64`) |
| OTEL config | `OTEL_EXPORTER_OTLP_ENDPOINT` env var; tracing disabled when unset (`internal/observability/tracing.go:24`) |
| Harness ports | `GW_PORT`, `ADMIN_PORT`, `MOCK_PORT`, `STUDIO_PORT` auto-assigned by `h_init` |
| k6 summary shape | `metrics["http_req_duration"]["p(95)"]`, `["http_req_failed"]["value"]`, `["http_reqs"]["count"]` |

### Pre-validated at planning time

These blocks were **executed**, not just written. Expected outputs below are real, not predicted:

| Artefact | Verification | Result |
|---|---|---|
| `recording.yml` | `promtool check rules` | `SUCCESS: 7 rules found` |
| `alerts.yml` | `promtool check rules` | `SUCCESS: 4 rules found` |
| `recording_test.yml` | `promtool test rules` | `SUCCESS` |
| `alerts_test.yml` | `promtool test rules` | `SUCCESS` |
| Alert mutation check | renamed a recorded series in `alerts.yml` | `FAILED … got:[]` — the alert stops firing, positive test catches it |
| `m24_baseline.js` (gateway only) | k6 at 50/s for 5s | `{"compositions":{"p95_ms":1.32,"error_rate":0,"reqs":251}}` |
| `m24_baseline.js` (with `STUDIO_URL`) | k6, both scenarios | `{"compositions":{…,"reqs":251},"studio":{"p95_ms":0.877,"reqs":25}}` |
| Gate script | `bash -n` | syntax OK (349 lines) |
| All 8 YAML blocks | `yaml.safe_load` | all parse |

Two defects were found and fixed during that validation, so **do not "restore" these**:
- promtool requires `exp_annotations` on every positive `exp_alerts` entry; omitting it asserts annotations are *empty* and fails.
- k6 only materialises a sub-metric when a threshold references it. The studio scenario needs its own thresholds or `handleSummary` silently reports `reqs: 0`.

**Registry-mode bootstrap caveat:** in registry mode the *entire* gateway config — server and admin blocks included — comes from the registry bundle, and the initial fetch is blocking and fatal (`cmd/restitch/run.go:84-92`). A fresh Studio has no configs, so the bundle is empty and the gateway starts with zero compositions. That is fine for the smoke test (the admin server still runs and Prometheus can scrape it), which is why the compose stack sets `RESTITCH_ADMIN_ENABLED`/`RESTITCH_ADMIN_PORT` as **env vars** rather than relying on the bundle to carry an admin block.

---

## Task 0: Fix PLAN.md status-table drift

Not part of M24. A standalone docs commit that lands **before** the milestone begins, so CLAUDE.md rule 2 still holds (the gate script is M24's first task) and the drift fix is not entangled with a milestone that could fail.

**Files:**
- Modify: `PLAN.md:33-34`

- [ ] **Step 1: Confirm the drift is real**

Run: `grep -nE "^\| M(19|2[0-5]) " PLAN.md`

Expected: rows for M19 and M23 only. M20, M21, M22 absent despite green gates.

- [ ] **Step 2: Confirm those gates actually passed**

Run: `grep -E "M2[012]\.gate" docs/plan-progress/LEDGER.md | tail -5`

Expected: `M20.gate … PASS`, `M21.gate … PASS`, `M22.gate … MANUAL-VERIFIED`.

- [ ] **Step 3: Insert the three missing rows**

In `PLAN.md`, immediately after the `| M19 — CI & Test Hardening | T19.1–T19.5 | DONE |` row and before the `| M23 — …` row, insert:

```markdown
| M20 — Config Registry & Centralized Management | T20.1–T20.6 | DONE |
| M21 — Gateway Registry Polling | T21.1–T21.5 | DONE |
| M22 — Dev Mode Orchestrator | T22.1–T22.5 | DONE |
```

- [ ] **Step 4: Verify the table now reads in order**

Run: `grep -nE "^\| M(1[6-9]|2[0-5]) " PLAN.md`

Expected: M16, M17, M18, M19, M20, M21, M22, M23 — contiguous, all DONE.

- [ ] **Step 5: Confirm nothing else regressed**

Run: `scripts/check-ledger.sh; echo "EXIT=$?"`

Expected: `COVERAGE: 100/100 green` and `EXIT=0`.

- [ ] **Step 6: Commit**

```bash
git add PLAN.md
git commit -m "docs: add missing M20-M22 rows to PLAN.md status table

All three milestones carry green gate evidence in LEDGER.md but were
absent from the completion table. Documentation-only; no behaviour change."
```

---

## Task 1: Replace the `scripts/gates/m24.sh` placeholder

**STOP GATE — the finished script must be shown to the user and explicitly approved before Task 2 begins.** Per CLAUDE.md rule 2, this is M24's first task; per the hard rules, gate changes need user approval and a `gate:` commit prefix.

The gate is written **before** the features exist and is *expected* to fail at this point. That failure is the proof the gate detects absence — the same discipline `m23.sh` used ("checks and both k6 runs to FAIL until those tasks are implemented", `scripts/gates/m23.sh:26`).

**Files:**
- Modify (full replacement): `scripts/gates/m24.sh`

**Interfaces:**
- Consumes from `scripts/lib/harness.sh`: `h_init`, `h_cleanup`, `h_task`, `h_pass`, `h_fail`, `h_log`, `h_evidence`, `h_run`, `h_require_tool`, `h_free_port`, `h_build`, `h_start_mockupstream`, `h_start_gateway`, `h_start_studio`, `h_config`, `h_finish`, and the `REPO_ROOT`/`H_TMP`/`GW_PORT`/`ADMIN_PORT`/`MOCK_PORT`/`STUDIO_PORT`/`H_PIDS` variables.
- Produces for later tasks: the exact file paths and thresholds every other task must satisfy — `deploy/.env.example` with `PROMETHEUS_VERSION`, `deploy/prometheus/rules/{recording,alerts}{,_test}.yml`, `deploy/docker-compose.yml`, `tests/loadtest/m24_baseline.js` writing its summary to `$M24_SUMMARY_OUT`.

- [ ] **Step 1: Read the placeholder you are replacing**

Run: `cat scripts/gates/m24.sh`

Expected: the 14-line placeholder ending in `h_fail "M24 gate not implemented…"`.

- [ ] **Step 2: Write the full gate script**

Replace the entire contents of `scripts/gates/m24.sh` with:

```bash
#!/usr/bin/env bash
# Gate M24 — Production Monitoring & Load Testing
#
# Four phases, cheapest first (spec decision D5):
#   T24.1/T24.2  promtool check+test on the rule files. Hermetic: synthetic
#                series in, exact alert assertions out. No stack, no scraping,
#                no clock-watching. This is where "alert fires" is proven.
#   T24.3        k6 against a real gateway + studio + mockupstream.
#   T24.4        one live compose smoke proving the real Prometheus loads the
#                rules and is scraping the gateway.
#
# Docker is hard-required. There is deliberately no skip path: a gate that can
# go green without ever starting the stack would defeat the purpose of the
# integration phase.
source "$(dirname "$0")/../lib/harness.sh"
h_init M24

DEPLOY_DIR="${REPO_ROOT}/deploy"
PROM_DIR="${DEPLOY_DIR}/prometheus"
RULES_DIR="${PROM_DIR}/rules"
ENV_FILE="${DEPLOY_DIR}/.env.example"
COMPOSE_PROJECT="restitch-m24-gate"

# ── Cleanup chaining ─────────────────────────────────────────────────────────
# h_init installed `trap h_cleanup EXIT`. scripts/lib/ is off-limits, so the
# compose teardown is chained here instead of being added to h_cleanup.
M24_COMPOSE_UP=false
m24_cleanup() {
    if [[ "${M24_COMPOSE_UP}" == "true" ]]; then
        (cd "${DEPLOY_DIR}" && docker compose -p "${COMPOSE_PROJECT}" \
            --env-file "${H_TMP}/gate.env" down -v --remove-orphans) \
            >/dev/null 2>&1 || true
    fi
}
trap 'm24_cleanup; h_cleanup' EXIT

# ── Helpers ──────────────────────────────────────────────────────────────────

# promtool ships inside the prometheus image; it is not installed on the host.
# The image entrypoint is /bin/prometheus, so promtool needs an explicit
# --entrypoint. The tag comes from deploy/.env.example so the gate validates
# rules with exactly the Prometheus that will load them.
m24_promtool() {
    docker run --rm -v "${PROM_DIR}:/p:ro" \
        --entrypoint=/bin/promtool \
        "prom/prometheus:${PROM_VERSION}" "$@"
}

# Guard against vacuous unit-test files: promtool exits 0 on a file declaring
# no tests at all, so exit status alone cannot distinguish "all tests passed"
# from "there were no tests".
m24_assert_min_count() {
    local file="$1" pattern="$2" min="$3" desc="$4"
    if [[ ! -f "${file}" ]]; then
        h_fail "${desc} (file not found: ${file})"
        return 0
    fi
    local n
    n="$(grep -c -- "${pattern}" "${file}" 2>/dev/null || true)"
    n="${n:-0}"
    h_evidence "$ grep -c -- '${pattern}' ${file} -> ${n} (minimum ${min})"
    if [[ "${n}" -ge "${min}" ]]; then
        h_pass "${desc} (${n} >= ${min})"
    else
        h_fail "${desc} (${n} < ${min})"
    fi
}

# Poll a URL until a python predicate over its JSON body is true.
# Usage: m24_wait_json <url> <python-expr over `data`> <timeout-secs>
m24_wait_json() {
    local url="$1" expr="$2" timeout="${3:-60}"
    local deadline=$((SECONDS + timeout)) body
    while [[ ${SECONDS} -lt ${deadline} ]]; do
        body="$(curl -s --max-time 5 "${url}" 2>/dev/null || true)"
        if [[ -n "${body}" ]] && python3 -c "
import json,sys
data = json.loads(sys.argv[1])
sys.exit(0 if (${expr}) else 1)
" "${body}" 2>/dev/null; then
            h_evidence "$ curl -s ${url}  -> predicate '${expr}' satisfied"
            h_evidence "${body}"
            return 0
        fi
        sleep 2
    done
    h_evidence "$ curl -s ${url}  -> predicate '${expr}' NEVER satisfied in ${timeout}s"
    h_evidence "last body: ${body:-<empty>}"
    return 1
}

# ── Preflight ────────────────────────────────────────────────────────────────
h_task M24.gate

if ! h_require_tool docker; then
    h_fail "docker not found on PATH — M24 requires Docker (no skip path)"
    h_finish
fi
if ! docker info >/dev/null 2>&1; then
    h_fail "docker daemon not reachable — start Docker and re-run"
    h_finish
fi
h_pass "docker available"

if ! h_require_tool k6; then
    h_fail "k6 not found on PATH — M24 requires k6"
    h_finish
fi
h_pass "k6 available"

if [[ ! -f "${ENV_FILE}" ]]; then
    h_fail "deploy/.env.example missing — PROMETHEUS_VERSION has no source of truth"
    h_finish
fi
PROM_VERSION="$(grep -E '^PROMETHEUS_VERSION=' "${ENV_FILE}" | head -1 | cut -d= -f2- | tr -d '"'"'"' \r')"
if [[ -z "${PROM_VERSION}" ]]; then
    h_fail "PROMETHEUS_VERSION unset in deploy/.env.example — refusing to fall back to :latest"
    h_finish
fi
h_pass "PROMETHEUS_VERSION=${PROM_VERSION} (single source of truth)"

h_log "Pre-pulling prom/prometheus:${PROM_VERSION} (first run may take a minute)..."
docker pull "prom/prometheus:${PROM_VERSION}" >> "${H_EVIDENCE_FILE}" 2>&1 || true

# ── T24.1 — recording rules ──────────────────────────────────────────────────
h_task T24.1

h_run "promtool check rules recording.yml" -- \
    m24_promtool check rules /p/rules/recording.yml

h_run "promtool test rules recording_test.yml" -- \
    m24_promtool test rules /p/rules/recording_test.yml

# 8 recorded series are specified (rate, errors, ratio, partials, p50/p95/p99).
m24_assert_min_count "${RULES_DIR}/recording.yml" "  - record:" 7 \
    "recording.yml defines the specified recorded series"
# Counts individual assertions, not test blocks: 3 promql_expr_test blocks
# hold 6 exp_samples assertions between them, and it is the assertion count
# that says whether the suite is vacuous.
m24_assert_min_count "${RULES_DIR}/recording_test.yml" "exp_samples:" 6 \
    "recording_test.yml exercises the recorded series (not vacuous)"

# ── T24.2 — alert rules ──────────────────────────────────────────────────────
h_task T24.2

h_run "promtool check rules alerts.yml" -- \
    m24_promtool check rules /p/rules/alerts.yml

h_run "promtool test rules alerts_test.yml" -- \
    m24_promtool test rules /p/rules/alerts_test.yml

m24_assert_min_count "${RULES_DIR}/alerts.yml" "  - alert:" 4 \
    "alerts.yml defines all four PLAN.md alerts"
# Four alerts x (positive + negative) = 8 alert_rule_test blocks minimum.
m24_assert_min_count "${RULES_DIR}/alerts_test.yml" "alertname:" 8 \
    "alerts_test.yml has a positive AND negative case per alert"

# A positive case per alert is what catches a recording rule renamed in
# recording.yml but not in alerts.yml — that defect leaves an alert that
# silently never fires, and a negative-only test suite would still pass.
for alert in RestitchHighP95Latency RestitchHighErrorRate \
             RestitchConfigReloadFailing RestitchGatewayDown; do
    if grep -q "alertname: ${alert}" "${RULES_DIR}/alerts_test.yml" 2>/dev/null; then
        h_pass "alerts_test.yml covers ${alert}"
    else
        h_fail "alerts_test.yml has no test for ${alert}"
    fi
done

# ── T24.3 — k6 load test ─────────────────────────────────────────────────────
h_task T24.3

M24_RATE="${M24_RATE:-1000}"
M24_DURATION_SECS="${M24_DURATION_SECS:-60}"
M24_P95_MS="${M24_P95_MS:-1000}"
M24_ERR_RATE="${M24_ERR_RATE:-0.01}"

if [[ ! -f "${REPO_ROOT}/tests/loadtest/m24_baseline.js" ]]; then
    h_fail "tests/loadtest/m24_baseline.js missing"
else
    h_start_mockupstream
    h_start_studio

    M24_CONFIG="$(h_config m24_load <<'YAML'
server:
  port: @GW_PORT@
admin:
  enabled: true
  port: @ADMIN_PORT@
upstreams:
  mock:
    url: "http://127.0.0.1:@MOCK_PORT@"
    timeout: 10s
    transport:
      max_idle_conns_per_host: 100
compositions:
  loadtest:
    path: "/loadtest"
    method: GET
    steps:
      - name: s1
        upstream: mock
        path: "/users/1"
    response:
      body:
        name: "{{ steps.s1.body.name }}"
YAML
)"
    h_start_gateway "${M24_CONFIG}"

    M24_SUMMARY_OUT="${H_TMP}/m24_summary.json"
    h_log "Running k6: rate=${M24_RATE}/s duration=${M24_DURATION_SECS}s (this takes ~${M24_DURATION_SECS}s)..."
    RATE="${M24_RATE}" \
    DURATION="${M24_DURATION_SECS}s" \
    P95_MS="${M24_P95_MS}" \
    ERR_RATE="${M24_ERR_RATE}" \
    GW_URL="http://127.0.0.1:${GW_PORT}/loadtest" \
    STUDIO_URL="http://127.0.0.1:${STUDIO_PORT}/api/v1/configs" \
    SUMMARY_OUT="${M24_SUMMARY_OUT}" \
        k6 run "${REPO_ROOT}/tests/loadtest/m24_baseline.js" \
        >> "${H_TMP}/k6_m24.log" 2>&1 || true

    h_evidence "--- tail of k6 output ---"
    h_evidence "$(tail -30 "${H_TMP}/k6_m24.log" 2>/dev/null || echo '(no k6 log)')"

    if [[ ! -f "${M24_SUMMARY_OUT}" ]]; then
        h_fail "k6 produced no summary at ${M24_SUMMARY_OUT} — handleSummary missing or k6 aborted"
    else
        h_evidence "--- k6 summary ---"
        h_evidence "$(cat "${M24_SUMMARY_OUT}")"

        read -r K6_P95 K6_ERR K6_REQS <<EOF
$(python3 - "${M24_SUMMARY_OUT}" <<'PY'
import json, sys
with open(sys.argv[1]) as fh:
    s = json.load(fh)["compositions"]
print(f'{s["p95_ms"]:.3f} {s["error_rate"]:.6f} {int(s["reqs"])}')
PY
)
EOF
        h_evidence "parsed: p95_ms=${K6_P95} error_rate=${K6_ERR} reqs=${K6_REQS}"

        # Magnitude floor: a k6 run that connected but drove almost no traffic
        # would otherwise satisfy the latency and error thresholds trivially.
        M24_MIN_REQS=$(python3 -c "print(int(${M24_RATE} * ${M24_DURATION_SECS} * 0.95))")
        if [[ "${K6_REQS}" -ge "${M24_MIN_REQS}" ]]; then
            h_pass "k6 drove ${K6_REQS} requests (>= 95% of ${M24_RATE}x${M24_DURATION_SECS} = ${M24_MIN_REQS})"
        else
            h_fail "k6 drove only ${K6_REQS} requests (< ${M24_MIN_REQS}) — offered load not achieved"
        fi

        if python3 -c "import sys; sys.exit(0 if ${K6_P95} < ${M24_P95_MS} else 1)"; then
            h_pass "compositions p95 ${K6_P95}ms < ${M24_P95_MS}ms"
        else
            h_fail "compositions p95 ${K6_P95}ms >= ${M24_P95_MS}ms"
        fi

        if python3 -c "import sys; sys.exit(0 if ${K6_ERR} < ${M24_ERR_RATE} else 1)"; then
            h_pass "compositions error rate ${K6_ERR} < ${M24_ERR_RATE}"
        else
            h_fail "compositions error rate ${K6_ERR} >= ${M24_ERR_RATE}"
        fi
    fi
fi

# ── T24.4 — deploy stack smoke ───────────────────────────────────────────────
h_task T24.4

if [[ ! -f "${DEPLOY_DIR}/docker-compose.yml" ]]; then
    h_fail "deploy/docker-compose.yml missing"
else
    # Ephemeral host ports so a gate run cannot collide with a stack the
    # developer already has running locally.
    C_GW_PORT="$(h_free_port)"
    C_ADMIN_PORT="$(h_free_port)"
    C_STUDIO_PORT="$(h_free_port)"
    C_PROM_PORT="$(h_free_port)"
    C_JAEGER_PORT="$(h_free_port)"

    {
        cat "${ENV_FILE}"
        echo "GATEWAY_PORT=${C_GW_PORT}"
        echo "GATEWAY_ADMIN_PORT=${C_ADMIN_PORT}"
        echo "STUDIO_PORT=${C_STUDIO_PORT}"
        echo "PROMETHEUS_PORT=${C_PROM_PORT}"
        echo "JAEGER_UI_PORT=${C_JAEGER_PORT}"
    } > "${H_TMP}/gate.env"
    h_evidence "--- gate compose env ---"
    h_evidence "$(cat "${H_TMP}/gate.env")"

    h_log "Building and starting the deploy stack (first build runs npm ci + two Go builds; expect several minutes)..."
    M24_COMPOSE_UP=true
    if (cd "${DEPLOY_DIR}" && docker compose -p "${COMPOSE_PROJECT}" \
            --env-file "${H_TMP}/gate.env" up -d --build) \
            >> "${H_EVIDENCE_FILE}" 2>&1; then
        h_pass "deploy stack started"

        PROM_API="http://127.0.0.1:${C_PROM_PORT}/api/v1"

        # Prometheus loaded the rule files, and loaded them without error.
        # Asserting only HTTP 200 would pass against a Prometheus that read
        # no rule files at all.
        if m24_wait_json "${PROM_API}/rules" \
            'len(data["data"]["groups"]) >= 2' 90; then
            h_pass "Prometheus reports >= 2 rule groups loaded"
        else
            h_fail "Prometheus never reported 2+ rule groups"
        fi

        if m24_wait_json "${PROM_API}/rules" \
            'sum(len(g["rules"]) for g in data["data"]["groups"]) >= 11' 30; then
            h_pass "Prometheus loaded >= 11 rules (7 recording + 4 alerting)"
        else
            h_fail "Prometheus rule count below the 11 specified"
        fi

        if m24_wait_json "${PROM_API}/rules" \
            'all(r.get("health") == "ok" for g in data["data"]["groups"] for r in g["rules"])' 30; then
            h_pass "every loaded rule reports health=ok"
        else
            h_fail "at least one rule reports a non-ok health state"
        fi

        # Prometheus being healthy proves nothing about whether it can reach
        # the gateway. Assert the scrape target is actually up.
        if m24_wait_json "${PROM_API}/query?query=up%7Bjob%3D%22restitch-gateway%22%7D" \
            'any(float(r["value"][1]) == 1 for r in data["data"]["result"])' 120; then
            h_pass "up{job=\"restitch-gateway\"} == 1 — Prometheus is scraping the gateway"
        else
            h_fail "Prometheus is not scraping the gateway (up != 1)"
        fi
    else
        h_fail "docker compose up failed — see evidence log"
    fi

    h_log "Tearing down the deploy stack..."
    (cd "${DEPLOY_DIR}" && docker compose -p "${COMPOSE_PROJECT}" \
        --env-file "${H_TMP}/gate.env" down -v --remove-orphans) \
        >> "${H_EVIDENCE_FILE}" 2>&1 || true
    M24_COMPOSE_UP=false
fi

# ── M24.unit — the promtool suites are this milestone's test suite ───────────
# M24 adds no Go code, so `make ci` is untouched and the promtool unit tests
# ARE the tests. Re-running them under their own task ID makes that explicit
# in the ledger rather than leaving M24 with no unit-test row.
h_task M24.unit

h_run "promtool test rules (all suites)" -- \
    m24_promtool test rules /p/rules/recording_test.yml /p/rules/alerts_test.yml

h_finish
```

- [ ] **Step 3: Make it executable and syntax-check it**

```bash
chmod +x scripts/gates/m24.sh
bash -n scripts/gates/m24.sh && echo "SYNTAX OK"
```

Expected: `SYNTAX OK`

- [ ] **Step 4: Run the gate and confirm it fails for the right reasons**

Run: `H_NO_EVIDENCE=true scripts/gates/m24.sh 2>&1 | tail -25`

Expected: `RESULT M24: FAIL`, with failures naming the **missing artefacts** — `deploy/.env.example missing`. It must NOT fail on a bash error, an unbound variable, or a harness misuse. `H_NO_EVIDENCE=true` keeps this diagnostic run out of `LEDGER.md`.

- [ ] **Step 5: Confirm no forbidden file was touched**

Run: `git status --porcelain`

Expected: exactly one modified path, `scripts/gates/m24.sh`. If `scripts/lib/`, `scripts/verify.sh`, or `scripts/check-ledger.sh` appear, revert them — that is a hard-rule violation.

- [ ] **Step 6: STOP — present the script to the user for approval**

Show the user the full script and the Step 4 output. State plainly: the gate currently fails because the artefacts do not exist yet, which is the intended pre-implementation state. **Do not proceed to Task 2 until the user approves.**

- [ ] **Step 7: Commit (only after approval)**

```bash
git add scripts/gates/m24.sh
git commit -m "gate: implement M24 gate — monitoring rules, k6 load test, deploy stack

Replaces the placeholder with the real gate encoding the PLAN.md M24
verification gate. Four phases: promtool check+test on recording and alert
rules (hermetic), k6 against a live gateway, and one compose smoke asserting
Prometheus loaded the rules and is scraping the gateway.

Approved by user before commit, per CLAUDE.md hard rules.

Pre-implementation run (expected FAIL — artefacts do not exist yet):
<paste the real tail of Step 4 output here>"
```

---

## Task 2: T24.1 — Recording rules

**Files:**
- Create: `deploy/.env.example`
- Create: `deploy/prometheus/rules/recording.yml`
- Create: `deploy/prometheus/rules/recording_test.yml`

**Interfaces:**
- Consumes: the gate's expectations from Task 1 — `PROMETHEUS_VERSION` in `deploy/.env.example`; ≥7 `  - record:` lines; ≥6 `exp_samples:` assertions.
- Produces: recorded series names that Task 3's alerts consume verbatim — `composition:restitch_request_duration_seconds:p95`, `composition:restitch_requests:error_ratio5m`.

`deploy/.env.example` is created here rather than in the deploy-stack task because the gate reads `PROMETHEUS_VERSION` from it during the very first phase. It is the pin file, and it is complete from the start.

- [ ] **Step 1: Create `deploy/.env.example`**

```bash
# Image version pins. Single source of truth: docker-compose.yml AND
# scripts/gates/m24.sh read PROMETHEUS_VERSION from this file. Never add a
# second copy of the version, and never fall back to :latest — the gate
# validates rules with exactly the Prometheus that loads them.
PROMETHEUS_VERSION=v3.13.1
JAEGER_VERSION=1.70.0
RESTITCH_IMAGE_TAG=dev

# Host port bindings. Prometheus is on 9091, NOT its conventional 9090,
# because the gateway admin/metrics server already owns 9090.
GATEWAY_PORT=8080
GATEWAY_ADMIN_PORT=9090
STUDIO_PORT=3080
PROMETHEUS_PORT=9091
JAEGER_UI_PORT=16686

# Set to a non-empty value to require X-Admin-Key on the gateway admin API.
# /metrics and /health stay unauthenticated either way, so Prometheus does
# not need this secret.
RESTITCH_ADMIN_API_KEY=

# Tracing. Unset disables the exporter entirely.
OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4318
```

- [ ] **Step 2: Create `deploy/prometheus/rules/recording.yml`**

```yaml
# T24.1 — pre-computed dashboard queries.
#
# Naming follows the Prometheus convention level:metric:operations.
#
# Partial responses are recorded SEPARATELY and are deliberately NOT folded
# into error_ratio5m. A partial response is the gateway's signature degraded
# state and returns a success status, so a pure 5xx ratio would show a
# composition as healthy while half its steps fail. Conflating the two would
# make the T24.2 error-rate alert fire on a condition operators routinely
# accept.
groups:
  - name: restitch_composition
    interval: 30s
    rules:
      - record: composition:restitch_requests:rate5m
        expr: sum by (composition) (rate(restitch_requests_total[5m]))

      - record: composition:restitch_requests_errors:rate5m
        expr: sum by (composition) (rate(restitch_requests_total{status=~"5.."}[5m]))

      # No `or vector(0)` fallback, on purpose. A composition that has served
      # zero 5xx produces no series from the numerator, so the ratio is ABSENT
      # rather than 0. Absent means nothing to alert on. A zero-fill would
      # manufacture a series for every composition forever and defeat
      # absent()-style checks.
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

- [ ] **Step 3: Create `deploy/prometheus/rules/recording_test.yml`**

`restitch_requests_total` carries `composition`, `method`, `status` (verified: `internal/composition/handler.go:256`), and `status` is the full numeric code as a string (`observability.StatusStr` → `strconv.Itoa`, `internal/observability/metrics.go:140`).

```yaml
# Unit tests for recording.yml. `promtool test rules` feeds these synthetic
# series into the real rule expressions and asserts the resulting values at
# exact timestamps.
rule_files:
  - recording.yml

evaluation_interval: 1m

tests:
  # 54 successes/min + 6 errors/min for "orders" => 10% error ratio.
  - interval: 1m
    input_series:
      - series: 'restitch_requests_total{composition="orders",method="GET",status="200"}'
        values: '0+54x10'
      - series: 'restitch_requests_total{composition="orders",method="GET",status="500"}'
        values: '0+6x10'
    promql_expr_test:
      - expr: composition:restitch_requests:rate5m
        eval_time: 10m
        exp_samples:
          - labels: 'composition:restitch_requests:rate5m{composition="orders"}'
            value: 1
      - expr: composition:restitch_requests_errors:rate5m
        eval_time: 10m
        exp_samples:
          - labels: 'composition:restitch_requests_errors:rate5m{composition="orders"}'
            value: 1.0E-1
      - expr: composition:restitch_requests:error_ratio5m
        eval_time: 10m
        exp_samples:
          - labels: 'composition:restitch_requests:error_ratio5m{composition="orders"}'
            value: 1.0E-1

  # A composition with zero 5xx must produce NO error_ratio series at all.
  # This is the test that would catch someone "helpfully" adding or vector(0).
  - interval: 1m
    input_series:
      - series: 'restitch_requests_total{composition="clean",method="GET",status="200"}'
        values: '0+60x10'
    promql_expr_test:
      - expr: composition:restitch_requests:error_ratio5m
        eval_time: 10m
        exp_samples: []

  # Partial responses are recorded on their own series and must never appear
  # in the error ratio.
  - interval: 1m
    input_series:
      - series: 'restitch_requests_total{composition="degraded",method="GET",status="200"}'
        values: '0+60x10'
      - series: 'restitch_partial_responses_total{composition="degraded"}'
        values: '0+30x10'
    promql_expr_test:
      - expr: composition:restitch_partial_responses:rate5m
        eval_time: 10m
        exp_samples:
          - labels: 'composition:restitch_partial_responses:rate5m{composition="degraded"}'
            value: 5.0E-1
      - expr: composition:restitch_requests:error_ratio5m
        eval_time: 10m
        exp_samples: []
```

- [ ] **Step 4: Run the rule checks directly and verify they pass**

```bash
PROM_VERSION=$(grep -E '^PROMETHEUS_VERSION=' deploy/.env.example | cut -d= -f2)
docker run --rm -v "$PWD/deploy/prometheus:/p:ro" --entrypoint=/bin/promtool \
  "prom/prometheus:${PROM_VERSION}" check rules /p/rules/recording.yml
docker run --rm -v "$PWD/deploy/prometheus:/p:ro" --entrypoint=/bin/promtool \
  "prom/prometheus:${PROM_VERSION}" test rules /p/rules/recording_test.yml
```

Expected: `SUCCESS` from both. If a `value:` assertion is off, read promtool's reported actual value and correct the **test's expected number** — do not loosen a rule to match a wrong expectation.

- [ ] **Step 5: Accept — run the gate's T24.1 phase**

Run: `H_NO_EVIDENCE=true scripts/gates/m24.sh 2>&1 | sed -n '/T24.1/,/T24.2/p'`

Expected: four `PASS` lines for T24.1 — `promtool check rules recording.yml`, `promtool test rules recording_test.yml`, and the two non-vacuity count assertions.

- [ ] **Step 6: Append the ledger row**

Append to `docs/plan-progress/LEDGER.md` (never edit existing rows):

```markdown
| 2026-07-27 | T24.1 | M24 | PASS | <short-sha> | evidence/<file>#T24.1 | recording rules + promtool unit tests |
```

- [ ] **Step 7: Commit**

```bash
git add deploy/.env.example deploy/prometheus/rules/recording.yml \
        deploy/prometheus/rules/recording_test.yml docs/plan-progress/LEDGER.md
git commit -m "feat(M24): add Prometheus recording rules with promtool unit tests

Seven recorded series: request rate, error rate, error ratio, partial-response
rate, and p50/p95/p99 latency, all by composition.

Partial responses are recorded separately rather than folded into the error
ratio — a partial returns a success status, so mixing them would make the
T24.2 error-rate alert fire on an accepted degraded state.

Accept output:
<paste the real Step 5 output here>"
```

---

## Task 3: T24.2 — Alert rules

**Files:**
- Create: `deploy/prometheus/rules/alerts.yml`
- Create: `deploy/prometheus/rules/alerts_test.yml`

**Interfaces:**
- Consumes: recorded series from Task 2 — `composition:restitch_request_duration_seconds:p95`, `composition:restitch_requests:error_ratio5m`. Names must match **exactly**; a mismatch produces an alert that silently never fires.
- Produces: alert names the gate greps for by name — `RestitchHighP95Latency`, `RestitchHighErrorRate`, `RestitchConfigReloadFailing`, `RestitchGatewayDown`.

- [ ] **Step 1: Create `deploy/prometheus/rules/alerts.yml`**

```yaml
# T24.2 — alert rules.
#
# There is deliberately no Alertmanager in the deploy stack (spec decision D4).
# A demo Alertmanager whose receiver discards every notification proves nothing.
# deploy/prometheus/prometheus.yml carries a commented alerting: block marking
# exactly where a real Alertmanager target goes.
groups:
  - name: restitch_alerts
    rules:
      # TUNING KNOB: the 1-second P95 threshold. Prometheus rule files have no
      # variable substitution, so this literal IS the configuration point.
      # Documented in deploy/README.md.
      - alert: RestitchHighP95Latency
        expr: composition:restitch_request_duration_seconds:p95 > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "P95 latency above 1s for composition {{ $labels.composition }}"
          description: "P95 is {{ $value | humanizeDuration }} over the last 5m."

      - alert: RestitchHighErrorRate
        expr: composition:restitch_requests:error_ratio5m > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "5xx rate above 5% for composition {{ $labels.composition }}"
          description: "Error ratio is {{ $value | humanizePercentage }} over the last 5m."

      # A 10m `for` is deliberate. M21's poller retries with exponential
      # backoff, so a single transient fetch failure is normal self-healing
      # behaviour. Alerting at 5m would page on something the system is
      # designed to absorb.
      - alert: RestitchConfigReloadFailing
        expr: rate(restitch_registry_polls_total{result!="success"}[5m]) > 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Gateway registry polls have been failing for 10m"
          description: "Failing poll rate is {{ $value }}/s. The gateway keeps serving its last known good config."

      - alert: RestitchGatewayDown
        expr: up{job="restitch-gateway"} == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Gateway target {{ $labels.instance }} is down"
          description: "Prometheus has failed to scrape the gateway for 2m."
```

- [ ] **Step 2: Create `deploy/prometheus/rules/alerts_test.yml`**

Every alert gets a **positive and a negative** case. The positive cases are load-bearing: they are what fails if a recorded series is renamed in `recording.yml` but not here.

```yaml
# Unit tests for alerts.yml. recording.yml is loaded too because three of the
# four alerts are expressed over recorded series — that coupling is exactly
# what these tests exist to protect.
rule_files:
  - recording.yml
  - alerts.yml

evaluation_interval: 1m

tests:
  # --- RestitchHighErrorRate: POSITIVE (10% > 5% threshold) ---
  - interval: 1m
    input_series:
      - series: 'restitch_requests_total{composition="orders",method="GET",status="200"}'
        values: '0+54x20'
      - series: 'restitch_requests_total{composition="orders",method="GET",status="500"}'
        values: '0+6x20'
    alert_rule_test:
      - eval_time: 15m
        alertname: RestitchHighErrorRate
        exp_alerts:
          - exp_labels:
              severity: critical
              composition: orders
            # promtool compares annotations too: omitting exp_annotations
            # asserts they are EMPTY, which fails against any alert that has
            # them. The rendered text is asserted verbatim, so an accidental
            # edit to a summary/description is caught here.
            exp_annotations:
              summary: "5xx rate above 5% for composition orders"
              description: "Error ratio is 10% over the last 5m."

  # --- RestitchHighErrorRate: NEGATIVE (1% < 5% threshold) ---
  - interval: 1m
    input_series:
      - series: 'restitch_requests_total{composition="orders",method="GET",status="200"}'
        values: '0+99x20'
      - series: 'restitch_requests_total{composition="orders",method="GET",status="500"}'
        values: '0+1x20'
    alert_rule_test:
      - eval_time: 15m
        alertname: RestitchHighErrorRate
        exp_alerts: []

  # --- RestitchHighP95Latency: POSITIVE (all observations in the +Inf bucket
  #     above 1s => p95 > 1) ---
  - interval: 1m
    input_series:
      - series: 'restitch_request_duration_seconds_bucket{composition="slow",le="0.5"}'
        values: '0+0x20'
      - series: 'restitch_request_duration_seconds_bucket{composition="slow",le="1"}'
        values: '0+0x20'
      - series: 'restitch_request_duration_seconds_bucket{composition="slow",le="2.5"}'
        values: '0+60x20'
      - series: 'restitch_request_duration_seconds_bucket{composition="slow",le="+Inf"}'
        values: '0+60x20'
    alert_rule_test:
      - eval_time: 15m
        alertname: RestitchHighP95Latency
        exp_alerts:
          - exp_labels:
              severity: warning
              composition: slow
            # 2.425s is the interpolated p95 for these buckets — verified by
            # running promtool, not estimated.
            exp_annotations:
              summary: "P95 latency above 1s for composition slow"
              description: "P95 is 2.425s over the last 5m."

  # --- RestitchHighP95Latency: NEGATIVE (everything under 0.5s) ---
  - interval: 1m
    input_series:
      - series: 'restitch_request_duration_seconds_bucket{composition="fast",le="0.5"}'
        values: '0+60x20'
      - series: 'restitch_request_duration_seconds_bucket{composition="fast",le="1"}'
        values: '0+60x20'
      - series: 'restitch_request_duration_seconds_bucket{composition="fast",le="2.5"}'
        values: '0+60x20'
      - series: 'restitch_request_duration_seconds_bucket{composition="fast",le="+Inf"}'
        values: '0+60x20'
    alert_rule_test:
      - eval_time: 15m
        alertname: RestitchHighP95Latency
        exp_alerts: []

  # --- RestitchConfigReloadFailing: POSITIVE (sustained failures past 10m) ---
  - interval: 1m
    input_series:
      - series: 'restitch_registry_polls_total{result="error"}'
        values: '0+6x20'
    alert_rule_test:
      - eval_time: 15m
        alertname: RestitchConfigReloadFailing
        exp_alerts:
          - exp_labels:
              severity: warning
              result: error
            exp_annotations:
              summary: "Gateway registry polls have been failing for 10m"
              description: "Failing poll rate is 0.1/s. The gateway keeps serving its last known good config."

  # --- RestitchConfigReloadFailing: NEGATIVE (only successful polls) ---
  - interval: 1m
    input_series:
      - series: 'restitch_registry_polls_total{result="success"}'
        values: '0+6x20'
    alert_rule_test:
      - eval_time: 15m
        alertname: RestitchConfigReloadFailing
        exp_alerts: []

  # --- RestitchGatewayDown: POSITIVE ---
  - interval: 1m
    input_series:
      - series: 'up{job="restitch-gateway",instance="gateway:9090"}'
        values: '0 0 0 0 0 0 0 0 0 0'
    alert_rule_test:
      - eval_time: 5m
        alertname: RestitchGatewayDown
        exp_alerts:
          - exp_labels:
              severity: critical
              job: restitch-gateway
              instance: gateway:9090
            exp_annotations:
              summary: "Gateway target gateway:9090 is down"
              description: "Prometheus has failed to scrape the gateway for 2m."

  # --- RestitchGatewayDown: NEGATIVE ---
  - interval: 1m
    input_series:
      - series: 'up{job="restitch-gateway",instance="gateway:9090"}'
        values: '1 1 1 1 1 1 1 1 1 1'
    alert_rule_test:
      - eval_time: 5m
        alertname: RestitchGatewayDown
        exp_alerts: []
```

Note on the `RestitchGatewayDown` positive case: the series must be `0` for the whole window. If promtool rejects the `0ffff0` shorthand, replace that `values:` line with the plain form `'0 0 0 0 0 0 0 0 0 0'` — both express the same thing.

- [ ] **Step 3: Run the checks directly**

```bash
PROM_VERSION=$(grep -E '^PROMETHEUS_VERSION=' deploy/.env.example | cut -d= -f2)
docker run --rm -v "$PWD/deploy/prometheus:/p:ro" --entrypoint=/bin/promtool \
  "prom/prometheus:${PROM_VERSION}" check rules /p/rules/alerts.yml
docker run --rm -v "$PWD/deploy/prometheus:/p:ro" --entrypoint=/bin/promtool \
  "prom/prometheus:${PROM_VERSION}" test rules /p/rules/alerts_test.yml
```

Expected: `SUCCESS` from both.

- [ ] **Step 4: Prove the positive tests actually bite (mutation check)**

Temporarily rename the recorded series in `alerts.yml` — change `composition:restitch_requests:error_ratio5m` to `composition:restitch_requests:error_ratio5m_TYPO` — then re-run the test command from Step 3.

Expected: **FAILURE**, reporting that `RestitchHighErrorRate` did not fire. This is the whole point of the positive cases. **Revert the typo immediately** and re-run to confirm `SUCCESS` again.

- [ ] **Step 5: Accept — run the gate's T24.2 phase**

Run: `H_NO_EVIDENCE=true scripts/gates/m24.sh 2>&1 | sed -n '/T24.2/,/T24.3/p'`

Expected: `PASS` for `promtool check rules alerts.yml`, `promtool test rules alerts_test.yml`, both count assertions, and one `PASS` per alert name coverage check (four lines).

- [ ] **Step 6: Append the ledger row**

```markdown
| 2026-07-27 | T24.2 | M24 | PASS | <short-sha> | evidence/<file>#T24.2 | 4 alerts, positive+negative promtool cases each |
```

- [ ] **Step 7: Commit**

```bash
git add deploy/prometheus/rules/alerts.yml deploy/prometheus/rules/alerts_test.yml \
        docs/plan-progress/LEDGER.md
git commit -m "feat(M24): add Prometheus alert rules with positive/negative unit tests

Four alerts per PLAN.md: P95 latency, error rate, config reload failures, and
gateway down. Each has a positive and a negative promtool case; the positive
cases are what catch a recorded series renamed in recording.yml but not here,
which would otherwise leave an alert that silently never fires.

RestitchConfigReloadFailing uses a 10m 'for' because M21's poller retries with
exponential backoff — a 5m window would page on self-healing behaviour.

Accept output:
<paste the real Step 5 output here>"
```

---

## Task 4: T24.3a — k6 load test script

**Files:**
- Create: `tests/loadtest/m24_baseline.js`

**Interfaces:**
- Consumes from the gate (Task 1): env vars `RATE`, `DURATION`, `P95_MS`, `ERR_RATE`, `GW_URL`, `STUDIO_URL`, `SUMMARY_OUT`.
- Produces: a JSON file at `$SUMMARY_OUT` with the exact shape `{"compositions": {"p95_ms": <number>, "error_rate": <number>, "reqs": <number>}}`. The gate parses these three keys by name — changing them breaks the gate.

- [ ] **Step 1: Create `tests/loadtest/m24_baseline.js`**

```js
// M24 baseline load scenario.
//
// PLAN.md phrases the target as "~1000 RPS (50 VUs)". Those are different
// things in k6: 50 VUs in an open loop produce whatever throughput the system
// allows, so if the gateway slows down the offered load drops with it and the
// test quietly stops measuring what it was built to measure.
// constant-arrival-rate holds the request rate fixed and treats VUs as a pool.
//
// Every knob is an env var so one script serves both the gate (full profile)
// and CI (reduced profile on a shared runner).
import http from 'k6/http';
import { check } from 'k6';

const GW_URL = __ENV.GW_URL;
const STUDIO_URL = __ENV.STUDIO_URL;
const DURATION = __ENV.DURATION || '60s';

if (!GW_URL) {
  throw new Error('GW_URL is required');
}

export const options = {
  scenarios: {
    compositions: {
      executor: 'constant-arrival-rate',
      exec: 'composition',
      rate: Number(__ENV.RATE || 1000),
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: Number(__ENV.VUS || 50),
      maxVUs: Number(__ENV.MAX_VUS || 200),
    },
  },
  thresholds: {
    // Sub-metric thresholds also CREATE the sub-metrics, which is what makes
    // the per-scenario keys available in handleSummary below.
    'http_req_duration{scenario:compositions}': [`p(95)<${__ENV.P95_MS || 1000}`],
    'http_req_failed{scenario:compositions}': [`rate<${__ENV.ERR_RATE || 0.01}`],
    'http_reqs{scenario:compositions}': ['count>0'],
  },
};

// Studio is the CONTROL plane — config CRUD and registry bundles. Driving it
// at 1000 RPS would measure SQLite write contention rather than anything an
// operator experiences, and would dominate the failure signal. 5 req/s proves
// it stays responsive while the data plane is saturated.
//
// The scenario is conditional because constant-arrival-rate rejects rate: 0,
// so it cannot be switched off by zeroing the rate. The gate sets STUDIO_URL;
// the gateway-only CI job leaves it unset.
if (STUDIO_URL) {
  options.scenarios.studio_api = {
    executor: 'constant-arrival-rate',
    exec: 'studio',
    rate: Number(__ENV.STUDIO_RATE || 5),
    timeUnit: '1s',
    duration: DURATION,
    preAllocatedVUs: 2,
    maxVUs: 10,
  };
  // These thresholds exist primarily to MATERIALISE the studio sub-metrics.
  // k6 only creates a sub-metric when a threshold references it, so without
  // these the handleSummary lookups below fall back to -1/0 and the evidence
  // reads as though the studio scenario served nothing. The p95 bound is
  // deliberately generous — this is a liveness check on the control plane,
  // not a latency SLO, and a tight bound here would be a flake source.
  options.thresholds['http_reqs{scenario:studio_api}'] = ['count>0'];
  options.thresholds['http_req_duration{scenario:studio_api}'] = [
    `p(95)<${__ENV.STUDIO_P95_MS || 5000}`,
  ];
}

export function composition() {
  const res = http.get(GW_URL);
  check(res, { 'composition 200': (r) => r.status === 200 });
}

export function studio() {
  const res = http.get(STUDIO_URL);
  check(res, { 'studio 200': (r) => r.status === 200 });
}

// Write a normalised summary the gate parses by key name. Deriving this
// ourselves rather than using --summary-export means the gate never has to
// guess how k6 spells its per-scenario sub-metric keys.
export function handleSummary(data) {
  const m = data.metrics;
  const pick = (name, field, fallback) => {
    const metric = m[name];
    if (!metric || !metric.values || metric.values[field] === undefined) {
      return fallback;
    }
    return metric.values[field];
  };

  const out = {
    compositions: {
      p95_ms: pick('http_req_duration{scenario:compositions}', 'p(95)', -1),
      error_rate: pick('http_req_failed{scenario:compositions}', 'rate', 1),
      reqs: pick('http_reqs{scenario:compositions}', 'count', 0),
    },
  };

  if (STUDIO_URL) {
    out.studio = {
      p95_ms: pick('http_req_duration{scenario:studio_api}', 'p(95)', -1),
      reqs: pick('http_reqs{scenario:studio_api}', 'count', 0),
    };
  }

  const path = __ENV.SUMMARY_OUT || 'm24_summary.json';
  const result = {};
  result[path] = JSON.stringify(out, null, 2);
  result.stdout = `\nM24 summary: ${JSON.stringify(out)}\n`;
  return result;
}
```

The fallbacks are deliberately *failing* values (`p95_ms: -1` would pass a `<` check, so the gate's separate `reqs` magnitude floor is what catches a broken run; `error_rate: 1` fails outright). Never change a fallback to a passing value.

- [ ] **Step 2: Smoke the script against a hand-started gateway at low rate**

```bash
make build && go build -o bin/mockupstream ./cmd/mockupstream
./bin/mockupstream -port 18081 &
cat > /tmp/m24smoke.yaml <<'YAML'
upstreams:
  mock:
    url: "http://127.0.0.1:18081"
    timeout: 10s
compositions:
  loadtest:
    path: "/loadtest"
    method: GET
    steps:
      - name: s1
        upstream: mock
        path: "/users/1"
    response:
      body:
        name: "{{ steps.s1.body.name }}"
YAML
./bin/restitch run -config /tmp/m24smoke.yaml -port 18080 &
sleep 2
RATE=50 DURATION=5s GW_URL=http://127.0.0.1:18080/loadtest \
  SUMMARY_OUT=/tmp/m24_smoke.json k6 run tests/loadtest/m24_baseline.js
cat /tmp/m24_smoke.json
kill %1 %2
```

Expected (this exact command was run at planning time against k6 v2.1.0):

```
M24 summary: {"compositions":{"p95_ms":1.3239999999999998,"error_rate":0,"reqs":251}}
```

`reqs` ≈ 251 (50/s × 5s), `error_rate` 0, and a real sub-millisecond `p95_ms`. If `reqs` is `0` or `p95_ms` is `-1`, the sub-metric key names are wrong for your k6 version — print `Object.keys(data.metrics)` in `handleSummary` to find the actual spelling and correct the `pick()` calls.

- [ ] **Step 3: Accept — run the gate's T24.3 phase**

Run: `H_NO_EVIDENCE=true scripts/gates/m24.sh 2>&1 | sed -n '/T24.3/,/T24.4/p'`

Expected: three `PASS` lines — the request-count magnitude floor, the p95 threshold, and the error-rate threshold. This phase takes ~60s plus build time.

- [ ] **Step 4: Append the ledger row**

```markdown
| 2026-07-27 | T24.3 | M24 | PASS | <short-sha> | evidence/<file>#T24.3 | k6 constant-arrival-rate, gate profile 1000/s |
```

- [ ] **Step 5: Commit**

```bash
git add tests/loadtest/m24_baseline.js docs/plan-progress/LEDGER.md
git commit -m "feat(M24): add parameterised k6 baseline load test

Uses constant-arrival-rate rather than an open VU loop so offered load stays
fixed when the gateway slows down. Studio runs as a separate low-rate scenario
(5 req/s) because it is the control plane; the scenario is conditional on
STUDIO_URL since constant-arrival-rate rejects rate: 0.

handleSummary writes a normalised JSON summary so the gate parses stable key
names instead of guessing k6's sub-metric spelling.

Accept output:
<paste the real Step 3 output here>"
```

---

## Task 5: T24.4 — Deploy stack

**Files:**
- Create: `deploy/prometheus/prometheus.yml`
- Create: `deploy/docker-compose.yml`
- Create: `deploy/README.md`

**Interfaces:**
- Consumes: `deploy/.env.example` (Task 2) for every version pin and host port; the rule files from Tasks 2–3.
- Produces: the running stack the gate's T24.4 phase asserts against — Prometheus on `${PROMETHEUS_PORT}` with a scrape job literally named `restitch-gateway`.

The job name **must** be `restitch-gateway`: `alerts.yml` matches `up{job="restitch-gateway"}` and the gate queries the same selector.

- [ ] **Step 1: Create `deploy/prometheus/prometheus.yml`**

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

# Rule files are listed EXPLICITLY rather than globbed so the *_test.yml unit
# test files living in the same directory are never loaded by the running
# server. They are promtool inputs, not rules.
rule_files:
  - /etc/prometheus/rules/recording.yml
  - /etc/prometheus/rules/alerts.yml

# No Alertmanager ships with this stack (spec decision D4): a demo receiver
# that discards every notification proves nothing and is one more service to
# keep healthy. Point this at your own Alertmanager to wire alerts up.
#
# alerting:
#   alertmanagers:
#     - static_configs:
#         - targets: ['alertmanager:9093']

scrape_configs:
  # The job name is load-bearing: alerts.yml matches up{job="restitch-gateway"}.
  # /metrics is served on the ADMIN port and sits outside requireKey, so no
  # credentials are needed here even when RESTITCH_ADMIN_API_KEY is set.
  - job_name: restitch-gateway
    static_configs:
      - targets: ['gateway:9090']

  - job_name: restitch-studio
    static_configs:
      - targets: ['studio:3080']

  - job_name: prometheus
    static_configs:
      - targets: ['localhost:9090']
```

- [ ] **Step 2: Create `deploy/docker-compose.yml`**

```yaml
# Production-shaped stack: gateway in REGISTRY mode against Studio, with
# Prometheus and Jaeger. This is the only place in the repo that exercises the
# M20 + M21 centralized-config loop as an operator would actually deploy it.
# examples/docker-compose/ remains the simple file-config quickstart.
name: restitch-deploy

services:
  studio:
    build:
      context: ..
      dockerfile: Dockerfile
      args:
        VERSION: ${RESTITCH_IMAGE_TAG:-dev}
    # The Dockerfile's ENTRYPOINT is /restitch, so the studio binary must be
    # selected with `entrypoint`, NOT `command` — a `command` override would
    # run `/restitch /restitch-studio` and fail.
    entrypoint: ["/restitch-studio"]
    command:
      - "-port=3080"
      - "-db-path=/data/studio.db"
      - "-gateway-admin-url=http://gateway:9090"
    environment:
      OTEL_EXPORTER_OTLP_ENDPOINT: ${OTEL_EXPORTER_OTLP_ENDPOINT:-}
      OTEL_SERVICE_NAME: restitch-studio
    volumes:
      - studio-data:/data
    ports:
      - "${STUDIO_PORT:-3080}:3080"
    healthcheck:
      test: ["CMD", "/restitch-studio", "-h"]
      interval: 5s
      timeout: 3s
      retries: 10
      start_period: 5s

  gateway:
    build:
      context: ..
      dockerfile: Dockerfile
      args:
        VERSION: ${RESTITCH_IMAGE_TAG:-dev}
    entrypoint: ["/restitch"]
    command:
      - "run"
      - "-registry-url=http://studio:3080"
      - "-poll-interval=10s"
      - "-port=8080"
      - "-log-format=json"
    environment:
      # In registry mode the whole gateway config comes from the registry
      # bundle, and a fresh Studio has no configs. Setting the admin server
      # through env vars means /metrics is up regardless of bundle contents,
      # so Prometheus has something to scrape from the first second.
      RESTITCH_ADMIN_ENABLED: "true"
      RESTITCH_ADMIN_PORT: "9090"
      RESTITCH_ADMIN_API_KEY: ${RESTITCH_ADMIN_API_KEY:-}
      OTEL_EXPORTER_OTLP_ENDPOINT: ${OTEL_EXPORTER_OTLP_ENDPOINT:-}
      OTEL_SERVICE_NAME: restitch-gateway
    ports:
      - "${GATEWAY_PORT:-8080}:8080"
      - "${GATEWAY_ADMIN_PORT:-9090}:9090"
    depends_on:
      studio:
        condition: service_started

  prometheus:
    image: prom/prometheus:${PROMETHEUS_VERSION}
    command:
      - "--config.file=/etc/prometheus/prometheus.yml"
      - "--storage.tsdb.path=/prometheus"
    volumes:
      - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - ./prometheus/rules:/etc/prometheus/rules:ro
      - prometheus-data:/prometheus
    # Prometheus binds its conventional 9090 INSIDE the network, but the
    # gateway admin server already owns 9090 on the host, so the host side
    # comes from PROMETHEUS_PORT (9091 by default).
    ports:
      - "${PROMETHEUS_PORT:-9091}:9090"
    depends_on:
      - gateway

  jaeger:
    image: jaegertracing/all-in-one:${JAEGER_VERSION}
    environment:
      COLLECTOR_OTLP_ENABLED: "true"
    ports:
      - "${JAEGER_UI_PORT:-16686}:16686"

volumes:
  studio-data:
  prometheus-data:
```

- [ ] **Step 3: Validate the compose file resolves**

```bash
cd deploy && docker compose --env-file .env.example config >/dev/null && echo "COMPOSE CONFIG OK"; cd ..
```

Expected: `COMPOSE CONFIG OK`. Any `variable is not set` warning means a key is missing from `.env.example` — add it there, not as a hardcoded default.

- [ ] **Step 4: Create `deploy/README.md`**

```markdown
# Restitch production stack

Gateway in **registry mode** against Studio, with Prometheus and Jaeger. This
is the stack that exercises the centralized-config loop (M20 + M21): Studio
owns configuration, and the gateway polls `/api/v1/registry/bundle` for it.

For a simple file-config quickstart instead, see `examples/docker-compose/`.

## Bring-up

```bash
cd deploy
cp .env.example .env      # then edit ports/keys as needed
docker compose up -d --build
```

The first build runs `npm ci` and two Go builds — expect several minutes.

| Service | Default URL |
|---------|-------------|
| Gateway | http://localhost:8080 |
| Gateway admin + `/metrics` | http://localhost:9090 |
| Studio | http://localhost:3080 |
| Prometheus | http://localhost:9091 |
| Jaeger UI | http://localhost:16686 |

**Prometheus is on 9091, not its conventional 9090** — the gateway admin
server already owns 9090 on the host.

## Seeding the registry

A fresh Studio has no compositions, so the registry bundle is empty and the
gateway starts serving nothing. Create one:

```bash
curl -X POST http://localhost:3080/api/v1/configs \
  -H 'Content-Type: application/json' \
  -d '{"name":"example","yaml":"compositions:\n  hello:\n    path: /hello\n    method: GET\n    steps: []\n"}'
```

The gateway picks it up within one poll interval (10s by default).

## Tuning the latency alert

`prometheus/rules/alerts.yml` → `RestitchHighP95Latency` → `expr:`. The `> 1`
literal is the P95 threshold in **seconds**. Prometheus rule files have no
variable substitution, so this literal is the configuration point. After
editing, re-run the unit tests:

```bash
docker run --rm -v "$PWD/prometheus:/p:ro" --entrypoint=/bin/promtool \
  prom/prometheus:$(grep -E '^PROMETHEUS_VERSION=' .env.example | cut -d= -f2) \
  test rules /p/rules/alerts_test.yml
```

## Alerting

No Alertmanager ships here on purpose — a demo receiver that discards every
notification proves nothing. `prometheus/prometheus.yml` has a commented
`alerting:` block marking where to point your own.

## Teardown

```bash
docker compose down -v     # -v also drops the Studio database and Prometheus TSDB
```
```

- [ ] **Step 5: Accept — run the gate's T24.4 phase**

Run: `H_NO_EVIDENCE=true scripts/gates/m24.sh 2>&1 | sed -n '/T24.4/,/M24.unit/p'`

Expected: five `PASS` lines — stack started, ≥2 rule groups, ≥11 rules, all rules `health=ok`, and `up{job="restitch-gateway"} == 1`. Allow several minutes on first run for the image build.

- [ ] **Step 6: Confirm teardown left nothing behind**

Run: `docker ps -a --filter "name=restitch-m24-gate" --format '{{.Names}}'; docker volume ls --filter "name=restitch-m24-gate" --format '{{.Name}}'`

Expected: both empty. If not, the trap chaining in the gate is broken — fix it before continuing.

- [ ] **Step 7: Append the ledger row**

```markdown
| 2026-07-27 | T24.4 | M24 | PASS | <short-sha> | evidence/<file>#T24.4 | compose stack, registry mode, rules loaded + gateway scraped |
```

- [ ] **Step 8: Commit**

```bash
git add deploy/docker-compose.yml deploy/prometheus/prometheus.yml deploy/README.md \
        docs/plan-progress/LEDGER.md
git commit -m "feat(M24): add deploy/ production stack with Prometheus and Jaeger

Gateway runs in registry mode against Studio, exercising the M20+M21
centralized-config loop. Prometheus binds host port 9091 because the gateway
admin server owns 9090. The admin server is configured via RESTITCH_ADMIN_*
env vars so /metrics is scrapeable even when the registry bundle is empty.

Uses entrypoint: rather than command: to select the studio binary — the
pattern in examples/docker-compose/ resolves to '/restitch /restitch-studio'
and cannot work.

Accept output:
<paste the real Step 5 output here>"
```

---

## Task 6: T24.3b — CI load-test job

Runs **after** Task 4 because the threshold is derived from a measured baseline, not guessed. The formula is fixed in advance — **observed P95 × 2, rounded up to the nearest 50ms** — so the number comes out of the evidence mechanically rather than being chosen to make the build pass.

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Read the current workflow**

Run: `sed -n '1,10p' .github/workflows/ci.yml`

Expected: `on: push:` / `pull_request: branches: [main]`, and `jobs:` with `go`, `studio`, `docker`.

- [ ] **Step 2: Add `workflow_dispatch`**

Change the `on:` block to:

```yaml
on:
  push:
  pull_request:
    branches: [main]
  workflow_dispatch:
```

Without this the job is unrunnable until the first `release/*` branch exists, and an untested CI job is not a delivered CI job.

- [ ] **Step 3: Append the `loadtest` job**

Add at the end of `.github/workflows/ci.yml`, at the same indent level as `docker:`. Note `P95_MS` is left as the placeholder `REPLACE_IN_STEP_6` deliberately — Step 6 replaces it with the measured value.

```yaml
  loadtest:
    # Release branches and tags only. A hard latency threshold on every push
    # would flake on shared-runner noise; here it gates the things that ship.
    # workflow_dispatch allows a manual run on any branch.
    if: >-
      github.event_name == 'workflow_dispatch' ||
      startsWith(github.ref, 'refs/heads/release/') ||
      startsWith(github.ref, 'refs/tags/v')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - uses: grafana/setup-k6-action@v1

      - name: Build binaries
        run: |
          go build -o bin/restitch ./cmd/restitch
          go build -o bin/mockupstream ./cmd/mockupstream

      # Binaries, not compose: this measures gateway throughput, and putting
      # Docker networking between k6 and the gateway would measure the bridge.
      - name: Start mockupstream and gateway
        run: |
          ./bin/mockupstream -port 18081 &
          cat > /tmp/ci_load.yaml <<'YAML'
          upstreams:
            mock:
              url: "http://127.0.0.1:18081"
              timeout: 10s
              transport:
                max_idle_conns_per_host: 100
          compositions:
            loadtest:
              path: "/loadtest"
              method: GET
              steps:
                - name: s1
                  upstream: mock
                  path: "/users/1"
              response:
                body:
                  name: "{{ steps.s1.body.name }}"
          YAML
          ./bin/restitch run -config /tmp/ci_load.yaml -port 18080 -log-format json &
          for i in $(seq 1 30); do
            curl -sf http://127.0.0.1:18080/loadtest >/dev/null && break
            sleep 1
          done

      # Reduced profile: a 2-core shared runner cannot honestly sustain the
      # gate's 1000/s. STUDIO_URL is intentionally unset, so the k6 script
      # omits its studio scenario entirely.
      - name: Run k6 load test
        env:
          RATE: "200"
          DURATION: "30s"
          P95_MS: "REPLACE_IN_STEP_6"
          ERR_RATE: "0.01"
          GW_URL: "http://127.0.0.1:18080/loadtest"
          SUMMARY_OUT: "/tmp/m24_ci_summary.json"
        run: k6 run tests/loadtest/m24_baseline.js

      - name: Show summary
        if: always()
        run: cat /tmp/m24_ci_summary.json || echo "no summary produced"

      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: k6-summary
          path: /tmp/m24_ci_summary.json
```

- [ ] **Step 4: Validate the workflow parses**

Run: `python3 -c "import yaml,sys; d=yaml.safe_load(open('.github/workflows/ci.yml')); print('JOBS:', list(d['jobs'])); print('TRIGGERS:', list(d[True] if True in d else d['on']))"`

Expected: `JOBS: ['go', 'studio', 'docker', 'loadtest']` and triggers including `workflow_dispatch`. If PyYAML is unavailable, run `pip install pyyaml` first, or use any YAML linter — do not skip this check.

- [ ] **Step 5: Commit the job with the placeholder threshold**

```bash
git add .github/workflows/ci.yml
git commit -m "feat(M24): add release-branch k6 load test job to CI

Runs the reduced profile (200/s, 30s) on release branches and tags, plus
workflow_dispatch for manual runs. P95_MS is a placeholder until measured
against a real runner in the follow-up commit."
```

- [ ] **Step 6: Measure on a real runner and set the threshold**

Trigger the job via `workflow_dispatch` (GitHub UI → Actions → CI → Run workflow, or `gh workflow run CI`). When it completes, download the `k6-summary` artifact and read the observed `compositions.p95_ms`.

Apply the fixed formula: **`ceil(observed_p95 × 2 / 50) × 50`**. Compute it, don't eyeball it:

```bash
python3 -c "import math; p=<OBSERVED_P95>; print(int(math.ceil(p*2/50)*50))"
```

Replace `REPLACE_IN_STEP_6` in `.github/workflows/ci.yml` with that number.

- [ ] **Step 7: Confirm no placeholder remains**

Run: `grep -n "REPLACE_IN_STEP_6" .github/workflows/ci.yml; echo "EXIT=$? (1 = clean)"`

Expected: `EXIT=1` — no output, placeholder gone.

- [ ] **Step 8: Re-run the job to confirm it passes at the real threshold**

Trigger `workflow_dispatch` again. Expected: the `loadtest` job passes with the computed threshold.

- [ ] **Step 9: Append the ledger row**

The T24.3 row already exists from Task 4; this appends a superseding row covering the CI half.

```markdown
| 2026-07-27 | T24.3 | M24 | PASS | <short-sha> | evidence/<file>#T24.3 | + CI job, P95_MS set from measured runner baseline |
```

- [ ] **Step 10: Commit**

```bash
git add .github/workflows/ci.yml docs/plan-progress/LEDGER.md
git commit -m "feat(M24): set CI load-test P95 threshold from measured baseline

Observed P95 on the GitHub runner: <OBSERVED>ms.
Threshold = ceil(<OBSERVED> * 2 / 50) * 50 = <COMPUTED>ms.

Derived mechanically from the workflow_dispatch run rather than chosen to
make the build pass.

Run output:
<paste the real k6 summary JSON here>"
```

---

## Task 7: Milestone close-out

**Files:**
- Modify: `PLAN.md` (status table, add M24)
- Modify: `docs/plan-progress/LEDGER.md` (append gate rows)
- Create: `docs/plan-progress/evidence/<date>-M24-<sha>.log` (written by the gate)

- [ ] **Step 1: Run the full gate for real**

Run: `scripts/verify.sh M24`

Expected: `RESULT M24: PASS`. This run **does** write the evidence file and append ledger rows — unlike the `H_NO_EVIDENCE=true` diagnostic runs in earlier tasks.

If it prints any `MANUAL` line, **stop**. Per CLAUDE.md rule 6, MANUAL lines are not yours to check off: list them for the user and wait for confirmation before adding a MANUAL-VERIFIED row.

- [ ] **Step 2: Confirm the evidence file exists and records the pass**

```bash
ls -t docs/plan-progress/evidence/*M24* | head -1
tail -5 "$(ls -t docs/plan-progress/evidence/*M24* | head -1)"
```

Expected: a file dated today, ending in `RESULT M24: PASS (…)`.

- [ ] **Step 3: Confirm the auto-appended ledger rows are the expected six IDs**

Run: `grep "| M24 |" docs/plan-progress/LEDGER.md | tail -8`

Expected: rows for exactly `T24.1`, `T24.2`, `T24.3`, `T24.4`, `M24.unit`, `M24.gate` — all `PASS`. Any other task ID means an `h_task` name violates the Global Constraints; fix the gate (a `gate:` commit, with user approval) rather than editing the ledger.

- [ ] **Step 4: Run the ledger check**

Run: `scripts/check-ledger.sh; echo "EXIT=$?"`

Expected: `EXIT=0`. `M24.unit` will be listed under UNKNOWN-IDS alongside `M20.unit`–`M23.unit` — that is expected and not a regression, because `M24.unit` is a gate pseudo-ID rather than a PLAN.md task.

- [ ] **Step 5: Mark M24 DONE in the PLAN.md status table**

Add after the M23 row:

```markdown
| M24 — Production Monitoring & Load Testing | T24.1–T24.4 | DONE |
```

- [ ] **Step 6: Verify the placeholder guard now agrees**

The old placeholder failed if M24 was marked DONE while the gate was unimplemented. Confirm the real gate still passes with the table updated:

Run: `scripts/verify.sh M24 2>&1 | tail -3`

Expected: `RESULT M24: PASS`.

- [ ] **Step 7: Commit**

```bash
git add PLAN.md docs/plan-progress/LEDGER.md docs/plan-progress/evidence/
git commit -m "docs: mark M24 DONE with committed gate evidence

scripts/verify.sh M24 output:
<paste the real Step 1 output here>"
```

- [ ] **Step 8: Report to the user**

State: M24 complete, gate PASS, evidence committed. Only M25 (Browser Session & User Preferences) remains, and it needs its own brainstorm → spec → plan cycle before any code is written.

---

## Self-Review

Checked against `docs/superpowers/specs/2026-07-27-m24-monitoring-loadtest-design.md`:

| Spec section | Covered by |
|---|---|
| D1 hybrid proof (promtool + compose smoke) | Task 1 gate phases T24.1/T24.2 + T24.4 |
| D2 scaled CI with hard thresholds | Task 6 (reduced profile, fixed derivation formula) |
| D3 registry-mode deploy stack | Task 5 (`-registry-url=http://studio:3080`) |
| D4 no Alertmanager, commented wiring point | Task 5 Step 1 |
| D5 approach A phase ordering | Task 1 gate structure |
| T24.1 recording rules | Task 2 |
| T24.2 alert rules | Task 3 |
| T24.3 k6 + CI | Tasks 4 and 6 |
| T24.4 deploy stack | Task 5 |
| Partial responses recorded separately | Task 2 Steps 2–3 (incl. a test asserting they stay out of the ratio) |
| `error_ratio5m` no zero-fill | Task 2 Step 3 (`exp_samples: []` test) |
| 10m `for` on config-reload alert | Task 3 Step 1 |
| Negative controls (5 rows in spec) | Task 1: count guards, rule-count assertion, `up==1` query, `reqs` floor, per-alert name checks; Task 3 Step 4 mutation check |
| `PROMETHEUS_VERSION` single source of truth | Task 2 Step 1, gate preflight, Task 5 compose |
| Ledger IDs exactly six | Global Constraints + Task 7 Step 3 |
| PLAN.md drift fix separate | Task 0 |
| Compose teardown in a chained trap | Task 1 (`trap 'm24_cleanup; h_cleanup' EXIT`) — the spec said "the existing `h_cleanup` trap", but `scripts/lib/` is off-limits, so chaining in the gate is the only conforming implementation |
| Cold-build warning | Task 1 (`h_log` before compose up), Task 5 README |

**Placeholder scan:** the only intentional placeholders are `<short-sha>`, `<file>`, `<paste … here>` in commit/ledger templates (filled from real output at execution time), and `REPLACE_IN_STEP_6`, which Task 6 Step 7 explicitly verifies is gone.

**Type/name consistency:** `composition:restitch_requests:error_ratio5m` and `composition:restitch_request_duration_seconds:p95` are spelled identically in Task 2 (definition), Task 3 (consumption), and Task 3 Step 4 (mutation check). Job name `restitch-gateway` matches across `alerts.yml`, `prometheus.yml`, and the gate's query. The k6 summary keys `p95_ms`/`error_rate`/`reqs` match between Task 4's `handleSummary` and Task 1's parser. Env var names `RATE`/`DURATION`/`P95_MS`/`ERR_RATE`/`GW_URL`/`STUDIO_URL`/`SUMMARY_OUT` match between the gate, the script, and the CI job.
