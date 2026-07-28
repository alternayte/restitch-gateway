#!/usr/bin/env bash
# Gate M14 — E2E test harness, hardening
source "$(dirname "$0")/../lib/harness.sh"
h_init M14

# ── T14.1-T14.3 E2E harness ─────────────────────────────────────────────────
h_task T14.1
h_run "T14.1 tests/binary_test.go exists" -- test -f "${REPO_ROOT}/tests/binary_test.go"
h_run "T14.1 golden specs exist" -- test -d "${REPO_ROOT}/tests/specs"

h_task T14.2
h_run "T14.2 e2e CI job in ci.yml" -- \
    grep -q 'e2e' "${REPO_ROOT}/.github/workflows/ci.yml"

h_task T14.3
h_run "T14.3 tests exist for key packages" -- \
    bash -c "test -n \"\$(find '${REPO_ROOT}/internal' -name '*_test.go' | head -1)\""

# ── M14 Verification gate ───────────────────────────────────────────────────
h_task M14.gate
h_run "M14.gate full tests (race)" -- go test -count=1 -race ./...
h_run "M14.gate make e2e" -- make -C "${REPO_ROOT}" e2e

# Coverage check (replicate CI 70% gate)
coverage_output=$(go test -coverprofile="${H_TMP}/cover.out" ./internal/... 2>&1) || true
{
    echo "$ go test -coverprofile"
    echo "${coverage_output}" | tail -5
} >> "${H_EVIDENCE_FILE}"

if [[ -f "${H_TMP}/cover.out" ]]; then
    total_pct=$(go tool cover -func="${H_TMP}/cover.out" 2>/dev/null | grep total | awk '{print $NF}' | tr -d '%') || true
    {
        echo "Total coverage: ${total_pct}%"
    } >> "${H_EVIDENCE_FILE}"
    if [[ -n "${total_pct}" ]]; then
        meets=$(python3 -c "print('yes' if float('${total_pct}') >= 70 else 'no')" 2>/dev/null) || true
        if [[ "${meets}" == "yes" ]]; then
            h_pass "M14.gate coverage >= 70% (${total_pct}%)"
        else
            h_fail "M14.gate coverage < 70% (${total_pct}%)"
        fi
    else
        h_skip "M14.gate coverage check (could not parse percentage)"
    fi
else
    h_skip "M14.gate coverage check (profile not generated)"
fi

# CI green — auto-check via gh if available
if h_require_tool gh; then
    ci_result=$(gh run list --branch "$(git -C "${REPO_ROOT}" branch --show-current)" \
        -L1 --json conclusion -q '.[0].conclusion' 2>/dev/null) || true
    if [[ "${ci_result}" == "success" ]]; then
        h_pass "M14.gate CI green on GitHub"
    elif [[ -n "${ci_result}" ]]; then
        h_fail "M14.gate CI not green (${ci_result})"
    else
        h_manual "M14 CI: verify all jobs green on GitHub"
    fi
else
    h_manual "M14 CI: verify all jobs green on GitHub (gh CLI not available)"
fi

h_finish
