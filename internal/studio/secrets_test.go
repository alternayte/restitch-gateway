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
	"reflect"
	"testing"
)

type requiredNamesResult struct {
	Names []struct {
		Name     string   `json:"name"`
		Optional bool     `json:"optional"`
		UsedBy   []string `json:"used_by"`
	} `json:"names"`
}

// getRequiredNames returns name → JSON of used_by, with " optional" appended
// when every reference gives a default.
func getRequiredNames(t *testing.T, s *Server, c []*http.Cookie, path string) map[string]string {
	t.Helper()
	rec := do(t, s, http.MethodGet, path, "", c, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body)
	}
	var out requiredNamesResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, n := range out.Names {
		b, _ := json.Marshal(n.UsedBy)
		got[n.Name] = string(b)
		if n.Optional {
			got[n.Name] += " optional"
		}
	}
	return got
}

func TestRequiredNamesListsEveryReference(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		seedValidDraft(t, s, c)
		// A nested auth field, a name used twice, a name with a default, and
		// a literal $$ that is not a reference.
		if code, _ := saveResource(t, s, c, stagingRes+"upstream/billing", 0, map[string]any{
			"url": "https://${BILLING_HOST:billing.internal}/api",
			"auth": map[string]any{"header": map[string]any{
				"name": "Authorization", "value": "Bearer ${API_TOKEN}",
			}},
		}); code != http.StatusOK {
			t.Fatalf("save billing: %d", code)
		}
		if code, _ := saveResource(t, s, c, stagingRes+"composition/user", 1, map[string]any{
			"path": "/users/{id}", "public": true,
			"steps": []any{map[string]any{
				"name": "u", "upstream": "users", "path": "/users/{{ req.params.id }}",
				"headers": map[string]any{"X-Key": "${API_TOKEN}", "X-Price": "$$5"},
			}},
			"response": map[string]any{"body": "{{ steps.u.body }}"},
		}); code != http.StatusOK {
			t.Fatalf("save composition: %d", code)
		}

		want := map[string]string{
			"API_TOKEN":    `["composition/user","upstream/billing"]`,
			"BILLING_HOST": `["upstream/billing"] optional`,
			"USERS_HOST":   `["upstream/users"]`,
		}
		if got := getRequiredNames(t, s, c, "/api/v1/environments/staging/secrets"); !reflect.DeepEqual(got, want) {
			t.Fatalf("draft names = %v, want %v", got, want)
		}

		if code, _, body := publish(t, s, c, "first"); code != http.StatusOK {
			t.Fatalf("publish: %d %s", code, body)
		}
		// The draft moves on; version 1 keeps the names it was published with.
		if code, _ := saveResource(t, s, c, stagingRes+"upstream/users", 1, map[string]any{"url": "https://users.internal/v1"}); code != http.StatusOK {
			t.Fatalf("save users: %d", code)
		}
		if got := getRequiredNames(t, s, c, "/api/v1/environments/staging/versions/1/secrets"); !reflect.DeepEqual(got, want) {
			t.Fatalf("version 1 names = %v, want %v", got, want)
		}
		delete(want, "USERS_HOST")
		if got := getRequiredNames(t, s, c, "/api/v1/environments/staging/secrets"); !reflect.DeepEqual(got, want) {
			t.Fatalf("draft names after edit = %v, want %v", got, want)
		}
	})
}

func TestSaveRefusesInvalidVarName(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		createEnvironment(t, s, c, "staging")
		for _, bad := range []string{"${API-TOKEN}", "${1TOKEN}", "${}", "${API TOKEN}"} {
			code, _ := saveResource(t, s, c, stagingRes+"upstream/billing", 0, map[string]any{
				"url":  "https://billing.internal",
				"auth": map[string]any{"header": map[string]any{"name": "Authorization", "value": bad}},
			})
			if code != http.StatusUnprocessableEntity {
				t.Errorf("%s: got %d, want 422", bad, code)
			}
		}
	})
}
