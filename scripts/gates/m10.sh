#!/usr/bin/env bash
# Gate M10 — Hot reload + pipeline swap
source "$(dirname "$0")/../lib/harness.sh"
h_init M10

# ── T10.1 Swappable pipeline ────────────────────────────────────────────────
h_task T10.1
h_run "T10.1 pipeline.go exists" -- test -f "${REPO_ROOT}/cmd/restitch/pipeline.go"
h_run "T10.1 reload.go exists" -- test -f "${REPO_ROOT}/cmd/restitch/reload.go"
h_run "T10.1 cmd/restitch tests (race)" -- \
    go test -count=1 -race ./cmd/restitch/

# ── M10 Verification gate ───────────────────────────────────────────────────
h_task M10.gate

h_start_mockupstream

# Write initial valid config to a temp file (never mutate repo files)
config=$(h_config m10 <<'YAML'
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

# Verify baseline works
h_assert_status "http://127.0.0.1:${GW_PORT}/api/users/1" 200 \
    "M10.gate baseline request returns 200"

# Corrupt the config file
echo 'garbage: [' >> "${config}"

# Try reload via admin API — should fail but keep serving
reload_body=$(curl -s -X POST "http://127.0.0.1:${ADMIN_PORT}/admin/api/reload" 2>/dev/null) || true
{
    echo "$ POST /admin/api/reload (with corrupted config)"
    echo "${reload_body}"
} >> "${H_EVIDENCE_FILE}"

# Write to temp file for JSON check
echo "${reload_body}" > "${H_TMP}/reload_check"
reload_ok=$(python3 -c "
import json, sys
with open('${H_TMP}/reload_check') as f:
    data = json.load(f)
print('false' if data.get('ok') == False else 'true')
" 2>/dev/null) || true

if [[ "${reload_ok}" == "false" ]]; then
    h_pass "M10.gate reload with bad config returns ok:false"
else
    h_fail "M10.gate reload with bad config should return ok:false (got ${reload_ok})"
fi

# Gateway still serves
h_assert_status "http://127.0.0.1:${GW_PORT}/api/users/1" 200 \
    "M10.gate still serves after failed reload"

# Restore config and test SIGHUP
# Copy original config back
h_config m10_restored <<'YAML' > /dev/null
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
cp "${H_TMP}/m10_restored.yaml" "${config}"

# Get the gateway PID and send SIGHUP
gw_pid="${H_PIDS[1]}"  # second PID is gateway (first is mockupstream)
if kill -0 "${gw_pid}" 2>/dev/null; then
    kill -HUP "${gw_pid}" 2>/dev/null || true
    sleep 2

    # Check log for reload message
    h_assert_log "gateway" "config reloaded\|config unchanged\|config file changed" \
        "M10.gate SIGHUP triggers reload log message"
else
    h_skip "M10.gate SIGHUP test (gateway process not found)"
fi

h_finish
