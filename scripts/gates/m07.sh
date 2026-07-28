#!/usr/bin/env bash
# Gate M7 — Per-step response caching
source "$(dirname "$0")/../lib/harness.sh"
h_init M7

# ── T7.1 TTL cache ──────────────────────────────────────────────────────────
h_task T7.1
h_run "T7.1 cache.go exists" -- test -f "${REPO_ROOT}/internal/upstream/cache.go"
h_run "T7.1 upstream + gwconfig tests" -- \
    go test -count=1 ./internal/upstream/ ./internal/gwconfig/

# ── M7 Verification gate ────────────────────────────────────────────────────
h_task M7.gate

h_start_mockupstream

config=$(h_config m7 <<'YAML'
server:
  port: @GW_PORT@
admin:
  port: @ADMIN_PORT@
upstreams:
  mock:
    url: "http://127.0.0.1:@MOCK_PORT@"
compositions:
  cached:
    path: "/cached"
    method: GET
    steps:
      - name: u
        upstream: mock
        path: "/users/1"
        cache:
          ttl: 30s
    response:
      body:
        user: "{{ steps.u.body }}"
YAML
)

h_start_gateway "${config}"

# Two requests — second should hit cache
curl -s "http://127.0.0.1:${GW_PORT}/cached" > /dev/null 2>&1
sleep 0.3
curl -s "http://127.0.0.1:${GW_PORT}/cached" > /dev/null 2>&1
sleep 0.5

# Check cache metrics
h_assert_metric "http://127.0.0.1:${ADMIN_PORT}/metrics" \
    "restitch_cache_hits_total" ">=" "1" \
    "M7.gate cache hit metric >= 1"

h_assert_metric "http://127.0.0.1:${ADMIN_PORT}/metrics" \
    "restitch_cache_misses_total" ">=" "1" \
    "M7.gate cache miss metric >= 1 (first request)"

h_finish
