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

package composition

import (
	"testing"
)

func TestCompileTemplate(t *testing.T) {
	env := map[string]any{
		"x": 42,
		"s": "hello",
		"req": map[string]any{
			"query": map[string]string{"id": "7"},
		},
		"steps": map[string]any{
			"user": map[string]any{
				"body": map[string]any{"name": "alice"},
			},
		},
	}

	tests := []struct {
		name     string
		raw      string
		wantErr  bool
		isSingle bool
		depsLen  int
	}{
		{"pure literal", "/users/123", false, false, 0},
		{"single expr", "{{ x }}", false, true, 0},
		{"single expr with spaces", "{{  x  }}", false, true, 0},
		{"multi expr", "/users/{{ x }}/name/{{ s }}", false, false, 0},
		{"step dep", "{{ steps.user.body.name }}", false, true, 1},
		{"multiple step deps", "{{ steps.user.body.name }} {{ steps.user.body }}", false, false, 1},
		{"unclosed", "{{ x", true, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := CompileTemplate(tt.raw, env)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tmpl.IsSingle != tt.isSingle {
				t.Errorf("IsSingle = %v, want %v", tmpl.IsSingle, tt.isSingle)
			}
			if len(tmpl.Deps) != tt.depsLen {
				t.Errorf("Deps len = %d, want %d (deps: %v)", len(tmpl.Deps), tt.depsLen, tmpl.Deps)
			}
		})
	}
}

func TestTemplate_EvalValue_PreservesType(t *testing.T) {
	env := map[string]any{"x": 42}

	tmpl, err := CompileTemplate("{{ 1 + 1 }}", env)
	if err != nil {
		t.Fatal(err)
	}

	val, err := tmpl.EvalValue(env)
	if err != nil {
		t.Fatal(err)
	}

	if v, ok := val.(int); !ok || v != 2 {
		t.Errorf("EvalValue = %v (%T), want int 2", val, val)
	}
}

func TestTemplate_EvalString_Interpolation(t *testing.T) {
	env := map[string]any{
		"req": map[string]any{
			"query": map[string]string{"id": "42"},
		},
	}

	tmpl, err := CompileTemplate("/users/{{ req.query.id }}/posts", env)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tmpl.EvalString(env, EscapeNone)
	if err != nil {
		t.Fatal(err)
	}

	if result != "/users/42/posts" {
		t.Errorf("got %q, want /users/42/posts", result)
	}
}

func TestTemplate_EvalString_PathEscape(t *testing.T) {
	env := map[string]any{
		"id": "a/b",
	}

	tmpl, err := CompileTemplate("/users/{{ id }}", env)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tmpl.EvalString(env, EscapePath)
	if err != nil {
		t.Fatal(err)
	}

	if result != "/users/a%2Fb" {
		t.Errorf("got %q, want /users/a%%2Fb", result)
	}
}

func TestTemplate_EvalString_QueryEscape(t *testing.T) {
	env := map[string]any{
		"q": "hello world",
	}

	tmpl, err := CompileTemplate("q={{ q }}", env)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tmpl.EvalString(env, EscapeQuery)
	if err != nil {
		t.Fatal(err)
	}

	if result != "q=hello+world" {
		t.Errorf("got %q, want q=hello+world", result)
	}
}

func TestTemplate_EvalString_JSONEscape(t *testing.T) {
	env := map[string]any{
		"name": `he said "hi"`,
	}

	tmpl, err := CompileTemplate(`{"name": {{ name }}}`, env)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tmpl.EvalString(env, EscapeJSON)
	if err != nil {
		t.Fatal(err)
	}

	expected := `{"name": "he said \"hi\""}`
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestTemplate_NoResubstitution(t *testing.T) {
	env := map[string]any{
		"val": "{{ sneaky }}",
	}

	tmpl, err := CompileTemplate("result={{ val }}", env)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tmpl.EvalString(env, EscapeNone)
	if err != nil {
		t.Fatal(err)
	}

	if result != "result={{ sneaky }}" {
		t.Errorf("got %q, want 'result={{ sneaky }}' (no re-substitution)", result)
	}
}

func TestTemplate_Deps(t *testing.T) {
	env := map[string]any{
		"steps": map[string]any{
			"user":   map[string]any{"body": map[string]any{}},
			"orders": map[string]any{"body": map[string]any{}},
		},
	}

	tmpl, err := CompileTemplate("{{ steps.user.body.id }} and {{ steps.orders.body }}", env)
	if err != nil {
		t.Fatal(err)
	}

	if len(tmpl.Deps) != 2 {
		t.Fatalf("expected 2 deps, got %d: %v", len(tmpl.Deps), tmpl.Deps)
	}

	depSet := map[string]bool{}
	for _, d := range tmpl.Deps {
		depSet[d] = true
	}
	if !depSet["user"] || !depSet["orders"] {
		t.Errorf("expected deps [user, orders], got %v", tmpl.Deps)
	}
}

func TestTemplate_PureLiteral(t *testing.T) {
	env := map[string]any{}

	tmpl, err := CompileTemplate("/static/path", env)
	if err != nil {
		t.Fatal(err)
	}

	if tmpl.IsSingle {
		t.Error("pure literal should not be IsSingle")
	}

	result, err := tmpl.EvalString(env, EscapeNone)
	if err != nil {
		t.Fatal(err)
	}

	if result != "/static/path" {
		t.Errorf("got %q, want /static/path", result)
	}
}
