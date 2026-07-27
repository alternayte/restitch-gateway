#!/usr/bin/env bash
# Gate M23 — Upstream HTTP Client Optimization (extends M5)
#
# Encodes PLAN.md "M23 Verification gate":
#   # Load test with k6: fan-out composition hitting 5 upstreams
#   # Before: observe connection churn in netstat, P95 latency > 200ms
#   # After: stable idle connections, P95 latency < 50ms
#
# Encoding decisions (approved design:
# docs/superpowers/specs/2026-07-27-m23-upstream-http-client-optimization-design.md):
#
#   * k6 is HARD-REQUIRED. The harness's skip-if-absent convention (docker, gh,
#     golangci-lint) would let M23 go green with zero performance evidence.
#   * "netstat" is replaced by server-side ground truth: mockupstream counts
#     accepted TCP connections via a ConnState hook and reports them at
#     /__stats. Cumulative and exact, where netstat would be sampled and
#     brittle to parse on macOS.
#   * The gate runs k6 TWICE against the same composition — max_idle_conns_per_host
#     2 vs 100 — and asserts the tuned run accepts strictly fewer connections.
#     This is the non-flaky signal, and it exercises the config surface T23.3 adds.
#   * baseline P95 is RECORDED but NOT asserted. PLAN's ">200ms before" was
#     measured against real network upstreams; on loopback, connection setup is
#     cheap enough that asserting it would make the gate flaky.
#
# Written before T23.0b/T23.2/T23.3 land (CLAUDE.md rule 2): expect the grep
# checks and both k6 runs to FAIL until those tasks are implemented. That is the
# intended state for this gate immediately after Task 1.
set -euo pipefail
source "$(dirname "$0")/../lib/harness.sh"
h_init M23

# ── k6 is hard-required ──────────────────────────────────────────────

h_task M23.gate
if h_require_tool k6; then
    h_evidence "$ k6 version"
    k6 version >> "${H_EVIDENCE_FILE}" 2>&1 || true
    h_pass "k6 available (hard requirement)"
else
    h_fail "k6 not installed — M23 gate requires k6 (brew install k6). Not skippable."
    h_finish
fi

# ── T23.1: pool + TLS defaults ───────────────────────────────────────

h_task T23.1
h_run "BuildTransport defined" -- grep -q 'func BuildTransport' internal/upstream/client.go
h_run "MaxIdleConnsPerHost defaults to 100" -- \
    grep -q 'maxIdleConnsPerHost = 100' internal/upstream/client.go
h_run "TLS 1.2 minimum enforced" -- \
    grep -q 'MinVersion:         tls.VersionTLS12' internal/upstream/client.go
h_run "transport defaults covered by tests" -- \
    go test -race -count=1 -run 'TestBuildTransport' ./internal/upstream/...

# ── T23.2: DrainAndClose coverage ────────────────────────────────────

h_task T23.2
h_run "DrainAndClose defined" -- grep -q 'func DrainAndClose' internal/upstream/drain.go
h_run "composition step drains" -- \
    grep -q 'DrainAndClose(resp.Body)' internal/composition/step.go
h_run "health check drains" -- \
    grep -q 'DrainAndClose(resp.Body)' internal/upstream/health.go
h_run "hotreload client drains" -- \
    grep -q 'DrainAndClose(resp.Body)' internal/hotreload/client.go
h_run "devmode writer drains" -- \
    grep -q 'DrainAndClose(resp.Body)' internal/devmode/writer.go
h_grep_repo 'defer resp.Body.Close()' \
    'internal/upstream internal/hotreload internal/devmode internal/composition' \
    --empty "no bare resp.Body.Close() on upstream response paths"

# ── T23.3: transport block wiring ────────────────────────────────────

h_task T23.3
h_run "TransportConfig type in config.go" -- \
    grep -q 'type TransportConfig struct' internal/composition/config.go
h_run "Upstream.Transport field bound to yaml" -- \
    grep -q 'Transport .*\`yaml:"transport"\`' internal/composition/config.go
