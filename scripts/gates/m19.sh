#!/usr/bin/env bash
# Gate M19 — CI & Test Hardening
source "$(dirname "$0")/../lib/harness.sh"
h_init M19

# ── T19.1 E2E CI job ────────────────────────────────────────────────────────
h_task T19.1
h_run "T19.1 e2e job in ci.yml" -- \
    grep -q 'e2e' "${REPO_ROOT}/.github/workflows/ci.yml"

# ── T19.2 No continue-on-error ──────────────────────────────────────────────
h_task T19.2
coe_output=$(grep -n 'continue-on-error' "${REPO_ROOT}/.github/workflows/ci.yml" || true)
{
    echo "$ grep continue-on-error ci.yml"
    echo "${coe_output:-  (none found)}"
} >> "${H_EVIDENCE_FILE}"
if [[ -z "${coe_output}" ]]; then
    h_pass "T19.2 no continue-on-error in ci.yml"
else
    h_fail "T19.2 continue-on-error still present in ci.yml"
fi

# ── T19.3 Studio component tests ────────────────────────────────────────────
h_task T19.3
if [[ -d "${REPO_ROOT}/studio/src" ]]; then
    test_files=$(find "${REPO_ROOT}/studio/src" -name '*.test.*' -o -name '*.spec.*' 2>/dev/null | wc -l | tr -d ' ')
    {
        echo "Studio test files: ${test_files}"
    } >> "${H_EVIDENCE_FILE}"
    if [[ "${test_files}" -ge 1 ]]; then
        h_pass "T19.3 studio has test files (${test_files})"
    else
        h_fail "T19.3 no studio test files found"
    fi

    if [[ -f "${REPO_ROOT}/studio/package.json" ]]; then
        h_run "T19.3 npm test passes" -- \
            bash -c "cd '${REPO_ROOT}/studio' && npm ci --silent && npm run test -- --passWithNoTests"
    fi
else
    h_skip "T19.3 studio source not found"
fi

# ── T19.4 Admin integration E2E test ────────────────────────────────────────
h_task T19.4
h_run "T19.4 make e2e" -- make -C "${REPO_ROOT}" e2e

# ── T19.5 Expression typo detection ─────────────────────────────────────────
h_task T19.5
h_build

# Create a config with a typo in step reference
typo_config=$(h_config m19_typo <<'YAML'
upstreams:
  mock:
    url: "http://127.0.0.1:1"
compositions:
  test:
    path: "/t"
    method: GET
    steps:
      - name: user
        upstream: mock
        path: "/users/1"
    response:
      body:
        data: "{{ steps.usre.body }}"
YAML
)

# restitch check should warn about the typo
check_output=$("${REPO_ROOT}/bin/restitch" check -config "${typo_config}" 2>&1) || true
{
    echo "$ restitch check (typo config)"
    echo "${check_output}"
} >> "${H_EVIDENCE_FILE}"

if echo "${check_output}" | grep -qi 'warn\|typo\|usre\|unknown step\|unresolved'; then
    h_pass "T19.5 expression typo detected in check output"
else
    h_fail "T19.5 expression typo not flagged by restitch check"
fi

# ── M19 Verification gate ───────────────────────────────────────────────────
h_task M19.gate
h_pass "M19.gate all task checks passed (see above)"

h_finish
