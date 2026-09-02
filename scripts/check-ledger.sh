#!/usr/bin/env bash
# Coverage checker: every in-scope PLAN.md task must have a green ledger row.
# Usage:
#   scripts/check-ledger.sh                    # check DONE-scope tasks (default)
#   scripts/check-ledger.sh --scope all        # check all tasks including future
#   scripts/check-ledger.sh --scope done       # same as default
#   scripts/check-ledger.sh --milestone M17    # check only M17 tasks

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PLAN_FILE="${REPO_ROOT}/PLAN.md"
LEDGER_FILE="${REPO_ROOT}/docs/plan-progress/LEDGER.md"
EVIDENCE_DIR="${REPO_ROOT}/docs/plan-progress/evidence"

SCOPE="done"
MILESTONE_FILTER=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --scope) SCOPE="$2"; shift 2 ;;
        --milestone) MILESTONE_FILTER="$2"; shift 2 ;;
        *) echo "Unknown arg: $1"; exit 2 ;;
    esac
done

# ── Step 1: Extract all task IDs from PLAN.md ────────────────────────────────
extract_task_ids() {
    {
        # Header-style: ### T<m>.<n> (M1–M15)
        grep -oE '^### T[0-9]+\.[0-9]+' "${PLAN_FILE}" | sed 's/^### //'
        # Table-style: | T<m>.<n> | (M16a–M25)
        grep -oE '^\| T[0-9]+\.[0-9]+' "${PLAN_FILE}" | sed 's/^| //'
    } | sort -t. -k1.2,1n -k2,2n | uniq
}

ALL_TASK_IDS=$(extract_task_ids)
TOTAL_TASKS=$(echo "${ALL_TASK_IDS}" | wc -l | tr -d ' ')

if [[ ${TOTAL_TASKS} -lt 70 ]]; then
    echo "FATAL: Only extracted ${TOTAL_TASKS} task IDs (expected >= 70)."
    echo "PLAN.md format may have changed — check extraction patterns."
    exit 2
fi

# ── Step 2: Determine DONE milestones from §0.1 status table ────────────────
# Parse lines like: | M1 — Foundation... | T1.1–T1.6 | DONE |
# and:              | M16 — Production... | T16.1–T16.10 | DONE |
DONE_MILESTONES=()
while IFS= read -r line; do
    # Extract milestone number from lines containing DONE
    if echo "${line}" | grep -qE 'DONE[[:space:]]*\|'; then
        local_num=$(echo "${line}" | grep -oE '\| M([0-9]+)' | head -1 | grep -oE '[0-9]+')
        if [[ -n "${local_num}" ]]; then
            DONE_MILESTONES+=("${local_num}")
        fi
    fi
done < <(grep -E '^\| M[0-9]+ ' "${PLAN_FILE}")

# Map task ID to milestone number
task_milestone() {
    local task_id="$1"
    echo "${task_id}" | sed 's/^T//' | cut -d. -f1
}

# Check if a milestone number is in the DONE set
is_done_milestone() {
    local num="$1"
    for m in "${DONE_MILESTONES[@]}"; do
        if [[ "${m}" == "${num}" ]]; then
            return 0
        fi
    done
    return 1
}

# ── Step 3: Filter tasks by scope ────────────────────────────────────────────
SCOPED_TASKS=()
while IFS= read -r task_id; do
    [[ -z "${task_id}" ]] && continue
    local_mnum=$(task_milestone "${task_id}")

    if [[ -n "${MILESTONE_FILTER}" ]]; then
        # Filter to specific milestone
        filter_num=$(echo "${MILESTONE_FILTER}" | sed 's/^[Mm]//')
        if [[ "${local_mnum}" == "${filter_num}" ]]; then
            SCOPED_TASKS+=("${task_id}")
        fi
    elif [[ "${SCOPE}" == "all" ]]; then
        SCOPED_TASKS+=("${task_id}")
    elif [[ "${SCOPE}" == "done" ]]; then
        if is_done_milestone "${local_mnum}"; then
            SCOPED_TASKS+=("${task_id}")
        fi
    fi
done <<< "${ALL_TASK_IDS}"

# Add milestone gate pseudo-IDs
GATE_IDS=()
if [[ -n "${MILESTONE_FILTER}" ]]; then
    GATE_IDS+=("${MILESTONE_FILTER}.gate")
else
    for m in "${DONE_MILESTONES[@]}"; do
        if [[ "${SCOPE}" == "done" || "${SCOPE}" == "all" ]]; then
            # Map milestone number to gate ID
            if [[ "${m}" -ge 16 && "${m}" -le 19 ]]; then
                case "${m}" in
                    16) GATE_IDS+=("M16a.gate") ;;
                    17) GATE_IDS+=("M17.gate") ;;
                    18) GATE_IDS+=("M18.gate") ;;
                    19) GATE_IDS+=("M19.gate") ;;
                esac
            else
                GATE_IDS+=("M${m}.gate")
            fi
        fi
    done
    if [[ "${SCOPE}" == "done" || "${SCOPE}" == "all" ]]; then
        GATE_IDS+=("final")
    fi
fi

echo "Scope: ${SCOPE}"
echo "Tasks in scope: ${#SCOPED_TASKS[@]}"
echo "Gate IDs in scope: ${#GATE_IDS[@]}"
echo ""

# ── Step 4: Read LEDGER.md — last row per task wins ─────────────────────────
declare -A LEDGER_STATUS
declare -A LEDGER_EVIDENCE