h_run "max_idle_conns_per_host yaml tag" -- \
    grep -q 'yaml:"max_idle_conns_per_host"' internal/composition/config.go
h_run "insecure_skip_verify yaml tag" -- \
    grep -q 'yaml:"insecure_skip_verify"' internal/composition/config.go
h_run "parser translates the transport block" -- \
    grep -q 'toUpstreamTransport(up.Transport)' internal/composition/parser.go
# Whitespace-tolerant: gofmt may realign the struct literal, so match on the
# shape rather than on an exact run of spaces.
h_run "parser no longer hardcodes empty transport" -- \
    bash -c '! grep -qE "Transport:[[:space:]]+upstream\.TransportConfig\{\}" internal/composition/parser.go'
h_run "transport_test.go exists" -- test -f internal/composition/transport_test.go
# Two holes to close here:
#   * `go test -run PATTERN` exits 0 when PATTERN matches nothing, so the exit
#     code alone would pass vacuously with no tests written.
#   * grepping only for a PASS line would pass even when a sibling
#     TestTransport* case FAILs.
# Require BOTH: at least one named test passed AND go test itself exited 0.
h_run "transport parser tests pass" -- \
    bash -c "out=\$(go test -race -count=1 -run 'TestTransport' -v ./internal/composition/... 2>&1); code=\$?; echo \"\$out\"; echo \"\$out\" | grep -q '^--- PASS: TestTransport' && [ \"\$code\" -eq 0 ]"

# ── Unit tests ───────────────────────────────────────────────────────

h_task M23.unit
h_run "go vet upstream" -- go vet ./internal/upstream/...
h_run "go vet composition" -- go vet ./internal/composition/...
h_run "go test upstream" -- go test -race -count=1 ./internal/upstream/...
h_run "go test composition" -- go test -race -count=1 ./internal/composition/...
h_run "go test full suite" -- go test -race -count=1 ./...

# ── A/B load test ────────────────────────────────────────────────────

h_task M23.gate

h_build
h_start_mockupstream

# Verify the mockupstream stats endpoint exists before relying on it.
if ! curl -sf "http://127.0.0.1:${MOCK_PORT}/__stats" > "${H_TMP}/stats_probe.json" 2>/dev/null; then
    h_fail "mockupstream /__stats endpoint missing (T23.0b not implemented)"
    h_finish
fi
h_evidence "$ curl /__stats (probe)"
h_evidence "$(cat "${H_TMP}/stats_probe.json")"
h_pass "mockupstream /__stats endpoint available"

# Generate a five-upstream fan-out config at a given max_idle_conns_per_host,
# pointed at a given mockupstream port. All five point at the same mockupstream
# process; upstream.Build gives each its own *http.Transport, so there are five
# independent connection pools.
m23_write_config() {
    local max_idle="$1" mock_port="$2" out="$3"
    {
        echo "upstreams:"
        local i
        for i in 1 2 3 4 5; do
            echo "  up${i}:"
            echo "    url: \"http://127.0.0.1:${mock_port}\""
            echo "    timeout: 10s"
            echo "    transport:"
            echo "      max_idle_conns_per_host: ${max_idle}"
        done
        echo "compositions:"
        echo "  fanout:"
        echo "    path: \"/fanout\""
        echo "    method: GET"
        echo "    steps:"
        for i in 1 2 3 4 5; do
            echo "      - name: s${i}"
            echo "        upstream: up${i}"
            echo "        path: \"/users/${i}\""
        done
        echo "    response:"
        echo "      body:"
        for i in 1 2 3 4 5; do
            echo "        u${i}: \"{{ steps.s${i}.body.name }}\""
        done
    } > "${out}"
}

