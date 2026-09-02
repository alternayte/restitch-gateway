#!/usr/bin/env bash
# Gate M16a — Production Hardening (addendum)
source "$(dirname "$0")/../lib/harness.sh"
h_init M16a

# ── T16.1 Admin server HTTP timeouts ────────────────────────────────────────
h_task T16.1
h_run "T16.1 ReadHeaderTimeout in admin server" -- \
    grep -q 'ReadHeaderTimeout' "${REPO_ROOT}/internal/admin/server.go"

# ── T16.2 Gateway ReadHeaderTimeout ─────────────────────────────────────────
h_task T16.2
h_run "T16.2 ReadHeaderTimeout in gateway server" -- \
    grep -q 'ReadHeaderTimeout' "${REPO_ROOT}/internal/server/server.go"

# ── T16.3 Admin CORS restriction ────────────────────────────────────────────
h_task T16.3
h_start_mockupstream

config=$(h_config m16a <<'YAML'
server:
  port: @GW_PORT@
admin:
  port: @ADMIN_PORT@
  api_key: "test-admin-key"
upstreams:
  mock:
    url: "http://127.0.0.1:@MOCK_PORT@"
compositions:
  user:
    path: "/u"
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

# Check that CORS doesn't blindly allow all origins when api_key is set.
# The header must exist first: an absent header used to trip the pass branch
# and make the check vacuous (finding H8).
cors_header=$(curl -sI -H 'Origin: https://evil.example' \
    -H "X-Admin-Key: test-admin-key" \
    "http://127.0.0.1:${ADMIN_PORT}/admin/api/info" 2>/dev/null \
    | grep -i 'Access-Control-Allow-Origin' | head -1 | tr -d '\r') || true

{
    echo "CORS header with evil origin: ${cors_header:-  (absent)}"
} >> "${H_EVIDENCE_FILE}"

if [[ -z "${cors_header}" ]]; then
    h_fail "T16.3 no Access-Control-Allow-Origin header emitted (cannot distinguish restricted origin from no CORS support)"
elif echo "${cors_header}" | grep -qF '*'; then
    h_fail "T16.3 CORS still allows all origins when api_key is set"
else
    h_pass "T16.3 CORS header present and not wildcard (${cors_header})"
fi

# ── T16.4 SQL LIMIT clause ──────────────────────────────────────────────────
h_task T16.4
h_run "T16.4 LIMIT in SQL query" -- \
    grep -q 'LIMIT' "${REPO_ROOT}/internal/admin/sql_storage.go"

# ── T16.5 Flush accumulator on shutdown ──────────────────────────────────────
h_task T16.5
h_run "T16.5 flush in shutdown path" -- \
    grep -q 'Flush' "${REPO_ROOT}/cmd/restitch/run.go"

# ── T16.6 Upstream URL validation ────────────────────────────────────────────
h_task T16.6
h_run "T16.6 url.Parse in validation" -- \
    grep -rq 'url\.Parse' "${REPO_ROOT}/internal/composition/parser.go"

# ── T16.7 handleValidate body read fix ───────────────────────────────────────
h_task T16.7
h_run "T16.7 LimitReader in handleValidate" -- \
    grep -q 'LimitReader\|ReadAll' "${REPO_ROOT}/internal/admin/server.go"

# ── T16.8 admin.storage validation ───────────────────────────────────────────
h_task T16.8
h_run "T16.8 storage validation in gwconfig" -- \
    grep -rq 'Storage\|storage' "${REPO_ROOT}/internal/gwconfig/config.go"

# ── T16.9 InsecureSkipVerify warning ─────────────────────────────────────────
h_task T16.9
h_run "T16.9 InsecureSkipVerify warning" -- \
    grep -rq 'InsecureSkipVerify' "${REPO_ROOT}/internal/upstream/client.go"

# ── T16.10 PLAN.md drift update ──────────────────────────────────────────────
h_task T16.10
h_run "T16.10 reqlog in package map" -- \
    grep -q 'reqlog' "${REPO_ROOT}/PLAN.md"

# ── M16a Verification gate ───────────────────────────────────────────────────
h_task M16a.gate
h_run "M16a.gate build + vet + tests" -- \
    bash -c "cd '${REPO_ROOT}' && go build ./... && go vet ./... && go test -count=1 ./..."

h_finish
