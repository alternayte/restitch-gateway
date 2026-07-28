# Restitch — rules for plan executors

PLAN.md is the single source of truth. docs/plan-progress/LEDGER.md is the
proof of work. scripts/verify.sh is the judge. You do not get to be the judge.

## Executing a milestone (M<n>)

1. EXPAND FIRST. Before writing any code for milestone M<n>, use the
   superpowers:writing-plans skill to expand M<n> from PLAN.md into a
   checkbox plan saved to docs/superpowers/plans/<date>-m<n>-<slug>.md.
   Every PLAN.md task T<n>.x must appear as its own task with its exact
   "Accept:" command copied in. Then execute with
   superpowers:subagent-driven-development or superpowers:executing-plans.
2. If milestone M<n> has no gate script yet (M20+), your FIRST task in the
   expansion is: replace scripts/gates/m<n>.sh's placeholder with a real
   gate script encoding the PLAN.md verification gate. Get user approval of
   that script before implementing features.
3. AFTER EVERY TASK: run the task's Accept command and paste its real
   output into the commit or task report. No output = not done.
4. AFTER EVERY TASK: append one row to docs/plan-progress/LEDGER.md
   (schema in that file's header). Commit the row with the work.
5. BEFORE marking a milestone DONE: run `scripts/verify.sh M<n>`. It must
   print `RESULT M<n>: PASS`. Commit the evidence file it wrote under
   docs/plan-progress/evidence/. Add the `M<n>.gate` ledger row.
6. MANUAL lines printed by verify.sh are not yours to check off. List them
   for the user and stop. Only after the user confirms may you add a
   MANUAL-VERIFIED ledger row, citing the user's confirmation in Notes.
7. Run `scripts/check-ledger.sh` before claiming the milestone complete.
   It must exit 0 (or list only not-yet-in-scope future tasks).

## Hard rules (no exceptions)

- NEVER edit scripts/verify.sh, scripts/check-ledger.sh, scripts/lib/, or
  scripts/gates/ to make a failing check pass. If a gate script is wrong,
  STOP and ask the user; change it only with their explicit approval, in a
  dedicated commit whose message starts `gate:`.
- NEVER edit or delete existing LEDGER.md rows. Append only. A failure is
  recorded as FAIL and later superseded by a PASS row.
- NEVER mark a PLAN.md task or milestone DONE without a green ledger row
  backed by a committed evidence file.
- NEVER weaken, skip, tag-out, or delete a test to make a gate pass.
- If an Accept command cannot run in your environment, record status FAIL
  or ask the user — do not record PASS on reasoning alone.
- Evidence means literal command output captured by the harness, not a
  summary you wrote.

## Project conventions

- Go module: `github.com/restitch/restitch-gateway`, Go 1.25.6
- Build: `make build` (gateway), `make build-all` (gateway + studio)
- Test: `make ci` (vet + lint + race), `make e2e` (golden specs)
- Frontend: `cd studio && npm ci && npm run build`
- Verification: `make verify GATE=M3`, `make verify-all`, `make ledger-check`
- Commit messages: `feat(M<m>): <task title>` or `fix:/test:/docs:` as appropriate
- Gate script changes: `gate: <description>` commit prefix, requires user approval
