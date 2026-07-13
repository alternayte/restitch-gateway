#!/usr/bin/env bash
# Shared test harness library for PLAN.md verification gates.
# Source this at the top of every gate script:
#   source "$(dirname "$0")/../lib/harness.sh"
#   h_init M3

set -euo pipefail

# ── State ────────────────────────────────────────────────────────────────────
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
H_TMP=""
H_GATE=""
H_COMMIT=""
H_DATE=""
H_EVIDENCE_FILE=""
H_EVIDENCE_DIR="${REPO_ROOT}/docs/plan-progress/evidence"
H_NO_EVIDENCE="${H_NO_EVIDENCE:-false}"
H_PIDS=()
H_PASS_COUNT=0
H_FAIL_COUNT=0
H_MANUAL_COUNT=0
H_SKIP_COUNT=0
H_CURRENT_TASK=""
H_BUILT=false

# Per-task outcome tracking (parallel indexed arrays — bash 3.2 safe)
H_TASK_NAMES=()
H_TASK_STATUS=()

# Ports assigned by h_free_port
GW_PORT=""
ADMIN_PORT=""
MOCK_PORT=""
STUDIO_PORT=""

# ── Initialization ───────────────────────────────────────────────────────────
h_init() {
    H_GATE="$1"
    H_COMMIT="$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
    H_DATE="$(date +%F)"
    H_TMP="$(mktemp -d)"

    if [[ "${H_NO_EVIDENCE}" != "true" ]]; then
        mkdir -p "${H_EVIDENCE_DIR}"
        H_EVIDENCE_FILE="${H_EVIDENCE_DIR}/${H_DATE}-${H_GATE}-${H_COMMIT}.log"
    else
        H_EVIDENCE_FILE="${H_TMP}/evidence.log"
    fi

    # Start evidence log
    {
        echo "=== GATE ${H_GATE} (commit ${H_COMMIT}, ${H_DATE}) ==="
        echo ""
    } > "${H_EVIDENCE_FILE}"

    # Assign ephemeral ports
    GW_PORT="$(h_free_port)"
    ADMIN_PORT="$(h_free_port)"
    MOCK_PORT="$(h_free_port)"
    STUDIO_PORT="$(h_free_port)"

    trap h_cleanup EXIT

    echo "=== GATE ${H_GATE} (commit ${H_COMMIT}, ${H_DATE}) ==="
}

# ── Cleanup ──────────────────────────────────────────────────────────────────
h_cleanup() {
    local exit_code=$?
    # Kill all tracked background processes
    for pid in "${H_PIDS[@]+"${H_PIDS[@]}"}"; do
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
            wait "$pid" 2>/dev/null || true
        fi
    done
    # Clean up temp dir
    if [[ -n "${H_TMP}" && -d "${H_TMP}" ]]; then
        rm -rf "${H_TMP}"
    fi
    return $exit_code
}

# ── Port allocation ──────────────────────────────────────────────────────────
h_free_port() {
    python3 -c '
import socket
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
'
}

# ── Building ─────────────────────────────────────────────────────────────────
h_build() {
    if [[ "${H_BUILT}" == "true" ]]; then
        return 0
    fi
    h_log "Building binaries..."
    (cd "${REPO_ROOT}" && make build 2>&1) >> "${H_EVIDENCE_FILE}" 2>&1
    # Also build mockupstream
    (cd "${REPO_ROOT}" && go build -o bin/mockupstream ./cmd/mockupstream 2>&1) >> "${H_EVIDENCE_FILE}" 2>&1
    H_BUILT=true
}

# ── Process management ───────────────────────────────────────────────────────
h_start_mockupstream() {
    h_build
    h_log "Starting mockupstream on port ${MOCK_PORT}..."
    "${REPO_ROOT}/bin/mockupstream" -port "${MOCK_PORT}" \
        > "${H_TMP}/mockupstream.log" 2>&1 &
    local pid=$!
    H_PIDS+=("$pid")
    h_wait_for_port "${MOCK_PORT}" "mockupstream" 10
}

