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
	"strings"
	"testing"
)

const stagingRes = "/api/v1/environments/staging/resources/"

type publishResult struct {
	Number  int    `json:"number"`
	YAML    string `json:"yaml"`
	Author  string `json:"author"`
	Message string `json:"message"`
}

type previewResult struct {
	Valid   bool     `json:"valid"`
	Errors  []string `json:"errors"`
	YAML    string   `json:"yaml"`
	Diff    string   `json:"diff"`
	Current int      `json:"current_version"`
}

func seedValidDraft(t *testing.T, s *Server, c []*http.Cookie) {
	t.Helper()
	createEnvironment(t, s, c, "staging")
	saveResource(t, s, c, stagingRes+"upstream/users", 0, map[string]any{"url": "https://${USERS_HOST}/v1"})
	saveResource(t, s, c, stagingRes+"composition/user", 0, map[string]any{
		"path": "/users/{id}", "public": true,
		"steps":    []any{map[string]any{"name": "u", "upstream": "users", "path": "/users/{{ request.path.id }}"}},
		"response": map[string]any{"body": "{{ steps.u.body }}"},
	})
}

func publish(t *testing.T, s *Server, c []*http.Cookie, msg string) (int, publishResult, string) {
	t.Helper()
	rec := do(t, s, http.MethodPost, "/api/v1/environments/staging/publish", testPublicURL, c, map[string]string{"message": msg})
	var out publishResult
	if rec.Code == http.StatusOK {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out, rec.Body.String()
}

func TestPublishRefusesInvalidDraft(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		createEnvironment(t, s, c, "staging")
		// The composition is not public and the draft has no auth policy,
		// and its step names an upstream the draft does not hold.
		saveResource(t, s, c, stagingRes+"composition/orders", 0, map[string]any{
			"path":     "/orders",
			"steps":    []any{map[string]any{"name": "o", "upstream": "missing", "path": "/orders"}},
			"response": map[string]any{"body": "{{ steps.o.body }}"},
		})
		code, _, body := publish(t, s, c, "first")
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("publish of an invalid draft: got %d %s", code, body)
		}
		rec := do(t, s, http.MethodGet, "/api/v1/environments/staging/versions", "", c, nil)
		if strings.Contains(rec.Body.String(), `"number"`) {
			t.Fatalf("an invalid publish wrote a version: %s", rec.Body)
		}
	})
}

func TestPublishWritesNumberedVersions(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		seedValidDraft(t, s, c)

		code, v1, body := publish(t, s, c, "first")
		if code != http.StatusOK || v1.Number != 1 || v1.Author != ownerEmail || v1.Message != "first" {
			t.Fatalf("publish 1: %d %s", code, body)
		}
		if !strings.Contains(v1.YAML, "https://${USERS_HOST}/v1") {
			t.Fatalf("the artifact must keep ${VAR} for the gateway to resolve:\n%s", v1.YAML)
		}

		saveResource(t, s, c, stagingRes+"upstream/users", 1, map[string]any{"url": "https://${USERS_HOST}/v2"})
		rec := do(t, s, http.MethodPost, "/api/v1/environments/staging/publish/preview", testPublicURL, c, nil)
		var pv previewResult
		_ = json.Unmarshal(rec.Body.Bytes(), &pv)
		if !pv.Valid || pv.Current != 1 || !strings.Contains(pv.Diff, "-") || !strings.Contains(pv.Diff, "/v2") {
			t.Fatalf("preview = %s", rec.Body)
		}

		code, v2, _ := publish(t, s, c, "second")
		if code != http.StatusOK || v2.Number != 2 {
			t.Fatalf("publish 2: %d", code)
		}
		rec = do(t, s, http.MethodGet, "/api/v1/environments/staging/versions/1", "", c, nil)
		var got publishResult
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if got.YAML != v1.YAML {
			t.Fatalf("version 1 changed after a later publish")
		}
	})
}

func TestPublishPreviewExplainsBareVarURL(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		createEnvironment(t, s, c, "staging")
		code, _ := saveResource(t, s, c, stagingRes+"upstream/users", 0, map[string]any{"url": "${USERS_URL}"})
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("a bare ${VAR} url: got %d, want 422", code)
		}
	})
}

func TestViewerCannotPublish(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		owner := signIn(t, s)
		seedValidDraft(t, s, owner)
		viewer := signInAs(t, s, "viewer@example.com", RoleViewer)
		if code, _, _ := publish(t, s, viewer, "x"); code != http.StatusForbidden {
			t.Fatalf("viewer publish: got %d, want 403", code)
		}
	})
}
