#!/usr/bin/env bash
# Gate M18 — OpenTelemetry Tracing
source "$(dirname "$0")/../lib/harness.sh"
h_init M18

# ── T18.1 OTel SDK ──────────────────────────────────────────────────────────
h_task T18.1
h_run "T18.1 OTel dependency" -- grep -q 'go.opentelemetry.io/otel' "${REPO_ROOT}/go.mod"
h_run "T18.1 tracing setup exists" -- \
    grep -rq 'SetupTracing\|InitTracing' "${REPO_ROOT}/internal/observability/"

# ── T18.2 Step child spans ───────────────────────────────────────────────────
h_task T18.2
h_run "T18.2 span creation in executor or step" -- \
    grep -rq 'StartSpan\|Start(ctx\|tracer\.Start' "${REPO_ROOT}/internal/composition/"

# ── T18.3 Trace propagation ─────────────────────────────────────────────────
h_task T18.3
h_run "T18.3 traceparent propagation" -- \
    grep -rq 'traceparent\|otelhttp\|Inject' "${REPO_ROOT}/internal/"

# ── T18.4 OTLP exporter config ──────────────────────────────────────────────
h_task T18.4
h_run "T18.4 OTLP exporter in code" -- \
    grep -rq 'otlptrace\|OTEL_EXPORTER' "${REPO_ROOT}/internal/observability/"

# ── T18.5 Trace ID in request records ────────────────────────────────────────
h_task T18.5
h_run "T18.5 trace_id field exists" -- \
    grep -rq 'TraceID\|trace_id' "${REPO_ROOT}/internal/reqlog/" "${REPO_ROOT}/internal/admin/"

# ── M18 Verification gate ───────────────────────────────────────────────────
h_task M18.gate

# Start OTLP sink to capture trace exports
h_start_otlp_sink

h_start_mockupstream

config=$(h_config m18 <<'YAML'
server:
  port: @GW_PORT@
admin:
  port: @ADMIN_PORT@
  api_key: "test-admin-key"
upstreams:
  mock:
    url: "http://127.0.0.1:@MOCK_PORT@"
compositions:
  echo:
    path: "/echo"
    method: GET
    steps:
      - name: e
        upstream: mock
        path: "/echo"
    response:
      body:
        result: "{{ steps.e.body }}"
YAML
)

# Start gateway with OTel env vars pointing at our sink
export OTEL_EXPORTER_OTLP_ENDPOINT="${OTLP_SINK_URL}"
export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"
export OTEL_SERVICE_NAME="restitch-test"
h_start_gateway "${config}"

# Make a request to generate a trace.
# OTel BatchSpanProcessor has a 5s default schedule delay, so wait 6s.
curl -s "http://127.0.0.1:${GW_PORT}/echo" > /dev/null 2>&1
sleep 6

# Check OTLP sink received trace data
if [[ -f "${H_TMP}/otlp.log" ]]; then
    otlp_hits=$(grep -c 'POST /v1/traces' "${H_TMP}/otlp.log" 2>/dev/null || echo "0")
    {
        echo "OTLP sink received ${otlp_hits} trace export(s)"
        cat "${H_TMP}/otlp.log" 2>/dev/null || true
    } >> "${H_EVIDENCE_FILE}"

    if [[ "${otlp_hits}" -ge 1 ]]; then
        h_pass "M18.gate OTLP sink received trace exports"
    else
        h_fail "M18.gate OTLP sink received no trace exports"
    fi
else
    h_fail "M18.gate OTLP sink log not found"
fi

# Check traceparent propagation via /echo
echo_body=$(curl -s "http://127.0.0.1:${GW_PORT}/echo" 2>/dev/null) || true
{
    echo "Echo body: ${echo_body}"
} >> "${H_EVIDENCE_FILE}"
if echo "${echo_body}" | grep -qi 'traceparent'; then
    h_pass "M18.gate traceparent propagated to upstream"
else
    h_fail "M18.gate traceparent not found in echo response"
fi

# Check trace_id in admin request records (admin key required since C3)
sleep 1
requests_body=$(curl -s -H "X-Admin-Key: test-admin-key" \
    "http://127.0.0.1:${ADMIN_PORT}/admin/api/requests" 2>/dev/null) || true
if echo "${requests_body}" | grep -qi 'trace_id'; then
    h_pass "M18.gate trace_id present in admin request records"
else
    h_fail "M18.gate trace_id not found in admin request records"
fi

h_finish
