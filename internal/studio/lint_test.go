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

func lint(t *testing.T, s *Server, c []*http.Cookie, template string, steps []string) []string {
	t.Helper()
	rec := do(t, s, http.MethodPost, "/api/v1/lint", testPublicURL, c, map[string]any{"template": template, "steps": steps})
	if rec.Code != http.StatusOK {
		t.Fatalf("lint: %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Errors []string `json:"errors"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Errors
}

func TestLintReportsUnknownStepReference(t *testing.T) {
	s := newTestServer(t, "sqlite:"+t.TempDir()+"/studio.db")
	c := signIn(t, s)
	if errs := lint(t, s, c, "/users/{{ steps.nope.body.id }}", []string{"user"}); len(errs) != 1 || !strings.Contains(errs[0], `no step "nope"`) {
		t.Fatalf("errors = %v", errs)
	}
	if errs := lint(t, s, c, "/users/{{ req.path.id }}", nil); len(errs) != 1 || !strings.Contains(errs[0], "is a string") {
		t.Fatalf("errors = %v", errs)
	}
	if errs := lint(t, s, c, "/users/{{ steps.user.body.id }}/{{ req.params.id }}", []string{"user"}); len(errs) != 0 {
		t.Fatalf("a valid template got errors: %v", errs)
	}
	if errs := lint(t, s, c, "/users/{{ req.params.id", nil); len(errs) != 1 || !strings.Contains(errs[0], "unclosed") {
		t.Fatalf("errors = %v", errs)
	}
}
