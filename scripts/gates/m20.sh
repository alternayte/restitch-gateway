#!/usr/bin/env bash
# Gate M20 — Config Registry & Centralized Management
#
# Encodes PLAN.md "M20 Verification gate":
#   curl -X POST localhost:8090/api/v1/configs -d '{"name":"test","yaml":"..."}' -> 201
#   curl localhost:8090/api/v1/configs -> list with version history
#   curl localhost:8090/api/v1/registry/bundle -> YAML bundle with ETag header
#   # Repeat bundle request with If-None-Match -> 304
#   # Invalid YAML -> 200 with valid:false (validate endpoint always returns 200)
#
# Written before T20.1-T20.6 are implemented (CLAUDE.md rule 2): expect the
# T20.x file/test checks and the smoke test below to FAIL until those tasks
# land. That is the intended state for this gate immediately after Task 0.
source "$(dirname "$0")/../lib/harness.sh"
h_init M20

# ── T20.1 Config registry domain types ──────────────────────────────────────
h_task T20.1
h_run "T20.1 registry types file exists" -- \
    test -f "${REPO_ROOT}/internal/registry/types.go"
h_run "T20.1 registry package has tests" -- \
    bash -c "test -n \"\$(find '${REPO_ROOT}/internal/registry' -maxdepth 1 -name '*_test.go' 2>/dev/null | head -1)\""

# ── T20.2 Registry store (CRUD, SQLite/Postgres via db package) ─────────────
h_task T20.2
h_run "T20.2 registry store file exists" -- \
    test -f "${REPO_ROOT}/internal/registry/store.go"
h_run "T20.2 registry store has tests" -- \
    test -f "${REPO_ROOT}/internal/registry/store_test.go"

# ── T20.3 Validation layer (parse -> compile -> build handler) ──────────────
h_task T20.3
h_run "T20.3 registry validator file exists" -- \
    test -f "${REPO_ROOT}/internal/registry/validator.go"
h_run "T20.3 registry validator has tests" -- \
    test -f "${REPO_ROOT}/internal/registry/validator_test.go"

# ── T20.4 YAML bundle generation with ETag ──────────────────────────────────
h_task T20.4
h_run "T20.4 store has GetBundledConfig" -- \
    grep -q 'GetBundledConfig' "${REPO_ROOT}/internal/registry/store.go"

# ── T20.5 CRUD API endpoints under /api/v1/configs ───────────────────────────
h_task T20.5
h_run "T20.5 studio API handler file exists" -- \
    test -f "${REPO_ROOT}/cmd/restitch-studio/api.go"

# ── T20.6 Database migration for registry schema ────────────────────────────
h_task T20.6
h_run "T20.6 registry migration file(s) exist" -- \
    bash -c "find '${REPO_ROOT}/internal/registry' -ipath '*migrat*' 2>/dev/null | grep -q ."

# ── Unit tests ────────────────────────────────────────────────────────────────
h_task M20.unit
h_run "M20.unit go vet internal/registry" -- \
    go vet ./internal/registry/...
h_run "M20.unit internal/registry tests (race)" -- \
    go test ./internal/registry/... -count=1 -race
h_run "M20.unit cmd/restitch-studio tests" -- \
    go test ./cmd/restitch-studio/... -count=1

# ── M20 Verification gate (smoke) ───────────────────────────────────────────
h_task M20.gate

h_start_mockupstream

gw_config=$(h_config m20 <<'YAML'
server:
  port: @GW_PORT@
admin:
  port: @ADMIN_PORT@
  api_key: "test-admin-key"
upstreams:
  mock:
    url: "http://127.0.0.1:@MOCK_PORT@"
compositions:
  passthrough:
    path: "/api/passthrough"
    method: GET
    steps:
      - name: p
        upstream: mock
        path: "/users/1"
    response:
      body:
        result: "{{ steps.p.body }}"
YAML
)

h_start_gateway "${gw_config}"

db_path="${H_TMP}/test.db"
h_log "Starting studio on port ${STUDIO_PORT} with registry db ${db_path}..."
# The studio registry API requires a key and the proxy forwards the gateway
# admin key (hardening C1/C2/C3).
h_start_studio -db-path "${db_path}" \
    -registry-key test-registry-key \
    -admin-key test-admin-key

studio_url="http://127.0.0.1:${STUDIO_PORT}"
reg_key_hdr=(-H "X-Admin-Key: test-registry-key")

valid_yaml=$(cat <<'YAML'
upstreams:
  mock:
    url: "http://localhost:8081"
compositions:
  test-comp:
    path: "/test"
    method: GET
    steps:
      - name: s1
        upstream: mock
        path: "/users/1"
    response:
      body:
        result: "{{ steps.s1.body }}"
YAML
)

# Build the create-config JSON payload with the YAML content safely escaped.
create_payload="${H_TMP}/create_payload.json"
python3 -c "
import json, sys
yaml_content = sys.stdin.read()
print(json.dumps({'name': 'test-comp', 'yaml_content': yaml_content}))
" <<<"${valid_yaml}" > "${create_payload}"

# ── Create config -> 201 ─────────────────────────────────────────────────────
create_response_file="${H_TMP}/create_response.json"
create_status=$(curl -s -o "${create_response_file}" -w '%{http_code}' \
    -X POST "${studio_url}/api/v1/configs" \
    -H 'Content-Type: application/json' \
    "${reg_key_hdr[@]}" \
    --data @"${create_payload}") || true
