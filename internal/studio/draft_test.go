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
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/alternayte/auth-all/plugins/admin"
	"github.com/alternayte/auth-all/plugins/apikeys"
)

// signInAs creates a user with role and returns its session cookies.
func signInAs(t *testing.T, s *Server, email, role string) []*http.Cookie {
	t.Helper()
	if _, _, err := s.admin.CreateUser(context.Background(), admin.CreateUserInput{
		Email: email, Password: ownerPassword, Role: role,
	}); err != nil {
		t.Fatalf("create %s: %v", role, err)
	}
	rec := do(t, s, http.MethodPost, "/api/auth/sign-in/email", testPublicURL, nil,
		map[string]string{"email": email, "password": ownerPassword})
	if rec.Code != http.StatusOK {
		t.Fatalf("sign-in %s: %d %s", email, rec.Code, rec.Body)
	}
	return rec.Result().Cookies()
}

// createAPIKey issues an auth-all API key owned by the user with email.
func createAPIKey(t *testing.T, s *Server, email string) string {
	t.Helper()
	u, err := s.auth.GetUserByEmail(context.Background(), email)
	if err != nil || u == nil {
		t.Fatalf("user %s: %v", email, err)
	}
	_, key, err := s.apikeys.Create(context.Background(), u, apikeys.CreateInput{UserID: u.ID, Name: "ci"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return key
}

func createEnvironment(t *testing.T, s *Server, owner []*http.Cookie, name string) {
	t.Helper()
	rec := do(t, s, http.MethodPost, "/api/v1/environments", testPublicURL, owner, map[string]string{"name": name})
	if rec.Code != http.StatusOK {
		t.Fatalf("create environment: %d %s", rec.Code, rec.Body)
	}
}

type savedResource struct {
	Kind     string         `json:"kind"`
	Name     string         `json:"name"`
	Revision int            `json:"revision"`
	Body     map[string]any `json:"body"`
}

func saveResource(t *testing.T, s *Server, cookies []*http.Cookie, path string, base int, body map[string]any) (int, savedResource) {
	t.Helper()
	rec := do(t, s, http.MethodPut, path, testPublicURL, cookies, map[string]any{"base_revision": base, "body": body})
	var out savedResource
	if rec.Code != http.StatusOK {
		t.Logf("PUT %s: %d %s", path, rec.Code, rec.Body)
	}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, out
}

const upstreamPath = "/api/v1/environments/staging/resources/upstream/users"

func TestStaleSaveReturns409(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		owner := signIn(t, s)
		createEnvironment(t, s, owner, "staging")

		code, first := saveResource(t, s, owner, upstreamPath, 0, map[string]any{"url": "http://users:8080"})
		if code != http.StatusOK || first.Revision != 1 {
			t.Fatalf("create: %d, revision %d", code, first.Revision)
		}
		code, second := saveResource(t, s, owner, upstreamPath, 1, map[string]any{"url": "http://users:9090"})
		if code != http.StatusOK || second.Revision != 2 {
			t.Fatalf("update: %d, revision %d", code, second.Revision)
		}
		// A second tab still holds revision 1.
		if code, _ := saveResource(t, s, owner, upstreamPath, 1, map[string]any{"url": "http://stale"}); code != http.StatusConflict {
			t.Fatalf("stale save: got %d, want 409", code)
		}
		rec := do(t, s, http.MethodGet, upstreamPath, "", owner, nil)
		var got savedResource
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if got.Body["url"] != "http://users:9090" {
			t.Fatalf("the stale save changed the resource: %v", got.Body)
		}
	})
}

func TestDuplicateCreateReturns409(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		owner := signIn(t, s)
		createEnvironment(t, s, owner, "staging")
		if code, _ := saveResource(t, s, owner, upstreamPath, 0, map[string]any{"url": "http://a"}); code != http.StatusOK {
			t.Fatalf("create: %d", code)
		}
		if code, _ := saveResource(t, s, owner, upstreamPath, 0, map[string]any{"url": "http://b"}); code != http.StatusConflict {
			t.Fatalf("second create of the same name: got %d, want 409", code)
		}
	})
}

