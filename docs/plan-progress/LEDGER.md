# Restitch Plan Ledger

Append-only evidence ledger. One row per task verification event.
Latest row per task ID wins. Never edit or delete existing rows.

**Status values:** PASS, FAIL, MANUAL-VERIFIED, DEFERRED, PENDING

**Rules:**
- MANUAL-VERIFIED requires a Notes entry naming who/when confirmed
- Commit = SHA the verification ran against (not the implementing commit)
- Milestone gates use pseudo-IDs: M3.gate, final, etc.
- FAIL rows are never deleted — a later PASS row supersedes

| Date | Task | Milestone | Status | Commit | Evidence | Notes |
|------|------|-----------|--------|--------|----------|-------|
