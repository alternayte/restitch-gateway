#!/usr/bin/env bash
# Gate M4 — Router replacement and path parameters
source "$(dirname "$0")/../lib/harness.sh"
h_init M4

# ── T4.1 Router tests ───────────────────────────────────────────────────────
h_task T4.1
h_run "T4.1 router tests" -- \
    go test -count=1 -race ./internal/server/ -run TestRouter

# ── T4.2 Handler: one closure per route, no duplicate matching ───────────────
h_task T4.2
# matchComposition and routeKey should be deleted
match_output=$(grep -n 'matchComposition\|routeKey' \
    "${REPO_ROOT}/internal/composition/handler.go" || true)
{
    echo "$ grep matchComposition|routeKey handler.go"
    echo "${match_output:-  (no matches)}"
} >> "${H_EVIDENCE_FILE}"

if [[ -z "${match_output}" ]]; then
    h_pass "T4.2 matchComposition/routeKey deleted from handler"
else
    h_fail "T4.2 matchComposition/routeKey still present"
fi

h_run "T4.2 RegisterRoutes exists" -- \
    grep -q 'func.*RegisterRoutes' "${REPO_ROOT}/internal/composition/handler.go"

# ── T4.3 Health/readiness fixes ──────────────────────────────────────────────
h_task T4.3
h_run "T4.3 server package tests" -- \
    go test -count=1 ./internal/server/

# ── M4 Verification gate ────────────────────────────────────────────────────
h_task M4.gate
h_run "M4.gate full tests (race)" -- go test -count=1 -race ./...

# Path parameter smoke test
h_start_mockupstream

config=$(h_config m4 <<'YAML'
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

# Path params work
h_assert_status "http://127.0.0.1:${GW_PORT}/api/users/42" 200 \
    "M4.gate path param request returns 200"
body="${H_LAST_BODY}"
h_assert_json_body "${body}" '"user" in data and data["user"].get("id") == "42"' \
    "M4.gate user-42 payload via path param"

# 405 on wrong method
status_405=$(curl -s -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${GW_PORT}/api/users/42" 2>/dev/null) || true
{
    echo "$ curl -X POST /api/users/42 → ${status_405}"
} >> "${H_EVIDENCE_FILE}"
if [[ "${status_405}" == "405" ]]; then
    h_pass "M4.gate POST returns 405"
else
    h_fail "M4.gate POST returns 405 (got ${status_405})"
fi

# HEAD returns 200
status_head=$(curl -s -o /dev/null -w '%{http_code}' -I "http://127.0.0.1:${GW_PORT}/api/users/42" 2>/dev/null) || true
{
    echo "$ curl -I /api/users/42 → ${status_head}"
} >> "${H_EVIDENCE_FILE}"
if [[ "${status_head}" == "200" ]]; then
    h_pass "M4.gate HEAD returns 200"
else
    h_fail "M4.gate HEAD returns 200 (got ${status_head})"
fi

h_finish
