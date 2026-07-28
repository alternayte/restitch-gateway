# M25 — Browser Session & User Preferences Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give each browser a cookie-identified, login-free session whose UI preferences (pinned compositions, sidebar collapse, default time range) persist across reloads.

**Architecture:** A new `internal/session` package holds cookie minting, a SQLite-backed store, and a typed+validated `Preferences` model. `cmd/restitch-studio` gains `GET`/`PUT /api/v1/preferences` handlers wrapped in session middleware. The React SPA paints instantly from a `localStorage` mirror, then reconciles against the server using an `initialized` flag that distinguishes "never written" from "written empty."

**Tech Stack:** Go 1.25.6, `modernc.org/sqlite`, goose migrations, `net/http.ServeMux` (Go 1.22+ pattern routing), React 19, Vite, Tailwind v4, vitest, `@testing-library/react`.

## Global Constraints

- Go module: `github.com/restitch/restitch-gateway`, Go 1.25.6
- Cookie name: `restitch_browser_id` — exact string, no variation
- Cookie: 256-bit random value, `Max-Age` 31536000, `HttpOnly`, `SameSite=Strict`, `Path=/`, `Secure` **only** when `r.TLS != nil`
- Valid `default_time_range` values: exactly `"1h"`, `"6h"`, `"24h"` — the set declared by `TimeRange` in `studio/src/components/charts/TimeRangeSelector.tsx`
- `PinnedCompositions`: max 50 entries, each non-empty and max 128 chars, no duplicates
- Request body cap: 16 KB (16384 bytes) via `http.MaxBytesReader`
- The `preferences` DB column is **nullable by design**. `NULL` means "no PUT has ever happened." Never default it to `'{}'` — that erases the distinction the reconcile rule depends on.
- Commit messages: `feat(M25): <task title>` (or `fix:`/`test:`/`docs:`). Gate script changes use the `gate:` prefix.
- After every task: append one row to `docs/plan-progress/LEDGER.md` and commit it with the work.
- **NEVER** edit `scripts/verify.sh`, `scripts/check-ledger.sh`, `scripts/lib/`, or other gate scripts to make a failing check pass.

## File Structure

| File | Responsibility |
|------|----------------|
| `scripts/gates/m25.sh` | **Replace placeholder.** Encodes PLAN.md's M25 verification gate |
| `internal/registry/migrations/20260728000000_browser_sessions.sql` | `browser_sessions` table |
| `internal/session/types.go` | `Preferences`, `DefaultPreferences()`, `Validate()`, validation error types |
| `internal/session/store.go` | `Store`: `EnsureSession`, `GetPreferences`, `PutPreferences` |
| `internal/session/session.go` | Cookie minting middleware, `NewID`, `FromContext`, mint predicates |
| `cmd/restitch-studio/preferences.go` | `PreferencesAPI` HTTP handlers |
| `cmd/restitch-studio/main.go` | Wire store, middleware, routes; `buildMux` takes a `muxDeps` struct |
| `studio/src/lib/api.ts` | `getPreferences()` / `putPreferences()` with `credentials: "same-origin"` |
| `studio/src/hooks/usePreferences.tsx` | Provider + hook: localStorage mirror, reconcile, debounced PUT |
| `studio/src/App.tsx` | Mount provider; sidebar collapse toggle |
| `studio/src/pages/Compositions.tsx` | Pin column; pinned rows sort first |
| `studio/src/pages/Dashboard.tsx` | Seed time range from preferences |

---

### Task 1: Gate script — replace the M25 placeholder

**BLOCKING: This task requires explicit user approval of the gate script before ANY feature code is written (CLAUDE.md rule 2). Do not proceed to Task 2 until the user approves.**

**Files:**
- Modify: `scripts/gates/m25.sh` (currently a 14-line placeholder)

**Interfaces:**
- Consumes: `scripts/lib/harness.sh` — `h_init`, `h_task`, `h_run`, `h_manual`, `h_finish`, `h_build`, `h_start_studio`, `h_assert_json_body`, `$STUDIO_PORT`, `$H_TMP`
- Produces: gate task IDs `T25.1`, `T25.2`, `T25.3`, `T25.4`, `M25.unit`, `M25.gate`

- [ ] **Step 1: Write the gate script**

Replace the entire contents of `scripts/gates/m25.sh`:

```bash
#!/usr/bin/env bash
# Gate M25 — Browser Session & User Preferences
set -euo pipefail
source "$(dirname "$0")/../lib/harness.sh"
h_init M25

# ── T25.1: session middleware + cookie ───────────────────────────────
h_task T25.1
h_run "session.go exists" -- test -f internal/session/session.go
h_run "session_test.go exists" -- test -f internal/session/session_test.go
h_run "CookieName is restitch_browser_id" -- \
  grep -q 'CookieName = "restitch_browser_id"' internal/session/session.go
h_run "Middleware defined" -- grep -q 'func Middleware' internal/session/session.go
h_run "FromContext defined" -- grep -q 'func FromContext' internal/session/session.go
h_run "uses crypto/rand" -- grep -q '"crypto/rand"' internal/session/session.go
h_run "SameSite=Strict" -- grep -q 'SameSite: *http.SameSiteStrictMode' internal/session/session.go
h_run "HttpOnly set" -- grep -q 'HttpOnly: *true' internal/session/session.go
h_run "Secure gated on TLS" -- grep -q 'r.TLS != nil' internal/session/session.go
h_run "1-year Max-Age" -- grep -q '31536000' internal/session/session.go
# Behavioural proof. `go test -run` with a non-matching pattern exits 0, so the
# -v output is checked for each expected PASS line (the vacuity hole M23 hit).
h_run "session tests pass (non-vacuously)" -- bash -c '
  out="$(go test -count=1 -v -run "TestNewID|TestMiddleware" ./internal/session/ 2>&1)"
  echo "$out"
  echo "$out" | grep -q -- "--- PASS: TestNewIDIsUnique" || exit 1
  echo "$out" | grep -q -- "--- PASS: TestMiddlewareMintsCookie" || exit 1
  echo "$out" | grep -q -- "--- PASS: TestMiddlewareDoesNotMintOnAsset" || exit 1
  echo "$out" | grep -q -- "--- PASS: TestMiddlewareSecureOnlyOverTLS" || exit 1
  ! echo "$out" | grep -q -- "--- FAIL"
'

# ── T25.2: preferences CRUD API ──────────────────────────────────────
h_task T25.2
h_run "preferences.go exists" -- test -f cmd/restitch-studio/preferences.go
h_run "preferences_test.go exists" -- test -f cmd/restitch-studio/preferences_test.go
h_run "GET route registered" -- \
  grep -q 'GET /api/v1/preferences' cmd/restitch-studio/main.go
h_run "PUT route registered" -- \
  grep -q 'PUT /api/v1/preferences' cmd/restitch-studio/main.go
h_run "types.go exists" -- test -f internal/session/types.go
h_run "Validate defined" -- grep -q 'func (p \*Preferences) Validate' internal/session/types.go
h_run "rejects unknown fields" -- \
  grep -q 'DisallowUnknownFields' cmd/restitch-studio/preferences.go
h_run "16KB body cap" -- grep -q 'MaxBytesReader' cmd/restitch-studio/preferences.go
h_run "validation tests pass (non-vacuously)" -- bash -c '
  out="$(go test -count=1 -v -run "TestValidate|TestPreferences" ./internal/session/ ./cmd/restitch-studio/ 2>&1)"
  echo "$out"
  echo "$out" | grep -q -- "--- PASS: TestValidateRejectsBadTimeRange" || exit 1
  echo "$out" | grep -q -- "--- PASS: TestValidateRejectsTooManyPins" || exit 1
  echo "$out" | grep -q -- "--- PASS: TestPreferencesTwoSessionsAreIndependent" || exit 1
  ! echo "$out" | grep -q -- "--- FAIL"
'

# ── T25.3: browser_sessions migration ────────────────────────────────
h_task T25.3
h_run "migration file exists" -- \
  test -f internal/registry/migrations/20260728000000_browser_sessions.sql
h_run "creates browser_sessions" -- \
  grep -q 'CREATE TABLE browser_sessions' internal/registry/migrations/20260728000000_browser_sessions.sql
h_run "preferences column is nullable" -- bash -c '
  ! grep -E "preferences[[:space:]]+TEXT[^,]*NOT NULL" \
    internal/registry/migrations/20260728000000_browser_sessions.sql
'
h_run "has goose Down" -- \
  grep -q 'DROP TABLE IF EXISTS browser_sessions' internal/registry/migrations/20260728000000_browser_sessions.sql
h_run "store tests pass (non-vacuously)" -- bash -c '
  out="$(go test -count=1 -v -run "TestStore" ./internal/session/ 2>&1)"
  echo "$out"
  echo "$out" | grep -q -- "--- PASS: TestStoreRoundTrip" || exit 1
  echo "$out" | grep -q -- "--- PASS: TestStoreUninitializedUntilPut" || exit 1
  ! echo "$out" | grep -q -- "--- FAIL"
'

# ── T25.4: frontend wiring ───────────────────────────────────────────
h_task T25.4
h_run "usePreferences hook exists" -- test -f studio/src/hooks/usePreferences.tsx
h_run "usePreferences test exists" -- test -f studio/src/hooks/usePreferences.test.tsx
h_run "api client sends cookies" -- \
  grep -q 'credentials: "same-origin"' studio/src/lib/api.ts
h_run "getPreferences in api client" -- grep -q 'getPreferences' studio/src/lib/api.ts
h_run "putPreferences in api client" -- grep -q 'putPreferences' studio/src/lib/api.ts
h_run "provider mounted in App" -- grep -q 'PreferencesProvider' studio/src/App.tsx
h_run "sidebar collapse wired" -- grep -q 'sidebarCollapsed' studio/src/App.tsx
h_run "pin control on compositions" -- grep -q 'pinnedCompositions' studio/src/pages/Compositions.tsx
h_run "dashboard seeds time range" -- grep -q 'defaultTimeRange' studio/src/pages/Dashboard.tsx
h_run "frontend tests pass" -- bash -c 'cd studio && npm run test 2>&1'

# ── Unit tests ───────────────────────────────────────────────────────
h_task M25.unit
h_run "go vet" -- go vet ./...
h_run "session tests" -- go test -race -count=1 ./internal/session/...
h_run "studio cmd tests" -- go test -race -count=1 ./cmd/restitch-studio/...
h_run "full test suite" -- go test -race -count=1 ./...

# ── Live gate ────────────────────────────────────────────────────────
h_task M25.gate
h_build
h_start_studio -db-path "${H_TMP}/gate.db"

BASE="http://127.0.0.1:${STUDIO_PORT}"
JAR_A="${H_TMP}/jar_a.txt"
JAR_B="${H_TMP}/jar_b.txt"

# Document request mints the cookie with the right attributes.
HDRS="${H_TMP}/doc_headers.txt"
curl -s -c "${JAR_A}" -D "${HDRS}" -H 'Accept: text/html' \
  -o /dev/null "${BASE}/" || true
h_evidence "$(cat "${HDRS}")"
h_run "document sets restitch_browser_id" -- \
  grep -qi 'set-cookie:.*restitch_browser_id=' "${HDRS}"
h_run "cookie is HttpOnly" -- grep -qi 'set-cookie:.*HttpOnly' "${HDRS}"
h_run "cookie is SameSite=Strict" -- grep -qi 'set-cookie:.*SameSite=Strict' "${HDRS}"
h_run "cookie has 1-year Max-Age" -- grep -qi 'set-cookie:.*Max-Age=31536000' "${HDRS}"
h_run "cookie is not Secure over plaintext" -- bash -c "
  ! grep -i 'set-cookie:.*restitch_browser_id' '${HDRS}' | grep -qi 'Secure'
"

# Static assets must NOT mint — this is the multi-session race regression test.
ASSET_HDRS="${H_TMP}/asset_headers.txt"
curl -s -D "${ASSET_HDRS}" -H 'Accept: */*' \
  -o /dev/null "${BASE}/favicon.svg" || true
h_run "static asset does not mint a session" -- bash -c "
  ! grep -qi 'set-cookie:.*restitch_browser_id' '${ASSET_HDRS}'
"

# Fresh session reads back defaults, uninitialized.
BODY="$(curl -s -b "${JAR_A}" -c "${JAR_A}" "${BASE}/api/v1/preferences")"
h_assert_json_body "${BODY}" \
  'data["initialized"] == False and data["pinned_compositions"] == [] and data["default_time_range"] == "1h"' \
  "fresh session returns uninitialized defaults"

# PUT persists.
PUT_BODY="$(curl -s -b "${JAR_A}" -c "${JAR_A}" -X PUT \
  -H 'Content-Type: application/json' \
  -d '{"pinned_compositions":["comp-1"],"sidebar_collapsed":true,"default_time_range":"6h"}' \
  "${BASE}/api/v1/preferences")"
h_assert_json_body "${PUT_BODY}" \
  'data["initialized"] == True and data["pinned_compositions"] == ["comp-1"]' \
  "PUT stores preferences"

# Re-read persists (the 'refresh browser' equivalent).
GET_BODY="$(curl -s -b "${JAR_A}" "${BASE}/api/v1/preferences")"
h_assert_json_body "${GET_BODY}" \
  'data["pinned_compositions"] == ["comp-1"] and data["sidebar_collapsed"] == True and data["default_time_range"] == "6h"' \
  "preferences persist across requests"

# A second cookie jar is an independent browser.
B_BODY="$(curl -s -c "${JAR_B}" -b "${JAR_B}" "${BASE}/api/v1/preferences")"
h_assert_json_body "${B_BODY}" \
  'data["initialized"] == False and data["pinned_compositions"] == []' \
  "different browser has independent preferences"

# Validation failures.
BAD_RANGE=$(curl -s -o /dev/null -w '%{http_code}' -b "${JAR_A}" -X PUT \
  -H 'Content-Type: application/json' \
  -d '{"pinned_compositions":[],"sidebar_collapsed":false,"default_time_range":"7d"}' \
  "${BASE}/api/v1/preferences")
h_run "invalid time range is rejected with 400" -- test "${BAD_RANGE}" = "400"

UNKNOWN=$(curl -s -o /dev/null -w '%{http_code}' -b "${JAR_A}" -X PUT \
  -H 'Content-Type: application/json' \
  -d '{"pinned_compositions":[],"sidebar_collapsed":false,"default_time_range":"1h","bogus":1}' \
  "${BASE}/api/v1/preferences")
h_run "unknown field is rejected with 400" -- test "${UNKNOWN}" = "400"

OVERSIZE=$(python3 -c 'print("x"*20000)')
TOOBIG=$(curl -s -o /dev/null -w '%{http_code}' -b "${JAR_A}" -X PUT \
  -H 'Content-Type: application/json' \
  -d "{\"pinned_compositions\":[\"${OVERSIZE}\"],\"sidebar_collapsed\":false,\"default_time_range\":\"1h\"}" \
  "${BASE}/api/v1/preferences")
h_run "oversized body is rejected with 413" -- test "${TOOBIG}" = "413"

h_manual "Open Studio in a real browser at ${BASE} — confirm a restitch_browser_id cookie is set in DevTools > Application > Cookies, collapse the sidebar and pin a composition, then reload and confirm both persist."

h_finish
```

