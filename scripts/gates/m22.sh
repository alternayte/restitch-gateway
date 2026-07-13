#!/usr/bin/env bash
# Gate M22 — placeholder (milestone not yet expanded)
source "$(dirname "$0")/../lib/harness.sh"
h_init M22

# Check if milestone is marked DONE in PLAN.md while this is still a placeholder
done_status=$(grep -E "^\| M22 " "${REPO_ROOT}/PLAN.md" | grep -c "DONE" || true)
if [[ "${done_status}" -gt 0 ]]; then
    h_fail "M22 marked DONE in PLAN.md but gate script is still a placeholder — protocol violation"
fi

h_fail "M22 gate not implemented: milestone M22 has not been expanded. Write this gate script (from PLAN.md 'M22 Verification gate') as the first task of the milestone expansion, per CLAUDE.md rule 2."

h_finish