func TestViewerCannotSave(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		owner := signIn(t, s)
		createEnvironment(t, s, owner, "staging")
		viewer := signInAs(t, s, "viewer@example.com", RoleViewer)
		if code, _ := saveResource(t, s, viewer, upstreamPath, 0, map[string]any{"url": "http://a"}); code != http.StatusForbidden {
			t.Fatalf("viewer save: got %d, want 403", code)
		}
		editor := signInAs(t, s, "editor@example.com", RoleEditor)
		if code, _ := saveResource(t, s, editor, upstreamPath, 0, map[string]any{"url": "http://a"}); code != http.StatusOK {
			t.Fatalf("editor save: got %d, want 200", code)
		}
		rec := do(t, s, http.MethodPost, "/api/v1/environments", testPublicURL, editor, map[string]string{"name": "prod"})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("editor creating an environment: got %d, want 403", rec.Code)
		}
	})
}

func TestSaveRefusesInvalidResource(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		owner := signIn(t, s)
		createEnvironment(t, s, owner, "staging")
		for name, body := range map[string]map[string]any{
			"unknown field": {"url": "http://a", "timeout_ms": 5},
			"bad scheme":    {"url": "ftp://a"},
		} {
			if code, _ := saveResource(t, s, owner, upstreamPath, 0, body); code != http.StatusUnprocessableEntity {
				t.Errorf("%s: got %d, want 422", name, code)
			}
		}
		if code, _ := saveResource(t, s, owner, "/api/v1/environments/staging/resources/widget/x", 0, map[string]any{}); code != http.StatusUnprocessableEntity && code != http.StatusNotFound {
			t.Errorf("unknown kind: got %d", code)
		}
	})
}

func TestListAndDeleteResources(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		owner := signIn(t, s)
		createEnvironment(t, s, owner, "staging")
		saveResource(t, s, owner, upstreamPath, 0, map[string]any{"url": "http://a"})

		rec := do(t, s, http.MethodGet, "/api/v1/environments/staging/resources", "", owner, nil)
		var list struct {
			Resources []savedResource `json:"resources"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &list)
		if len(list.Resources) != 1 || list.Resources[0].Name != "users" {
			t.Fatalf("list = %s", rec.Body)
		}

		if rec := do(t, s, http.MethodDelete, upstreamPath+"?base_revision=7", testPublicURL, owner, nil); rec.Code != http.StatusConflict {
			t.Fatalf("stale delete: got %d, want 409", rec.Code)
		}
		if rec := do(t, s, http.MethodDelete, upstreamPath+"?base_revision=1", testPublicURL, owner, nil); rec.Code != http.StatusNoContent {
			t.Fatalf("delete: got %d %s", rec.Code, rec.Body)
		}
		if rec := do(t, s, http.MethodGet, upstreamPath, "", owner, nil); rec.Code != http.StatusNotFound {
			t.Fatalf("get after delete: got %d", rec.Code)
		}
	})
}

func TestSchemaRouteNamesEveryKind(t *testing.T) {
	s := newTestServer(t, "sqlite:"+t.TempDir()+"/studio.db")
	c := signIn(t, s)
	rec := do(t, s, http.MethodGet, "/api/v1/schema", "", c, nil)
	var out struct {
		Artifact struct {
			Defs map[string]any `json:"$defs"`
		} `json:"artifact"`
		Definitions map[string]string `json:"definitions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("schema: %d %v", rec.Code, err)
	}
	for kind, def := range out.Definitions {
		if _, ok := out.Artifact.Defs[def]; !ok {
			t.Errorf("kind %s names %s, which the schema lacks", kind, def)
		}
	}
	if len(out.Definitions) != 4 {
		t.Errorf("definitions = %v", out.Definitions)
	}
}