create_body=$(cat "${create_response_file}" 2>/dev/null || true)

{
    echo "$ curl -X POST ${studio_url}/api/v1/configs"
    echo "payload=$(cat "${create_payload}")"
    echo "status=${create_status}"
    echo "body=${create_body}"
} >> "${H_EVIDENCE_FILE}"

if [[ "${create_status}" == "201" ]]; then
    h_pass "M20.gate create config returns 201"
else
    h_fail "M20.gate create config returns 201 (got ${create_status})"
fi

# ── List configs -> includes the created config ─────────────────────────────
list_body=$(curl -s "${reg_key_hdr[@]}" "${studio_url}/api/v1/configs") || true
list_status=$(curl -s -o /dev/null -w '%{http_code}' "${reg_key_hdr[@]}" "${studio_url}/api/v1/configs") || true
{
    echo "$ curl ${studio_url}/api/v1/configs"
    echo "status=${list_status}"
} >> "${H_EVIDENCE_FILE}"
if [[ "${list_status}" == "200" ]]; then
    h_pass "M20.gate list configs returns 200"
else
    h_fail "M20.gate list configs returns 200 (got ${list_status})"
fi
if echo "${list_body}" | grep -qF 'test-comp'; then
    h_pass "M20.gate list contains created config"
else
    h_fail "M20.gate list contains created config"
fi

# ── Bundle endpoint -> YAML with composition names + ETag header ────────────
bundle_headers="${H_TMP}/bundle_headers.txt"
bundle_body=$(curl -s -D "${bundle_headers}" "${reg_key_hdr[@]}" "${studio_url}/api/v1/registry/bundle") || true
{
    echo "$ curl -sD - ${studio_url}/api/v1/registry/bundle"
    echo "--- headers ---"
    cat "${bundle_headers}" 2>/dev/null
    echo "--- body ---"
    echo "${bundle_body}"
} >> "${H_EVIDENCE_FILE}"

if echo "${bundle_body}" | grep -qF 'test-comp'; then
    h_pass "M20.gate bundle contains composition name"
else
    h_fail "M20.gate bundle contains composition name"
fi

etag=$(grep -i '^etag:' "${bundle_headers}" 2>/dev/null | head -1 | sed 's/^[Ee][Tt][Aa][Gg]: *//' | tr -d '\r')
if [[ -n "${etag}" ]]; then
    h_pass "M20.gate bundle response has ETag header (${etag})"
else
    h_fail "M20.gate bundle response has ETag header"
fi

# ── Repeat bundle request with If-None-Match -> 304 ─────────────────────────
if [[ -n "${etag}" ]]; then
    conditional_status=$(curl -s -o /dev/null -w '%{http_code}' \
        -H "If-None-Match: ${etag}" \
        "${reg_key_hdr[@]}" \
        "${studio_url}/api/v1/registry/bundle") || true
    {
        echo "$ curl -H 'If-None-Match: ${etag}' ${studio_url}/api/v1/registry/bundle"
        echo "status=${conditional_status}"
    } >> "${H_EVIDENCE_FILE}"
    if [[ "${conditional_status}" == "304" ]]; then
        h_pass "M20.gate conditional bundle request returns 304"
    else
        h_fail "M20.gate conditional bundle request returns 304 (got ${conditional_status})"
    fi
else
    h_fail "M20.gate conditional bundle request returns 304 (no ETag to replay)"
fi

# ── Invalid YAML -> 200 with valid:false ──────────────────────────────────────
invalid_yaml=$(cat <<'YAML'
upstreams:
  mock
    url: "http://localhost:8081"
compositions: [this is not valid
YAML
)

validate_payload="${H_TMP}/validate_payload.json"
python3 -c "
import json, sys
yaml_content = sys.stdin.read()
print(json.dumps({'yaml_content': yaml_content}))
" <<<"${invalid_yaml}" > "${validate_payload}"

validate_response_file="${H_TMP}/validate_response.json"
validate_status=$(curl -s -o "${validate_response_file}" -w '%{http_code}' \
    -X POST "${studio_url}/api/v1/configs/validate" \
    -H 'Content-Type: application/json' \
    "${reg_key_hdr[@]}" \
    --data @"${validate_payload}") || true
validate_body=$(cat "${validate_response_file}" 2>/dev/null || true)

{
    echo "$ curl -X POST ${studio_url}/api/v1/configs/validate"
    echo "payload=$(cat "${validate_payload}")"
    echo "status=${validate_status}"
    echo "body=${validate_body}"
} >> "${H_EVIDENCE_FILE}"

if [[ "${validate_status}" == "200" ]]; then
    h_pass "M20.gate validate endpoint returns 200"
else
    h_fail "M20.gate validate endpoint returns 200 (got ${validate_status})"
fi

h_assert_json_body "${validate_body}" 'data.get("valid") is False' \
    "M20.gate invalid YAML validate response has valid:false"

# ── Existing gateway-admin proxy pass-through still works ───────────────────
# T20.5 adds studio-local handlers under /api/v1/... alongside the existing
# catch-all "/api/" -> gateway admin reverse proxy (cmd/restitch-studio/main.go
# buildMux). This checks that pre-existing proxy behavior (e.g. /api/info)
# was not broken by routing changes made to add the registry endpoints.
h_assert_status "${studio_url}/api/info" 200 \
    "M20.gate existing gateway-admin proxy pass-through (/api/info) still works"

h_finish
