#!/usr/bin/env bash
# Gate M22 — Dev Mode Orchestrator
set -euo pipefail
source "$(dirname "$0")/../lib/harness.sh"
h_init M22

# ── T22.1: PrefixWriter + WaitForHealth ─────────────────────────────
h_task T22.1
h_run "writer.go exists" -- test -f internal/devmode/writer.go
h_run "writer_test.go exists" -- test -f internal/devmode/writer_test.go
h_run "PrefixWriter defined" -- grep -q 'type PrefixWriter struct' internal/devmode/writer.go
h_run "WaitForHealth defined" -- grep -q 'func WaitForHealth' internal/devmode/writer.go
h_run "NO_COLOR support" -- grep -q 'NO_COLOR' internal/devmode/writer.go

# ── T22.2: ProcessManager ───────────────────────────────────────────
h_task T22.2
h_run "manager.go exists" -- test -f internal/devmode/manager.go
h_run "manager_test.go exists" -- test -f internal/devmode/manager_test.go
h_run "ProcessConfig defined" -- grep -q 'type ProcessConfig struct' internal/devmode/manager.go
h_run "ProcessManager defined" -- grep -q 'type ProcessManager struct' internal/devmode/manager.go
h_run "uses cenkalti/backoff" -- grep -q 'cenkalti/backoff' internal/devmode/manager.go

# ── T22.3: Backoff consolidation ────────────────────────────────────
h_task T22.3
h_run "backoff.go deleted" -- test ! -f internal/hotreload/backoff.go
h_run "backoff_test.go deleted" -- test ! -f internal/hotreload/backoff_test.go
h_run "no backoffDuration references" -- \
  bash -c '! grep -rn "backoffDuration" internal/'
h_run "poller uses cenkalti" -- grep -q 'cenkalti/backoff' internal/hotreload/poller.go

# ── T22.4: CLI dev command ──────────────────────────────────────────
h_task T22.4
h_run "dev.go exists" -- test -f cmd/restitch/dev.go
h_run "devCmd function" -- grep -q 'func devCmd' cmd/restitch/dev.go
h_run "findStudioBinary function" -- grep -q 'func findStudioBinary' cmd/restitch/dev.go
h_run "dev case in main.go" -- grep -q '"dev"' cmd/restitch/main.go
h_run "usage updated" -- grep -q 'dev' cmd/restitch/main.go

# ── T22.5: child-process arg passthrough ────────────────────────────
h_task T22.5
h_run "--gateway-args flag defined" -- grep -q '"gateway-args"' cmd/restitch/dev.go
h_run "--studio-args flag defined" -- grep -q '"studio-args"' cmd/restitch/dev.go
# The unit tests below exercise pure functions, so they cannot prove runDev
# actually calls them. These two greps are the wiring proof.
h_run "buildGatewayArgs wired into ProcessConfig" -- \
  grep -q 'Args:.*buildGatewayArgs(' cmd/restitch/dev.go
h_run "buildStudioArgs wired into ProcessConfig" -- \
  grep -q 'Args:.*buildStudioArgs(' cmd/restitch/dev.go
# Behavioural proof, not presence: these tests assert the FULL arg slice
# including order, so a builder that prepends instead of appends fails here.
# The -v output is checked for both PASS lines because `go test -run` with a
# pattern matching nothing exits 0 — the vacuity hole M23 hit.
h_run "arg-builder tests pass (non-vacuously)" -- bash -c '
  out="$(go test -count=1 -v -run "TestBuildGatewayArgs|TestBuildStudioArgs" ./cmd/restitch/ 2>&1)"
  echo "$out"
  echo "$out" | grep -q -- "--- PASS: TestBuildGatewayArgs" || exit 1
  echo "$out" | grep -q -- "--- PASS: TestBuildStudioArgs" || exit 1
  ! echo "$out" | grep -q -- "--- FAIL"
'

# ── Unit tests ──────────────────────────────────────────────────────
h_task M22.unit
h_run "go vet" -- go vet ./...
h_run "devmode tests" -- go test -race -count=1 ./internal/devmode/...
h_run "hotreload tests" -- go test -race -count=1 ./internal/hotreload/...
h_run "full test suite" -- go test -race -count=1 ./...

# ── Build + help smoke ──────────────────────────────────────────────
h_task M22.gate
h_build
h_run "restitch dev --help exits 0" -- "${REPO_ROOT}/bin/restitch" dev --help

# MANUAL: live smoke test (spawn gateway + studio, verify colored output, Ctrl+C)
h_manual "Run 'make build-all && bin/restitch dev' — verify colored output, Ctrl+C shutdown"

h_finish
