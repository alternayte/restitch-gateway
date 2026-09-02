#!/usr/bin/env bash
# Gate M11 — CLI subcommands
source "$(dirname "$0")/../lib/harness.sh"
h_init M11

# ── T11.1 Subcommand dispatch ───────────────────────────────────────────────
h_task T11.1
h_build
h_run "T11.1 version prints" -- "${REPO_ROOT}/bin/restitch" version
# Unknown command → exit 2. The exit code is captured BEFORE any `|| true`
# so the assertion below cannot be vacuously satisfied (finding H8). The `if`
# guard keeps the command off the set -e path while preserving its exit code.
if "${REPO_ROOT}/bin/restitch" nonsense > "${H_TMP}/nonsense.log" 2>&1; then
    nonsense_exit=0
else
    nonsense_exit=$?
fi
{
    echo "$ bin/restitch nonsense → exit ${nonsense_exit}"
    cat "${H_TMP}/nonsense.log"
} >> "${H_EVIDENCE_FILE}"
if [[ "${nonsense_exit}" -eq 2 ]]; then
    h_pass "T11.1 unknown command exits 2 (got ${nonsense_exit})"
else
    h_fail "T11.1 unknown command exit = ${nonsense_exit}, want 2"
fi

# ── T11.2 restitch check ────────────────────────────────────────────────────
h_task T11.2
h_run "T11.2 check restitch.yaml" -- \
    "${REPO_ROOT}/bin/restitch" check -config "${REPO_ROOT}/restitch.yaml"

# ── T11.3 restitch import openapi ────────────────────────────────────────────
h_task T11.3
if [[ -f "${REPO_ROOT}/cmd/restitch/testdata/petstore.yaml" ]]; then
    h_run "T11.3 import openapi generates YAML" -- \
        "${REPO_ROOT}/bin/restitch" import openapi \
        "${REPO_ROOT}/cmd/restitch/testdata/petstore.yaml" \
        --upstream pets -o "${H_TMP}/pets.yaml"
    if [[ -f "${H_TMP}/pets.yaml" ]]; then
        h_run "T11.3 generated YAML passes check" -- \
            "${REPO_ROOT}/bin/restitch" check -config "${H_TMP}/pets.yaml"
    fi
else
    h_skip "T11.3 import openapi (testdata/petstore.yaml not found)"
fi

# ── M11 Verification gate ───────────────────────────────────────────────────
h_task M11.gate
h_run "M11.gate cmd tests" -- go test -count=1 ./cmd/...

h_finish
