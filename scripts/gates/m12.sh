#!/usr/bin/env bash
# Gate M12 — Studio backend
source "$(dirname "$0")/../lib/harness.sh"
h_init M12

# ── T12.1 Studio server ─────────────────────────────────────────────────────
h_task T12.1
h_run "T12.1 studio server tests" -- \
    go test -count=1 ./cmd/restitch-studio/
h_run "T12.1 studio binary builds" -- \
    go build -o "${H_TMP}/restitch-studio" ./cmd/restitch-studio

# ── M12 Verification gate ───────────────────────────────────────────────────
h_task M12.gate

h_start_mockupstream

config=$(h_config m12 <<'YAML'
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
h_start_studio

# Studio proxies /api/info
h_assert_status "http://127.0.0.1:${STUDIO_PORT}/api/info" 200 \
    "M12.gate studio proxies /api/info"

# Studio serves HTML at /
root_body=$(curl -s "http://127.0.0.1:${STUDIO_PORT}/" 2>/dev/null) || true
{
    echo "$ curl studio /"
    echo "${root_body}" | head -5
} >> "${H_EVIDENCE_FILE}"
if echo "${root_body}" | grep -qi 'html\|studio\|restitch'; then
    h_pass "M12.gate studio serves HTML at root"
else
    h_fail "M12.gate studio root is not HTML"
fi

h_finish
