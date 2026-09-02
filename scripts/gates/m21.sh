#!/usr/bin/env bash
# Gate M21 — Gateway Registry Polling (extends M10)
#
# Encodes PLAN.md "M21 Verification gate":
#   restitch gateway --config-source=registry --registry-url=http://localhost:8090
#   # Verify it loads compositions from Studio bundle
#   curl localhost:8081/admin/api/registry/status -> last_poll, etag, count
#   # Update a config in Studio -> gateway picks it up within poll interval
#   # Kill Studio -> gateway retries with backoff, keeps serving last known config
#   # Send SIGHUP -> immediate poll
#
# Written before T21.1-T21.5 are implemented (CLAUDE.md rule 2): expect the
# T21.x file/grep checks and the smoke test below to FAIL until those tasks
# land. That is the intended state for this gate immediately after Task 0.
set -euo pipefail
source "$(dirname "$0")/../lib/harness.sh"
h_init M21

# ── Per-task file & grep checks ──────────────────────────────────────

h_task T21.1
h_run "client.go exists" -- test -f internal/hotreload/client.go
h_run "client_test.go exists" -- test -f internal/hotreload/client_test.go
h_run "FetchResult type defined" -- grep -q 'type FetchResult struct' internal/hotreload/client.go

h_task T21.2
h_run "poller.go exists" -- test -f internal/hotreload/poller.go
h_run "backoff implemented in poller" -- grep -q 'backoff' internal/hotreload/poller.go
h_run "PollStatus type defined" -- grep -q 'type PollStatus struct' internal/hotreload/poller.go
h_run "Trigger method exists" -- grep -q 'func.*Poller.*Trigger' internal/hotreload/poller.go

h_task T21.3
h_run "registry-url flag in run.go" -- grep -q 'registry-url' cmd/restitch/run.go
h_run "poll-interval flag in run.go" -- grep -q 'poll-interval' cmd/restitch/run.go
h_run "registry-key flag in run.go" -- grep -q 'registry-key' cmd/restitch/run.go
h_run "RESTITCH_REGISTRY_URL env" -- grep -q 'RESTITCH_REGISTRY_URL' cmd/restitch/run.go

h_task T21.4
h_run "RegistryStatus in admin Deps" -- grep -q 'RegistryStatus' internal/admin/server.go
h_run "registry/status endpoint" -- grep -q 'registry/status' internal/admin/server.go

h_task T21.5
h_run "triggerCh in poller" -- grep -q 'triggerCh' internal/hotreload/poller.go

# ── Unit tests ───────────────────────────────────────────────────────

h_task M21.unit
h_run "go vet hotreload" -- go vet ./internal/hotreload/...
h_run "go test hotreload" -- go test -race -count=1 ./internal/hotreload/...
h_run "go test admin" -- go test -race -count=1 ./internal/admin/...
h_run "go test full suite" -- go test -race -count=1 ./...

# ── Smoke: registry polling mode ─────────────────────────────────────

h_task M21.gate

h_start_mockupstream

# The studio registry API requires a key; the gateway polls with the same
# key (hardening C1).
h_start_studio -db-path "$H_TMP/studio.db" -registry-key test-registry-key

sleep 1

# Create a composition config in the registry via Studio API
COMP_YAML="upstreams:
  mock:
    url: \"http://localhost:${MOCK_PORT}\"
compositions:
  regtest:
    path: \"/regtest\"
    steps:
      - name: e
        upstream: mock
        path: \"/echo\"
    response:
      body:
        result: \"{{ steps.e.body }}\"
"
CREATE_RESP=$(curl -s -X POST "http://localhost:${STUDIO_PORT}/api/v1/configs" \
  -H "Content-Type: application/json" \
  -H "X-Admin-Key: test-registry-key" \
  -d "$(python3 -c "import json,sys; print(json.dumps({'name':'regtest','yaml_content':sys.stdin.read()}))" <<< "$COMP_YAML")") || true
h_evidence "config create response: ${CREATE_RESP}"
h_run "config created in registry" -- test -n "$CREATE_RESP"

# Start gateway in registry mode
h_build
GW_PID_FILE="$H_TMP/gw_reg.pid"
RESTITCH_ADMIN_PORT="$ADMIN_PORT" \
RESTITCH_ADMIN_API_KEY="test-admin-key" \
"${REPO_ROOT}/bin/restitch" run \
  -registry-url "http://localhost:${STUDIO_PORT}" \
  -registry-key "test-registry-key" \
  -poll-interval 2s \
  -port "$GW_PORT" \
  -log-format json \
  > "$H_TMP/gw_reg.log" 2>&1 &
gw_reg_pid=$!
H_PIDS+=("$gw_reg_pid")
echo "$gw_reg_pid" > "$GW_PID_FILE"
h_wait_for_port "$GW_PORT" "gw_reg" 10

# Verify gateway serves the registry composition
h_assert_status "http://localhost:${GW_PORT}/regtest" 200 "gateway serves registry composition"

# Verify admin registry status endpoint
reg_status_http=$(curl -s -o /dev/null -w '%{http_code}' \
  -H "X-Admin-Key: test-admin-key" \
  "http://localhost:${ADMIN_PORT}/admin/api/registry/status") || true
if [[ "${reg_status_http}" == "200" ]]; then
  h_pass "registry status endpoint returns 200"
else
  h_fail "registry status endpoint returns 200 (got ${reg_status_http})"
fi
REG_STATUS=$(curl -sf -H "X-Admin-Key: test-admin-key" "http://localhost:${ADMIN_PORT}/admin/api/registry/status" 2>/dev/null) || true
h_evidence "registry status: ${REG_STATUS}"
h_run "status shows registry mode" -- \
  python3 -c "import json,sys; d=json.loads(sys.argv[1]); assert d['mode']=='registry', f'mode={d[\"mode\"]}'" "$REG_STATUS"

# Verify SIGHUP triggers immediate poll
kill -HUP "$(cat "$GW_PID_FILE")" 2>/dev/null || true
sleep 2
h_run "SIGHUP logged" -- grep -q "registry poll triggered" "$H_TMP/gw_reg.log"

h_finish
