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
	"strings"
	"testing"
)

type stepDepsOut struct {
	Steps map[string]struct {
		Explicit []string `json:"explicit"`
		Inferred []string `json:"inferred"`
	} `json:"steps"`
	Error string `json:"error"`
}

func stepDeps(t *testing.T, s *Server, c []*http.Cookie, comp map[string]any) stepDepsOut {
	t.Helper()
	rec := do(t, s, http.MethodPost, "/api/v1/step-dependencies", testPublicURL, c, map[string]any{"composition": comp})
	if rec.Code != http.StatusOK {
		t.Fatalf("step-dependencies: %d %s", rec.Code, rec.Body)
	}
	var out stepDepsOut
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestStepDependenciesIncludeTemplateReferences(t *testing.T) {
	s := newTestServer(t, "sqlite:"+t.TempDir()+"/studio.db")
	c := signIn(t, s)

	out := stepDeps(t, s, c, map[string]any{
		"path": "/x", "method": "GET",
		"steps": []any{
			map[string]any{"name": "user", "upstream": "u", "path": "/users/1"},
			map[string]any{"name": "audit", "upstream": "u", "path": "/audit"},
			map[string]any{"name": "orders", "upstream": "u", "path": "/orders/{{ steps.user.body.id }}?since={{ steps.audit.body.ts }}"},
			map[string]any{"name": "notify", "upstream": "u", "path": "/notify", "depends_on": []any{"orders"},
				"headers": map[string]any{"X-User": "{{ steps.user.body.name }}"}, "body": `{"o": {{ steps.orders.body }}}`},
		},
	})
	if out.Error != "" {
		t.Fatalf("error = %q", out.Error)
	}
	type deps struct{ explicit, inferred []string }
	want := map[string]deps{
		"user":   {[]string{}, []string{}},
		"audit":  {[]string{}, []string{}},
		"orders": {[]string{}, []string{"audit", "user"}},
		// orders is explicit, so it is not listed again as inferred.
		"notify": {[]string{"orders"}, []string{"user"}},
	}
	if len(out.Steps) != len(want) {
		t.Fatalf("steps = %v", out.Steps)
	}
	for name, w := range want {
		got := out.Steps[name]
		if !reflect.DeepEqual(got.Explicit, w.explicit) || !reflect.DeepEqual(got.Inferred, w.inferred) {
			t.Errorf("%s: explicit %v inferred %v, want %v %v", name, got.Explicit, got.Inferred, w.explicit, w.inferred)
		}
	}
}

func TestStepDependenciesReportTheCompileError(t *testing.T) {
	s := newTestServer(t, "sqlite:"+t.TempDir()+"/studio.db")
	c := signIn(t, s)

	// a and b reach each other: a through depends_on, b through a template.
	out := stepDeps(t, s, c, map[string]any{"steps": []any{
		map[string]any{"name": "a", "upstream": "u", "path": "/a", "depends_on": []any{"b"}},
		map[string]any{"name": "b", "upstream": "u", "path": "/b/{{ steps.a.body.id }}"},
	}})
	if !strings.Contains(out.Error, "circular dependency") {
		t.Fatalf("error = %q", out.Error)
	}
	if got := out.Steps["b"].Inferred; !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("b inferred = %v", got)
	}

	// A broken template still leaves the other steps' dependencies.
	out = stepDeps(t, s, c, map[string]any{"steps": []any{
		map[string]any{"name": "a", "upstream": "u", "path": "/a/{{ req.params.id"},
		map[string]any{"name": "b", "upstream": "u", "path": "/b/{{ steps.a.body.id }}"},
	}})
	if !strings.Contains(out.Error, "step a") {
		t.Fatalf("error = %q", out.Error)
	}
	if got := out.Steps["b"].Inferred; !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("b inferred = %v", got)
	}
}
