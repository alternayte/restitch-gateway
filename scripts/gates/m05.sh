#!/usr/bin/env bash
# Gate M5 — Upstream clients, auth ownership, config hygiene
source "$(dirname "$0")/../lib/harness.sh"
h_init M5

# ── T5.1 Per-upstream client construction ────────────────────────────────────
h_task T5.1
h_run "T5.1 upstream package exists" -- \
    test -d "${REPO_ROOT}/internal/upstream"
h_run "T5.1 upstream client.go exists" -- \
    test -f "${REPO_ROOT}/internal/upstream/client.go"

# ── T5.2 Auth RoundTripper scoping ──────────────────────────────────────────
h_task T5.2
h_run "T5.2 auth tests pass" -- \
    go test -count=1 -race ./internal/auth/

# ── T5.3 OAuth2 bounded refresh ─────────────────────────────────────────────
h_task T5.3
h_run "T5.3 OAuth2 test exists" -- \
    grep -rq 'TestOAuth2' "${REPO_ROOT}/internal/auth/"

# ── T5.4 Transport config ───────────────────────────────────────────────────
h_task T5.4
h_run "T5.4 TransportConfig struct exists" -- \
    grep -q 'TransportConfig' "${REPO_ROOT}/internal/upstream/client.go"

# ── T5.5 Old packages deleted ────────────────────────────────────────────────
h_task T5.5
old_refs=$(grep -rn 'internal/client\|"internal/config"' \
    "${REPO_ROOT}/cmd/" "${REPO_ROOT}/internal/" --include='*.go' \
    | grep -v gwconfig | grep -v _test || true)
{
    echo "$ grep internal/client or internal/config (excluding gwconfig)"
    echo "${old_refs:-  (no matches)}"
} >> "${H_EVIDENCE_FILE}"
if [[ -z "${old_refs}" ]]; then
    h_pass "T5.5 old client/config packages not imported"
else
    h_fail "T5.5 old packages still referenced: ${old_refs}"
fi

# ── T5.6 Strict env expansion ───────────────────────────────────────────────
h_task T5.6
h_run "T5.6 gwconfig package exists" -- \
    test -d "${REPO_ROOT}/internal/gwconfig"

# ── M5 Verification gate ────────────────────────────────────────────────────
h_task M5.gate
h_run "M5.gate full tests (race)" -- go test -count=1 -race ./...
h_run "M5.gate go vet" -- go vet ./...

# Authorization scoping smoke: SECRET should not leak to upstreams
h_start_mockupstream

config=$(h_config m5 <<'YAML'
server:
  port: @GW_PORT@
admin:
  port: @ADMIN_PORT@
upstreams:
  mock:
    url: "http://127.0.0.1:@MOCK_PORT@"
compositions:
  e:
    path: "/e"
    method: GET
    steps:
      - name: x
        upstream: mock
        path: "/echo"
    response:
      body:
        h: "{{ steps.x.body.headers }}"
YAML
)

h_start_gateway "${config}"

# Send with Authorization header — should NOT be forwarded to upstream
body=$(curl -s -H 'Authorization: Bearer SECRET' "http://127.0.0.1:${GW_PORT}/e" 2>/dev/null) || true
{
    echo "$ curl -H 'Authorization: Bearer SECRET' /e"
    echo "${body}"
} >> "${H_EVIDENCE_FILE}"

secret_count=$(echo "${body}" | grep -c 'SECRET' || true)
if [[ "${secret_count}" -eq 0 ]]; then
    h_pass "M5.gate Authorization header not leaked to upstream"
else
    h_fail "M5.gate Authorization header leaked (found SECRET ${secret_count} times)"
fi

h_finish
