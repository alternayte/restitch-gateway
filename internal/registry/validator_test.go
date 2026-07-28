package registry

import (
	"strings"
	"testing"
)

const validYAML = `
upstreams:
  mock:
    url: "http://localhost:8081"
compositions:
  test:
    path: "/test"
    method: GET
    steps:
      - name: s1
        upstream: mock
        path: "/users/1"
    response:
      body:
        result: "{{ steps.s1.body }}"
`

const invalidSyntaxYAML = `compositions: [broken`

const missingUpstreamYAML = `
compositions:
  test:
    path: "/test"
    steps:
      - name: s1
        upstream: nonexistent
        path: "/x"
    response:
      body: "{{ steps.s1.body }}"
`

func TestValidate_ValidConfig(t *testing.T) {
	result := Validate([]byte(validYAML))
	if result == nil {
		t.Fatal("Validate returned nil")
	}
	if !result.Valid {
		t.Fatalf("expected Valid=true, got Valid=false, errors=%+v", result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %+v", result.Errors)
	}
}

func TestValidate_InvalidYAMLSyntax(t *testing.T) {
	result := Validate([]byte(invalidSyntaxYAML))
	if result == nil {
		t.Fatal("Validate returned nil")
	}
	if result.Valid {
		t.Fatalf("expected Valid=false, got Valid=true")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
	found := false
	for _, e := range result.Errors {
		if e.Line > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected at least one error with a line number, got %+v", result.Errors)
	}
}

func TestValidate_MissingUpstreamRef(t *testing.T) {
	result := Validate([]byte(missingUpstreamYAML))
	if result == nil {
		t.Fatal("Validate returned nil")
	}
	if result.Valid {
		t.Fatalf("expected Valid=false, got Valid=true")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "nonexistent") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an error message mentioning %q, got %+v", "nonexistent", result.Errors)
	}
}

func TestValidate_EmptyInput(t *testing.T) {
	result := Validate([]byte(""))
	if result == nil {
		t.Fatal("Validate returned nil")
	}
	if !result.Valid {
		t.Fatalf("expected Valid=true for empty input, got Valid=false, errors=%+v", result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %+v", result.Errors)
	}
}

func TestValidate_InvalidExpression(t *testing.T) {
	invalidExprYAML := `
upstreams:
  mock:
    url: "http://localhost:8081"
compositions:
  test:
    path: "/test"
    method: GET
    steps:
      - name: s1
        upstream: mock
        path: "/users/{{ bad ## expr }}"
    response:
      body:
        result: "{{ steps.s1.body }}"
`
	result := Validate([]byte(invalidExprYAML))
	if result == nil {
		t.Fatal("Validate returned nil")
	}
	if result.Valid {
		t.Fatalf("expected Valid=false, got Valid=true")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
}

func TestValidate_EnvVarExpansion(t *testing.T) {
	t.Setenv("SOME_VAR", "")
	envVarYAML := `
upstreams:
  mock:
    url: "${SOME_VAR}"
compositions:
  test:
    path: "/test"
    method: GET
    steps:
      - name: s1
        upstream: mock
        path: "/users/1"
    response:
      body:
        result: "{{ steps.s1.body }}"
`
	result := Validate([]byte(envVarYAML))
	if result == nil {
		t.Fatal("Validate returned nil")
	}
	if result.Valid {
		t.Fatalf("expected Valid=false, got Valid=true")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "SOME_VAR") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an error message mentioning %q, got %+v", "SOME_VAR", result.Errors)
	}
}
