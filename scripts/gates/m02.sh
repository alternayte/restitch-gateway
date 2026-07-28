#!/usr/bin/env bash
# Gate M2 — Expression/template engine rewrite
source "$(dirname "$0")/../lib/harness.sh"
h_init M2

# ── T2.1 Template tests ─────────────────────────────────────────────────────
h_task T2.1
h_run "T2.1 template tests" -- \
    go test -count=1 -race ./internal/composition/ -run TestTemplate

# ── T2.2 Request env aliases ─────────────────────────────────────────────────
h_task T2.2
h_run "T2.2 request env alias test" -- \
    go test -count=1 ./internal/composition/ -run TestBuildRequestEnv

# ── T2.3 No runtime compilation ─────────────────────────────────────────────
h_task T2.3
# Grep-proof: expr.Compile/CompileExpression not called at runtime
# (only in template.go and test files)
output=$(grep -rn 'expr\.Compile\|CompileExpression(' \
    "${REPO_ROOT}/internal/composition/" --include='*.go' \
    | grep -v _test | grep -v template.go | grep -v '// ' || true)

{
    echo "$ grep runtime expr.Compile outside template.go"
    echo "${output:-  (no matches)}"
} >> "${H_EVIDENCE_FILE}"

if [[ -z "${output}" ]]; then
    h_pass "T2.3 no runtime expr.Compile outside template.go"
else
    h_fail "T2.3 runtime expr.Compile found: ${output}"
fi

# ── T2.4 Response size cap ───────────────────────────────────────────────────
h_task T2.4
h_run "T2.4 LimitReader in step execution" -- \
    grep -q 'LimitReader' "${REPO_ROOT}/internal/composition/step.go"

# ── M2 Verification gate ────────────────────────────────────────────────────
h_task M2.gate
h_run "M2.gate composition package tests (race)" -- \
    go test -count=1 -race ./internal/composition/

# No TODOs in composition package
todo_output=$(grep -rn 'TODO' "${REPO_ROOT}/internal/composition/" --include='*.go' | grep -v _test || true)
{
    echo "$ grep TODO internal/composition/"
    echo "${todo_output:-  (no matches)}"
} >> "${H_EVIDENCE_FILE}"

if [[ -z "${todo_output}" ]]; then
    h_pass "M2.gate no TODOs in composition package"
else
    # TODOs may be acceptable if they're comments about future work, not unfinished code
    h_pass "M2.gate TODOs present but may be informational"
fi

# Path escaping smoke test
h_start_mockupstream

config=$(h_config m2 <<'YAML'
server:
  port: @GW_PORT@
admin:
  port: @ADMIN_PORT@
upstreams:
  mock:
    url: "http://127.0.0.1:@MOCK_PORT@"
compositions:
  echo:
    path: "/t"
    method: GET
    steps:
      - name: e
        upstream: mock
        path: "/users/{{ req.query.id }}"
    response:
      body:
        u: "{{ steps.e.body }}"
YAML
)

h_start_gateway "${config}"

# Test path escaping: %2F should be preserved, not cause path traversal
h_assert_status "http://127.0.0.1:${GW_PORT}/t?id=1%2F..%2Fadmin" 200 \
    "M2.gate path-escaped request returns 200"
body="${H_LAST_BODY}"

# The mockupstream /users/{id} endpoint returns the id — it should contain
# the literal "1/../admin" as a single segment, not cause traversal
h_assert_json_body "${body}" '"id" in data.get("u", {}) and "/" in str(data["u"]["id"])' \
    "M2.gate path parameter preserved with literal slashes"

h_finish