h_start_gateway() {
    local config="$1"
    shift
    h_build
    h_log "Starting gateway on port ${GW_PORT} (admin ${ADMIN_PORT})..."
    "${REPO_ROOT}/bin/restitch" run \
        -config "${config}" \
        -port "${GW_PORT}" \
        -log-format json \
        -log-level debug \
        "$@" \
        > "${H_TMP}/gateway.log" 2>&1 &
    local pid=$!
    H_PIDS+=("$pid")
    h_wait_for_port "${GW_PORT}" "gateway" 10
}

h_start_studio() {
    h_build
    # Build studio binary if not already built
    if [[ ! -f "${REPO_ROOT}/bin/restitch-studio" ]]; then
        (cd "${REPO_ROOT}" && go build -o bin/restitch-studio ./cmd/restitch-studio 2>&1) >> "${H_EVIDENCE_FILE}" 2>&1
    fi
    h_log "Starting studio on port ${STUDIO_PORT}..."
    "${REPO_ROOT}/bin/restitch-studio" -port "${STUDIO_PORT}" \
        -gateway-admin-url "http://127.0.0.1:${ADMIN_PORT}" \
        "$@" \
        > "${H_TMP}/studio.log" 2>&1 &
    local pid=$!
    H_PIDS+=("$pid")
    h_wait_for_port "${STUDIO_PORT}" "studio" 10
}

h_start_otlp_sink() {
    local sink_port
    sink_port="$(h_free_port)"
    OTLP_SINK_PORT="${sink_port}"
    OTLP_SINK_URL="http://127.0.0.1:${sink_port}"

    h_log "Starting OTLP sink on port ${sink_port}..."
    python3 -c "
import http.server, json, sys, threading

class OTLPHandler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(length)
        with open('${H_TMP}/otlp.log', 'a') as f:
            f.write(f'POST {self.path} Content-Length={length}\n')
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(b'{}')
    def log_message(self, format, *args):
        pass  # suppress stderr

server = http.server.HTTPServer(('127.0.0.1', ${sink_port}), OTLPHandler)
server.serve_forever()
" > "${H_TMP}/otlp_sink.log" 2>&1 &
    local pid=$!
    H_PIDS+=("$pid")
    h_wait_for_port "${sink_port}" "otlp-sink" 5
}

h_wait_for_port() {
    local port="$1" name="$2" timeout="${3:-10}"
    local deadline=$((SECONDS + timeout))
    while ! python3 -c "import socket; s=socket.socket(); s.settimeout(0.5); s.connect(('127.0.0.1', ${port})); s.close()" 2>/dev/null; do
        if [[ $SECONDS -ge $deadline ]]; then
            echo "FATAL: ${name} failed to start on port ${port} within ${timeout}s" >&2
            if [[ -f "${H_TMP}/${name}.log" ]]; then
                echo "--- ${name} log ---" >> "${H_EVIDENCE_FILE}"
                tail -20 "${H_TMP}/${name}.log" >> "${H_EVIDENCE_FILE}" 2>/dev/null || true
            fi
            h_fail "${name} failed to start on port ${port} within ${timeout}s"
            # This is truly fatal — can't continue gate without the process
            h_finish
        fi
        sleep 0.2
    done
}

# ── Config generation ────────────────────────────────────────────────────────
h_config() {
    local name="$1"
    local content
    content="$(cat)"  # read heredoc from stdin
    # Substitute port tokens
    content="${content//@MOCK_PORT@/${MOCK_PORT}}"
    content="${content//@GW_PORT@/${GW_PORT}}"
    content="${content//@ADMIN_PORT@/${ADMIN_PORT}}"
    content="${content//@STUDIO_PORT@/${STUDIO_PORT}}"
    echo "${content}" > "${H_TMP}/${name}.yaml"
    echo "${H_TMP}/${name}.yaml"
}