- [ ] **Step 2: Verify the script self-fails cleanly before implementation**

Run: `scripts/gates/m25.sh; echo "EXIT=$?"`
Expected: many `FAIL` lines (nothing is implemented yet) and a non-zero exit. This confirms the gate is not vacuous — a gate that passes before any code is written is worthless.

- [ ] **Step 3: STOP — get user approval**

Present the script to the user. Do not continue until they approve. Per CLAUDE.md, this commit uses the `gate:` prefix.

- [ ] **Step 4: Commit (only after approval)**

```bash
git add scripts/gates/m25.sh
git commit -m "gate: implement M25 verification gate (browser session & preferences)"
```

---

### Task 2: `browser_sessions` migration (T25.3)

**Files:**
- Create: `internal/registry/migrations/20260728000000_browser_sessions.sql`

**Interfaces:**
- Produces: table `browser_sessions(id TEXT PK, preferences TEXT NULL, created_at, updated_at)`, applied by the existing `registry.RunMigrations`

- [ ] **Step 1: Write the migration**

Create `internal/registry/migrations/20260728000000_browser_sessions.sql`:

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

`preferences` is intentionally nullable — `NULL` means "never written," which the reconcile rule in Task 7 depends on.

- [ ] **Step 2: Verify the migration applies**

Run:
```bash
go test -count=1 -run TestRunMigrations ./internal/registry/ -v 2>&1 | tail -20
```
Expected: PASS, or no such test — in which case verify manually:
```bash
go run ./cmd/restitch-studio -db-path /tmp/m25check.db -port 0 &
sleep 2; kill %1
sqlite3 /tmp/m25check.db ".schema browser_sessions"
rm -f /tmp/m25check.db*
```
Expected: the `CREATE TABLE browser_sessions` DDL prints.

- [ ] **Step 3: Commit**

```bash
git add internal/registry/migrations/20260728000000_browser_sessions.sql
git commit -m "feat(M25): add browser_sessions migration (T25.3)"
```

- [ ] **Step 4: Append ledger row**

Add to `docs/plan-progress/LEDGER.md` (append only, never edit existing rows), then amend the commit to include it.

---

### Task 3: `Preferences` type and validation (T25.1)

**Files:**
- Create: `internal/session/types.go`
- Test: `internal/session/types_test.go`

**Interfaces:**
- Produces:
  - `type Preferences struct { PinnedCompositions []string; SidebarCollapsed bool; DefaultTimeRange string }`
  - `func DefaultPreferences() Preferences`
  - `func (p *Preferences) Validate() error`
  - `type FieldError struct { Field, Message string }`
  - `type ValidationError struct { Errors []FieldError }` implementing `error`

- [ ] **Step 1: Write the failing tests**

Create `internal/session/types_test.go`:

```go
package session

import "testing"

func TestDefaultPreferences(t *testing.T) {
	p := DefaultPreferences()
	if p.DefaultTimeRange != "1h" {
		t.Errorf("DefaultTimeRange = %q, want 1h", p.DefaultTimeRange)
	}
	if p.SidebarCollapsed {
		t.Error("SidebarCollapsed should default to false")
	}
	if p.PinnedCompositions == nil {
		t.Error("PinnedCompositions must be an empty slice, not nil (it marshals to [] not null)")
	}
	if len(p.PinnedCompositions) != 0 {
		t.Errorf("PinnedCompositions = %v, want empty", p.PinnedCompositions)
	}
}

func TestValidateAcceptsEveryTimeRange(t *testing.T) {
	for _, tr := range []string{"1h", "6h", "24h"} {
		p := Preferences{PinnedCompositions: []string{}, DefaultTimeRange: tr}
		if err := p.Validate(); err != nil {
			t.Errorf("Validate() with range %q returned %v, want nil", tr, err)
		}
	}
}

func TestValidateRejectsBadTimeRange(t *testing.T) {
	for _, tr := range []string{"", "7d", "1H", "60m"} {
		p := Preferences{PinnedCompositions: []string{}, DefaultTimeRange: tr}
		if err := p.Validate(); err == nil {
			t.Errorf("Validate() with range %q returned nil, want error", tr)
		}
	}
}

func TestValidateRejectsTooManyPins(t *testing.T) {
	pins := make([]string, 51)
	for i := range pins {
		pins[i] = string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	p := Preferences{PinnedCompositions: pins, DefaultTimeRange: "1h"}
	if err := p.Validate(); err == nil {
		t.Fatal("Validate() with 51 pins returned nil, want error")
	}
}

func TestValidateRejectsEmptyAndOverlongPins(t *testing.T) {
	long := make([]byte, 129)
	for i := range long {
		long[i] = 'x'
	}
	cases := map[string][]string{
		"empty pin":    {""},
		"overlong pin": {string(long)},
	}
	for name, pins := range cases {
		p := Preferences{PinnedCompositions: pins, DefaultTimeRange: "1h"}
		if err := p.Validate(); err == nil {
			t.Errorf("%s: Validate() returned nil, want error", name)
		}
	}
}

func TestValidateRejectsDuplicatePins(t *testing.T) {
	p := Preferences{PinnedCompositions: []string{"a", "a"}, DefaultTimeRange: "1h"}
	if err := p.Validate(); err == nil {
		t.Fatal("Validate() with duplicate pins returned nil, want error")
	}
}

func TestValidateNormalisesNilPins(t *testing.T) {
	p := Preferences{PinnedCompositions: nil, DefaultTimeRange: "1h"}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate() returned %v, want nil", err)
	}
	if p.PinnedCompositions == nil {
		t.Error("Validate() must normalise nil pins to an empty slice")
	}
}

func TestValidationErrorNamesFields(t *testing.T) {
	p := Preferences{PinnedCompositions: []string{""}, DefaultTimeRange: "nope"}
	err := p.Validate()
	if err == nil {
		t.Fatal("Validate() returned nil, want error")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("Validate() returned %T, want *ValidationError", err)
	}
	if len(ve.Errors) != 2 {
		t.Errorf("got %d field errors, want 2: %+v", len(ve.Errors), ve.Errors)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/ -v`
