// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateOpenAPI = flag.Bool("update", false, "rewrite api/openapi.yaml")

const (
	testPublicURL = "http://studio.test"
	ownerEmail    = "owner@example.com"
	ownerPassword = "correct horse battery staple"
)

// forEachStore runs fn against SQLite, and against Postgres when
// RESTITCH_TEST_DATABASE_URL names a database.
func forEachStore(t *testing.T, fn func(t *testing.T, s *Server)) {
	t.Run("sqlite", func(t *testing.T) {
		fn(t, newTestServer(t, "sqlite:"+filepath.Join(t.TempDir(), "studio.db")))
	})
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("RESTITCH_TEST_DATABASE_URL")
		if dsn == "" {
			t.Skip("RESTITCH_TEST_DATABASE_URL is not set")
		}
		fn(t, newTestServer(t, isolatedPostgres(t, dsn)))
	})
}

func newTestServer(t *testing.T, databaseURL string) *Server {
	t.Helper()
	s, err := New(context.Background(), Config{
		DatabaseURL:       databaseURL,
		PublicURL:         testPublicURL,
		BootstrapEmail:    ownerEmail,
		BootstrapPassword: ownerPassword,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func do(t *testing.T, s *Server, method, path, origin string, cookies []*http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, testPublicURL+path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func signIn(t *testing.T, s *Server) []*http.Cookie {
	t.Helper()
	rec := do(t, s, http.MethodPost, "/api/auth/sign-in/email", testPublicURL, nil,
		map[string]string{"email": ownerEmail, "password": ownerPassword})
	if rec.Code != http.StatusOK {
		t.Fatalf("sign-in: %d %s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("sign-in set no cookie")
	}
	return cookies
}

func TestAnonymousAPICallReturns401(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		rec := do(t, s, http.MethodGet, "/api/v1/me", "", nil, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("GET /api/v1/me without a session: got %d, want 401", rec.Code)
		}
	})
}

func TestBootstrapOwnerSignsIn(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		rec := do(t, s, http.MethodGet, "/api/v1/me", "", signIn(t, s), nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/v1/me: %d %s", rec.Code, rec.Body)
		}
		var me struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
			t.Fatal(err)
		}
		if me.Email != ownerEmail || me.Role != RoleOwner {
			t.Fatalf("me = %+v, want %s as %s", me, ownerEmail, RoleOwner)
		}
	})
}

func TestCrossSiteWriteReturns403(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		cookies := signIn(t, s)
		prefs := map[string]any{"theme": "dark"}

		rec := do(t, s, http.MethodPut, "/api/v1/me/preferences", "https://evil.example", cookies, prefs)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("cross-site PUT: got %d, want 403", rec.Code)
		}
		rec = do(t, s, http.MethodPut, "/api/v1/me/preferences", testPublicURL, cookies, prefs)
		if rec.Code != http.StatusOK {
			t.Fatalf("same-site PUT: %d %s", rec.Code, rec.Body)
		}
		rec = do(t, s, http.MethodGet, "/api/v1/me/preferences", "", cookies, nil)
		if !strings.Contains(rec.Body.String(), `"dark"`) {
			t.Fatalf("preferences were not stored: %s", rec.Body)
		}
	})
}

func TestPublicSignUpIsRefused(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		rec := do(t, s, http.MethodPost, "/api/auth/sign-up/email", testPublicURL, nil,
			map[string]string{"email": "stranger@example.com", "password": ownerPassword})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("public sign-up: got %d, want 404", rec.Code)
		}
	})
}

func TestBootstrapRunsOnce(t *testing.T) {
	path := "sqlite:" + filepath.Join(t.TempDir(), "studio.db")
	_ = newTestServer(t, path).Close()
	s := newTestServer(t, path) // second start with the same env changes no user
	if rec := do(t, s, http.MethodGet, "/api/v1/me", "", signIn(t, s), nil); rec.Code != http.StatusOK {
		t.Fatalf("owner after restart: %d", rec.Code)
	}
}

func TestOpenAPIUpToDate(t *testing.T) {
	s := newTestServer(t, "sqlite:"+filepath.Join(t.TempDir(), "studio.db"))
	got, err := s.OpenAPIYAML()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("..", "..", "api", "openapi.yaml")
	if *updateOpenAPI {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s (run go test ./internal/studio -run TestOpenAPIUpToDate -update): %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s is stale: run go test ./internal/studio -run TestOpenAPIUpToDate -update", path)
	}
	if bytes.Contains(got, []byte("signUpEmail")) {
		t.Fatal("the OpenAPI document must not offer public sign-up")
	}
}