# Non-fatal port wait. h_wait_for_port calls h_finish (which exits) on timeout;
# because m23_run_arm below runs inside a $(...) capture, that exit would kill
# only the subshell — appending a stray ledger row mid-run and feeding a failure
# message into `read` as if it were metrics. Return non-zero instead so the
# caller can fail with the real reason.
m23_wait_port() {
    local port="$1" deadline=$((SECONDS + ${2:-15}))
    while ! python3 -c "import socket; s=socket.socket(); s.settimeout(0.5); s.connect(('127.0.0.1', ${port})); s.close()" 2>/dev/null; do
        if [[ ${SECONDS} -ge ${deadline} ]]; then
            return 1
        fi
        sleep 0.2
    done
    return 0
}

# Fetch /__stats with retries. Under the baseline arm's connection churn a
# single curl can lose the race for a loopback ephemeral port; the previous
# version swallowed that and emitted conns_accepted=-1, which is a fabricated
# measurement. Fail the arm instead of inventing a number.
m23_fetch_stats() {
    local mock_port="$1" attempt
    for attempt in 1 2 3; do
        if curl -sf --max-time 10 "http://127.0.0.1:${mock_port}/__stats" 2>/dev/null; then
            return 0
        fi
        sleep 1
    done
    return 1
}

# Run one arm of the A/B and echo "<p95_ms> <error_rate> <conns> <reqs>".
# Returns non-zero (with a diagnostic on stdout) if the arm could not run.
#
# Each arm gets its OWN mockupstream and gateway on their own ports. Sharing one
# mockupstream made the arms interfere: the baseline arm's churn exhausted
# loopback ephemeral ports, so the tuned arm that followed measured ~9.8% errors
# and half the throughput it achieves in isolation (0% errors, 132k requests).
# That is contamination, not a property of the code under test.
m23_run_arm() {
    local label="$1" max_idle="$2"
    local mock_port gw_port config
    mock_port="$(h_free_port)"
    gw_port="$(h_free_port)"
    config="${H_TMP}/fanout_${label}.yaml"

    "${REPO_ROOT}/bin/mockupstream" -port "${mock_port}" \
        > "${H_TMP}/mock_${label}.log" 2>&1 &
    local mock_pid=$!
    H_PIDS+=("${mock_pid}")
    if ! m23_wait_port "${mock_port}" 15; then
        echo "mockupstream for arm '${label}' did not listen on ${mock_port} within 15s"
        kill "${mock_pid}" 2>/dev/null || true
        return 1
    fi

    m23_write_config "${max_idle}" "${mock_port}" "${config}"

    "${REPO_ROOT}/bin/restitch" run \
        -config "${config}" \
        -port "${gw_port}" \
        -log-format json \
        > "${H_TMP}/gw_${label}.log" 2>&1 &
    local gw_pid=$!
    H_PIDS+=("${gw_pid}")

    if ! m23_wait_port "${gw_port}" 15; then
        echo "gateway for arm '${label}' did not listen on ${gw_port} within 15s"
        kill "${gw_pid}" "${mock_pid}" 2>/dev/null || true
        return 1
    fi

    # This mockupstream is fresh, so its counters start at zero; the reset only
    # discards the connections our own readiness probes opened.
    if ! curl -sf --max-time 10 -X POST "http://127.0.0.1:${mock_port}/__stats/reset" \
            > "${H_TMP}/reset_${label}.json" 2>/dev/null; then
        echo "POST /__stats/reset failed before arm '${label}'"
        kill "${gw_pid}" "${mock_pid}" 2>/dev/null || true
        return 1
    fi
    h_evidence "$ curl -X POST /__stats/reset  (arm ${label}) -> $(cat "${H_TMP}/reset_${label}.json")"

    GW_URL="http://127.0.0.1:${gw_port}" k6 run \
        --summary-export="${H_TMP}/k6_${label}.json" \
        "${REPO_ROOT}/tests/loadtest/m23_fanout.js" \
        >> "${H_TMP}/k6_${label}.log" 2>&1 || true

    local stats
    if ! stats="$(m23_fetch_stats "${mock_port}")"; then
        echo "GET /__stats failed after arm '${label}' (3 attempts) — refusing to report a placeholder count"
        kill "${gw_pid}" "${mock_pid}" 2>/dev/null || true
        return 1
    fi
    h_evidence "$ curl /__stats  (arm ${label}) -> ${stats}"

    kill "${gw_pid}" "${mock_pid}" 2>/dev/null || true
    wait "${gw_pid}" 2>/dev/null || true
    wait "${mock_pid}" 2>/dev/null || true

    python3 - "${H_TMP}/k6_${label}.json" "${stats}" <<'PY'
import json, sys
with open(sys.argv[1]) as fh:
    m = json.load(fh)["metrics"]
stats = json.loads(sys.argv[2] or "{}")
if "conns_accepted" not in stats:
    sys.exit("stats payload missing conns_accepted: %r" % (stats,))
p95 = m["http_req_duration"]["p(95)"]          # milliseconds
err = m["http_req_failed"]["value"]            # rate, 0.0-1.0
reqs = m["http_reqs"]["count"]
print(f'{p95:.3f} {err:.6f} {stats["conns_accepted"]} {reqs}')
PY
}

