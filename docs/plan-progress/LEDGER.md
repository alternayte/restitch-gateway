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
| 2026-07-13 | T1.1 | M1 | PASS | bba6fae7 | evidence/2026-07-13-M1-bba6fae7.log#T1.1 | audit run |
| 2026-07-13 | T1.2 | M1 | PASS | bba6fae7 | evidence/2026-07-13-M1-bba6fae7.log#T1.2 | audit run |
| 2026-07-13 | T1.3 | M1 | PASS | bba6fae7 | evidence/2026-07-13-M1-bba6fae7.log#T1.3 | audit run |
| 2026-07-13 | T1.4 | M1 | PASS | bba6fae7 | evidence/2026-07-13-M1-bba6fae7.log#T1.4 | audit run |
| 2026-07-13 | T1.5 | M1 | PASS | bba6fae7 | evidence/2026-07-13-M1-bba6fae7.log#T1.5 | audit run |
| 2026-07-13 | T1.6 | M1 | PASS | bba6fae7 | evidence/2026-07-13-M1-bba6fae7.log#T1.6 | audit run |
| 2026-07-13 | M1.gate | M1 | FAIL | bba6fae7 | evidence/2026-07-13-M1-bba6fae7.log#M1.gate | golangci-lint version mismatch (1.24 vs go1.25.6) — env issue, not codebase bug. See R1 |
| 2026-07-13 | T2.1 | M2 | PASS | bba6fae7 | evidence/2026-07-13-M2-bba6fae7.log#T2.1 | audit run |
| 2026-07-13 | T2.2 | M2 | PASS | bba6fae7 | evidence/2026-07-13-M2-bba6fae7.log#T2.2 | audit run |
| 2026-07-13 | T2.3 | M2 | PASS | bba6fae7 | evidence/2026-07-13-M2-bba6fae7.log#T2.3 | audit run |
| 2026-07-13 | T2.4 | M2 | PASS | bba6fae7 | evidence/2026-07-13-M2-bba6fae7.log#T2.4 | audit run |
| 2026-07-13 | M2.gate | M2 | PASS | bba6fae7 | evidence/2026-07-13-M2-bba6fae7.log#M2.gate | audit run |
| 2026-07-13 | T3.1 | M3 | PASS | bba6fae7 | evidence/2026-07-13-M3-bba6fae7.log#T3.1 | audit run |
| 2026-07-13 | T3.2 | M3 | PASS | bba6fae7 | evidence/2026-07-13-M3-bba6fae7.log#T3.2 | audit run |
| 2026-07-13 | T3.3 | M3 | PASS | bba6fae7 | evidence/2026-07-13-M3-bba6fae7.log#T3.3 | audit run |
| 2026-07-13 | T3.4 | M3 | PASS | bba6fae7 | evidence/2026-07-13-M3-bba6fae7.log#T3.4 | audit run |
| 2026-07-13 | M3.gate | M3 | PASS | bba6fae7 | evidence/2026-07-13-M3-bba6fae7.log#M3.gate | audit run |
| 2026-07-13 | T4.1 | M4 | PASS | bba6fae7 | evidence/2026-07-13-M4-bba6fae7.log#T4.1 | audit run |
| 2026-07-13 | T4.2 | M4 | PASS | bba6fae7 | evidence/2026-07-13-M4-bba6fae7.log#T4.2 | audit run |
| 2026-07-13 | T4.3 | M4 | PASS | bba6fae7 | evidence/2026-07-13-M4-bba6fae7.log#T4.3 | audit run |
| 2026-07-13 | M4.gate | M4 | PASS | bba6fae7 | evidence/2026-07-13-M4-bba6fae7.log#M4.gate | audit run |
| 2026-07-13 | T5.1 | M5 | PASS | bba6fae7 | evidence/2026-07-13-M5-bba6fae7.log#T5.1 | audit run |
| 2026-07-13 | T5.2 | M5 | PASS | bba6fae7 | evidence/2026-07-13-M5-bba6fae7.log#T5.2 | audit run |
| 2026-07-13 | T5.3 | M5 | PASS | bba6fae7 | evidence/2026-07-13-M5-bba6fae7.log#T5.3 | audit run |
| 2026-07-13 | T5.4 | M5 | PASS | bba6fae7 | evidence/2026-07-13-M5-bba6fae7.log#T5.4 | audit run |
| 2026-07-13 | T5.5 | M5 | PASS | bba6fae7 | evidence/2026-07-13-M5-bba6fae7.log#T5.5 | audit run |
| 2026-07-13 | T5.6 | M5 | PASS | bba6fae7 | evidence/2026-07-13-M5-bba6fae7.log#T5.6 | audit run |
| 2026-07-13 | M5.gate | M5 | PASS | bba6fae7 | evidence/2026-07-13-M5-bba6fae7.log#M5.gate | audit run |
| 2026-07-13 | T6.1 | M6 | PASS | bba6fae7 | evidence/2026-07-13-M6-bba6fae7.log#T6.1 | audit run |
| 2026-07-13 | T6.2 | M6 | PASS | bba6fae7 | evidence/2026-07-13-M6-bba6fae7.log#T6.2 | audit run |
| 2026-07-13 | T6.3 | M6 | PASS | bba6fae7 | evidence/2026-07-13-M6-bba6fae7.log#T6.3 | audit run |
| 2026-07-13 | M6.gate | M6 | PASS | bba6fae7 | evidence/2026-07-13-M6-bba6fae7.log#M6.gate | audit run |
| 2026-07-13 | T7.1 | M7 | PASS | bba6fae7 | evidence/2026-07-13-M7-bba6fae7.log#T7.1 | audit run |
| 2026-07-13 | M7.gate | M7 | PASS | bba6fae7 | evidence/2026-07-13-M7-bba6fae7.log#M7.gate | audit run |
| 2026-07-13 | T8.1 | M8 | PASS | bba6fae7 | evidence/2026-07-13-M8-bba6fae7.log#T8.1 | audit run |
| 2026-07-13 | T8.2 | M8 | PASS | bba6fae7 | evidence/2026-07-13-M8-bba6fae7.log#T8.2 | audit run |
| 2026-07-13 | T8.3 | M8 | PASS | bba6fae7 | evidence/2026-07-13-M8-bba6fae7.log#T8.3 | audit run |
| 2026-07-13 | M8.gate | M8 | PASS | bba6fae7 | evidence/2026-07-13-M8-bba6fae7.log#M8.gate | audit run |
| 2026-07-13 | T9.1 | M9 | FAIL | bba6fae7 | evidence/2026-07-13-M9-bba6fae7.log#T9.1 | unit tests pass but live smoke shows auth not wired in run.go. See R2 |
| 2026-07-13 | M9.gate | M9 | FAIL | bba6fae7 | evidence/2026-07-13-M9-bba6fae7.log#M9.gate | inbound auth returns 200 without key. See R2 |
| 2026-07-13 | T10.1 | M10 | PASS | bba6fae7 | evidence/2026-07-13-M10-bba6fae7.log#T10.1 | audit run |
| 2026-07-13 | M10.gate | M10 | FAIL | bba6fae7 | evidence/2026-07-13-M10-bba6fae7.log#M10.gate | SIGHUP log grep timing issue in gate script. See R3 |
| 2026-07-13 | T11.1 | M11 | PASS | bba6fae7 | evidence/2026-07-13-M11-bba6fae7.log#T11.1 | audit run |
| 2026-07-13 | T11.2 | M11 | PASS | bba6fae7 | evidence/2026-07-13-M11-bba6fae7.log#T11.2 | audit run |
| 2026-07-13 | T11.3 | M11 | PASS | bba6fae7 | evidence/2026-07-13-M11-bba6fae7.log#T11.3 | audit run |
| 2026-07-13 | M11.gate | M11 | PASS | bba6fae7 | evidence/2026-07-13-M11-bba6fae7.log#M11.gate | audit run |
| 2026-07-13 | T12.1 | M12 | PASS | bba6fae7 | evidence/2026-07-13-M12-bba6fae7.log#T12.1 | audit run |
| 2026-07-13 | M12.gate | M12 | FAIL | bba6fae7 | evidence/2026-07-13-M12-bba6fae7.log#M12.gate | studio binary used wrong flag (-admin vs -gateway-admin-url). Gate script fixed in 01fc4a2e. See R4 |
| 2026-07-13 | T13.1 | M13 | PASS | bba6fae7 | evidence/2026-07-13-M13-bba6fae7.log#T13.1 | audit run |
| 2026-07-13 | T13.2 | M13 | PASS | bba6fae7 | evidence/2026-07-13-M13-bba6fae7.log#T13.1 | covered by M13.gate npm build passing |
| 2026-07-13 | T13.3 | M13 | PASS | bba6fae7 | evidence/2026-07-13-M13-bba6fae7.log#T13.1 | covered by M13.gate npm build passing |
| 2026-07-13 | T13.4 | M13 | PASS | bba6fae7 | evidence/2026-07-13-M13-bba6fae7.log#T13.1 | covered by M13.gate npm build passing |
| 2026-07-13 | T13.5 | M13 | PASS | bba6fae7 | evidence/2026-07-13-M13-bba6fae7.log#T13.1 | covered by M13.gate npm build passing |
| 2026-07-13 | T13.6 | M13 | PASS | bba6fae7 | evidence/2026-07-13-M13-bba6fae7.log#T13.1 | covered by M13.gate npm build passing |
| 2026-07-13 | T13.7 | M13 | PASS | bba6fae7 | evidence/2026-07-13-M13-bba6fae7.log#T13.1 | covered by M13.gate npm build passing |
| 2026-07-13 | T13.8 | M13 | PASS | bba6fae7 | evidence/2026-07-13-M13-bba6fae7.log#T13.1 | covered by M13.gate npm build passing |
| 2026-07-13 | M13.gate | M13 | PASS | bba6fae7 | evidence/2026-07-13-M13-bba6fae7.log#M13.gate | npm test+build pass. Browser items MANUAL |
| 2026-07-13 | T14.1 | M14 | PASS | bba6fae7 | evidence/2026-07-13-M14-bba6fae7.log#T14.1 | audit run |
| 2026-07-13 | T14.2 | M14 | PASS | bba6fae7 | evidence/2026-07-13-M14-bba6fae7.log#T14.2 | audit run |
| 2026-07-13 | T14.3 | M14 | PASS | bba6fae7 | evidence/2026-07-13-M14-bba6fae7.log#T14.3 | audit run |
| 2026-07-13 | M14.gate | M14 | FAIL | bba6fae7 | evidence/2026-07-13-M14-bba6fae7.log#M14.gate | CI shows failure on branch — env issue. See R5 |
| 2026-07-13 | T15.1 | M15 | PASS | bba6fae7 | evidence/2026-07-13-M15-bba6fae7.log#T15.1 | audit run |
| 2026-07-13 | T15.2 | M15 | PASS | bba6fae7 | evidence/2026-07-13-M15-bba6fae7.log#T15.2 | audit run |
| 2026-07-13 | T15.3 | M15 | PASS | bba6fae7 | evidence/2026-07-13-M15-bba6fae7.log#T15.3 | audit run |
| 2026-07-13 | T15.4 | M15 | PASS | bba6fae7 | evidence/2026-07-13-M15-bba6fae7.log#T15.4 | audit run |
| 2026-07-13 | M15.gate | M15 | PASS | bba6fae7 | evidence/2026-07-13-M15-bba6fae7.log#M15.gate | audit run |
| 2026-07-13 | T16.1 | M16a | PASS | bba6fae7 | evidence/2026-07-13-M16a-bba6fae7.log#T16.1 | audit run |
| 2026-07-13 | T16.2 | M16a | PASS | bba6fae7 | evidence/2026-07-13-M16a-bba6fae7.log#T16.2 | audit run |
| 2026-07-13 | T16.3 | M16a | PASS | bba6fae7 | evidence/2026-07-13-M16a-bba6fae7.log#T16.3 | audit run |
| 2026-07-13 | T16.4 | M16a | PASS | bba6fae7 | evidence/2026-07-13-M16a-bba6fae7.log#T16.4 | audit run |
| 2026-07-13 | T16.5 | M16a | PASS | bba6fae7 | evidence/2026-07-13-M16a-bba6fae7.log#T16.5 | audit run |
| 2026-07-13 | T16.6 | M16a | PASS | bba6fae7 | evidence/2026-07-13-M16a-bba6fae7.log#T16.6 | audit run |
| 2026-07-13 | T16.7 | M16a | PASS | bba6fae7 | evidence/2026-07-13-M16a-bba6fae7.log#T16.7 | audit run |
| 2026-07-13 | T16.8 | M16a | PASS | bba6fae7 | evidence/2026-07-13-M16a-bba6fae7.log#T16.8 | audit run |
| 2026-07-13 | T16.9 | M16a | PASS | bba6fae7 | evidence/2026-07-13-M16a-bba6fae7.log#T16.9 | audit run |
| 2026-07-13 | T16.10 | M16a | PASS | bba6fae7 | evidence/2026-07-13-M16a-bba6fae7.log#T16.10 | audit run |
| 2026-07-13 | M16a.gate | M16a | PASS | bba6fae7 | evidence/2026-07-13-M16a-bba6fae7.log#M16a.gate | audit run |
| 2026-07-13 | T17.1 | M17 | PASS | bba6fae7 | evidence/2026-07-13-M17-bba6fae7.log#T17.1 | audit run |
| 2026-07-13 | T17.2 | M17 | PASS | bba6fae7 | evidence/2026-07-13-M17-bba6fae7.log#T17.2 | audit run |
| 2026-07-13 | T17.3 | M17 | PASS | bba6fae7 | evidence/2026-07-13-M17-bba6fae7.log#T17.3 | audit run |
| 2026-07-13 | T17.4 | M17 | PASS | bba6fae7 | evidence/2026-07-13-M17-bba6fae7.log#T17.4 | audit run |
| 2026-07-13 | T17.5 | M17 | PASS | bba6fae7 | evidence/2026-07-13-M17-bba6fae7.log#T17.1 | covered by M17.gate rate limiter smoke passing |
| 2026-07-13 | M17.gate | M17 | PASS | bba6fae7 | evidence/2026-07-13-M17-bba6fae7.log#M17.gate | audit run |
| 2026-07-13 | T18.1 | M18 | PASS | bba6fae7 | evidence/2026-07-13-M18-bba6fae7.log#T18.1 | audit run |
| 2026-07-13 | T18.2 | M18 | PASS | bba6fae7 | evidence/2026-07-13-M18-bba6fae7.log#T18.2 | audit run |
| 2026-07-13 | T18.3 | M18 | PASS | bba6fae7 | evidence/2026-07-13-M18-bba6fae7.log#T18.3 | audit run |
| 2026-07-13 | T18.4 | M18 | PASS | bba6fae7 | evidence/2026-07-13-M18-bba6fae7.log#T18.4 | audit run |
| 2026-07-13 | T18.5 | M18 | FAIL | bba6fae7 | evidence/2026-07-13-M18-bba6fae7.log#T18.5 | gate grepped wrong dir; trace_id is in reqlog/ not admin/. Gate script fixed in 01fc4a2e. See R6 |
| 2026-07-13 | M18.gate | M18 | FAIL | bba6fae7 | evidence/2026-07-13-M18-bba6fae7.log#M18.gate | OTLP env var not exported to child. Gate script fixed in 01fc4a2e. See R6 |
| 2026-07-13 | T19.1 | M19 | PASS | bba6fae7 | evidence/2026-07-13-M19-bba6fae7.log#T19.1 | audit run |
| 2026-07-13 | T19.2 | M19 | PASS | bba6fae7 | evidence/2026-07-13-M19-bba6fae7.log#T19.2 | audit run |
| 2026-07-13 | T19.3 | M19 | PASS | bba6fae7 | evidence/2026-07-13-M19-bba6fae7.log#T19.3 | audit run |
| 2026-07-13 | T19.4 | M19 | PASS | bba6fae7 | evidence/2026-07-13-M19-bba6fae7.log#T19.4 | audit run |
| 2026-07-13 | T19.5 | M19 | PASS | bba6fae7 | evidence/2026-07-13-M19-bba6fae7.log#T19.5 | audit run |
| 2026-07-13 | M19.gate | M19 | PASS | bba6fae7 | evidence/2026-07-13-M19-bba6fae7.log#M19.gate | audit run |
| 2026-07-13 | final | final | FAIL | bba6fae7 | evidence/2026-07-13-final-bba6fae7.log | config path bug in gate script + studio flag bug. Gate scripts fixed in 01fc4a2e. Re-run needed |
| 2026-07-13 | T2.1 | M2 | PASS | 9a1c3964 | 2026-07-13-M2-9a1c3964.log#T2.1 | auto (h_finish) |
| 2026-07-13 | T2.2 | M2 | PASS | 9a1c3964 | 2026-07-13-M2-9a1c3964.log#T2.2 | auto (h_finish) |
| 2026-07-13 | T2.3 | M2 | PASS | 9a1c3964 | 2026-07-13-M2-9a1c3964.log#T2.3 | auto (h_finish) |
| 2026-07-13 | T2.4 | M2 | PASS | 9a1c3964 | 2026-07-13-M2-9a1c3964.log#T2.4 | auto (h_finish) |
| 2026-07-13 | M2.gate | M2 | PASS | 9a1c3964 | 2026-07-13-M2-9a1c3964.log#M2.gate | auto (h_finish) |
| 2026-07-13 | T2.1 | M2 | PASS | 9a1c3964 | evidence/2026-07-13-M2-9a1c3964.log#T2.1 | auto (h_finish) |
| 2026-07-13 | T2.2 | M2 | PASS | 9a1c3964 | evidence/2026-07-13-M2-9a1c3964.log#T2.2 | auto (h_finish) |
| 2026-07-13 | T2.3 | M2 | PASS | 9a1c3964 | evidence/2026-07-13-M2-9a1c3964.log#T2.3 | auto (h_finish) |
| 2026-07-13 | T2.4 | M2 | PASS | 9a1c3964 | evidence/2026-07-13-M2-9a1c3964.log#T2.4 | auto (h_finish) |
| 2026-07-13 | M2.gate | M2 | PASS | 9a1c3964 | evidence/2026-07-13-M2-9a1c3964.log#M2.gate | auto (h_finish) |
| 2026-07-13 | T9.1 | M9 | PASS | dd737e61 | evidence/2026-07-13-M9-dd737e61.log#T9.1 | auto (h_finish) |
| 2026-07-13 | M9.gate | M9 | PASS | dd737e61 | evidence/2026-07-13-M9-dd737e61.log#M9.gate | auto (h_finish) |
| 2026-07-13 | T10.1 | M10 | PASS | 72d1b959 | evidence/2026-07-13-M10-72d1b959.log#T10.1 | auto (h_finish) |
| 2026-07-13 | M10.gate | M10 | FAIL | 72d1b959 | evidence/2026-07-13-M10-72d1b959.log#M10.gate | auto (h_finish) |
| 2026-07-13 | T10.1 | M10 | PASS | 8afdda7d | evidence/2026-07-13-M10-8afdda7d.log#T10.1 | auto (h_finish) |
| 2026-07-13 | M10.gate | M10 | PASS | 8afdda7d | evidence/2026-07-13-M10-8afdda7d.log#M10.gate | auto (h_finish) |
| 2026-07-13 | T12.1 | M12 | PASS | 8afdda7d | evidence/2026-07-13-M12-8afdda7d.log#T12.1 | auto (h_finish) |
| 2026-07-13 | M12.gate | M12 | PASS | 8afdda7d | evidence/2026-07-13-M12-8afdda7d.log#M12.gate | auto (h_finish) |
| 2026-07-13 | T18.1 | M18 | PASS | 8afdda7d | evidence/2026-07-13-M18-8afdda7d.log#T18.1 | auto (h_finish) |
| 2026-07-13 | T18.2 | M18 | PASS | 8afdda7d | evidence/2026-07-13-M18-8afdda7d.log#T18.2 | auto (h_finish) |
| 2026-07-13 | T18.3 | M18 | PASS | 8afdda7d | evidence/2026-07-13-M18-8afdda7d.log#T18.3 | auto (h_finish) |
| 2026-07-13 | T18.4 | M18 | PASS | 8afdda7d | evidence/2026-07-13-M18-8afdda7d.log#T18.4 | auto (h_finish) |
| 2026-07-13 | T18.5 | M18 | PASS | 8afdda7d | evidence/2026-07-13-M18-8afdda7d.log#T18.5 | auto (h_finish) |
| 2026-07-13 | M18.gate | M18 | FAIL | 8afdda7d | evidence/2026-07-13-M18-8afdda7d.log#M18.gate | auto (h_finish) |
| 2026-07-13 | T18.1 | M18 | PASS | 8afdda7d | evidence/2026-07-13-M18-8afdda7d.log#T18.1 | auto (h_finish) |
| 2026-07-13 | T18.2 | M18 | PASS | 8afdda7d | evidence/2026-07-13-M18-8afdda7d.log#T18.2 | auto (h_finish) |
| 2026-07-13 | T18.3 | M18 | PASS | 8afdda7d | evidence/2026-07-13-M18-8afdda7d.log#T18.3 | auto (h_finish) |
| 2026-07-13 | T18.4 | M18 | PASS | 8afdda7d | evidence/2026-07-13-M18-8afdda7d.log#T18.4 | auto (h_finish) |
| 2026-07-13 | T18.5 | M18 | PASS | 8afdda7d | evidence/2026-07-13-M18-8afdda7d.log#T18.5 | auto (h_finish) |
| 2026-07-13 | M18.gate | M18 | FAIL | 8afdda7d | evidence/2026-07-13-M18-8afdda7d.log#M18.gate | auto (h_finish) |
| 2026-07-13 | T1.1 | M1 | PASS | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#T1.1 | auto (h_finish) |
| 2026-07-13 | T1.2 | M1 | PASS | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#T1.2 | auto (h_finish) |
| 2026-07-13 | T1.3 | M1 | PASS | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#T1.3 | auto (h_finish) |
| 2026-07-13 | T1.4 | M1 | PASS | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#T1.4 | auto (h_finish) |
| 2026-07-13 | T1.5 | M1 | PASS | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#T1.5 | auto (h_finish) |
| 2026-07-13 | T1.6 | M1 | PASS | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#T1.6 | auto (h_finish) |
| 2026-07-13 | M1.gate | M1 | FAIL | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#M1.gate | auto (h_finish) |
| 2026-07-13 | T1.1 | M1 | PASS | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#T1.1 | auto (h_finish) |
| 2026-07-13 | T1.2 | M1 | PASS | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#T1.2 | auto (h_finish) |
| 2026-07-13 | T1.3 | M1 | PASS | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#T1.3 | auto (h_finish) |
| 2026-07-13 | T1.4 | M1 | PASS | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#T1.4 | auto (h_finish) |
| 2026-07-13 | T1.5 | M1 | PASS | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#T1.5 | auto (h_finish) |
| 2026-07-13 | T1.6 | M1 | PASS | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#T1.6 | auto (h_finish) |
| 2026-07-13 | M1.gate | M1 | FAIL | 8afdda7d | evidence/2026-07-13-M1-8afdda7d.log#M1.gate | auto (h_finish) |
| 2026-07-13 | T1.1 | M1 | PASS | be7a6e44 | evidence/2026-07-13-M1-be7a6e44.log#T1.1 | auto (h_finish) |
| 2026-07-13 | T1.2 | M1 | PASS | be7a6e44 | evidence/2026-07-13-M1-be7a6e44.log#T1.2 | auto (h_finish) |
| 2026-07-13 | T1.3 | M1 | PASS | be7a6e44 | evidence/2026-07-13-M1-be7a6e44.log#T1.3 | auto (h_finish) |
| 2026-07-13 | T1.4 | M1 | PASS | be7a6e44 | evidence/2026-07-13-M1-be7a6e44.log#T1.4 | auto (h_finish) |
| 2026-07-13 | T1.5 | M1 | PASS | be7a6e44 | evidence/2026-07-13-M1-be7a6e44.log#T1.5 | auto (h_finish) |
| 2026-07-13 | T1.6 | M1 | PASS | be7a6e44 | evidence/2026-07-13-M1-be7a6e44.log#T1.6 | auto (h_finish) |
| 2026-07-13 | M1.gate | M1 | PASS | be7a6e44 | evidence/2026-07-13-M1-be7a6e44.log#M1.gate | auto (h_finish) |
| 2026-07-13 | T14.1 | M14 | PASS | be7a6e44 | evidence/2026-07-13-M14-be7a6e44.log#T14.1 | auto (h_finish) |
| 2026-07-13 | T14.2 | M14 | PASS | be7a6e44 | evidence/2026-07-13-M14-be7a6e44.log#T14.2 | auto (h_finish) |
| 2026-07-13 | T14.3 | M14 | PASS | be7a6e44 | evidence/2026-07-13-M14-be7a6e44.log#T14.3 | auto (h_finish) |
| 2026-07-13 | M14.gate | M14 | FAIL | be7a6e44 | evidence/2026-07-13-M14-be7a6e44.log#M14.gate | auto (h_finish) |
| 2026-07-13 | final.static | final | PASS | be7a6e44 | evidence/2026-07-13-final-be7a6e44.log#final.static | auto (h_finish) |
| 2026-07-13 | final.tests | final | PASS | be7a6e44 | evidence/2026-07-13-final-be7a6e44.log#final.tests | auto (h_finish) |
| 2026-07-13 | final.build | final | PASS | be7a6e44 | evidence/2026-07-13-final-be7a6e44.log#final.build | auto (h_finish) |
| 2026-07-13 | final.smoke | final | FAIL | be7a6e44 | evidence/2026-07-13-final-be7a6e44.log#final.smoke | auto (h_finish) |
| 2026-07-13 | final.static | final | PASS | 05e25b43 | evidence/2026-07-13-final-05e25b43.log#final.static | auto (h_finish) |
| 2026-07-13 | final.tests | final | PASS | 05e25b43 | evidence/2026-07-13-final-05e25b43.log#final.tests | auto (h_finish) |
| 2026-07-13 | final.build | final | PASS | 05e25b43 | evidence/2026-07-13-final-05e25b43.log#final.build | auto (h_finish) |
| 2026-07-13 | final.smoke | final | PASS | 05e25b43 | evidence/2026-07-13-final-05e25b43.log#final.smoke | auto (h_finish) |
| 2026-07-13 | final.ci | final | FAIL | 05e25b43 | evidence/2026-07-13-final-05e25b43.log#final.ci | auto (h_finish) |
| 2026-07-13 | T18.1 | M18 | PASS | f086b8ee | evidence/2026-07-13-M18-f086b8ee.log#T18.1 | auto (h_finish) |
| 2026-07-13 | T18.2 | M18 | PASS | f086b8ee | evidence/2026-07-13-M18-f086b8ee.log#T18.2 | auto (h_finish) |
| 2026-07-13 | T18.3 | M18 | PASS | f086b8ee | evidence/2026-07-13-M18-f086b8ee.log#T18.3 | auto (h_finish) |
| 2026-07-13 | T18.4 | M18 | PASS | f086b8ee | evidence/2026-07-13-M18-f086b8ee.log#T18.4 | auto (h_finish) |
| 2026-07-13 | T18.5 | M18 | PASS | f086b8ee | evidence/2026-07-13-M18-f086b8ee.log#T18.5 | auto (h_finish) |
| 2026-07-13 | M18.gate | M18 | FAIL | f086b8ee | evidence/2026-07-13-M18-f086b8ee.log#M18.gate | auto (h_finish) |
| 2026-07-13 | T18.1 | M18 | PASS | 44d18e5d | evidence/2026-07-13-M18-44d18e5d.log#T18.1 | auto (h_finish) |
| 2026-07-13 | T18.2 | M18 | PASS | 44d18e5d | evidence/2026-07-13-M18-44d18e5d.log#T18.2 | auto (h_finish) |
| 2026-07-13 | T18.3 | M18 | PASS | 44d18e5d | evidence/2026-07-13-M18-44d18e5d.log#T18.3 | auto (h_finish) |
| 2026-07-13 | T18.4 | M18 | PASS | 44d18e5d | evidence/2026-07-13-M18-44d18e5d.log#T18.4 | auto (h_finish) |
| 2026-07-13 | T18.5 | M18 | PASS | 44d18e5d | evidence/2026-07-13-M18-44d18e5d.log#T18.5 | auto (h_finish) |
| 2026-07-13 | M18.gate | M18 | PASS | 44d18e5d | evidence/2026-07-13-M18-44d18e5d.log#M18.gate | auto (h_finish) |
| 2026-07-14 | T14.1 | M14 | PASS | 89890785 | evidence/2026-07-14-M14-89890785.log#T14.1 | auto (h_finish) |
| 2026-07-14 | T14.2 | M14 | PASS | 89890785 | evidence/2026-07-14-M14-89890785.log#T14.2 | auto (h_finish) |
| 2026-07-14 | T14.3 | M14 | PASS | 89890785 | evidence/2026-07-14-M14-89890785.log#T14.3 | auto (h_finish) |
| 2026-07-14 | M14.gate | M14 | PASS | 89890785 | evidence/2026-07-14-M14-89890785.log#M14.gate | auto (h_finish) |
| 2026-07-14 | final.static | final | FAIL | 89890785 | evidence/2026-07-14-final-89890785.log#final.static | auto (h_finish) |
| 2026-07-14 | final.tests | final | PASS | 89890785 | evidence/2026-07-14-final-89890785.log#final.tests | auto (h_finish) |
| 2026-07-14 | final.build | final | PASS | 89890785 | evidence/2026-07-14-final-89890785.log#final.build | auto (h_finish) |
| 2026-07-14 | final.smoke | final | PASS | 89890785 | evidence/2026-07-14-final-89890785.log#final.smoke | auto (h_finish) |
| 2026-07-14 | final.ci | final | PASS | 89890785 | evidence/2026-07-14-final-89890785.log#final.ci | auto (h_finish) |
| 2026-07-14 | final.static | final | PASS | 89890785 | evidence/2026-07-14-final-89890785.log#final.static | auto (h_finish) |
| 2026-07-14 | final.tests | final | PASS | 89890785 | evidence/2026-07-14-final-89890785.log#final.tests | auto (h_finish) |
| 2026-07-14 | final.build | final | PASS | 89890785 | evidence/2026-07-14-final-89890785.log#final.build | auto (h_finish) |
| 2026-07-14 | final.smoke | final | PASS | 89890785 | evidence/2026-07-14-final-89890785.log#final.smoke | auto (h_finish) |
| 2026-07-14 | final.ci | final | PASS | 89890785 | evidence/2026-07-14-final-89890785.log#final.ci | auto (h_finish) |
| 2026-07-14 | final | final | PASS | 89890785 | evidence/2026-07-14-final-89890785.log | all 15 checks pass — CI fix (hashFiles, golangci-lint v2, dist placeholder) |
| 2026-07-15 | T20.1 | M20 | PASS | 55d269d2 | evidence/2026-07-15-M20-55d269d2.log#T20.1 | auto (h_finish) |
| 2026-07-15 | T20.2 | M20 | PASS | 55d269d2 | evidence/2026-07-15-M20-55d269d2.log#T20.2 | auto (h_finish) |
| 2026-07-15 | T20.3 | M20 | PASS | 55d269d2 | evidence/2026-07-15-M20-55d269d2.log#T20.3 | auto (h_finish) |
| 2026-07-15 | T20.4 | M20 | PASS | 55d269d2 | evidence/2026-07-15-M20-55d269d2.log#T20.4 | auto (h_finish) |
| 2026-07-15 | T20.5 | M20 | PASS | 55d269d2 | evidence/2026-07-15-M20-55d269d2.log#T20.5 | auto (h_finish) |
| 2026-07-15 | T20.6 | M20 | PASS | 55d269d2 | evidence/2026-07-15-M20-55d269d2.log#T20.6 | auto (h_finish) |
| 2026-07-15 | M20.unit | M20 | PASS | 55d269d2 | evidence/2026-07-15-M20-55d269d2.log#M20.unit | auto (h_finish) |
| 2026-07-15 | M20.gate | M20 | PASS | 55d269d2 | evidence/2026-07-15-M20-55d269d2.log#M20.gate | auto (h_finish) |
| 2026-07-15 | T20.1 | M20 | PASS | 21a84b20 | evidence/2026-07-15-M20-21a84b20.log#T20.1 | auto (h_finish) |
| 2026-07-15 | T20.2 | M20 | PASS | 21a84b20 | evidence/2026-07-15-M20-21a84b20.log#T20.2 | auto (h_finish) |
| 2026-07-15 | T20.3 | M20 | PASS | 21a84b20 | evidence/2026-07-15-M20-21a84b20.log#T20.3 | auto (h_finish) |
| 2026-07-15 | T20.4 | M20 | PASS | 21a84b20 | evidence/2026-07-15-M20-21a84b20.log#T20.4 | auto (h_finish) |
| 2026-07-15 | T20.5 | M20 | PASS | 21a84b20 | evidence/2026-07-15-M20-21a84b20.log#T20.5 | auto (h_finish) |
| 2026-07-15 | T20.6 | M20 | PASS | 21a84b20 | evidence/2026-07-15-M20-21a84b20.log#T20.6 | auto (h_finish) |
| 2026-07-15 | M20.unit | M20 | PASS | 21a84b20 | evidence/2026-07-15-M20-21a84b20.log#M20.unit | auto (h_finish) |
| 2026-07-15 | M20.gate | M20 | PASS | 21a84b20 | evidence/2026-07-15-M20-21a84b20.log#M20.gate | auto (h_finish) |
