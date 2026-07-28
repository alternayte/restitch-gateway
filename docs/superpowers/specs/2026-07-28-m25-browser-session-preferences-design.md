# M25 — Browser Session & User Preferences

**Date:** 2026-07-28
**Milestone:** M25 (PLAN.md §"M25 — Browser Session & User Preferences (extends M12)")
**Status:** design approved, awaiting spec review

---

## Context

M25 is the last unbuilt milestone in PLAN.md. M1–M24 all carry gate evidence;
`scripts/check-ledger.sh` reports `COVERAGE: 124/124 green`. It exits 1 only
because of 10 `UNKNOWN-IDS` (`M20.unit`–`M24.unit`, `final.static`,
`final.tests`, `final.build`, `final.smoke`, `final.ci`) — harness-emitted
pseudo-IDs that PLAN.md never declares. That is bookkeeping noise and is
explicitly **out of scope** for M25.

`scripts/gates/m25.sh` is still the 14-line placeholder that self-fails, and
the ledger contains zero M25 rows.

### Environment facts this design depends on

Verified in the working tree at design time:

| Fact | Value |
|------|-------|
| Studio frontend | React 19 + Vite + Tailwind v4 + Base UI/shadcn + react-router 7 |
| Studio backend | `cmd/restitch-studio/`, plain `http.ServeMux` — **not** Huma |
| `internal/studio/` package | does not exist (PLAN.md's reference paths are from the experimental repo) |
| Migrations | goose, embedded FS in `internal/registry/db.go`, single `goose_db_version` table |
| Existing migration | `internal/registry/migrations/20260715000000_registry_schema.sql` |
| Studio DB | unconditional — `main.go` calls `log.Fatal` if `openDB` fails |
| Studio default port | `3080` (`main.go:25`); `8090` is what `restitch dev` assigns |
| `localStorage` / `sessionStorage` in `studio/src` | **none** — no persistence exists today |
| Sidebar (`App.tsx:52`) | fixed `w-52` nav, **no collapse feature** |
| `Compositions.tsx` | renders a **table**, not cards; **no pin concept** anywhere |
| `TimeRangeSelector` | exists; `Dashboard.tsx:12` holds `useState<TimeRange>("1h")` |
| `TimeRange` union | `"1h" \| "6h" \| "24h"` |

### Scope decision recorded up front

PLAN.md T25.4 reads "pin button on composition cards, persist sidebar state,
restore on reload." Two of those three name UI that **does not exist**: there
is no pin control and no collapsible sidebar. Only the time range is
currently stateful.

**Decision (user, 2026-07-28): build the UI, honoring plan intent.** T25.4
therefore includes creating the sidebar-collapse and pin features, then
persisting all three preferences. This is a larger frontend surface than the
task text implies, and is deliberate — without it the M25 gate line "Refresh
browser → pinned compositions persist" cannot be verified honestly.

---

## Architecture

### New Go package `internal/session`

Mirrors the shape of `internal/registry`.

| File | Contents |
|------|----------|
| `session.go` | `Middleware(store)`; `FromContext(ctx) (string, bool)`; ID generation |
| `store.go` | `Store` over `*sql.DB`: `EnsureSession`, `GetPreferences`, `PutPreferences` |
| `types.go` | `Preferences`, `PreferencesRequest`, `PreferencesResponse`, `Validate()` |

Handlers live in `cmd/restitch-studio/preferences.go`, following the existing
`RegistryAPI` pattern in `cmd/restitch-studio/api.go`.

### Session identity

- Cookie name `restitch_browser_id`
- Value: 32 random bytes from `crypto/rand`, base64url-encoded, no padding
- `Max-Age`: 31536000 (1 year)
- `HttpOnly`: true
- `SameSite`: `Strict`
- `Path`: `/`
- `Secure`: **conditional on `r.TLS != nil`**

The conditional `Secure` flag is deliberate. PLAN.md does not mention it, but
setting it unconditionally would make the cookie undeliverable over
`http://localhost`, which is Studio's primary deployment mode and the mode
both `restitch dev` and the gate script use.

### Where the middleware mints — and why it is restricted

The middleware wraps the SPA handler and the preferences routes. It mints a
new session **only** when the cookie is absent **and** the request is either:

1. a SPA document request (`/`, or a path that falls through to `index.html`), or
2. any `/api/v1/preferences` request.

Static asset requests never mint.

**Reason:** a cold browser load fires `index.html` plus several asset requests
near-simultaneously, all cookie-less. Minting on every cookie-less request
would race and create **several session rows for a single browser**.
Restricting minting to document and preferences requests removes the race on
the normal path while keeping `curl -c cookies.txt .../preferences` working,
which the verification gate depends on.

The gateway admin proxy pass-through (`mux.Handle("/api/", proxy)`) is **not**
wrapped — those requests are forwarded to a different process and have no use
for a Studio session.

Route registration order matters: `/api/v1/preferences` must be registered
before the `/api/` proxy catch-all. Go 1.22+ `ServeMux` prefers the more
specific pattern, so this is satisfied by registering it alongside the
existing `/api/v1/configs` routes inside `buildMux`.

### Preferences model

Typed struct, validated field-by-field, persisted as a JSON column.

```go
type Preferences struct {
    PinnedCompositions []string `json:"pinned_compositions"`
    SidebarCollapsed   bool     `json:"sidebar_collapsed"`
    DefaultTimeRange   string   `json:"default_time_range"`
}
```

`Validate()` enforces:

- `PinnedCompositions`: at most 50 entries; each non-empty and at most 128
  characters; duplicates rejected
- `DefaultTimeRange`: must be exactly one of `"1h"`, `"6h"`, `"24h"` — the set
  declared by `TimeRange` in
  `studio/src/components/charts/TimeRangeSelector.tsx`. If that union ever
  gains a member, this validator must be updated in the same change or the new
  range will 400
- Decoding uses `Decoder.DisallowUnknownFields`, so a misspelled preference
  key fails loudly with a 400 instead of being silently dropped

**Request and response types are separate.** The response carries an
`initialized` field (see reconcile rule below); the request does not. Reusing
one struct would make `DisallowUnknownFields` reject a client that echoed back
what it just received.

```go
type PreferencesRequest  struct { /* the three preference fields only */ }
type PreferencesResponse struct { /* the three fields + Initialized bool */ }
```

### Migration

`internal/registry/migrations/20260728000000_browser_sessions.sql`:

```sql
-- +goose Up
CREATE TABLE browser_sessions (
    id          TEXT PRIMARY KEY,
    preferences TEXT,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS browser_sessions;
```

`preferences` is nullable **by design** — `NULL` means "no PUT has ever
happened," which the reconcile rule below depends on. It is not defaulted to
`'{}'`, because that would erase the distinction between "never written" and
"written empty."

The migration lives in `internal/registry/migrations/` rather than a new
package directory. goose tracks applied versions in a single
`goose_db_version` table; a second embedded FS calling `goose.Up` against the
same database would interleave version numbers and report the other package's
migrations as missing. One migration directory is the correct call here even
though the table is not conceptually part of the config registry.

---

## API

### `GET /api/v1/preferences`

Mints a session if the cookie is absent. Always 200.

```json
{
  "pinned_compositions": [],
  "sidebar_collapsed": false,
  "default_time_range": "1h",
  "initialized": false
}
```

`initialized` is `false` when the `preferences` column is `NULL`.

### `PUT /api/v1/preferences`

Body is the three preference fields. Returns 200 with the stored
`PreferencesResponse` (`initialized: true`).

---

## Data flow

### Cold load

1. `GET /` — no cookie → middleware mints ID, `EnsureSession` inserts the row,
   sets the cookie, serves `index.html`
2. SPA boots → reads `localStorage["restitch.prefs"]` **synchronously** → first
   paint is already correct, no layout flash
3. SPA issues `GET /api/v1/preferences` in parallel (cookie auto-attached)
4. Reconcile (below) → write result to `localStorage` → render

### The reconcile rule

Plain "server wins" is wrong in one reachable case: a user clears cookies but
not localStorage. The server then sees a brand-new session with no stored
preferences, and "server wins" would **silently wipe their real preferences**.

The rule is therefore:

- `initialized == true` → **server wins**; overwrite localStorage with the
  server payload
- `initialized == false` → **adopt localStorage** and immediately `PUT` it to
  seed the new session

This is the entire reason the `initialized` flag and the nullable
`preferences` column exist.

### Writes

Preference changes update React state immediately, write through to
localStorage synchronously, and `PUT` to the server debounced at ~500 ms.

---

## Frontend

### `studio/src/hooks/usePreferences.ts`

Exposes a context provider plus a `usePreferences()` hook returning the current
preferences and typed setters. Owns localStorage mirroring, the reconcile rule,
and debounced PUTs. Provider is mounted in `App.tsx` above `BrowserRouter`.

`studio/src/lib/api.ts` gains `getPreferences()` and `putPreferences()`. Both
must send `credentials: "same-origin"` so the `HttpOnly` cookie is attached —
without it `fetch` omits cookies and every preference silently lands on a new
session.

### New UI

**Sidebar collapse** (`App.tsx`): a toggle button collapses the `w-52` nav to
an icon-only rail. Collapsed state is read from `usePreferences` and persisted
on change.

**Composition pinning** (`Compositions.tsx`): a leading icon-button column
holds the pin control. Because this page is a table rather than cards, the pin
is a table cell affordance, not a card affordance — this is an intentional
divergence from PLAN.md's "composition cards" wording, which describes UI that
does not exist. Pinned rows sort to the top of the table.

**Default time range** (`Dashboard.tsx`): `useState<TimeRange>("1h")` is
seeded from preferences and writes back on change.

---

## Error handling

| Case | Behavior |
|------|----------|
| Missing/unknown cookie on `PUT` | Mint a fresh session and apply. There is no auth concept in Studio, so 401 would be wrong |
| Malformed JSON | 400 |
| Validation failure | 400 with per-field errors, matching `internal/registry/validator.go`'s structured-error style |
| Oversized body | `http.MaxBytesReader` capped at 16 KB → 413 |
| DB error | 500, logged via `slog` |
| Frontend fetch failure | Retain localStorage state, never block the UI, retry on next mutation |

---

## Testing

### Go

- Session ID: correct length, and uniqueness across many generations
- Cookie attributes: `HttpOnly`, `SameSite=Strict`, `Max-Age`, `Path`
- `Secure` absent over plaintext, present when `r.TLS != nil`
- **Static-asset requests do not mint a session** — this is the regression test
  for the multi-session race the middleware placement is designed to avoid
- Store round-trip: `EnsureSession` → `PutPreferences` → `GetPreferences`
- `EnsureSession` is idempotent for an existing ID
- `initialized` is `false` before first PUT and `true` after
- `Validate()` table tests covering every rejection branch
- `DisallowUnknownFields` rejects an unknown key with 400
- Body over 16 KB returns 413
- **Two-cookie-jar handler test**: two independent session IDs hold independent
  preferences — this automates the gate's "different browser" line

### Frontend (vitest)

- `usePreferences` reconcile: `initialized: true` → server payload wins
- `usePreferences` reconcile: `initialized: false` → localStorage adopted and
  PUT issued (the cookie-cleared case)
- Debounce coalesces rapid successive changes into one PUT

---

## Verification gate

**Per CLAUDE.md rule 2, replacing `scripts/gates/m25.sh`'s placeholder is the
first task of the milestone, and the user approves that script before any
feature code is written.**

The gate encodes PLAN.md's M25 verification block, with two corrections:

1. **Port.** PLAN.md's gate says `localhost:8090`. That is wrong for this
   binary — `restitch-studio` defaults to `3080`, and `8090` is what
   `restitch dev` assigns. The gate script starts Studio on an explicit
   ephemeral port rather than hardcoding either value.
2. **Anti-vacuity.** Test assertions follow the discipline already used in
   `scripts/gates/m22.sh`: capture `go test -v` output and assert the expected
   `--- PASS:` lines are literally present, because `go test -run` with a
   pattern matching nothing exits 0. This is the hole M23 hit.

### MANUAL line

PLAN.md's gate opens with "Open Studio in browser → cookie is set." That is a
manual check. Under CLAUDE.md rule 6 it is surfaced to the user and **not**
checked off by the executor; the automated `curl -c cookies.txt` equivalent
covers the mechanism, but the literal browser step requires user confirmation
before a `MANUAL-VERIFIED` ledger row may be added.

---

## Out of scope

- The 10 `UNKNOWN-IDS` ledger rows (`*.unit`, `final.*`)
- The M24 CI load-test placeholder `P95_MS: "REPLACE_IN_STEP_6"`
  (`.github/workflows/ci.yml:135`), which needs a measured run on a real
  GitHub runner
- Any authentication or multi-user concept — sessions are per-browser and
  unauthenticated by design
- Server-side session pruning. Rows are small and bounded by browser count;
  adding a reaper is not justified at this scale