Expected: FAIL — the package does not exist yet (`no Go files in .../internal/session`).

- [ ] **Step 3: Write the implementation**

Create `internal/session/types.go`:

```go
// Package session provides cookie-identified, login-free browser sessions
// for Studio and the per-browser UI preferences attached to them.
package session

import (
	"fmt"
	"strings"
)

const (
	// maxPinnedCompositions caps how many compositions one browser may pin.
	maxPinnedCompositions = 50
	// maxPinnedNameLen caps the length of a single pinned composition name.
	maxPinnedNameLen = 128
)

// validTimeRanges mirrors the TimeRange union in
// studio/src/components/charts/TimeRangeSelector.tsx. If that union gains a
// member, this map must be updated in the same change or the new range 400s.
var validTimeRanges = map[string]bool{"1h": true, "6h": true, "24h": true}

// Preferences is the per-browser UI state Studio persists.
type Preferences struct {
	PinnedCompositions []string `json:"pinned_compositions"`
	SidebarCollapsed   bool     `json:"sidebar_collapsed"`
	DefaultTimeRange   string   `json:"default_time_range"`
}

// FieldError describes one validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError aggregates per-field validation failures.
type ValidationError struct {
	Errors []FieldError `json:"errors"`
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Errors))
	for _, fe := range e.Errors {
		parts = append(parts, fe.Field+": "+fe.Message)
	}
	return "invalid preferences: " + strings.Join(parts, "; ")
}

// DefaultPreferences returns the preferences a browser starts with. The
// pinned slice is non-nil so it marshals to [] rather than null.
func DefaultPreferences() Preferences {
	return Preferences{
		PinnedCompositions: []string{},
		SidebarCollapsed:   false,
		DefaultTimeRange:   "1h",
	}
}

// Validate checks p field by field and normalises a nil pinned slice to an
// empty one. It returns a *ValidationError listing every problem found.
func (p *Preferences) Validate() error {
	var errs []FieldError

	if p.PinnedCompositions == nil {
		p.PinnedCompositions = []string{}
	}

	if len(p.PinnedCompositions) > maxPinnedCompositions {
		errs = append(errs, FieldError{
			Field:   "pinned_compositions",
			Message: fmt.Sprintf("at most %d entries allowed, got %d", maxPinnedCompositions, len(p.PinnedCompositions)),
		})
	}

	seen := make(map[string]bool, len(p.PinnedCompositions))
	for i, name := range p.PinnedCompositions {
		switch {
		case name == "":
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("pinned_compositions[%d]", i),
				Message: "must not be empty",
			})
		case len(name) > maxPinnedNameLen:
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("pinned_compositions[%d]", i),
				Message: fmt.Sprintf("must be at most %d characters", maxPinnedNameLen),
			})
		case seen[name]:
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("pinned_compositions[%d]", i),
				Message: "duplicate entry " + name,
			})
		}
		seen[name] = true
	}

	if !validTimeRanges[p.DefaultTimeRange] {
		errs = append(errs, FieldError{
			Field:   "default_time_range",
			Message: "must be one of 1h, 6h, 24h",
		})
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/ -v`
Expected: PASS for all eight tests.

- [ ] **Step 5: Commit**

```bash
git add internal/session/types.go internal/session/types_test.go
git commit -m "feat(M25): preferences type and field validation (T25.1)"
```

- [ ] **Step 6: Append ledger row and amend the commit**

---

### Task 4: Session store (T25.1)

**Files:**
- Create: `internal/session/store.go`
- Test: `internal/session/store_test.go`

**Interfaces:**
- Consumes: `Preferences`, `DefaultPreferences()` from Task 3; the `browser_sessions` table from Task 2
- Produces:
  - `func NewStore(db *sql.DB) *Store`
  - `func (s *Store) EnsureSession(ctx context.Context, id string) error`
  - `func (s *Store) GetPreferences(ctx context.Context, id string) (Preferences, bool, error)` — returns prefs, `initialized`, error
  - `func (s *Store) PutPreferences(ctx context.Context, id string, p Preferences) error`

- [ ] **Step 1: Write the failing tests**

Create `internal/session/store_test.go`:

```go
package session

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (registers as "sqlite")

	"github.com/restitch/restitch-gateway/internal/registry"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := registry.RunMigrations(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStore(db)
}

func TestStoreUninitializedUntilPut(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.EnsureSession(ctx, "sess-1"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	prefs, initialized, err := s.GetPreferences(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if initialized {
		t.Error("initialized = true before any PUT, want false")
	}
	if prefs.DefaultTimeRange != "1h" {
		t.Errorf("DefaultTimeRange = %q, want the 1h default", prefs.DefaultTimeRange)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.EnsureSession(ctx, "sess-1"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	want := Preferences{
		PinnedCompositions: []string{"comp-a", "comp-b"},
		SidebarCollapsed:   true,
		DefaultTimeRange:   "6h",
	}
	if err := s.PutPreferences(ctx, "sess-1", want); err != nil {
		t.Fatalf("PutPreferences: %v", err)
	}

	got, initialized, err := s.GetPreferences(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if !initialized {
		t.Error("initialized = false after PUT, want true")
	}
	if got.DefaultTimeRange != "6h" || !got.SidebarCollapsed {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if len(got.PinnedCompositions) != 2 || got.PinnedCompositions[0] != "comp-a" {
		t.Errorf("PinnedCompositions = %v, want %v", got.PinnedCompositions, want.PinnedCompositions)
	}
}

func TestStoreEnsureSessionIsIdempotent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.EnsureSession(ctx, "sess-1"); err != nil {
		t.Fatalf("first EnsureSession: %v", err)
	}
	if err := s.PutPreferences(ctx, "sess-1", Preferences{
		PinnedCompositions: []string{"keep-me"}, DefaultTimeRange: "1h",
	}); err != nil {
		t.Fatalf("PutPreferences: %v", err)
	}
	if err := s.EnsureSession(ctx, "sess-1"); err != nil {
		t.Fatalf("second EnsureSession: %v", err)
	}

	got, initialized, err := s.GetPreferences(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if !initialized || len(got.PinnedCompositions) != 1 {
		t.Errorf("EnsureSession clobbered existing preferences: %+v initialized=%v", got, initialized)
	}
}

func TestStoreUnknownSessionReturnsDefaults(t *testing.T) {
	s := testStore(t)
	prefs, initialized, err := s.GetPreferences(context.Background(), "never-seen")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if initialized {
		t.Error("initialized = true for unknown session, want false")
	}
	if prefs.DefaultTimeRange != "1h" {
		t.Errorf("DefaultTimeRange = %q, want 1h", prefs.DefaultTimeRange)
	}
}

func TestStoreSessionsAreIsolated(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for _, id := range []string{"a", "b"} {
		if err := s.EnsureSession(ctx, id); err != nil {
			t.Fatalf("EnsureSession(%s): %v", id, err)
		}
	}
	if err := s.PutPreferences(ctx, "a", Preferences{
		PinnedCompositions: []string{"only-a"}, DefaultTimeRange: "24h",
	}); err != nil {
		t.Fatalf("PutPreferences: %v", err)
	}

	got, initialized, err := s.GetPreferences(ctx, "b")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if initialized || len(got.PinnedCompositions) != 0 {
		t.Errorf("session b saw session a's preferences: %+v initialized=%v", got, initialized)
	}
}

func TestStorePutOverwrites(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.EnsureSession(ctx, "sess-1"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	first := Preferences{PinnedCompositions: []string{"x"}, DefaultTimeRange: "1h"}
	second := Preferences{PinnedCompositions: []string{"y", "z"}, DefaultTimeRange: "24h"}
	if err := s.PutPreferences(ctx, "sess-1", first); err != nil {
		t.Fatalf("first PutPreferences: %v", err)
	}
	if err := s.PutPreferences(ctx, "sess-1", second); err != nil {
		t.Fatalf("second PutPreferences: %v", err)
	}

	got, _, err := s.GetPreferences(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetPreferences: %v", err)
	}
	if got.DefaultTimeRange != "24h" || len(got.PinnedCompositions) != 2 {
		t.Errorf("got %+v, want %+v", got, second)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/ -run TestStore -v`
Expected: FAIL — `undefined: NewStore`.

- [ ] **Step 3: Write the implementation**

Create `internal/session/store.go`:

```go
package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Store provides data access for browser sessions and their preferences.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store wrapping db.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// EnsureSession inserts a row for id if none exists. It never overwrites
// existing preferences, so it is safe to call on every request.
func (s *Store) EnsureSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO browser_sessions (id) VALUES (?) ON CONFLICT(id) DO NOTHING`, id)
	if err != nil {
		return fmt.Errorf("ensure session: %w", err)
	}
	return nil
}