# Each arm is captured, so a failure inside it must be surfaced here with the
# gateway log attached — otherwise `read` would parse the diagnostic text as
# metrics and the real cause would show up as a Python ValueError.
M23_COOLDOWN_SECS="${M23_COOLDOWN_SECS:-15}"

m23_arm_or_die() {
    local label="$1" max_idle="$2" out=""
    if ! out="$(m23_run_arm "${label}" "${max_idle}")"; then
        h_evidence "arm '${label}' failed: ${out}"
        h_evidence "--- tail of ${H_TMP}/gw_${label}.log ---"
        h_evidence "$(tail -20 "${H_TMP}/gw_${label}.log" 2>/dev/null || echo '(no log)')"
        h_fail "k6 ${label} arm could not run: ${out}"
        h_finish
    fi
    echo "${out}"
}

h_log "Running k6 baseline arm (max_idle_conns_per_host: 2)..."
read -r BASE_P95 BASE_ERR BASE_CONNS BASE_REQS <<< "$(m23_arm_or_die baseline 2)"

# The baseline arm leaves tens of thousands of loopback sockets in TIME_WAIT.
# Without this pause the tuned arm competes for ephemeral ports and reports
# errors that belong to the harness, not to the gateway.
h_log "Cooling down ${M23_COOLDOWN_SECS}s so loopback TIME_WAIT sockets drain..."
sleep "${M23_COOLDOWN_SECS}"

h_log "Running k6 tuned arm (max_idle_conns_per_host: 100)..."
read -r TUNED_P95 TUNED_ERR TUNED_CONNS TUNED_REQS <<< "$(m23_arm_or_die tuned 100)"

h_evidence "baseline  max_idle=2    p95=${BASE_P95}ms  error_rate=${BASE_ERR}  conns_accepted=${BASE_CONNS}  http_reqs=${BASE_REQS}"
h_evidence "tuned     max_idle=100  p95=${TUNED_P95}ms error_rate=${TUNED_ERR} conns_accepted=${TUNED_CONNS} http_reqs=${TUNED_REQS}"

h_run "tuned run accepted fewer upstream connections than baseline" -- \
    python3 -c "import sys; b=int(sys.argv[1]); t=int(sys.argv[2]); assert b>0 and t>0, f'no connections recorded: baseline={b} tuned={t}'; assert t<b, f'tuned={t} not < baseline={b}'" \
    "${BASE_CONNS}" "${TUNED_CONNS}"

h_run "tuned P95 < 50ms" -- \
    python3 -c "import sys; p=float(sys.argv[1]); assert p<50.0, f'p95={p}ms'" "${TUNED_P95}"

h_run "tuned error rate < 1%" -- \
    python3 -c "import sys; e=float(sys.argv[1]); assert e<0.01, f'error_rate={e}'" "${TUNED_ERR}"

h_run "tuned run actually generated load" -- \
    python3 -c "import sys; n=int(sys.argv[1]); assert n>1000, f'http_reqs={n}'" "${TUNED_REQS}"

h_finish
