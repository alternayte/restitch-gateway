#!/usr/bin/env bash
# Gate M15 — Docs, examples, packaging
source "$(dirname "$0")/../lib/harness.sh"
h_init M15

# ── T15.1 README + readme test ───────────────────────────────────────────────
h_task T15.1
h_run "T15.1 README.md exists" -- test -f "${REPO_ROOT}/README.md"
h_run "T15.1 readme_test.go exists" -- test -f "${REPO_ROOT}/tests/readme_test.go"

# ── T15.2 docs/ + examples/ ─────────────────────────────────────────────────
h_task T15.2
h_run "T15.2 docs directory populated" -- \
    bash -c "test \$(find '${REPO_ROOT}/docs' -name '*.md' -not -path '*/superpowers/*' -not -path '*/plan-progress/*' | wc -l) -ge 5"
h_run "T15.2 examples directory exists" -- test -d "${REPO_ROOT}/examples"
h_run "T15.2 examples_test.go exists" -- test -f "${REPO_ROOT}/tests/examples_test.go"

# ── T15.3 Dockerfile ────────────────────────────────────────────────────────
h_task T15.3
h_run "T15.3 Dockerfile exists" -- test -f "${REPO_ROOT}/Dockerfile"

# ── T15.4 Project docs ──────────────────────────────────────────────────────
h_task T15.4
h_run "T15.4 CHANGELOG.md exists" -- test -f "${REPO_ROOT}/CHANGELOG.md"

# ── M15 Verification gate ───────────────────────────────────────────────────
h_task M15.gate
h_run "M15.gate make e2e (includes readme + examples tests)" -- \
    make -C "${REPO_ROOT}" e2e

h_build
h_run "M15.gate quickstart config check" -- \
    "${REPO_ROOT}/bin/restitch" check -config "${REPO_ROOT}/examples/quickstart/restitch.yaml"

# Docker build
if h_require_tool docker; then
    h_run "M15.gate docker build" -- make -C "${REPO_ROOT}" docker
else
    h_manual "M15 docker build (docker not available locally, verified by CI)"
fi

h_finish
