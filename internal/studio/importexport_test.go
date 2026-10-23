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

func exportDraft(t *testing.T, s *Server, c []*http.Cookie, env string) string {
	t.Helper()
	rec := do(t, s, http.MethodGet, "/api/v1/environments/"+env+"/export", "", c, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("export: %d %s", rec.Code, rec.Body)
	}
	var out struct {
		YAML string `json:"yaml"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out.YAML
}

func TestExportThenImportRoundTrips(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		seedValidDraft(t, s, c)
		saveResource(t, s, c, stagingRes+"auth/default", 0, map[string]any{"api_keys": []any{"${GATEWAY_KEY}"}})
		exported := exportDraft(t, s, c, "staging")

		createEnvironment(t, s, c, "prod")
		rec := do(t, s, http.MethodPost, "/api/v1/environments/prod/import", testPublicURL, c, map[string]string{"yaml": exported})
		if rec.Code != http.StatusOK {
			t.Fatalf("import: %d %s", rec.Code, rec.Body)
		}
		if again := exportDraft(t, s, c, "prod"); again != exported {
			t.Fatalf("the round trip changed the artifact:\n%s\n---\n%s", exported, again)
		}
	})
}

func TestImportRefusesInvalidFile(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		seedValidDraft(t, s, c)
		before := exportDraft(t, s, c, "staging")
		for name, doc := range map[string]string{
			"v1 block":      "server:\n  port: 1\n",
			"unknown key":   "upstreams:\n  a:\n    url: https://a\n    timeout_ms: 1\n",
			"fail-open":     "upstreams:\n  a:\n    url: https://a\ncompositions:\n  c:\n    path: /c\n    steps: [{name: s, upstream: a, path: /}]\n    response: {body: x}\n",
			"not a mapping": "- a\n- b\n",
		} {
			rec := do(t, s, http.MethodPost, "/api/v1/environments/staging/import", testPublicURL, c, map[string]string{"yaml": doc})
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s: got %d %s", name, rec.Code, rec.Body)
			}
		}
		if after := exportDraft(t, s, c, "staging"); after != before {
			t.Fatal("a refused import changed the draft")
		}
	})
}

func TestAPIKeyCanPublish(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		seedValidDraft(t, s, c)
		key := createAPIKey(t, s, ownerEmail)
		rec := pull(s, key, "/api/v1/environments/staging/versions", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("API key on a user route: %d %s", rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), `"number"`) {
			t.Fatal("no version expected yet")
		}
	})
}
