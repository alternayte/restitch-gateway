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
# Positive precondition first: a bare negated grep also passes when the file or
# the column is absent, which would let a missing migration read as compliant.
h_run "preferences column is nullable" -- bash -c '
  f=internal/registry/migrations/20260728000000_browser_sessions.sql
  test -f "$f" || exit 1
  grep -qE "^[[:space:]]*preferences[[:space:]]+TEXT" "$f" || exit 1
  ! grep -qE "^[[:space:]]*preferences[[:space:]]+TEXT[^,]*NOT NULL" "$f"
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
# The Set-Cookie line must exist before its absence-of-Secure means anything.
h_run "cookie is not Secure over plaintext" -- bash -c "
  grep -qi 'set-cookie:.*restitch_browser_id' '${HDRS}' || exit 1
  ! grep -i 'set-cookie:.*restitch_browser_id' '${HDRS}' | grep -qi 'Secure'
"

# Static assets must NOT mint — this is the multi-session race regression test.
ASSET_HDRS="${H_TMP}/asset_headers.txt"
curl -s -D "${ASSET_HDRS}" -H 'Accept: */*' \
  -o /dev/null "${BASE}/favicon.svg" || true
# The asset fetch must have SUCCEEDED before 'no Set-Cookie' proves anything —
# otherwise a failed request (studio down, asset renamed) reads as a pass and
# this regression test silently stops guarding the race it exists to catch.
h_run "static asset does not mint a session" -- bash -c "
  # Minting must be PROVEN to work on the document request first. Without that,
  # 'the asset set no cookie' cannot distinguish selective minting from there
  # being no middleware at all — it would pass against an empty implementation.
  grep -qi 'set-cookie:.*restitch_browser_id' '${HDRS}' || exit 1
  head -1 '${ASSET_HDRS}' | grep -qE 'HTTP/[0-9.]+ 200' || exit 1
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
