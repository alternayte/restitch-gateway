#!/usr/bin/env bash
# Gate M20 — placeholder (milestone not yet expanded)
source "$(dirname "$0")/../lib/harness.sh"
h_init M20

# Check if milestone is marked DONE in PLAN.md while this is still a placeholder
done_status=$(grep -E "^\| M20 " "${REPO_ROOT}/PLAN.md" | grep -c "DONE" || true)
if [[ "${done_status}" -gt 0 ]]; then
    h_fail "M20 marked DONE in PLAN.md but gate script is still a placeholder — protocol violation"
fi

h_fail "M20 gate not implemented: milestone M20 has not been expanded. Write this gate script (from PLAN.md 'M20 Verification gate') as the first task of the milestone expansion, per CLAUDE.md rule 2."

h_finish