# ── Task tracking ────────────────────────────────────────────────────────────
h_task() {
    H_CURRENT_TASK="$1"
    H_TASK_NAMES+=("$1")
    H_TASK_STATUS+=("NONE")
    {
        echo ""
        echo "##[${H_CURRENT_TASK}]"
    } >> "${H_EVIDENCE_FILE}"
}

# Upgrade the current task's status (severity: FAIL > MANUAL > PASS > NONE)
_h_upgrade_task_status() {
    local new_status="$1"
    local idx=$((${#H_TASK_NAMES[@]} - 1))
    if [[ $idx -lt 0 ]]; then
        return
    fi
    local current="${H_TASK_STATUS[$idx]}"
    case "${current}" in
        FAIL) ;; # FAIL is highest severity, never downgrade
        MANUAL)
            if [[ "${new_status}" == "FAIL" ]]; then
                H_TASK_STATUS[$idx]="FAIL"
            fi
            ;;
        PASS)
            if [[ "${new_status}" == "FAIL" || "${new_status}" == "MANUAL" ]]; then
                H_TASK_STATUS[$idx]="${new_status}"
            fi
            ;;
        NONE)
            H_TASK_STATUS[$idx]="${new_status}"
            ;;
    esac
}

# ── Logging ──────────────────────────────────────────────────────────────────
h_log() {
    echo "  ... $*"
    echo "# $*" >> "${H_EVIDENCE_FILE}"
}

h_evidence() {
    echo "$*" >> "${H_EVIDENCE_FILE}"
}

# ── Status reporting ─────────────────────────────────────────────────────────
h_pass() {
    H_PASS_COUNT=$((H_PASS_COUNT + 1))
    _h_upgrade_task_status "PASS"
    echo "PASS $*"
    echo "PASS $*" >> "${H_EVIDENCE_FILE}"
}

h_fail() {
    H_FAIL_COUNT=$((H_FAIL_COUNT + 1))
    _h_upgrade_task_status "FAIL"
    echo "FAIL $*"
    echo "FAIL $*" >> "${H_EVIDENCE_FILE}"
}

h_manual() {
    H_MANUAL_COUNT=$((H_MANUAL_COUNT + 1))
    _h_upgrade_task_status "MANUAL"
    echo "MANUAL $*"
    echo "MANUAL $*" >> "${H_EVIDENCE_FILE}"
}

h_skip() {
    H_SKIP_COUNT=$((H_SKIP_COUNT + 1))
    echo "SKIP $*"
    echo "SKIP $*" >> "${H_EVIDENCE_FILE}"
}

# ── Assertions ───────────────────────────────────────────────────────────────

# Run a command, PASS if exit 0, FAIL otherwise.
# Never aborts the gate script — failures are recorded, not fatal.
# Usage: h_run "description" -- cmd arg1 arg2
h_run() {
    local desc="$1"
    shift
    if [[ "$1" == "--" ]]; then shift; fi

    local output exit_code=0
    output=$("$@" 2>&1) || exit_code=$?

    {
        echo "$ $*"
        echo "${output}"
        echo "exit_code=${exit_code}"
    } >> "${H_EVIDENCE_FILE}"

    if [[ $exit_code -eq 0 ]]; then
        h_pass "${desc}"
    else
        h_fail "${desc}"
    fi
    return 0
}

# Assert HTTP status code. Saves the response body to $H_LAST_BODY.
# Usage: h_assert_status "http://..." 200 "desc"
#        body="$H_LAST_BODY"
H_LAST_BODY=""
h_assert_status() {
    local url="$1" expected="$2" desc="${3:-status ${expected} from ${url}}"
    local status
    local tmpfile="${H_TMP}/curl_response_$$"

    status=$(curl -s -o "${tmpfile}" -w '%{http_code}' "${url}" 2>/dev/null) || true
    H_LAST_BODY=$(cat "${tmpfile}" 2>/dev/null || true)
    rm -f "${tmpfile}"

    {
        echo "$ curl -s -w '%{http_code}' ${url}"
        echo "status=${status}"
        echo "body=${H_LAST_BODY}"
    } >> "${H_EVIDENCE_FILE}"

    if [[ "${status}" == "${expected}" ]]; then
        h_pass "${desc}"
    else
        h_fail "${desc} (got ${status}, expected ${expected})"
    fi
}

