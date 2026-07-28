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
