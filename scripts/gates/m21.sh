#!/usr/bin/env bash
# Gate M21 — placeholder (milestone not yet expanded)
source "$(dirname "$0")/../lib/harness.sh"
h_init M21

# Check if milestone is marked DONE in PLAN.md while this is still a placeholder
done_status=$(grep -E "^\| M21 " "${REPO_ROOT}/PLAN.md" | grep -c "DONE" || true)
if [[ "${done_status}" -gt 0 ]]; then
    h_fail "M21 marked DONE in PLAN.md but gate script is still a placeholder — protocol violation"
fi

h_fail "M21 gate not implemented: milestone M21 has not been expanded. Write this gate script (from PLAN.md 'M21 Verification gate') as the first task of the milestone expansion, per CLAUDE.md rule 2."

h_finish
