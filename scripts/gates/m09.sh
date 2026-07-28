#!/usr/bin/env bash
# Gate M9 — Inbound authentication
source "$(dirname "$0")/../lib/harness.sh"
h_init M9

# ── T9.1 Inbound auth middleware ─────────────────────────────────────────────
h_task T9.1
h_run "T9.1 inbound package exists" -- test -d "${REPO_ROOT}/internal/inbound"
h_run "T9.1 inbound + composition tests" -- \
    go test -count=1 -race ./internal/inbound/ ./internal/composition/

# ── M9 Verification gate ────────────────────────────────────────────────────
h_task M9.gate
h_run "M9.gate full tests (race)" -- go test -count=1 -race ./...

h_start_mockupstream

config=$(h_config m9 <<'YAML'
server:
  port: @GW_PORT@
  auth:
    api_keys: ["test-key-123"]
admin:
  port: @ADMIN_PORT@
upstreams:
  mock:
    url: "http://127.0.0.1:@MOCK_PORT@"
compositions:
  protected:
    path: "/protected"
    method: GET
    steps:
      - name: u
        upstream: mock
        path: "/users/1"
    response:
      body:
        user: "{{ steps.u.body }}"
YAML
)

h_start_gateway "${config}"

# No key → 401
h_assert_status "http://127.0.0.1:${GW_PORT}/protected" 401 \
    "M9.gate no API key returns 401"

# With key → 200
status_ok=$(curl -s -o /dev/null -w '%{http_code}' \
    -H 'X-API-Key: test-key-123' \
    "http://127.0.0.1:${GW_PORT}/protected" 2>/dev/null) || true
{
    echo "$ curl -H 'X-API-Key: test-key-123' /protected → ${status_ok}"
} >> "${H_EVIDENCE_FILE}"
if [[ "${status_ok}" == "200" ]]; then
    h_pass "M9.gate valid API key returns 200"
else
    h_fail "M9.gate valid API key returns 200 (got ${status_ok})"
fi

# /health is always open
h_assert_status "http://127.0.0.1:${GW_PORT}/health" 200 \
    "M9.gate /health open without key"

h_finish