// GetPreferences returns the preferences for id along with whether they have
// ever been written. An unknown session, or one whose preferences column is
// still NULL, yields DefaultPreferences() and initialized=false.
func (s *Store) GetPreferences(ctx context.Context, id string) (Preferences, bool, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT preferences FROM browser_sessions WHERE id = ?`, id).Scan(&raw)
	switch {
	case err == sql.ErrNoRows:
		return DefaultPreferences(), false, nil
	case err != nil:
		return DefaultPreferences(), false, fmt.Errorf("get preferences: %w", err)
	}

	if !raw.Valid || raw.String == "" {
		return DefaultPreferences(), false, nil
	}

	prefs := DefaultPreferences()
	if err := json.Unmarshal([]byte(raw.String), &prefs); err != nil {
		return DefaultPreferences(), false, fmt.Errorf("decode preferences: %w", err)
	}
	if prefs.PinnedCompositions == nil {
		prefs.PinnedCompositions = []string{}
	}
	return prefs, true, nil
}

// PutPreferences persists p for id, creating the session row if needed.
func (s *Store) PutPreferences(ctx context.Context, id string, p Preferences) error {
	if p.PinnedCompositions == nil {
		p.PinnedCompositions = []string{}
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode preferences: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO browser_sessions (id, preferences) VALUES (?, ?)
		ON CONFLICT(id) DO UPDATE SET
			preferences = excluded.preferences,
			updated_at  = CURRENT_TIMESTAMP`, id, string(encoded))
	if err != nil {
		return fmt.Errorf("put preferences: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -count=1 ./internal/session/ -v`
Expected: PASS for all store tests.

- [ ] **Step 5: Commit**

```bash
git add internal/session/store.go internal/session/store_test.go
git commit -m "feat(M25): browser session store with initialized tracking (T25.1)"
```

- [ ] **Step 6: Append ledger row and amend the commit**

---

### Task 5: Cookie middleware (T25.1)

**Files:**
- Create: `internal/session/session.go`
- Test: `internal/session/session_test.go`

**Interfaces:**
- Consumes: `*Store` and `EnsureSession` from Task 4
- Produces:
  - `const CookieName = "restitch_browser_id"`
  - `func NewID() (string, error)`
  - `func FromContext(ctx context.Context) (string, bool)`
  - `func Middleware(store *Store, shouldMint func(*http.Request) bool) func(http.Handler) http.Handler`
  - `func MintOnDocument(r *http.Request) bool`
  - `func AlwaysMint(r *http.Request) bool`

- [ ] **Step 1: Write the failing tests**

Create `internal/session/session_test.go`:

```go
package session

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// handlerCapturingSession records the session ID the middleware injected.
func handlerCapturingSession(got *string, ok *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got, *ok = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
}

func TestNewIDIsUnique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatalf("NewID: %v", err)
		}
		if len(id) < 40 {
			t.Fatalf("NewID returned %q (len %d), want a 256-bit base64url value", id, len(id))
		}
		if seen[id] {
			t.Fatalf("NewID produced a duplicate: %q", id)
		}
		seen[id] = true
	}
}

func TestMiddlewareMintsCookie(t *testing.T) {
	store := testStore(t)
	var gotID string
	var ok bool
	h := Middleware(store, AlwaysMint)(handlerCapturingSession(&gotID, &ok))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != CookieName {
		t.Errorf("cookie name = %q, want %q", c.Name, CookieName)
	}
	if !c.HttpOnly {
		t.Error("cookie must be HttpOnly")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", c.SameSite)
	}
	if c.MaxAge != 31536000 {
		t.Errorf("MaxAge = %d, want 31536000", c.MaxAge)
	}
	if c.Path != "/" {
		t.Errorf("Path = %q, want /", c.Path)
	}
	if !ok || gotID != c.Value {
		t.Errorf("context session = %q (ok=%v), want %q", gotID, ok, c.Value)
	}
}

func TestMiddlewareReusesExistingCookie(t *testing.T) {
	store := testStore(t)
	var gotID string
	var ok bool
	h := Middleware(store, AlwaysMint)(handlerCapturingSession(&gotID, &ok))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "existing-session"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if len(rec.Result().Cookies()) != 0 {
		t.Error("middleware re-set a cookie that was already present")
	}
	if !ok || gotID != "existing-session" {
		t.Errorf("context session = %q (ok=%v), want existing-session", gotID, ok)
	}
}

func TestMiddlewareDoesNotMintOnAsset(t *testing.T) {
	store := testStore(t)
	var gotID string
	var ok bool
	h := Middleware(store, MintOnDocument)(handlerCapturingSession(&gotID, &ok))

	req := httptest.NewRequest("GET", "/assets/index-abc123.js", nil)
	req.Header.Set("Accept", "*/*")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if len(rec.Result().Cookies()) != 0 {
		t.Error("static asset request minted a session — this causes the multi-session race")
	}
	if ok {
		t.Errorf("asset request got a session in context: %q", gotID)
	}
}

func TestMiddlewareMintsOnDocumentRequests(t *testing.T) {
	cases := map[string]struct {
		path   string
		accept string
	}{
		"root path":   {"/", "*/*"},
		"html accept": {"/compositions", "text/html,application/xhtml+xml"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := testStore(t)
			var gotID string
			var ok bool
			h := Middleware(store, MintOnDocument)(handlerCapturingSession(&gotID, &ok))

			req := httptest.NewRequest("GET", tc.path, nil)
			req.Header.Set("Accept", tc.accept)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if len(rec.Result().Cookies()) != 1 {
				t.Fatalf("got %d cookies, want 1", len(rec.Result().Cookies()))
			}
			if !ok || gotID == "" {
				t.Error("document request did not get a session in context")
			}
		})
	}
}

func TestMiddlewareSecureOnlyOverTLS(t *testing.T) {
	store := testStore(t)
	h := Middleware(store, AlwaysMint)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	t.Run("plaintext", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Result().Cookies()[0].Secure {
			t.Error("Secure set over plaintext — this breaks http://localhost")
		}
	})

	t.Run("tls", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.TLS = &tls.ConnectionState{}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if !rec.Result().Cookies()[0].Secure {
			t.Error("Secure not set over TLS")
		}
	})
}

func TestMiddlewarePersistsMintedSession(t *testing.T) {
	store := testStore(t)
	h := Middleware(store, AlwaysMint)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	id := rec.Result().Cookies()[0].Value
	if _, _, err := store.GetPreferences(req.Context(), id); err != nil {
		t.Fatalf("minted session not readable from store: %v", err)
	}
}

func TestFromContextEmptyWhenAbsent(t *testing.T) {
	if id, ok := FromContext(httptest.NewRequest("GET", "/", nil).Context()); ok || id != "" {
		t.Errorf("FromContext on bare context = (%q, %v), want (\"\", false)", id, ok)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/ -run "TestNewID|TestMiddleware|TestFromContext" -v`
Expected: FAIL — `undefined: Middleware`, `undefined: NewID`, `undefined: FromContext`.

- [ ] **Step 3: Write the implementation**

Create `internal/session/session.go`:

```go
package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"
)

// CookieName is the browser session cookie. Do not vary this string.
const CookieName = "restitch_browser_id"

// cookieMaxAge is one year in seconds.
const cookieMaxAge = 31536000

// idBytes is the entropy of a session ID: 256 bits.
const idBytes = 32

type ctxKey struct{}

// NewID returns a new 256-bit session ID, base64url-encoded without padding.
func NewID() (string, error) {
	b := make([]byte, idBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// FromContext returns the session ID attached by Middleware, if any.
func FromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(ctxKey{}).(string)
	return id, ok && id != ""
}

// AlwaysMint mints a session for every cookie-less request. Use it for the
// preferences API, where a caller such as curl has no document request to
// piggyback on.
func AlwaysMint(*http.Request) bool { return true }

// MintOnDocument reports whether r looks like a browser document navigation.
//
// Minting on every cookie-less request would race: a cold load fires
// index.html plus several asset requests near-simultaneously, all without a
// cookie, producing several session rows for one browser. Restricting minting
// to document requests removes that race.
func MintOnDocument(r *http.Request) bool {
	if r.URL.Path == "/" {
		return true
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// Middleware reads the session cookie, minting a new session when the cookie
// is absent and shouldRequestMint reports true. Requests that neither carry a
// cookie nor qualify for minting pass through without a session.
func Middleware(store *Store, shouldRequestMint func(*http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := ""
			if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
				id = c.Value
			}

			if id == "" {
				if !shouldRequestMint(r) {
					next.ServeHTTP(w, r)
					return
				}
				newID, err := NewID()
				if err != nil {
					slog.Error("generate session id", "error", err)
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
				id = newID
				http.SetCookie(w, &http.Cookie{
					Name:     CookieName,
					Value:    id,
					Path:     "/",
					MaxAge:   cookieMaxAge,
					HttpOnly: true,
					SameSite: http.SameSiteStrictMode,
					// Secure would make the cookie undeliverable over
					// http://localhost, which is Studio's primary mode.
					Secure: r.TLS != nil,
				})
			}

			if err := store.EnsureSession(r.Context(), id); err != nil {
				slog.Error("ensure session", "error", err, "session_id", id)
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
		})
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race -count=1 ./internal/session/ -v`
Expected: PASS for every test in the package.

- [ ] **Step 5: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "feat(M25): browser session cookie middleware (T25.1)"
```

- [ ] **Step 6: Append ledger row and amend the commit**

---

### Task 6: Preferences API and wiring (T25.2)

**Files:**
- Create: `cmd/restitch-studio/preferences.go`
- Test: `cmd/restitch-studio/preferences_test.go`
- Modify: `cmd/restitch-studio/main.go:62`, `cmd/restitch-studio/main.go:81` (`buildMux` signature and body)
- Modify: `cmd/restitch-studio/main_test.go:22`, `cmd/restitch-studio/main_test.go:37` (call sites)
- Modify: `cmd/restitch-studio/api_test.go:70` (call site)

**Interfaces:**
- Consumes: `session.Store`, `session.Middleware`, `session.AlwaysMint`, `session.MintOnDocument`, `session.FromContext`, `session.Preferences`, `session.ValidationError`
- Produces:
  - `func NewPreferencesAPI(store *session.Store) *PreferencesAPI`
  - handlers `handleGetPreferences`, `handlePutPreferences`
  - `type muxDeps struct { gatewayAdminURL, adminKey string; registryAPI *RegistryAPI; prefsAPI *PreferencesAPI; sessionStore *session.Store }`
  - `func buildMux(d muxDeps) *http.ServeMux`

- [ ] **Step 1: Write the failing tests**

Create `cmd/restitch-studio/preferences_test.go`:

```go
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (registers as "sqlite")

	"github.com/restitch/restitch-gateway/internal/registry"
	"github.com/restitch/restitch-gateway/internal/session"
)

func prefsMux(t *testing.T) *http.ServeMux {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := registry.RunMigrations(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := session.NewStore(db)
	return buildMux(muxDeps{
		gatewayAdminURL: "http://localhost:9999",
		prefsAPI:        NewPreferencesAPI(store),
		sessionStore:    store,
	})
}

// doPrefs issues a request carrying sessionID (empty means no cookie) and
// returns the recorder.
func doPrefs(t *testing.T, mux *http.ServeMux, method, sessionID, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, "/api/v1/preferences", nil)
	} else {
		r = httptest.NewRequest(method, "/api/v1/preferences", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if sessionID != "" {
		r.AddCookie(&http.Cookie{Name: session.CookieName, Value: sessionID})
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

func TestPreferencesGetMintsAndReturnsDefaults(t *testing.T) {
	mux := prefsMux(t)
	rec := doPrefs(t, mux, "GET", "", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["initialized"] != false {
		t.Errorf("initialized = %v, want false", got["initialized"])
	}
	if got["default_time_range"] != "1h" {
		t.Errorf("default_time_range = %v, want 1h", got["default_time_range"])
	}
	if len(rec.Result().Cookies()) != 1 {
		t.Error("GET without a cookie should mint a session")
	}
}

func TestPreferencesPutThenGet(t *testing.T) {
	mux := prefsMux(t)
	put := doPrefs(t, mux, "PUT", "sess-1",
		`{"pinned_compositions":["comp-1"],"sidebar_collapsed":true,"default_time_range":"6h"}`)
	if put.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200; body=%s", put.Code, put.Body.String())
	}

	get := doPrefs(t, mux, "GET", "sess-1", "")
	var got map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["initialized"] != true {
		t.Errorf("initialized = %v, want true", got["initialized"])
	}
	if got["default_time_range"] != "6h" || got["sidebar_collapsed"] != true {
		t.Errorf("preferences did not persist: %+v", got)
	}
}

func TestPreferencesTwoSessionsAreIndependent(t *testing.T) {
	mux := prefsMux(t)
	if rec := doPrefs(t, mux, "PUT", "browser-a",
		`{"pinned_compositions":["only-a"],"sidebar_collapsed":true,"default_time_range":"24h"}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", rec.Code)
	}

	rec := doPrefs(t, mux, "GET", "browser-b", "")
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["initialized"] != false {
		t.Errorf("browser-b initialized = %v, want false", got["initialized"])
	}
	pins, _ := got["pinned_compositions"].([]any)
	if len(pins) != 0 {
		t.Errorf("browser-b saw browser-a's pins: %v", pins)
	}
}

func TestPreferencesRejectsInvalidTimeRange(t *testing.T) {
	mux := prefsMux(t)
	rec := doPrefs(t, mux, "PUT", "sess-1",
		`{"pinned_compositions":[],"sidebar_collapsed":false,"default_time_range":"7d"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "default_time_range") {
		t.Errorf("error body does not name the bad field: %s", rec.Body.String())
	}
}

func TestPreferencesRejectsUnknownField(t *testing.T) {
	mux := prefsMux(t)
	rec := doPrefs(t, mux, "PUT", "sess-1",
		`{"pinned_compositions":[],"sidebar_collapsed":false,"default_time_range":"1h","bogus":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestPreferencesRejectsMalformedJSON(t *testing.T) {
	mux := prefsMux(t)
	if rec := doPrefs(t, mux, "PUT", "sess-1", `{not json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPreferencesRejectsOversizedBody(t *testing.T) {
	mux := prefsMux(t)
	huge := strings.Repeat("x", 20000)
	rec := doPrefs(t, mux, "PUT", "sess-1",
		`{"pinned_compositions":["`+huge+`"],"sidebar_collapsed":false,"default_time_range":"1h"}`)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", rec.Code, rec.Body.String())
	}
}

func TestPreferencesPutWithoutCookieMintsSession(t *testing.T) {
	mux := prefsMux(t)
	rec := doPrefs(t, mux, "PUT", "",
		`{"pinned_compositions":["x"],"sidebar_collapsed":false,"default_time_range":"1h"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 1 {
		t.Error("PUT without a cookie should mint a session rather than 401")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/restitch-studio/ -run TestPreferences -v`
Expected: FAIL — `undefined: muxDeps`, `undefined: NewPreferencesAPI`.

- [ ] **Step 3: Write the handlers**

Create `cmd/restitch-studio/preferences.go`:

```go
package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/restitch/restitch-gateway/internal/session"
)

// maxPreferencesBody caps a preferences PUT at 16 KB.
const maxPreferencesBody = 16 * 1024

// PreferencesAPI holds HTTP handlers for /api/v1/preferences.
type PreferencesAPI struct {
	store *session.Store
}

// NewPreferencesAPI creates a PreferencesAPI backed by store.
func NewPreferencesAPI(store *session.Store) *PreferencesAPI {
	return &PreferencesAPI{store: store}
}

// preferencesResponse is the wire shape returned by both handlers. It carries
// Initialized, which the request shape deliberately does not have — the
// decoder rejects unknown fields, so a client echoing a response back as a
// request would 400 if the shapes were shared.
type preferencesResponse struct {
	PinnedCompositions []string `json:"pinned_compositions"`
	SidebarCollapsed   bool     `json:"sidebar_collapsed"`
	DefaultTimeRange   string   `json:"default_time_range"`
	Initialized        bool     `json:"initialized"`
}

func toResponse(p session.Preferences, initialized bool) preferencesResponse {
	if p.PinnedCompositions == nil {
		p.PinnedCompositions = []string{}
	}
	return preferencesResponse{
		PinnedCompositions: p.PinnedCompositions,
		SidebarCollapsed:   p.SidebarCollapsed,
		DefaultTimeRange:   p.DefaultTimeRange,
		Initialized:        initialized,
	}
}

// handleGetPreferences handles GET /api/v1/preferences.
func (a *PreferencesAPI) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	id, ok := session.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "no browser session")
		return
	}

	prefs, initialized, err := a.store.GetPreferences(r.Context(), id)
	if err != nil {
		slog.Error("get preferences", "error", err, "session_id", id)
		writeError(w, http.StatusInternalServerError, "could not read preferences")
		return
	}

	writeJSON(w, http.StatusOK, toResponse(prefs, initialized))
}

// handlePutPreferences handles PUT /api/v1/preferences.
func (a *PreferencesAPI) handlePutPreferences(w http.ResponseWriter, r *http.Request) {
	id, ok := session.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "no browser session")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPreferencesBody)

	var prefs session.Preferences
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&prefs); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body exceeds 16KB")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if err := prefs.Validate(); err != nil {
		var ve *session.ValidationError
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, ve)
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.store.PutPreferences(r.Context(), id, prefs); err != nil {
		slog.Error("put preferences", "error", err, "session_id", id)
		writeError(w, http.StatusInternalServerError, "could not save preferences")
		return
	}

	writeJSON(w, http.StatusOK, toResponse(prefs, true))
}
```

- [ ] **Step 4: Rewrite `buildMux` to take a deps struct**

In `cmd/restitch-studio/main.go`, add the `session` import, then replace the `buildMux` signature and the routing block. Replace line 81's signature:

```go
// muxDeps carries everything buildMux needs. It is a struct rather than a
// positional list because three of the five values are optional in tests.
type muxDeps struct {
	gatewayAdminURL string
	adminKey        string
	registryAPI     *RegistryAPI
	prefsAPI        *PreferencesAPI
	sessionStore    *session.Store
}

func buildMux(d muxDeps) *http.ServeMux {
	target, err := url.Parse(d.gatewayAdminURL)
	if err != nil {
		log.Fatalf("invalid gateway admin URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		ResponseHeaderTimeout: 15 * time.Second,
	}

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		if strings.HasPrefix(req.URL.Path, "/api/") {
			req.URL.Path = "/admin" + req.URL.Path
		}
		if d.adminKey != "" {
			req.Header.Set("X-Admin-Key", d.adminKey)
		}
	}

	mux := http.NewServeMux()

	// V1 routes (Studio-native) — registered before proxy catch-all.
	if d.registryAPI != nil {
		mux.HandleFunc("POST /api/v1/configs/validate", d.registryAPI.handleValidateConfig)
		mux.HandleFunc("POST /api/v1/configs", d.registryAPI.handleCreateConfig)
		mux.HandleFunc("GET /api/v1/configs", d.registryAPI.handleListConfigs)
		mux.HandleFunc("GET /api/v1/configs/{id}", d.registryAPI.handleGetConfig)
		mux.HandleFunc("PUT /api/v1/configs/{id}", d.registryAPI.handleUpdateConfigContent)
		mux.HandleFunc("PATCH /api/v1/configs/{id}", d.registryAPI.handleUpdateConfigMetadata)
		mux.HandleFunc("DELETE /api/v1/configs/{id}", d.registryAPI.handleDeleteConfig)
		mux.HandleFunc("GET /api/v1/configs/{id}/versions", d.registryAPI.handleListVersions)
		mux.HandleFunc("POST /api/v1/configs/{id}/versions/{version}/activate", d.registryAPI.handleActivateVersion)
		mux.HandleFunc("GET /api/v1/registry/bundle", d.registryAPI.handleGetBundle)
	}

	// Preferences routes always mint, so curl and other cookie-less clients work.
	if d.prefsAPI != nil && d.sessionStore != nil {
		prefsMW := session.Middleware(d.sessionStore, session.AlwaysMint)
		mux.Handle("GET /api/v1/preferences", prefsMW(http.HandlerFunc(d.prefsAPI.handleGetPreferences)))
		mux.Handle("PUT /api/v1/preferences", prefsMW(http.HandlerFunc(d.prefsAPI.handlePutPreferences)))
	}

	// Proxy routes (gateway admin pass-through). Not session-wrapped — these
	// are forwarded to another process that has no use for a Studio session.
	mux.Handle("/api/", proxy)
	mux.Handle("/metrics", proxy)

	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		log.Fatal(err)
	}
	var spaHandler http.Handler = spaFileServer(http.FS(sub))
	// Document requests mint; static assets do not, which avoids creating
	// several sessions for one cold page load.
	if d.sessionStore != nil {
		spaHandler = session.Middleware(d.sessionStore, session.MintOnDocument)(spaHandler)
	}
	mux.Handle("/", spaHandler)

	return mux
}
```

- [ ] **Step 5: Update the `main()` call site**

Replace `cmd/restitch-studio/main.go:62`:

```go
	sessionStore := session.NewStore(db)
	prefsAPI := NewPreferencesAPI(sessionStore)

	mux := buildMux(muxDeps{
		gatewayAdminURL: *gatewayAdminURL,
		adminKey:        *adminKey,
		registryAPI:     registryAPI,
		prefsAPI:        prefsAPI,
		sessionStore:    sessionStore,
	})
```

- [ ] **Step 6: Update the three existing test call sites**

`cmd/restitch-studio/main_test.go:22` becomes:

```go
	mux := buildMux(muxDeps{gatewayAdminURL: admin.URL, adminKey: adminKey})
```

`cmd/restitch-studio/main_test.go:37` becomes:

```go
	mux := buildMux(muxDeps{gatewayAdminURL: "http://localhost:9999"})
```

`cmd/restitch-studio/api_test.go:70` becomes:

```go
	return buildMux(muxDeps{gatewayAdminURL: "http://localhost:9999", registryAPI: api})
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go build ./... && go vet ./... && go test -race -count=1 ./cmd/restitch-studio/ ./internal/session/ -v`
Expected: PASS, including the pre-existing `TestProxyRewrite` and `TestSPAFallback`.

- [ ] **Step 8: Commit**

```bash
git add cmd/restitch-studio/preferences.go cmd/restitch-studio/preferences_test.go \
        cmd/restitch-studio/main.go cmd/restitch-studio/main_test.go cmd/restitch-studio/api_test.go
git commit -m "feat(M25): preferences CRUD API scoped to browser session (T25.2)"
```

- [ ] **Step 9: Append ledger row and amend the commit**

---

### Task 7: Frontend API client and `usePreferences` hook (T25.4)

**Files:**
- Modify: `studio/src/lib/api.ts` (add types + two calls; add `credentials` to `get`)
- Create: `studio/src/hooks/usePreferences.tsx`
- Test: `studio/src/hooks/usePreferences.test.tsx`

**Interfaces:**
- Consumes: `GET`/`PUT /api/v1/preferences` from Task 6
- Produces:
  - `interface Preferences { pinnedCompositions: string[]; sidebarCollapsed: boolean; defaultTimeRange: TimeRange }`
  - `api.getPreferences(): Promise<PreferencesResponse>`, `api.putPreferences(p): Promise<PreferencesResponse>`
  - `<PreferencesProvider>` and `usePreferences(): { prefs, setSidebarCollapsed, togglePin, setDefaultTimeRange }`
  - `const PREFS_STORAGE_KEY = "restitch.prefs"`

- [ ] **Step 1: Write the failing tests**

Create `studio/src/hooks/usePreferences.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"
import { render, screen, waitFor, act } from "@testing-library/react"

const getPreferences = vi.fn()
const putPreferences = vi.fn()

vi.mock("@/lib/api", () => ({
  api: {
    getPreferences: (...a: unknown[]) => getPreferences(...a),
    putPreferences: (...a: unknown[]) => putPreferences(...a),
  },
}))

import { PreferencesProvider, usePreferences, PREFS_STORAGE_KEY } from "./usePreferences"

function Probe() {
  const { prefs, togglePin } = usePreferences()
  return (
    <div>
      <span data-testid="range">{prefs.defaultTimeRange}</span>
      <span data-testid="collapsed">{String(prefs.sidebarCollapsed)}</span>
      <span data-testid="pins">{prefs.pinnedCompositions.join(",")}</span>
      <button onClick={() => togglePin("new-comp")}>pin</button>
    </div>
  )
}

beforeEach(() => {
  localStorage.clear()
  getPreferences.mockReset()
  putPreferences.mockReset()
  putPreferences.mockResolvedValue({
    pinned_compositions: [], sidebar_collapsed: false,
    default_time_range: "1h", initialized: true,
  })
  vi.useFakeTimers({ shouldAdvanceTime: true })
})

afterEach(() => {
  vi.useRealTimers()
})

describe("usePreferences", () => {
  it("paints from localStorage before the server responds", async () => {
    localStorage.setItem(PREFS_STORAGE_KEY, JSON.stringify({
      pinnedCompositions: ["cached"], sidebarCollapsed: true, defaultTimeRange: "24h",
    }))
    getPreferences.mockReturnValue(new Promise(() => {})) // never resolves

    render(<PreferencesProvider><Probe /></PreferencesProvider>)

    expect(screen.getByTestId("range").textContent).toBe("24h")
    expect(screen.getByTestId("collapsed").textContent).toBe("true")
    expect(screen.getByTestId("pins").textContent).toBe("cached")
  })

  it("server wins when the record is initialized", async () => {
    localStorage.setItem(PREFS_STORAGE_KEY, JSON.stringify({
      pinnedCompositions: ["stale"], sidebarCollapsed: false, defaultTimeRange: "1h",
    }))
    getPreferences.mockResolvedValue({
      pinned_compositions: ["from-server"], sidebar_collapsed: true,
      default_time_range: "6h", initialized: true,
    })

    render(<PreferencesProvider><Probe /></PreferencesProvider>)

    await waitFor(() => {
      expect(screen.getByTestId("pins").textContent).toBe("from-server")
    })
    expect(screen.getByTestId("range").textContent).toBe("6h")
    expect(putPreferences).not.toHaveBeenCalled()
  })

  it("adopts localStorage and seeds the server when uninitialized", async () => {
    localStorage.setItem(PREFS_STORAGE_KEY, JSON.stringify({
      pinnedCompositions: ["keep-me"], sidebarCollapsed: true, defaultTimeRange: "24h",
    }))
    getPreferences.mockResolvedValue({
      pinned_compositions: [], sidebar_collapsed: false,
      default_time_range: "1h", initialized: false,
    })

    render(<PreferencesProvider><Probe /></PreferencesProvider>)

    await waitFor(() => {
      expect(putPreferences).toHaveBeenCalledWith({
        pinned_compositions: ["keep-me"],
        sidebar_collapsed: true,
        default_time_range: "24h",
      })
    })
    // The cookie-cleared case must not wipe the user's real preferences.
    expect(screen.getByTestId("pins").textContent).toBe("keep-me")
  })

  it("keeps local state when the fetch fails", async () => {
    localStorage.setItem(PREFS_STORAGE_KEY, JSON.stringify({
      pinnedCompositions: ["offline"], sidebarCollapsed: false, defaultTimeRange: "6h",
    }))
    getPreferences.mockRejectedValue(new Error("network down"))

    render(<PreferencesProvider><Probe /></PreferencesProvider>)

    await waitFor(() => expect(getPreferences).toHaveBeenCalled())
    expect(screen.getByTestId("pins").textContent).toBe("offline")
    expect(screen.getByTestId("range").textContent).toBe("6h")
  })

  it("debounces rapid changes into a single PUT", async () => {
    getPreferences.mockResolvedValue({
      pinned_compositions: [], sidebar_collapsed: false,
      default_time_range: "1h", initialized: true,
    })

    render(<PreferencesProvider><Probe /></PreferencesProvider>)
    await waitFor(() => expect(getPreferences).toHaveBeenCalled())

    const button = screen.getByText("pin")
    act(() => { button.click(); button.click(); button.click() })
    act(() => { vi.advanceTimersByTime(600) })

    await waitFor(() => expect(putPreferences).toHaveBeenCalledTimes(1))
  })

  it("mirrors state back to localStorage", async () => {
    getPreferences.mockResolvedValue({
      pinned_compositions: ["srv"], sidebar_collapsed: true,
      default_time_range: "6h", initialized: true,
    })

    render(<PreferencesProvider><Probe /></PreferencesProvider>)

    await waitFor(() => {
      const raw = localStorage.getItem(PREFS_STORAGE_KEY)
      expect(raw).not.toBeNull()
      expect(JSON.parse(raw as string).pinnedCompositions).toEqual(["srv"])
    })
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd studio && npx vitest run src/hooks/usePreferences.test.tsx`
Expected: FAIL — cannot resolve `./usePreferences`.

- [ ] **Step 3: Extend the API client**

In `studio/src/lib/api.ts`, add `credentials: "same-origin"` to the existing `get` helper so the `HttpOnly` cookie is attached — without it `fetch` omits cookies and every preference lands on a fresh session:

```ts
async function get<T>(path: string): Promise<T> {
  const res = await fetch(path, { credentials: "same-origin" })
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json()
}
```

Then append these types and a `put` helper near the other helpers:

```ts
export interface PreferencesPayload {
  pinned_compositions: string[]
  sidebar_collapsed: boolean
  default_time_range: string
}

export interface PreferencesResponse extends PreferencesPayload {
  initialized: boolean
}

async function put<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "PUT",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json()
}
```

And add these two entries inside the exported `api` object:

```ts
  getPreferences: () => get<PreferencesResponse>("/api/v1/preferences"),
  putPreferences: (p: PreferencesPayload) =>
    put<PreferencesResponse>("/api/v1/preferences", p),
```

- [ ] **Step 4: Write the hook**

Create `studio/src/hooks/usePreferences.tsx`:

```tsx
import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react"
import { api, type PreferencesPayload, type PreferencesResponse } from "@/lib/api"
import type { TimeRange } from "@/components/charts/TimeRangeSelector"

export const PREFS_STORAGE_KEY = "restitch.prefs"

const PUT_DEBOUNCE_MS = 500

export interface Preferences {
  pinnedCompositions: string[]
  sidebarCollapsed: boolean
  defaultTimeRange: TimeRange
}

const DEFAULT_PREFS: Preferences = {
  pinnedCompositions: [],
  sidebarCollapsed: false,
  defaultTimeRange: "1h",
}

function toPayload(p: Preferences): PreferencesPayload {
  return {
    pinned_compositions: p.pinnedCompositions,
    sidebar_collapsed: p.sidebarCollapsed,
    default_time_range: p.defaultTimeRange,
  }
}

function fromResponse(r: PreferencesResponse): Preferences {
  return {
    pinnedCompositions: r.pinned_compositions ?? [],
    sidebarCollapsed: r.sidebar_collapsed ?? false,
    defaultTimeRange: (r.default_time_range as TimeRange) ?? "1h",
  }
}

// readLocal paints the first frame. A malformed or absent entry falls back to
// defaults rather than throwing during render.
function readLocal(): Preferences {
  try {
    const raw = localStorage.getItem(PREFS_STORAGE_KEY)
    if (!raw) return DEFAULT_PREFS
    return { ...DEFAULT_PREFS, ...JSON.parse(raw) }
  } catch {
    return DEFAULT_PREFS
  }
}

function writeLocal(p: Preferences) {
  try {
    localStorage.setItem(PREFS_STORAGE_KEY, JSON.stringify(p))
  } catch {
    // Private-browsing quota errors are not worth failing a render over.
  }
}

interface PreferencesContextValue {
  prefs: Preferences
  setSidebarCollapsed: (v: boolean) => void
  togglePin: (name: string) => void
  setDefaultTimeRange: (v: TimeRange) => void
}

const PreferencesContext = createContext<PreferencesContextValue | null>(null)

export function PreferencesProvider({ children }: { children: ReactNode }) {
  const [prefs, setPrefs] = useState<Preferences>(readLocal)
  const hydrated = useRef(false)
  // Serialised payload the server is already known to hold. The mirror effect
  // compares against it so reconciling does not immediately PUT back the value
  // it just received. A bare `hydrated` flag cannot do this: .finally() sets it
  // before React re-renders from the reconcile's setPrefs, so the mirror effect
  // would see hydrated=true and push a redundant write.
  const lastSynced = useRef<string | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    let cancelled = false
    api
      .getPreferences()
      .then((res) => {
        if (cancelled) return
        if (res.initialized) {
          // Server has a record: it wins.
          const merged = fromResponse(res)
          lastSynced.current = JSON.stringify(toPayload(merged))
          setPrefs(merged)
          writeLocal(merged)
        } else {
          // Never written (e.g. the user cleared cookies but not
          // localStorage). Adopt local state and seed the new session, rather
          // than letting empty server defaults wipe real preferences.
          const local = readLocal()
          const payload = toPayload(local)
          lastSynced.current = JSON.stringify(payload)
          setPrefs(local)
          writeLocal(local)
          void api.putPreferences(payload).catch(() => {})
        }
      })
      .catch(() => {
        // Offline or server error: keep whatever localStorage gave us.
      })
      .finally(() => {
        if (!cancelled) hydrated.current = true
      })
    return () => {
      cancelled = true
    }
  }, [])

  // Mirror every change locally and push it to the server, debounced.
  useEffect(() => {
    writeLocal(prefs)
    if (!hydrated.current) return

    const payload = toPayload(prefs)
    const serialised = JSON.stringify(payload)
    if (serialised === lastSynced.current) return

    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => {
      lastSynced.current = serialised
      void api.putPreferences(payload).catch(() => {
        // Failed push: drop the marker so the next change retries this state.
        lastSynced.current = null
      })
    }, PUT_DEBOUNCE_MS)
    return () => {
      if (timer.current) clearTimeout(timer.current)
    }
  }, [prefs])

  const value: PreferencesContextValue = {
    prefs,
    setSidebarCollapsed: (v) => setPrefs((p) => ({ ...p, sidebarCollapsed: v })),
    setDefaultTimeRange: (v) => setPrefs((p) => ({ ...p, defaultTimeRange: v })),
    togglePin: (name) =>
      setPrefs((p) => ({
        ...p,
        pinnedCompositions: p.pinnedCompositions.includes(name)
          ? p.pinnedCompositions.filter((n) => n !== name)
          : [...p.pinnedCompositions, name],
      })),
  }

  return <PreferencesContext.Provider value={value}>{children}</PreferencesContext.Provider>
}

export function usePreferences(): PreferencesContextValue {
  const ctx = useContext(PreferencesContext)
  if (!ctx) throw new Error("usePreferences must be used inside a PreferencesProvider")
  return ctx
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd studio && npx vitest run src/hooks/usePreferences.test.tsx`
Expected: PASS — all six tests.

- [ ] **Step 6: Commit**

```bash
git add studio/src/lib/api.ts studio/src/hooks/usePreferences.tsx studio/src/hooks/usePreferences.test.tsx
git commit -m "feat(M25): preferences hook with localStorage mirror and reconcile (T25.4)"
```

- [ ] **Step 7: Append ledger row and amend the commit**

---

### Task 8: Sidebar collapse (T25.4)

**Files:**
- Modify: `studio/src/App.tsx`

**Interfaces:**
- Consumes: `PreferencesProvider`, `usePreferences` from Task 7
- Produces: a persisted collapsible sidebar. `App` is split so the provider can wrap the shell.

- [ ] **Step 1: Restructure `App.tsx`**

The provider must sit above the component that reads it, so `App` becomes a thin wrapper around a new `Shell`. Replace the imports and the `export default function App()` block in `studio/src/App.tsx`:

```tsx
import { BrowserRouter, Routes, Route, NavLink } from "react-router-dom"
import {
  LayoutDashboard, Activity, GitBranch, Hammer, Settings, Zap, RefreshCw,
  PanelLeftClose, PanelLeftOpen,
} from "lucide-react"
import { Toaster, toast } from "sonner"
import { useState } from "react"
import { usePoll } from "./hooks/usePoll"
import { PreferencesProvider, usePreferences } from "./hooks/usePreferences"
import { api } from "./lib/api"
import Dashboard from "./pages/Dashboard"
import Compositions from "./pages/Compositions"
import CompositionDetail from "./pages/CompositionDetail"
import Requests from "./pages/Requests"
import Builder from "./pages/Builder"
import Config from "./pages/Config"

const navItems = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard },
  { to: "/compositions", label: "Compositions", icon: GitBranch },
  { to: "/requests", label: "Requests", icon: Activity },
  { to: "/builder", label: "Builder", icon: Hammer },
  { to: "/config", label: "Config", icon: Settings },
]

export default function App() {
  return (
    <PreferencesProvider>
      <Shell />
    </PreferencesProvider>
  )
}

function Shell() {
  const { data: info } = usePoll(() => api.info(), 30000)
  const [reloading, setReloading] = useState(false)
  const { prefs, setSidebarCollapsed } = usePreferences()
  const sidebarCollapsed = prefs.sidebarCollapsed

  const handleReload = async () => {
    if (!confirm("Reload gateway configuration?")) return
    setReloading(true)
    try {
      const res = await api.reload()
      if (res.ok) {
        toast.success("Config reloaded", { description: `Hash: ${res.config_hash?.slice(0, 8)}` })
      } else {
        toast.error("Reload failed", { description: res.errors?.join(", ") })
      }
    } catch (e) {
      toast.error("Reload failed", { description: String(e) })
    } finally {
      setReloading(false)
    }
  }

  return (
    <BrowserRouter>
      <Toaster
        theme="dark"
        toastOptions={{
          style: { background: "#1c1c1f", border: "1px solid rgba(178,182,189,0.12)", color: "#fff" },
        }}
      />
      <div className="flex h-screen">
        <nav
          className={`${sidebarCollapsed ? "w-14" : "w-52"} flex flex-col border-r border-hairline bg-canvas transition-[width] duration-150`}
        >
          <div className="px-4 py-5 border-b border-hairline flex items-center gap-2.5">
            <Zap size={18} className="text-rs-accent shrink-0" />
            {!sidebarCollapsed && (
              <>
                <span className="text-[15px] font-semibold tracking-[-0.2px] text-ink">
                  Restitch
                </span>
                <span className="text-[11px] font-semibold tracking-[0.6px] uppercase text-ink-subtle ml-auto">
                  Studio
                </span>
              </>
            )}
          </div>

          <div className="flex-1 px-2 pt-3">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === "/"}
                title={sidebarCollapsed ? item.label : undefined}
                className={({ isActive }) =>
                  `flex items-center gap-2.5 px-3 py-[7px] rounded-lg text-[13px] font-medium mb-0.5 transition-colors ${
                    isActive
                      ? "bg-surface-2 text-ink"
                      : "text-ink-muted hover:text-ink hover:bg-surface-1"
                  }`
                }
              >
                <item.icon size={16} strokeWidth={1.8} className="shrink-0" />
                {!sidebarCollapsed && item.label}
              </NavLink>
            ))}
          </div>

          <div className="px-2 pb-2">
            <button
              onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
              aria-label={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
              className="flex items-center gap-2.5 w-full px-3 py-[7px] rounded-lg text-[13px] font-medium text-ink-muted hover:text-ink hover:bg-surface-1 transition-colors"
            >
              {sidebarCollapsed
                ? <PanelLeftOpen size={16} strokeWidth={1.8} className="shrink-0" />
                : <PanelLeftClose size={16} strokeWidth={1.8} className="shrink-0" />}
              {!sidebarCollapsed && "Collapse"}
            </button>
          </div>

          {info && !sidebarCollapsed && (
            <div className="px-4 py-3 border-t border-hairline-soft">
              <div className="flex items-center justify-between mb-1">
                <div className="text-[11px] font-semibold tracking-[0.6px] uppercase text-ink-subtle">
                  Gateway
                </div>
                <button
                  onClick={handleReload}
                  disabled={reloading}
                  title="Reload config"
                  className="p-1 rounded text-ink-subtle hover:text-rs-accent hover:bg-surface-2 transition-colors disabled:opacity-40"
                >
                  <RefreshCw size={12} className={reloading ? "animate-spin" : ""} />
                </button>
              </div>
              <div className="text-[12px] text-ink-muted font-mono">
                {info.config_hash.slice(0, 8)}
              </div>
            </div>
          )}
        </nav>

        <main className="flex-1 overflow-auto bg-canvas">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/compositions" element={<Compositions />} />
            <Route path="/compositions/:name" element={<CompositionDetail />} />
            <Route path="/requests" element={<Requests />} />
            <Route path="/builder" element={<Builder />} />
            <Route path="/config" element={<Config />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}
```

- [ ] **Step 2: Verify the build and existing tests**

Run: `cd studio && npx tsc -b && npm run test`
Expected: type-check clean; existing `Dashboard.test.tsx`, `Requests.test.tsx`, `CompositionDetail.test.tsx` still pass. Those tests render pages directly rather than `App`, so they need no provider.

- [ ] **Step 3: Commit**

```bash
git add studio/src/App.tsx
git commit -m "feat(M25): persisted collapsible sidebar (T25.4)"
```

- [ ] **Step 4: Append ledger row and amend the commit**

---

### Task 9: Composition pinning (T25.4)

**Files:**
- Modify: `studio/src/pages/Compositions.tsx`
- Test: `studio/src/pages/Compositions.test.tsx` (create)

**Interfaces:**
- Consumes: `usePreferences` from Task 7
- Produces: a pin column; pinned rows sort above unpinned ones

- [ ] **Step 1: Write the failing test**

Create `studio/src/pages/Compositions.test.tsx`:

```tsx
import { describe, it, expect, vi } from "vitest"
import { render, screen } from "@testing-library/react"
import { MemoryRouter } from "react-router-dom"

const comps = [
  { name: "alpha", path: "/a", method: "GET", public: true, steps: [], waves: [] },
  { name: "beta", path: "/b", method: "GET", public: false, steps: [], waves: [] },
  { name: "gamma", path: "/g", method: "GET", public: false, steps: [], waves: [] },
]

vi.mock("@/lib/api", () => ({ api: { compositions: vi.fn() } }))
vi.mock("@/hooks/usePoll", () => ({
  usePoll: () => ({ data: comps, error: null, refresh: vi.fn() }),
}))
vi.mock("@/hooks/usePreferences", () => ({
  usePreferences: () => ({
    prefs: { pinnedCompositions: ["gamma"], sidebarCollapsed: false, defaultTimeRange: "1h" },
    togglePin: vi.fn(),
    setSidebarCollapsed: vi.fn(),
    setDefaultTimeRange: vi.fn(),
  }),
}))

import Compositions from "./Compositions"

describe("Compositions", () => {
  it("sorts pinned compositions to the top", () => {
    render(<MemoryRouter><Compositions /></MemoryRouter>)
    const rows = screen.getAllByTestId("composition-row")
    expect(rows[0]).toHaveTextContent("gamma")
  })

  it("renders a pin control per row", () => {
    render(<MemoryRouter><Compositions /></MemoryRouter>)
    expect(screen.getAllByRole("button", { name: /pin/i })).toHaveLength(3)
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd studio && npx vitest run src/pages/Compositions.test.tsx`
Expected: FAIL — no `composition-row` test IDs and no pin buttons.

- [ ] **Step 3: Add the pin column**

Rewrite `studio/src/pages/Compositions.tsx`:

```tsx
import { useNavigate } from "react-router-dom"
import { Pin, PinOff } from "lucide-react"
import { usePoll } from "../hooks/usePoll"
import { usePreferences } from "../hooks/usePreferences"
import { api } from "../lib/api"

export default function Compositions() {
  const { data: compositions } = usePoll(() => api.compositions(), 10000)
  const { prefs, togglePin } = usePreferences()
  const navigate = useNavigate()

  if (!compositions) {
    return (
      <div className="p-8">
        <div className="h-6 w-48 bg-surface-1 rounded-md animate-pulse" />
      </div>
    )
  }

  const isPinned = (name: string) => prefs.pinnedCompositions.includes(name)

  // Pinned first, then original order preserved within each group.
  const ordered = [...compositions].sort((a, b) => {
    const pa = isPinned(a.name) ? 0 : 1
    const pb = isPinned(b.name) ? 0 : 1
    return pa - pb
  })

  return (
    <div className="p-8 max-w-[1280px]">
      <div className="mb-8">
        <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
          Routes
        </div>
        <h1 className="text-[28px] font-semibold leading-[1.21] tracking-[-0.6px] text-ink">
          Compositions
        </h1>
      </div>

      <div className="bg-surface-1 rounded-xl border border-hairline overflow-hidden">
        <table className="w-full text-[13px]">
          <thead>
            <tr className="border-b border-hairline-soft">
              <th className="w-10 px-2 py-2.5" />
              <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Name</th>
              <th className="px-4 py-2.5 text-left font-medium text-ink-muted">Route</th>
              <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Steps</th>
              <th className="px-4 py-2.5 text-right font-medium text-ink-muted">Waves</th>
              <th className="px-4 py-2.5 text-center font-medium text-ink-muted">Public</th>
            </tr>
          </thead>
          <tbody>
            {ordered.map((c) => (
              <tr
                key={c.name}
                data-testid="composition-row"
                onClick={() => navigate(`/compositions/${c.name}`)}
                className="border-t border-hairline-soft cursor-pointer hover:bg-surface-2 transition-colors"
              >
                <td className="px-2 py-2.5 text-center">
                  <button
                    aria-label={isPinned(c.name) ? `Unpin ${c.name}` : `Pin ${c.name}`}
                    onClick={(e) => {
                      e.stopPropagation()
                      togglePin(c.name)
                    }}
                    className={`p-1 rounded transition-colors ${
                      isPinned(c.name)
                        ? "text-rs-accent"
                        : "text-ink-subtle hover:text-ink hover:bg-surface-2"
                    }`}
                  >
                    {isPinned(c.name)
                      ? <Pin size={14} strokeWidth={1.8} />
                      : <PinOff size={14} strokeWidth={1.8} />}
                  </button>
                </td>
                <td className="px-4 py-2.5 font-medium text-ink">{c.name}</td>
                <td className="px-4 py-2.5 font-mono text-[12px] text-ink-muted">
                  <span className="text-rs-accent font-semibold">{c.method}</span> {c.path}
                </td>
                <td className="px-4 py-2.5 text-right text-ink-muted tabular-nums">{c.steps.length}</td>
                <td className="px-4 py-2.5 text-right text-ink-muted tabular-nums">{c.waves.length}</td>
                <td className="px-4 py-2.5 text-center">
                  {c.public && (
                    <span className="inline-block px-2 py-0.5 rounded text-[11px] font-semibold bg-success/15 text-success">
                      public
                    </span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
```

Note: the `stopPropagation` call is required — the row has a navigate handler, so without it every pin click also navigates away.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd studio && npx vitest run src/pages/Compositions.test.tsx`
Expected: PASS — both tests.

- [ ] **Step 5: Commit**

```bash
git add studio/src/pages/Compositions.tsx studio/src/pages/Compositions.test.tsx
git commit -m "feat(M25): pin compositions with pinned-first ordering (T25.4)"
```

- [ ] **Step 6: Append ledger row and amend the commit**

---

### Task 10: Dashboard default time range (T25.4)

**Files:**
- Modify: `studio/src/pages/Dashboard.tsx:12` and the `TimeRangeSelector` usage at `studio/src/pages/Dashboard.tsx:53`

**Interfaces:**
- Consumes: `usePreferences` from Task 7

- [ ] **Step 1: Seed and persist the range**

In `studio/src/pages/Dashboard.tsx`, add the import:

```tsx
import { usePreferences } from "../hooks/usePreferences"
```

Replace the local range state (`const [range, setRange] = useState<TimeRange>("1h")` at line 12) with preference-backed state:

```tsx
  const { prefs, setDefaultTimeRange } = usePreferences()
  const range = prefs.defaultTimeRange
  const setRange = setDefaultTimeRange
```

The existing `useState` import may now be unused on this line only — leave the import if other state remains in the file, remove it if `tsc` reports it unused.

The existing `<TimeRangeSelector value={range} onChange={setRange} />` at line 53 needs no change.

- [ ] **Step 2: Update the existing Dashboard test**

`studio/src/pages/Dashboard.test.tsx` mocks `@/hooks/usePoll` but not preferences. Add this mock alongside the others:

```tsx
vi.mock("@/hooks/usePreferences", () => ({
  usePreferences: () => ({
    prefs: { pinnedCompositions: [], sidebarCollapsed: false, defaultTimeRange: "1h" },
    togglePin: vi.fn(),
    setSidebarCollapsed: vi.fn(),
    setDefaultTimeRange: vi.fn(),
  }),
}))
```

- [ ] **Step 3: Run the full frontend suite**

Run: `cd studio && npx tsc -b && npm run test`
Expected: type-check clean, all tests PASS.

- [ ] **Step 4: Commit**

```bash
git add studio/src/pages/Dashboard.tsx studio/src/pages/Dashboard.test.tsx
git commit -m "feat(M25): persist dashboard default time range (T25.4)"
```

- [ ] **Step 5: Append ledger row and amend the commit**

---

### Task 11: Rebuild embedded assets, run the gate, close the milestone

**Files:**
- Modify: `cmd/restitch-studio/dist/` (rebuilt frontend bundle)
- Modify: `docs/plan-progress/LEDGER.md`
- Modify: `PLAN.md` (M25 status row)
- Create: `docs/plan-progress/evidence/<date>-M25-<sha>.log` (written by the harness)

- [ ] **Step 1: Rebuild the frontend into the embedded dist**

The Go binary serves `//go:embed all:dist` from `cmd/restitch-studio/dist`. The gate's live checks run against the built binary, so the bundle must be current.

`studio/vite.config.ts` sets `build.outDir` to `../cmd/restitch-studio/dist` with `emptyOutDir: true`, so the Vite build writes the embedded bundle directly — no copy step.

Run:
```bash
make studio && make build-all
```
Expected: `cmd/restitch-studio/dist/index.html` and `cmd/restitch-studio/dist/assets/` are regenerated, and `bin/restitch-studio` builds clean.

- [ ] **Step 2: Run the full CI check**

Run: `make ci`
Expected: vet + lint + race tests all clean.

- [ ] **Step 3: Run the gate**

Run: `scripts/verify.sh M25`
Expected: `RESULT M25: PASS`, plus the MANUAL line from Task 1's gate script.

If any check fails, fix the **code** — never the gate. Per CLAUDE.md, changing a gate script requires user approval and a dedicated `gate:` commit.

- [ ] **Step 4: Commit the evidence file**

```bash
git add docs/plan-progress/evidence/
git commit -m "test(M25): gate evidence for browser session & preferences"
```

- [ ] **Step 5: Add the `M25.gate` ledger row**

Append one row to `docs/plan-progress/LEDGER.md` using the schema in its header, citing the evidence file path and the commit the gate ran against.

- [ ] **Step 6: STOP — surface the MANUAL line to the user**

The gate prints a MANUAL line (browser cookie + persistence check). Per CLAUDE.md rule 6, **you may not check this off**. List it for the user and stop. Only after they confirm may you append a `MANUAL-VERIFIED` ledger row citing their confirmation in Notes.

- [ ] **Step 7: Run the ledger check**

Run: `scripts/check-ledger.sh`
Expected: `COVERAGE: N/N green`.

Note: the command currently exits 1 because of 10 pre-existing `UNKNOWN-IDS` (`M20.unit`–`M24.unit`, `final.*`) that predate M25 and are out of scope. `M25.unit` will add an eleventh. Confirm the COVERAGE line is fully green and that no *missing* task rows are reported; do not attempt to fix the UNKNOWN-IDS as part of this milestone.

- [ ] **Step 8: Mark M25 DONE in PLAN.md**

Only after the user has confirmed the MANUAL line. Add the status-table row after the M24 row at `PLAN.md:38`:

```
| M25 — Browser Session & User Preferences | T25.1–T25.4 | DONE |
```

```bash
git add PLAN.md docs/plan-progress/LEDGER.md
git commit -m "docs: mark M25 DONE with committed gate evidence"
```

---

## Deferred / out of scope

These are recorded so they are not silently lost:

- The 10 pre-existing `UNKNOWN-IDS` ledger rows
- M24's CI load-test placeholder `P95_MS: "REPLACE_IN_STEP_6"` (`.github/workflows/ci.yml:135`) — needs a measured run on a real GitHub runner via `ceil(observed_p95 * 2 / 50) * 50`
- Server-side session pruning
- Any authentication or multi-user concept
