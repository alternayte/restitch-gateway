#!/usr/bin/env bash
# Gate M8 — Observability: metrics, access log, admin server
source "$(dirname "$0")/../lib/harness.sh"
h_init M8

# ── T8.1 Prometheus metrics ─────────────────────────────────────────────────
h_task T8.1
h_run "T8.1 metrics.go exists" -- \
    test -f "${REPO_ROOT}/internal/observability/metrics.go"
h_run "T8.1 prometheus dependency" -- \
    grep -q 'prometheus/client_golang' "${REPO_ROOT}/go.mod"

# ── T8.2 Access log with latency split ───────────────────────────────────────
h_task T8.2
h_run "T8.2 gateway_overhead_ms in handler" -- \
    grep -rq 'gateway_overhead_ms\|gateway_overhead' "${REPO_ROOT}/internal/composition/handler.go"

# ── T8.3 Admin server + ring buffer ─────────────────────────────────────────
h_task T8.3
h_run "T8.3 admin package exists" -- test -d "${REPO_ROOT}/internal/admin"
h_run "T8.3 admin tests" -- go test -count=1 ./internal/admin/

# ── M8 Verification gate ────────────────────────────────────────────────────
h_task M8.gate
h_run "M8.gate full tests (race)" -- go test -count=1 -race ./...

h_start_mockupstream

config=$(h_config m8 <<'YAML'
server:
  port: @GW_PORT@
admin:
  port: @ADMIN_PORT@
upstreams:
  mock:
    url: "http://127.0.0.1:@MOCK_PORT@"
compositions:
  user:
    path: "/api/users/{id}"
    method: GET
    steps:
      - name: u
        upstream: mock
        path: "/users/{{ req.params.id }}"
    response:
      body:
        user: "{{ steps.u.body }}"
YAML
)

h_start_gateway "${config}"

# Make a request first
curl -s "http://127.0.0.1:${GW_PORT}/api/users/1" > /dev/null 2>&1
sleep 0.5

# Check metrics
h_assert_body_contains "http://127.0.0.1:${ADMIN_PORT}/metrics" \
    "restitch_requests_total" \
    "M8.gate restitch_requests_total metric present"

# Check admin API requests endpoint
h_assert_status "http://127.0.0.1:${ADMIN_PORT}/admin/api/requests" 200 \
    "M8.gate admin requests endpoint returns 200"
body="${H_LAST_BODY}"
h_assert_json_body "${body}" 'isinstance(data, list) and len(data) >= 1' \
    "M8.gate admin requests contains at least 1 record"

# Check compositions endpoint
h_assert_status "http://127.0.0.1:${ADMIN_PORT}/admin/api/compositions" 200 \
    "M8.gate admin compositions endpoint returns 200"

# /health/upstreams on data port should be gone (moved to admin)
status_old=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${GW_PORT}/health/upstreams" 2>/dev/null) || true
{
    echo "$ curl /health/upstreams on data port → ${status_old}"
} >> "${H_EVIDENCE_FILE}"
if [[ "${status_old}" == "404" || "${status_old}" == "405" ]]; then
    h_pass "M8.gate /health/upstreams not on data port"
else
    h_fail "M8.gate /health/upstreams still on data port (got ${status_old})"
fi

h_finish
