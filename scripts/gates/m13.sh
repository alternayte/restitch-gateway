#!/usr/bin/env bash
# Gate M13 — Studio frontend
source "$(dirname "$0")/../lib/harness.sh"
h_init M13

# ── T13.1-T13.8 Studio scaffold + pages ─────────────────────────────────────
h_task T13.1
h_run "T13.1 studio/package.json exists" -- test -f "${REPO_ROOT}/studio/package.json"

# ── M13 Verification gate ───────────────────────────────────────────────────
h_task M13.gate

# npm test + build
if [[ -f "${REPO_ROOT}/studio/package.json" ]]; then
    h_run "M13.gate npm ci" -- bash -c "cd '${REPO_ROOT}/studio' && npm ci"
    h_run "M13.gate npm test" -- bash -c "cd '${REPO_ROOT}/studio' && npm run test -- --passWithNoTests"
    h_run "M13.gate npm build" -- bash -c "cd '${REPO_ROOT}/studio' && npm run build"
else
    h_skip "M13.gate studio not found"
fi

# Full binaries build
h_run "M13.gate make build-all" -- make -C "${REPO_ROOT}" build-all

# Browser checklist items — MANUAL
h_manual "M13 browser: dashboard tiles populate with data"
h_manual "M13 browser: composition DAG shows correct waves"
h_manual "M13 browser: request waterfall shows failed/skipped steps"
h_manual "M13 browser: Config page validates YAML"
h_manual "M13 browser: Builder generates valid YAML round-trip"

h_finish
