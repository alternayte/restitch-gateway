#!/usr/bin/env bash
# Gate M6 — Resilience: retries, circuit breaker, coalescing
source "$(dirname "$0")/../lib/harness.sh"
h_init M6

# ── T6.1 Retry RoundTripper ─────────────────────────────────────────────────
h_task T6.1
h_run "T6.1 retry.go exists" -- test -f "${REPO_ROOT}/internal/upstream/retry.go"
h_run "T6.1 upstream retry tests" -- \
    go test -count=1 -race ./internal/upstream/ -run TestRetry

# ── T6.2 Circuit breaker ────────────────────────────────────────────────────
h_task T6.2
h_run "T6.2 gobreaker dependency present" -- \
    grep -q 'sony/gobreaker' "${REPO_ROOT}/go.mod"
h_run "T6.2 breaker tests" -- \
    go test -count=1 -race ./internal/upstream/ -run TestBreaker

# ── T6.3 Request coalescing ─────────────────────────────────────────────────
h_task T6.3
h_run "T6.3 coalescing tests" -- \
    go test -count=1 ./internal/upstream/ -run "TestCoalesce\|TestSingleflight"

# ── M6 Verification gate ────────────────────────────────────────────────────
h_task M6.gate
h_run "M6.gate upstream + composition tests (race)" -- \
    go test -count=1 -race ./internal/upstream/ ./internal/composition/

# Breaker demo: 503 status triggers breaker
h_start_mockupstream

config=$(h_config m6 <<'YAML'
server:
  port: @GW_PORT@
admin:
  port: @ADMIN_PORT@
upstreams:
  flaky:
    url: "http://127.0.0.1:@MOCK_PORT@"
    retry:
      max_attempts: 1
    circuit_breaker:
      max_failures: 3
      timeout: 5s
compositions:
  f:
    path: "/f"
    method: GET
    steps:
      - name: s
        upstream: flaky
        path: "/status/503"
    response:
      body:
        r: "{{ steps.s.status }}"
YAML
)

h_start_gateway "${config}"

# Hit it 6 times — breaker should trip after enough failures
statuses=""
for i in $(seq 1 6); do
    s=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${GW_PORT}/f" 2>/dev/null) || true
    statuses="${statuses} ${s}"
    sleep 0.1
done

{
    echo "Status codes from 6 requests: ${statuses}"
} >> "${H_EVIDENCE_FILE}"

# Check that we got at least one 502 (breaker open)
if echo "${statuses}" | grep -q '502'; then
    h_pass "M6.gate circuit breaker tripped (502 seen)"
else
    h_fail "M6.gate circuit breaker did not trip (no 502: ${statuses})"
fi

# Check metrics for breaker state
sleep 0.5
breaker_metric=$(curl -s "http://127.0.0.1:${ADMIN_PORT}/metrics" 2>/dev/null \
    | grep 'restitch_breaker_state' | grep -v '#' || true)
{
    echo "Breaker metric: ${breaker_metric}"
} >> "${H_EVIDENCE_FILE}"
if [[ -n "${breaker_metric}" ]]; then
    h_pass "M6.gate breaker state metric present"
else
    h_fail "M6.gate breaker state metric not found"
fi

h_finish
