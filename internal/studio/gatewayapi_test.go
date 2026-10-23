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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type issuedToken struct {
	ID     string `json:"id"`
	Token  string `json:"token"`
	Prefix string `json:"prefix"`
}

func issueToken(t *testing.T, s *Server, c []*http.Cookie, env string) issuedToken {
	t.Helper()
	rec := do(t, s, http.MethodPost, "/api/v1/environments/"+env+"/gateway-tokens", testPublicURL, c, map[string]string{"name": "gw"})
	if rec.Code != http.StatusOK {
		t.Fatalf("issue token: %d %s", rec.Code, rec.Body)
	}
	var tok issuedToken
	_ = json.Unmarshal(rec.Body.Bytes(), &tok)
	if !strings.HasPrefix(tok.Token, gatewayTokenPrefix) {
		t.Fatalf("token %q lacks the %s prefix", tok.Token, gatewayTokenPrefix)
	}
	return tok
}

func pull(s *Server, token, path, etag string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, testPublicURL+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestTokenCannotPullOtherEnvironment(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		seedValidDraft(t, s, c)
		publish(t, s, c, "first")
		createEnvironment(t, s, c, "prod")
		prodToken := issueToken(t, s, c, "prod")
		stagingToken := issueToken(t, s, c, "staging")

		if rec := pull(s, prodToken.Token, "/gateway/v1/artifact", ""); rec.Code != http.StatusNotFound {
			t.Fatalf("prod has no version: got %d %s", rec.Code, rec.Body)
		}
		rec := pull(s, stagingToken.Token, "/gateway/v1/artifact", "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "USERS_HOST") {
			t.Fatalf("staging pull: %d %s", rec.Code, rec.Body)
		}
		if rec.Header().Get(ArtifactVersionHeader) != "1" {
			t.Fatalf("version header = %q", rec.Header().Get(ArtifactVersionHeader))
		}
	})
}

func TestPullUsesETag(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		seedValidDraft(t, s, c)
		publish(t, s, c, "first")
		tok := issueToken(t, s, c, "staging")

		first := pull(s, tok.Token, "/gateway/v1/artifact", "")
		etag := first.Header().Get("ETag")
		if etag == "" {
			t.Fatal("no ETag")
		}
		if rec := pull(s, tok.Token, "/gateway/v1/artifact", etag); rec.Code != http.StatusNotModified {
			t.Fatalf("same ETag: got %d, want 304", rec.Code)
		}
		if rec := pull(s, tok.Token, "/gateway/v1/artifact/versions/1", ""); rec.Code != http.StatusOK ||
			rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Fatalf("immutable version url: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
		}
		saveResource(t, s, c, stagingRes+"upstream/users", 1, map[string]any{"url": "https://${USERS_HOST}/v2"})
		publish(t, s, c, "second")
		if rec := pull(s, tok.Token, "/gateway/v1/artifact", etag); rec.Code != http.StatusOK || rec.Header().Get(ArtifactVersionHeader) != "2" {
			t.Fatalf("after publish: %d version %q", rec.Code, rec.Header().Get(ArtifactVersionHeader))
		}
	})
}

func TestRevokedTokenGets401(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		seedValidDraft(t, s, c)
		publish(t, s, c, "first")
		tok := issueToken(t, s, c, "staging")

		rec := do(t, s, http.MethodGet, "/api/v1/environments/staging/gateway-tokens", "", c, nil)
		if strings.Contains(rec.Body.String(), tok.Token) {
			t.Fatal("the token list must never return the token")
		}
		if rec := do(t, s, http.MethodDelete, "/api/v1/environments/staging/gateway-tokens/"+tok.ID, testPublicURL, c, nil); rec.Code != http.StatusNoContent {
			t.Fatalf("revoke: %d %s", rec.Code, rec.Body)
		}
		if rec := pull(s, tok.Token, "/gateway/v1/artifact", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("revoked token: got %d, want 401", rec.Code)
		}
		if rec := pull(s, "", "/gateway/v1/artifact", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("no token: got %d, want 401", rec.Code)
		}
	})
}

func TestGatewayTokenIsNotAUserCredential(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		createEnvironment(t, s, c, "staging")
		tok := issueToken(t, s, c, "staging")
		if rec := pull(s, tok.Token, "/api/v1/environments", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("gateway token on a user route: got %d, want 401", rec.Code)
		}
		editor := signInAs(t, s, "editor@example.com", RoleEditor)
		rec := do(t, s, http.MethodPost, "/api/v1/environments/staging/gateway-tokens", testPublicURL, editor, map[string]string{"name": "x"})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("editor issuing a token: got %d, want 403", rec.Code)
		}
	})
}
