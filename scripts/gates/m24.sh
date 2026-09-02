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
#
# Each alert therefore needs BOTH a positive and a negative case: require >= 2
# alertname occurrences per alert. A bare `grep -q` here proved vacuous — a
# suite whose every case was negative satisfied it for all four alerts, i.e.
# the exact omission this block exists to catch.
for alert in RestitchHighP95Latency RestitchHighErrorRate \
             RestitchConfigReloadFailing RestitchGatewayDown; do
    alert_cases="$(grep -c "alertname: ${alert}" "${RULES_DIR}/alerts_test.yml" 2>/dev/null || true)"
    alert_cases="${alert_cases:-0}"
    h_evidence "$ grep -c 'alertname: ${alert}' alerts_test.yml -> ${alert_cases} (minimum 2)"
    if [[ "${alert_cases}" -ge 2 ]]; then
        h_pass "alerts_test.yml covers ${alert} with ${alert_cases} cases"
    else
        h_fail "alerts_test.yml has ${alert_cases} case(s) for ${alert}; need >= 2 (positive AND negative)"
    fi
done

# Per-alert counts cannot tell a positive case from a negative one, so assert
# the distribution as well. `exp_alerts:` followed by entries is a positive
# case (the alert MUST fire); `exp_alerts: []` is a negative one (it must stay
# silent). Four alerts => at least four of each.
m24_assert_min_count "${RULES_DIR}/alerts_test.yml" "exp_alerts:$" 4 \
    "alerts_test.yml has positive cases (alerts that must FIRE)"
m24_assert_min_count "${RULES_DIR}/alerts_test.yml" "exp_alerts: \[\]" 4 \
    "alerts_test.yml has negative cases (alerts that must stay silent)"

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
    # The k6 studio scenario polls the registry API, which requires the key
    # (hardening C1).
    h_start_studio -registry-key test-registry-key

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
    STUDIO_KEY="test-registry-key" \
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

# ── M24.gate roll-up ─────────────────────────────────────────────────────────
# h_finish writes one ledger row per h_task name, and the preflight block above
# also used the M24.gate ID. Without this closing roll-up the M24.gate row would
# record the PREFLIGHT result only — a PASS even when T24.1-T24.4 failed, i.e. a
# false PASS sitting in the ledger next to a FAIL RESULT line. Re-opening the
# task here makes its row reflect the whole run. (M23's gate did the same.)
h_task M24.gate
if [[ ${H_FAIL_COUNT} -eq 0 ]]; then
    h_pass "M24 gate: all phases passed"
else
    h_fail "M24 gate: ${H_FAIL_COUNT} assertion(s) failed across the run"
fi

h_finish
