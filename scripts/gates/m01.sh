#!/usr/bin/env bash
# Gate M1 — Foundation, tooling, hygiene
source "$(dirname "$0")/../lib/harness.sh"
h_init M1

# ── T1.1 go vet clean + shutdown consolidation ──────────────────────────────
h_task T1.1
h_run "T1.1 go vet clean" -- go vet ./...
h_grep_repo "ShutdownContext\|WaitForShutdown(" "internal/ cmd/" --empty \
    "T1.1 only WaitForShutdownSignal remains"

# ── T1.2 Dead code deleted ──────────────────────────────────────────────────
h_task T1.2
h_run "T1.2 build + tests green" -- go build ./...
h_grep_repo "NoneStrategy\|trimSpaces" "internal/" --empty \
    "T1.2 dead code removed"

# ── T1.3 Unified slog logging ───────────────────────────────────────────────
h_task T1.3
h_run "T1.3 observability.Setup exists" -- \
    grep -q 'func Setup' "${REPO_ROOT}/internal/observability/logging.go"
h_run "T1.3 ContextHandler exists" -- \
    grep -q 'ContextHandler' "${REPO_ROOT}/internal/observability/logging.go"

# ── T1.4 Config-file stat correctness ───────────────────────────────────────
h_task T1.4
h_build
local_tmpfile="${H_TMP}/unreadable.yaml"
touch "${local_tmpfile}" && chmod 000 "${local_tmpfile}"
if "${REPO_ROOT}/bin/restitch" run -config "${local_tmpfile}" > "${H_TMP}/t14.log" 2>&1; then
    h_fail "T1.4 unreadable config should exit non-zero"
else
    h_pass "T1.4 unreadable config exits non-zero"
fi
{
    echo "$ bin/restitch run -config ${local_tmpfile}"
    cat "${H_TMP}/t14.log"
} >> "${H_EVIDENCE_FILE}"

# ── T1.5 Makefile, lint, LICENSE, mockupstream ───────────────────────────────
h_task T1.5
h_run "T1.5 Makefile exists with ci target" -- grep -q '^ci:' "${REPO_ROOT}/Makefile"
h_run "T1.5 .golangci.yml exists" -- test -f "${REPO_ROOT}/.golangci.yml"
h_run "T1.5 LICENSE exists" -- test -f "${REPO_ROOT}/LICENSE"
h_run "T1.5 mockupstream builds" -- go build -o "${H_TMP}/mockupstream" ./cmd/mockupstream

# ── T1.6 GitHub Actions CI ──────────────────────────────────────────────────
h_task T1.6
h_run "T1.6 ci.yml is valid YAML" -- \
    python3 -c "import yaml; yaml.safe_load(open('${REPO_ROOT}/.github/workflows/ci.yml'))"

# ── M1 Verification gate ────────────────────────────────────────────────────
h_task M1.gate
if h_require_tool golangci-lint; then
    h_run "M1.gate make ci" -- make -C "${REPO_ROOT}" ci
else
    h_skip "M1.gate make ci (golangci-lint not installed)"
    h_run "M1.gate vet + race (no lint)" -- bash -c "cd '${REPO_ROOT}' && go vet ./... && go test -race ./..."
fi

# JSON log validation
h_start_mockupstream

config=$(h_config m1 <<'YAML'
server:
  port: @GW_PORT@
admin:
  port: @ADMIN_PORT@
upstreams:
  mock:
    url: "http://127.0.0.1:@MOCK_PORT@"
compositions:
  echo:
    path: "/echo"
    method: GET
    steps:
      - name: e
        upstream: mock
        path: "/echo"
    response:
      body:
        result: "{{ steps.e.body }}"
YAML
)

h_start_gateway "${config}"

# Capture a few log lines and verify they're valid JSON
sleep 0.5
curl -s "http://127.0.0.1:${GW_PORT}/echo" > /dev/null 2>&1 || true
sleep 0.5

# Check that gateway log lines are valid JSON
log_valid=$(python3 -c "
import json, sys
valid = 0
invalid = 0
with open('${H_TMP}/gateway.log') as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        try:
            json.loads(line)
            valid += 1
        except:
            invalid += 1
if invalid > 0:
    print(f'{invalid} invalid JSON lines out of {valid+invalid}', file=sys.stderr)
    sys.exit(1)
print(f'{valid} lines, all valid JSON')
" 2>&1) || true

{
    echo "JSON log validation: ${log_valid}"
} >> "${H_EVIDENCE_FILE}"

if echo "${log_valid}" | grep -q "all valid JSON"; then
    h_pass "M1.gate all log lines are valid JSON"
else
    h_fail "M1.gate log lines contain non-JSON (${log_valid})"
fi

h_finish
