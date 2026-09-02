#!/usr/bin/env bash
# Final verification — definition of done (after M15)
source "$(dirname "$0")/../lib/harness.sh"
h_init final

# ── 1. Static ────────────────────────────────────────────────────────────────
h_task final.static
h_run "final.1 go vet" -- go vet ./...

if h_require_tool golangci-lint; then
    h_run "final.1 golangci-lint" -- golangci-lint run
else
    h_skip "final.1 golangci-lint (not installed)"
fi

# TODO/FIXME grep (informational in non-test code)
todo_output=$(grep -rn 'TODO\|FIXME' "${REPO_ROOT}/internal" "${REPO_ROOT}/cmd" \
    --include='*.go' | grep -v _test.go || true)
{
    echo "$ grep TODO/FIXME"
    echo "${todo_output:-  (none)}"
} >> "${H_EVIDENCE_FILE}"
if [[ -z "${todo_output}" ]]; then
    h_pass "final.1 no TODO/FIXME in non-test code"
else
    count=$(echo "${todo_output}" | wc -l | tr -d ' ')
    h_fail "final.1 ${count} TODO/FIXME found in non-test code"
fi

# ── 2. Tests ─────────────────────────────────────────────────────────────────
h_task final.tests
h_run "final.2 go test -race" -- go test -count=1 -race ./...
h_run "final.2 make e2e" -- make -C "${REPO_ROOT}" e2e

if [[ -f "${REPO_ROOT}/studio/package.json" ]]; then
    h_run "final.2 studio tests + build" -- \
        bash -c "cd '${REPO_ROOT}/studio' && npm ci && npm run test -- --passWithNoTests && npm run build"
else
    h_skip "final.2 studio (not found)"
fi

# ── 3. Build artifacts ───────────────────────────────────────────────────────
h_task final.build
h_run "final.3 make build-all" -- make -C "${REPO_ROOT}" build-all
h_run "final.3 restitch version" -- "${REPO_ROOT}/bin/restitch" version
h_run "final.3 quickstart check" -- \
    "${REPO_ROOT}/bin/restitch" check -config "${REPO_ROOT}/examples/quickstart/restitch.yaml"

# ── 4. Live smoke ────────────────────────────────────────────────────────────
h_task final.smoke
h_start_mockupstream

config=$(h_config final <<'YAML'
server:
  port: @GW_PORT@
admin:
  port: @ADMIN_PORT@
  api_key: "test-admin-key"
upstreams:
  mock:
    url: "http://127.0.0.1:@MOCK_PORT@"
compositions:
  user-posts:
    path: "/api/user-posts"
    method: GET
    steps:
      - name: user
        upstream: mock
        path: "/users/{{ req.query.id }}"
      - name: orders
        upstream: mock
        path: "/orders?userId={{ steps.user.body.id }}"
    response:
      body:
        user: "{{ steps.user.body }}"
        orders: "{{ steps.orders.body }}"
  echo:
    path: "/api/echo"
    method: GET
    steps:
      - name: e
        upstream: mock
        path: "/echo"
    response:
      body:
        echo: "{{ steps.e.body }}"
YAML
)

h_start_gateway "${config}"

# Basic data-plane request (matches quickstart example)
h_assert_status "http://127.0.0.1:${GW_PORT}/api/user-posts?id=1" 200 \
    "final.4 data-plane request returns 200"

# Metrics
metrics_count=$(curl -s "http://127.0.0.1:${ADMIN_PORT}/metrics" 2>/dev/null \
    | grep -c '^restitch_' || true)
{
    echo "Metric families: ${metrics_count}"
} >> "${H_EVIDENCE_FILE}"
if [[ "${metrics_count}" -ge 8 ]]; then
    h_pass "final.4 >= 8 restitch_ metric families"
else
    h_fail "final.4 only ${metrics_count} restitch_ metric families (expected >= 8)"
fi

# Admin API requests (admin key required since hardening C3)
admin_status=$(curl -s -o /dev/null -w '%{http_code}' \
    -H "X-Admin-Key: test-admin-key" \
    "http://127.0.0.1:${ADMIN_PORT}/admin/api/requests") || true
if [[ "${admin_status}" == "200" ]]; then
    h_pass "final.4 admin requests endpoint"
else
    h_fail "final.4 admin requests endpoint (got ${admin_status})"
fi

# SIGHUP
gw_pid="${H_PIDS[1]}"
if kill -0 "${gw_pid}" 2>/dev/null; then
    kill -HUP "${gw_pid}" 2>/dev/null || true
    sleep 1
    h_assert_log "gateway" "config reloaded|config unchanged" \
        "final.4 SIGHUP triggers config reload/unchanged log"
fi

# Studio proxy (if binary available). The proxy forwards the gateway admin
# key (hardening C3).
if [[ -f "${REPO_ROOT}/bin/restitch-studio" ]]; then
    h_start_studio -admin-key test-admin-key
    h_assert_status "http://127.0.0.1:${STUDIO_PORT}/api/info" 200 \
        "final.4 studio proxy works"
else
    h_skip "final.4 studio proxy (binary not built)"
fi

# ── 5. CI ────────────────────────────────────────────────────────────────────
h_task final.ci
if h_require_tool gh; then
    ci_result=$(gh run list --branch "$(git -C "${REPO_ROOT}" branch --show-current)" \
        -L1 --json conclusion -q '.[0].conclusion' 2>/dev/null) || true
    if [[ "${ci_result}" == "success" ]]; then
        h_pass "final.5 CI green on GitHub"
    elif [[ -n "${ci_result}" ]]; then
        h_fail "final.5 CI not green (${ci_result})"
    else
        h_manual "final.5 CI: verify all 4 jobs green on GitHub"
    fi
else
    h_manual "final.5 CI: verify all 4 jobs green on GitHub (gh CLI not available)"
fi

h_finish
