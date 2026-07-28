#!/usr/bin/env bash
# Gate M17 — Rate Limiting & Request Validation
source "$(dirname "$0")/../lib/harness.sh"
h_init M17

# ── T17.1 Per-composition rate limiter ───────────────────────────────────────
h_task T17.1
h_run "T17.1 ratelimit package exists" -- \
    bash -c "test -d '${REPO_ROOT}/internal/ratelimit' || grep -rq 'rate_limit\|ratelimit' '${REPO_ROOT}/internal/composition/'"

# ── T17.2 Global gateway rate limiter ────────────────────────────────────────
h_task T17.2
h_run "T17.2 global rate limit in config" -- \
    grep -rq 'RateLimit\|rate_limit' "${REPO_ROOT}/internal/gwconfig/"

# ── T17.3 Request body size limit ────────────────────────────────────────────
h_task T17.3
h_run "T17.3 max_request_bytes config" -- \
    grep -rq 'MaxRequestBytes\|max_request_bytes' "${REPO_ROOT}/internal/"

# ── T17.4 JSON Schema validation ────────────────────────────────────────────
h_task T17.4
h_run "T17.4 jsonschema dependency" -- \
    grep -q 'jsonschema' "${REPO_ROOT}/go.mod"

# ── T17.5 Admin API rate limiter ─────────────────────────────────────────────
h_task T17.5

# ── M17 Verification gate ───────────────────────────────────────────────────
h_task M17.gate

h_start_mockupstream

config=$(h_config m17 <<'YAML'
server:
  port: @GW_PORT@
  rate_limit:
    requests_per_second: 2
    burst: 2
admin:
  port: @ADMIN_PORT@
upstreams:
  mock:
    url: "http://127.0.0.1:@MOCK_PORT@"
compositions:
  limited:
    path: "/limited"
    method: GET
    steps:
      - name: u
        upstream: mock
        path: "/users/1"
    response:
      body:
        user: "{{ steps.u.body }}"
YAML
)

h_start_gateway "${config}"

# Rapid-fire requests — should get a 429 eventually
got_429=false
for i in $(seq 1 10); do
    status=$(curl -s -o /dev/null -w '%{http_code}' \
        "http://127.0.0.1:${GW_PORT}/limited" 2>/dev/null) || true
    if [[ "${status}" == "429" ]]; then
        got_429=true
        break
    fi
done

{
    echo "429 seen: ${got_429}"
} >> "${H_EVIDENCE_FILE}"

if [[ "${got_429}" == "true" ]]; then
    h_pass "M17.gate rate limiter returns 429"
else
    h_fail "M17.gate rate limiter did not return 429 after 10 rapid requests"
fi

# Check 429 has Retry-After header
if [[ "${got_429}" == "true" ]]; then
    retry_header=$(curl -sI "http://127.0.0.1:${GW_PORT}/limited" 2>/dev/null \
        | grep -i 'Retry-After' || true)
    # Fire more to trigger 429 again
    for i in $(seq 1 10); do
        retry_header=$(curl -sI "http://127.0.0.1:${GW_PORT}/limited" 2>/dev/null \
            | grep -i 'Retry-After' || true)
        [[ -n "${retry_header}" ]] && break
    done
    {
        echo "Retry-After: ${retry_header}"
    } >> "${H_EVIDENCE_FILE}"
    if [[ -n "${retry_header}" ]]; then
        h_pass "M17.gate 429 includes Retry-After header"
    else
        h_fail "M17.gate 429 missing Retry-After header"
    fi
fi

h_finish
