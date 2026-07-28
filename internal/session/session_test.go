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
