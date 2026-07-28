#!/usr/bin/env bash
# PLAN.md verification driver.
# Usage:
#   scripts/verify.sh M3            # run one gate
#   scripts/verify.sh all           # run all gates (continue on failure)
#   scripts/verify.sh --list        # list available gates
#   scripts/verify.sh M3 --no-evidence   # dry run (evidence to $TMP only)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATES_DIR="${SCRIPT_DIR}/gates"

# Normalize gate ID to filename: M3 → m03, M16a → m16a, final → final
normalize_gate() {
    local input="$1"
    input="$(echo "${input}" | tr '[:upper:]' '[:lower:]')"

    # Handle "final" directly
    if [[ "${input}" == "final" ]]; then
        echo "final"
        return
    fi

    # Strip leading 'm' if present, handle the suffix
    local num suffix=""
    if [[ "${input}" =~ ^m?([0-9]+)([a-z]*)$ ]]; then
        num="${BASH_REMATCH[1]}"
        suffix="${BASH_REMATCH[2]}"
        printf "m%02d%s" "${num}" "${suffix}"
    else
        echo "${input}"
    fi
}

# Get a one-line description for a gate
gate_description() {
    local gate="$1"
    case "${gate}" in
        m01) echo "Foundation, tooling, hygiene" ;;
        m02) echo "Expression/template engine rewrite" ;;
        m03) echo "Executor semantics, partial-response contract" ;;
        m04) echo "Router replacement, path parameters" ;;
        m05) echo "Upstream clients, auth, config hygiene" ;;
        m06) echo "Resilience: retries, circuit breaker, coalescing" ;;
        m07) echo "Per-step response caching" ;;
        m08) echo "Observability: metrics, admin server" ;;
        m09) echo "Inbound authentication" ;;
        m10) echo "Hot reload + pipeline swap" ;;
        m11) echo "CLI subcommands" ;;
        m12) echo "Studio backend" ;;
        m13) echo "Studio frontend" ;;
        m14) echo "E2E test harness, hardening" ;;
        m15) echo "Docs, examples, packaging" ;;
        m16a) echo "Production hardening (addendum)" ;;
        m17) echo "Rate limiting & request validation" ;;
        m18) echo "OpenTelemetry tracing" ;;
        m19) echo "CI & test hardening" ;;
        m20) echo "Config registry & centralized management [FUTURE]" ;;
        m21) echo "Gateway registry polling [FUTURE]" ;;
        m22) echo "Dev mode orchestrator [FUTURE]" ;;
        m23) echo "Upstream HTTP client optimization [FUTURE]" ;;
        m24) echo "Production monitoring & load testing [FUTURE]" ;;
        m25) echo "Browser session & user preferences [FUTURE]" ;;
        final) echo "Final verification (definition of done)" ;;
        *) echo "Unknown gate" ;;
    esac
}

# All gates in order
ALL_GATES=(
    m01 m02 m03 m04 m05 m06 m07 m08 m09 m10
    m11 m12 m13 m14 m15
    m16a m17 m18 m19
    m20 m21 m22 m23 m24 m25
    final
)

# ── Commands ─────────────────────────────────────────────────────────────────

list_gates() {
    echo "Available verification gates:"
    echo ""
    for gate in "${ALL_GATES[@]}"; do
        local script="${GATES_DIR}/${gate}.sh"
        local status="  "
        if [[ -f "${script}" ]]; then
            status="OK"
        else
            status="--"
        fi
        printf "  %-8s [%s]  %s\n" "${gate}" "${status}" "$(gate_description "${gate}")"
    done
    echo ""
    echo "  OK = gate script exists, -- = missing"
}

run_gate() {
    local gate="$1"
    local script="${GATES_DIR}/${gate}.sh"

    if [[ ! -f "${script}" ]]; then
        echo "SKIP ${gate} (no gate script at ${script})"
        return 2
    fi

    if [[ ! -x "${script}" ]]; then
        chmod +x "${script}"
    fi

    bash "${script}"
}

run_all() {
    local results=()
    local any_fail=false

    echo "Running all gates..."
    echo "════════════════════════════════════════════════════════════════"
    echo ""

    for gate in "${ALL_GATES[@]}"; do
        local script="${GATES_DIR}/${gate}.sh"
        if [[ ! -f "${script}" ]]; then
            echo "SKIP ${gate} (no gate script)"
            results+=("${gate}:SKIP")
            continue
        fi

        echo "────────────────────────────────────────────────────────────────"
        echo "  Running gate: ${gate} — $(gate_description "${gate}")"
        echo "────────────────────────────────────────────────────────────────"

        local exit_code=0
        bash "${script}" || exit_code=$?

        if [[ $exit_code -eq 0 ]]; then
            results+=("${gate}:PASS")
        else
            results+=("${gate}:FAIL")
            any_fail=true
        fi
        echo ""
    done

    # Summary table
    echo ""
    echo "════════════════════════════════════════════════════════════════"
    echo "  SUMMARY"
    echo "════════════════════════════════════════════════════════════════"
    for entry in "${results[@]}"; do
        local gate="${entry%%:*}"
        local result="${entry##*:}"
        printf "  %-8s  %s\n" "${gate}" "${result}"
    done
    echo "════════════════════════════════════════════════════════════════"

    if [[ "${any_fail}" == "true" ]]; then
        echo ""
        echo "  One or more gates FAILED."
        exit 1
    else
        echo ""
        echo "  All gates passed."
        exit 0
    fi
}

# ── Main ─────────────────────────────────────────────────────────────────────

if [[ $# -lt 1 ]]; then
    echo "Usage: scripts/verify.sh <gate|all|--list> [--no-evidence]"
    exit 2
fi

arg="$1"
shift

# Check for --no-evidence flag
for a in "$@"; do
    if [[ "${a}" == "--no-evidence" ]]; then
        export H_NO_EVIDENCE=true
    fi
done

case "${arg}" in
    --list|-l)
        list_gates
        ;;
    all)
        run_all
        ;;
    *)
        gate="$(normalize_gate "${arg}")"
        run_gate "${gate}"
        ;;
esac
