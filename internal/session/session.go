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