# Assert an HTTP response header value.
# Usage: h_assert_header "http://..." "X-Header" "expected-value" "desc"
h_assert_header() {
    local url="$1" header="$2" expected="$3" desc="${4:-header ${header}=${expected}}"
    local headers
    headers=$(curl -sI "${url}" 2>/dev/null) || true

    {
        echo "$ curl -sI ${url}"
        echo "${headers}"
    } >> "${H_EVIDENCE_FILE}"

    local value
    value=$(echo "${headers}" | grep -i "^${header}:" | head -1 | sed 's/^[^:]*: *//' | tr -d '\r') || true

    if [[ "${value}" == "${expected}" ]]; then
        h_pass "${desc}"
    else
        h_fail "${desc} (got '${value}', expected '${expected}')"
    fi
}

# Assert a Python expression against JSON from a URL.
# Usage: h_assert_json "http://..." 'data["key"] == "value"' "desc"
h_assert_json() {
    local url="$1" expr="$2" desc="${3:-json assertion at ${url}}"
    local body
    body=$(curl -s "${url}" 2>/dev/null) || true

    {
        echo "$ curl -s ${url}"
        echo "${body}"
        echo "assert: ${expr}"
    } >> "${H_EVIDENCE_FILE}"

    local result exit_code=0
    result=$(python3 -c "
import json, sys
try:
    data = json.loads('''${body}''')
    result = ${expr}
    if result:
        sys.exit(0)
    else:
        print(f'Expression evaluated to: {result}', file=sys.stderr)
        sys.exit(1)
except Exception as e:
    print(f'Error: {e}', file=sys.stderr)
    sys.exit(1)
" 2>&1) || exit_code=$?

    if [[ $exit_code -eq 0 ]]; then
        h_pass "${desc}"
    else
        h_fail "${desc} (${result})"
    fi
    return 0
}

# Assert a JSON body (passed as string) matches a Python expression.
# Usage: h_assert_json_body "$body" 'data.get("key") == "val"' "desc"
h_assert_json_body() {
    local body="$1" expr="$2" desc="${3:-json body assertion}"

    {
        echo "body=${body}"
        echo "assert: ${expr}"
    } >> "${H_EVIDENCE_FILE}"

    # Write body to temp file to avoid shell quoting issues
    local tmpfile="${H_TMP}/json_assert_$$"
    echo "${body}" > "${tmpfile}"

    local result exit_code=0
    result=$(python3 -c "
import json, sys
try:
    with open('${tmpfile}') as f:
        data = json.load(f)
    result = ${expr}
    if result:
        sys.exit(0)
    else:
        print(f'Expression evaluated to: {result}', file=sys.stderr)
        sys.exit(1)
except Exception as e:
    print(f'Error: {e}', file=sys.stderr)
    sys.exit(1)
" 2>&1) || exit_code=$?

    rm -f "${tmpfile}"
    if [[ $exit_code -eq 0 ]]; then
        h_pass "${desc}"
    else
        h_fail "${desc} (${result})"
    fi
    return 0
}

# Assert response body contains a substring.
# Usage: h_assert_body_contains "http://..." "substring" "desc"
h_assert_body_contains() {
    local url="$1" substr="$2" desc="${3:-body contains '${substr}'}"
    local body
    body=$(curl -s "${url}" 2>/dev/null) || true

    {
        echo "$ curl -s ${url}"
        echo "${body}"
    } >> "${H_EVIDENCE_FILE}"

    if echo "${body}" | grep -qF "${substr}"; then
        h_pass "${desc}"
    else
        h_fail "${desc}"
    fi
}

# Assert a Prometheus metric value.
# Usage: h_assert_metric "http://admin/metrics" "metric_name" ">=" "1" "desc"
h_assert_metric() {
    local url="$1" metric="$2" cmp="$3" expected="$4" desc="${5:-metric ${metric} ${cmp} ${expected}}"
    local metrics_body
    metrics_body=$(curl -s "${url}" 2>/dev/null) || true

    local value
    value=$(echo "${metrics_body}" | grep "^${metric}" | head -1 | awk '{print $NF}') || true

    {
        echo "$ curl -s ${url} | grep ${metric}"
        echo "value=${value}"
        echo "expected: ${cmp} ${expected}"
    } >> "${H_EVIDENCE_FILE}"

    if [[ -z "${value}" ]]; then
        h_fail "${desc} (metric not found)"
        return 0
    fi

    local exit_code=0
    python3 -c "
import sys
v = float('${value}')
e = float('${expected}')
ops = {'>=': v >= e, '<=': v <= e, '==': v == e, '>': v > e, '<': v < e, '!=': v != e}
sys.exit(0 if ops.get('${cmp}', False) else 1)
" 2>&1 || exit_code=$?

    if [[ $exit_code -eq 0 ]]; then
        h_pass "${desc}"
    else
        h_fail "${desc} (got ${value})"
    fi
    return 0
}

# Assert a regex matches in a process log file.
# Usage: h_assert_log "gateway" "config reloaded" "desc"
h_assert_log() {
    local proc="$1" pattern="$2" desc="${3:-log contains '${pattern}'}"
    local logfile="${H_TMP}/${proc}.log"

    if [[ ! -f "${logfile}" ]]; then
        h_fail "${desc} (log file not found: ${logfile})"
        return 0
    fi

    {
        echo "$ grep -E '${pattern}' ${logfile}"
    } >> "${H_EVIDENCE_FILE}"

    if grep -qE "${pattern}" "${logfile}"; then
        grep -E "${pattern}" "${logfile}" | head -3 >> "${H_EVIDENCE_FILE}"
        h_pass "${desc}"
    else
        h_fail "${desc}"
        echo "--- last 10 lines of ${proc}.log ---" >> "${H_EVIDENCE_FILE}"
        tail -10 "${logfile}" >> "${H_EVIDENCE_FILE}" 2>/dev/null || true
    fi
}

# Assert a regex does NOT match in a process log file.
# Usage: h_assert_no_log "gateway" "bonus.*upstream" "desc"
h_assert_no_log() {
    local proc="$1" pattern="$2" desc="${3:-log does not contain '${pattern}'}"
    local logfile="${H_TMP}/${proc}.log"

    if [[ ! -f "${logfile}" ]]; then
        h_pass "${desc} (log file not found — no match possible)"
        return 0
    fi

    {
        echo "$ grep -E '${pattern}' ${logfile} (expecting no match)"
    } >> "${H_EVIDENCE_FILE}"

    if grep -qE "${pattern}" "${logfile}"; then
        h_fail "${desc}"
        grep -E "${pattern}" "${logfile}" | head -3 >> "${H_EVIDENCE_FILE}"
    else
        h_pass "${desc}"
    fi
}

# Assert a grep on the repo matches or doesn't.
# Usage: h_grep_repo "pattern" "path/glob" --empty "desc"
#    or: h_grep_repo "pattern" "path/glob" --matches 3 "desc"
h_grep_repo() {
    local pattern="$1" paths="$2" mode="$3"
    shift 3

    local count_expected="" desc=""
    if [[ "${mode}" == "--empty" ]]; then
        count_expected=0
        desc="${1:-grep '${pattern}' in ${paths} is empty}"
    elif [[ "${mode}" == "--matches" ]]; then
        count_expected="$1"
        shift
        desc="${1:-grep '${pattern}' in ${paths} matches ${count_expected}}"
    fi

    local output count=0
    output=$(grep -rn "${pattern}" ${REPO_ROOT}/${paths} 2>/dev/null) || true
    if [[ -n "${output}" ]]; then
        count=$(echo "${output}" | wc -l | tr -d ' ')
    fi

    {
        echo "$ grep -rn '${pattern}' ${paths}"
        echo "${output:-  (no matches)}"
        echo "count=${count}"
    } >> "${H_EVIDENCE_FILE}"

    if [[ "${count_expected}" == "0" && "${count}" == "0" ]]; then
        h_pass "${desc}"
    elif [[ "${count_expected}" == "0" && "${count}" != "0" ]]; then
        h_fail "${desc} (got ${count} matches)"
    elif [[ "${count_expected}" != "0" && "${count}" -ge "${count_expected}" ]]; then
        h_pass "${desc}"
    else
        h_fail "${desc} (got ${count}, expected >= ${count_expected})"
    fi
}

# ── Tool checks ──────────────────────────────────────────────────────────────
h_require_tool() {
    local tool="$1" desc="${2:-${1}}"
    if command -v "${tool}" &>/dev/null; then
        return 0
    else
        return 1
    fi
}

# ── Finish ───────────────────────────────────────────────────────────────────
h_finish() {
    local total=$((H_PASS_COUNT + H_FAIL_COUNT + H_MANUAL_COUNT + H_SKIP_COUNT))
    local result="PASS"
    if [[ ${H_FAIL_COUNT} -gt 0 ]]; then
        result="FAIL"
    fi

    echo ""
    echo "RESULT ${H_GATE}: ${result} (${H_PASS_COUNT} pass, ${H_FAIL_COUNT} fail, ${H_MANUAL_COUNT} manual, ${H_SKIP_COUNT} skip)"

    {
        echo ""
        echo "RESULT ${H_GATE}: ${result} (${H_PASS_COUNT} pass, ${H_FAIL_COUNT} fail, ${H_MANUAL_COUNT} manual, ${H_SKIP_COUNT} skip)"
    } >> "${H_EVIDENCE_FILE}"

    if [[ ${H_MANUAL_COUNT} -gt 0 ]]; then
        echo ""
        echo "  ┌─────────────────────────────────────────────────────────────┐"
        echo "  │  ${H_MANUAL_COUNT} MANUAL item(s) require human verification.            │"
        echo "  │  Review the evidence log and confirm with the user.         │"
        echo "  └─────────────────────────────────────────────────────────────┘"
    fi

    # Auto-append ledger rows + evidence INDEX when not in dry-run mode
    if [[ "${H_NO_EVIDENCE}" != "true" ]]; then
        local ledger_file="${REPO_ROOT}/docs/plan-progress/LEDGER.md"
        local evidence_basename
        evidence_basename="$(basename "${H_EVIDENCE_FILE}")"

        # Append one ledger row per tracked task
        if [[ -f "${ledger_file}" ]]; then
            local i
            for i in $(seq 0 $((${#H_TASK_NAMES[@]} - 1))); do
                local task_name="${H_TASK_NAMES[$i]}"
                local task_status="${H_TASK_STATUS[$i]}"
                local ledger_status=""
                case "${task_status}" in
                    PASS) ledger_status="PASS" ;;
                    FAIL) ledger_status="FAIL" ;;
                    MANUAL) ledger_status="PENDING" ;;
                    NONE) continue ;; # no assertions ran for this task
                esac
                echo "| ${H_DATE} | ${task_name} | ${H_GATE} | ${ledger_status} | ${H_COMMIT} | evidence/${evidence_basename}#${task_name} | auto (h_finish) |" >> "${ledger_file}"
            done
        fi

        local index_file="${H_EVIDENCE_DIR}/INDEX.md"
        echo "| ${H_DATE} | ${H_GATE} | ${H_COMMIT} | ${result} | ${evidence_basename} |" >> "${index_file}"
    fi

    if [[ ${H_FAIL_COUNT} -gt 0 ]]; then
        exit 1
    fi
    exit 0
}