if [[ -f "${LEDGER_FILE}" ]]; then
    while IFS='|' read -r _ date task milestone status commit evidence notes _; do
        # Skip header/separator rows
        task=$(echo "${task}" | xargs)
        [[ -z "${task}" || "${task}" == "Task" || "${task}" == "---"* ]] && continue
        status=$(echo "${status}" | xargs)
        evidence=$(echo "${evidence}" | xargs)
        LEDGER_STATUS["${task}"]="${status}"
        LEDGER_EVIDENCE["${task}"]="${evidence}"
    done < "${LEDGER_FILE}"
fi

# ── Step 5: Run checks ──────────────────────────────────────────────────────
MISSING=()
STALE_FAIL=()
BAD_EVIDENCE=()
UNKNOWN_IDS=()
GREEN_COUNT=0
ALL_REQUIRED=("${SCOPED_TASKS[@]}" "${GATE_IDS[@]}")

for task_id in "${ALL_REQUIRED[@]}"; do
    status="${LEDGER_STATUS["${task_id}"]:-}"
    evidence="${LEDGER_EVIDENCE["${task_id}"]:-}"

    if [[ -z "${status}" ]]; then
        MISSING+=("${task_id}")
    elif [[ "${status}" == "PASS" || "${status}" == "MANUAL-VERIFIED" ]]; then
        GREEN_COUNT=$((GREEN_COUNT + 1))
        # Check evidence file exists
        if [[ -n "${evidence}" ]]; then
            local_path="${evidence%%#*}"  # strip anchor
            if [[ ! -f "${REPO_ROOT}/docs/plan-progress/${local_path}" ]]; then
                BAD_EVIDENCE+=("${task_id}: ${local_path}")
            fi
        fi
    elif [[ "${status}" == "FAIL" ]]; then
        STALE_FAIL+=("${task_id}")
    elif [[ "${status}" == "DEFERRED" ]]; then
        GREEN_COUNT=$((GREEN_COUNT + 1))
    fi
done

# Check all ledger task IDs exist in PLAN.md (hallucination detection)
for task_id in "${!LEDGER_STATUS[@]}"; do
    # Skip gate pseudo-IDs and "final"
    if [[ "${task_id}" == *".gate" || "${task_id}" == "final" ]]; then
        continue
    fi
    # Skip documented pseudo-ID families that never appear in PLAN.md:
    #   - M<n>.unit    — per-milestone unit-test task rows written by gates
    #   - final.*      — per-phase rows of the final gate
    #   - HARD.*       — open-source hardening rows (hardening-doc.md plan)
    if [[ "${task_id}" == *".unit" || "${task_id}" == "final."* || "${task_id}" == "HARD."* ]]; then
        continue
    fi
    if ! echo "${ALL_TASK_IDS}" | grep -qF "${task_id}"; then
        UNKNOWN_IDS+=("${task_id}")
    fi
done

# Append-only enforcement
APPEND_VIOLATION=false
if git -C "${REPO_ROOT}" diff HEAD --numstat -- docs/plan-progress/LEDGER.md 2>/dev/null | grep -qE '^[0-9]+\s+[1-9]'; then
    APPEND_VIOLATION=true
fi

# Anti-tamper warning
TAMPER_WARNING=false
if git -C "${REPO_ROOT}" diff --name-only HEAD 2>/dev/null | grep -qE '^scripts/(gates|lib)/' ; then
    if git -C "${REPO_ROOT}" diff --name-only HEAD 2>/dev/null | grep -qvE '^(scripts/|docs/plan-progress/)'; then
        TAMPER_WARNING=true
    fi
fi

# ── Step 6: Report ──────────────────────────────────────────────────────────
TOTAL_REQUIRED=${#ALL_REQUIRED[@]}
HAS_ERRORS=false

if [[ ${#MISSING[@]} -gt 0 ]]; then
    HAS_ERRORS=true
    echo "MISSING (${#MISSING[@]} tasks have no ledger entry):"
    for id in "${MISSING[@]}"; do
        echo "  - ${id}"
    done
    echo ""
fi

if [[ ${#STALE_FAIL[@]} -gt 0 ]]; then
    HAS_ERRORS=true
    echo "STALE-FAIL (${#STALE_FAIL[@]} tasks have FAIL as latest status):"
    for id in "${STALE_FAIL[@]}"; do
        echo "  - ${id}"
    done
    echo ""
fi

if [[ ${#BAD_EVIDENCE[@]} -gt 0 ]]; then
    HAS_ERRORS=true
    echo "BAD-EVIDENCE (${#BAD_EVIDENCE[@]} evidence files missing):"
    for entry in "${BAD_EVIDENCE[@]}"; do
        echo "  - ${entry}"
    done
    echo ""
fi

if [[ ${#UNKNOWN_IDS[@]} -gt 0 ]]; then
    HAS_ERRORS=true
    echo "UNKNOWN-IDS (${#UNKNOWN_IDS[@]} ledger entries not found in PLAN.md):"
    for id in "${UNKNOWN_IDS[@]}"; do
        echo "  - ${id}"
    done
    echo ""
fi

if [[ "${APPEND_VIOLATION}" == "true" ]]; then
    HAS_ERRORS=true
    echo "APPEND-ONLY VIOLATION: LEDGER.md has deleted lines in the working diff."
    echo ""
fi

if [[ "${TAMPER_WARNING}" == "true" ]]; then
    echo "WARNING: gate scripts modified in this change — requires explicit user approval per CLAUDE.md"
    echo ""
fi

echo "COVERAGE: ${GREEN_COUNT}/${TOTAL_REQUIRED} green"

if [[ "${HAS_ERRORS}" == "true" ]]; then
    exit 1
else
    exit 0
fi
