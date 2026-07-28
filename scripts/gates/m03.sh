#!/usr/bin/env bash
# Gate M3 — Executor semantics and the partial-response contract
# Verifies D1/D2 fixes: partial responses, dependency skipping, error taxonomy.
source "$(dirname "$0")/../lib/harness.sh"
h_init M3

# ── T3.1 Regression test: inferred dependency skipping ───────────────────────
h_task T3.1
h_run "T3.1 TestExecutor_InferredDependencySkipped" -- \
    go test -count=1 -race ./internal/composition/ -run TestExecutor_InferredDependencySkipped

# ── T3.2 Step status model ───────────────────────────────────────────────────
h_task T3.2
h_run "T3.2 step status types exist" -- \
    grep -q 'StepFailed.*=.*"failed"' "${REPO_ROOT}/internal/composition/executor.go"
h_run "T3.2 error detail has Status field" -- \
    grep -q 'Status.*string.*json:"status"' "${REPO_ROOT}/internal/composition/errors.go"

# ── T3.3 Completeness headers + D1 regression test ──────────────────────────
h_task T3.3
h_run "T3.3 TestHandler_PartialResponse_OptionalStepInTemplate" -- \
    go test -count=1 -race ./internal/composition/ -run TestHandler_PartialResponse_OptionalStepInTemplate

# ── T3.4 Error taxonomy ─────────────────────────────────────────────────────
h_task T3.4
h_run "T3.4 RequiredStepError type exists" -- \
    grep -q 'type RequiredStepError struct' "${REPO_ROOT}/internal/composition/errors.go"
h_run "T3.4 writeError does not leak err.Error" -- \
    python3 -c "
import re, sys
with open('${REPO_ROOT}/internal/composition/handler.go') as f:
    content = f.read()
# Find writeError function — it should never pass err.Error() to the response body
# It should use sanitized messages like 'upstream error', 'upstream timeout', 'internal error'
if 'err.Error()' in content.split('func writeError')[1].split('\nfunc ')[0] if 'func writeError' in content else '':
    print('writeError leaks err.Error()', file=sys.stderr)
    sys.exit(1)
sys.exit(0)
"

# ── M3 Verification gate: full package tests ────────────────────────────────
h_task M3.gate
h_run "M3.gate composition package tests (race)" -- \
    go test -count=1 -race ./internal/composition/

# ── M3 Verification gate: live partial-response smoke ────────────────────────
h_start_mockupstream

config=$(h_config m3 <<'YAML'
server:
  port: @GW_PORT@
admin:
  port: @ADMIN_PORT@
upstreams:
  mock:
    url: "http://127.0.0.1:@MOCK_PORT@"
  dead:
    url: "http://127.0.0.1:1"
    timeout: 1s
compositions:
  partial:
    path: "/p"
    method: GET
    steps:
      - name: user
        upstream: mock
        path: "/users/1"
      - name: loyalty
        upstream: dead
        path: "/x"
        optional: true
      - name: bonus
        upstream: mock
        path: "/users/{{ steps.loyalty.body.id }}"
        optional: true
    response:
      body:
        user: "{{ steps.user.body }}"
        points: "{{ steps.loyalty.body.points }}"
YAML
)

h_start_gateway "${config}"

# Fetch the partial response
h_assert_status "http://127.0.0.1:${GW_PORT}/p" 200 "M3.gate partial response returns 200"
body="${H_LAST_BODY}"

h_assert_header "http://127.0.0.1:${GW_PORT}/p" "X-Restitch-Complete" "false" \
    "M3.gate X-Restitch-Complete: false"

# JSON assertions on the body
h_assert_json_body "${body}" 'data.get("user") is not None' \
    "M3.gate user field is populated"

h_assert_json_body "${body}" 'data.get("points") is None' \
    "M3.gate points field is null"

h_assert_json_body "${body}" '"_errors" in data and len(data["_errors"]) >= 1' \
    "M3.gate _errors array present"

h_assert_json_body "${body}" 'any(e.get("step") == "loyalty" and e.get("status") == "failed" for e in data.get("_errors", []))' \
    "M3.gate loyalty step marked failed in _errors"

h_assert_json_body "${body}" 'any(e.get("step") == "bonus" and e.get("status") == "skipped" and e.get("message") == "dependency_failed" for e in data.get("_errors", []))' \
    "M3.gate bonus step marked skipped/dependency_failed in _errors"

# Verify bonus step was never called (no request to mockupstream for it)
h_assert_no_log "gateway" '"step":"bonus".*"upstream"' \
    "M3.gate bonus step never executed (no upstream call)"

h_finish
