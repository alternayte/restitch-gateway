package composition

import (
	"strings"
	"testing"
)

func TestParseConfig_Valid(t *testing.T) {
	yaml := `
upstreams:
  user-service:
    url: "https://users.example.com"
  order-service:
    url: "https://orders.example.com"

compositions:
  user-with-orders:
    path: "/api/user-orders"
    steps:
      - name: user
        upstream: user-service
        path: "/users/{{ req.query.user_id }}"
      - name: orders
        upstream: order-service
        path: "/orders?user_id={{ req.query.user_id }}"
    response:
      status: 200
      body:
        user: "{{ steps.user.body }}"
        orders: "{{ steps.orders.body }}"
`

	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	// Verify upstreams
	if len(cfg.Upstreams) != 2 {
		t.Errorf("expected 2 upstreams, got %d", len(cfg.Upstreams))
	}

	if cfg.Upstreams["user-service"].URL != "https://users.example.com" {
		t.Errorf("user-service URL mismatch")
	}

	// Verify composition
	comp, exists := cfg.Compositions["user-with-orders"]
	if !exists {
		t.Fatal("composition user-with-orders not found")
	}

	if comp.Path != "/api/user-orders" {
		t.Errorf("expected path /api/user-orders, got %s", comp.Path)
	}

	// Verify defaults applied
	if comp.Method != "GET" {
		t.Errorf("expected default method GET, got %s", comp.Method)
	}

	if comp.Response.ContentType != "application/json" {
		t.Errorf("expected default content_type application/json, got %s", comp.Response.ContentType)
	}

	// Verify steps
	if len(comp.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(comp.Steps))
	}

	if comp.Steps[0].Name != "user" {
		t.Errorf("expected step name user, got %s", comp.Steps[0].Name)
	}

	if comp.Steps[0].Method != "GET" {
		t.Errorf("expected default step method GET, got %s", comp.Steps[0].Method)
	}
}

func TestParseConfig_InvalidYAML(t *testing.T) {
	yaml := `invalid: yaml: syntax:`

	_, err := ParseConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}

	if !strings.Contains(err.Error(), "invalid YAML") {
		t.Errorf("error should mention invalid YAML, got: %v", err)
	}
}

func TestParseConfig_MissingUpstream(t *testing.T) {
	yaml := `
upstreams:
  user-service:
    url: "https://users.example.com"

compositions:
  test:
    path: "/test"
    steps:
      - name: step1
        upstream: nonexistent
        path: "/test"
    response:
      status: 200
      body: {}
`

	_, err := ParseConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing upstream")
	}

	if !strings.Contains(err.Error(), "upstream") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention upstream not found, got: %v", err)
	}
}

func TestParseConfig_DuplicateStepNames(t *testing.T) {
	yaml := `
upstreams:
  user-service:
    url: "https://users.example.com"

compositions:
  test:
    path: "/test"
    steps:
      - name: step1
        upstream: user-service
        path: "/test1"
      - name: step1
        upstream: user-service
        path: "/test2"
    response:
      status: 200
      body: {}
`

	_, err := ParseConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for duplicate step names")
	}

	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention duplicate, got: %v", err)
	}
}

func TestParseConfig_MissingStepName(t *testing.T) {
	yaml := `
upstreams:
  user-service:
    url: "https://users.example.com"

compositions:
  test:
    path: "/test"
    steps:
      - upstream: user-service
        path: "/test"
    response:
      status: 200
      body: {}
`

	_, err := ParseConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing step name")
	}

	if !strings.Contains(err.Error(), "no name") {
		t.Errorf("error should mention no name, got: %v", err)
	}
}

func TestCompileConfig_Valid(t *testing.T) {
	yaml := `
upstreams:
  user-service:
    url: "https://users.example.com"

compositions:
  test:
    path: "/test"
    steps:
      - name: user
        upstream: user-service
        path: "/users/{{ req.query.user_id }}"
    response:
      status: 200
      body:
        user: "{{ steps.user.body }}"
`

	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	compiled, err := CompileConfig(cfg)
	if err != nil {
		t.Fatalf("CompileConfig failed: %v", err)
	}

	if compiled.Config != cfg {
		t.Error("CompiledConfig should reference original config")
	}

	// Check compiled composition exists
	comp, exists := compiled.Compositions["test"]
	if !exists {
		t.Fatal("compiled composition test not found")
	}

	// Check compiled step
	step, exists := comp.Steps["user"]
	if !exists {
		t.Fatal("compiled step user not found")
	}

	if step.PathExpr == nil {
		t.Error("step path expression should be compiled")
	}

	if step.PathExpr.Raw == "" {
		t.Error("compiled expression should have raw value")
	}

	// Check compiled response
	if comp.Response == nil {
		t.Fatal("compiled response should exist")
	}

	if len(comp.Response.BodyExprs) == 0 {
		t.Error("response body expressions should be compiled")
	}
}

func TestCompileConfig_InvalidExpression(t *testing.T) {
	yaml := `
upstreams:
  user-service:
    url: "https://users.example.com"

compositions:
  test:
    path: "/test"
    steps:
      - name: user
        upstream: user-service
        path: "/users/{{ invalid syntax @ }}"
    response:
      status: 200
      body: {}
`

	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	_, err = CompileConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid expression")
	}

	if !strings.Contains(err.Error(), "test") {
		t.Errorf("error should mention composition name, got: %v", err)
	}
}

func TestCompileConfig_InvalidResponseExpression(t *testing.T) {
	yaml := `
upstreams:
  user-service:
    url: "https://users.example.com"

compositions:
  test:
    path: "/test"
    steps:
      - name: user
        upstream: user-service
        path: "/users/1"
    response:
      status: "{{ invalid @ }}"
      body: {}
`

	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	_, err = CompileConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid response expression")
	}

	if !strings.Contains(err.Error(), "response") {
		t.Errorf("error should mention response, got: %v", err)
	}
}

func TestCompileConfig_NestedBodyExpressions(t *testing.T) {
	yaml := `
upstreams:
  user-service:
    url: "https://users.example.com"

compositions:
  test:
    path: "/test"
    steps:
      - name: user
        upstream: user-service
        path: "/users/1"
      - name: orders
        upstream: user-service
        path: "/orders"
    response:
      status: 200
      body:
        user:
          id: "{{ steps.user.body.id }}"
          name: "{{ steps.user.body.name }}"
        orders: "{{ steps.orders.body }}"
`

	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	compiled, err := CompileConfig(cfg)
	if err != nil {
		t.Fatalf("CompileConfig failed: %v", err)
	}

	comp := compiled.Compositions["test"]
	if len(comp.Response.BodyExprs) < 3 {
		t.Errorf("expected at least 3 compiled body expressions, got %d", len(comp.Response.BodyExprs))
	}

	// Check that nested paths are compiled
	foundUserID := false
	foundOrders := false
	for path := range comp.Response.BodyExprs {
		if strings.Contains(path, "user.id") {
			foundUserID = true
		}
		if strings.Contains(path, "orders") {
			foundOrders = true
		}
	}

	if !foundUserID {
		t.Error("expected user.id expression to be compiled")
	}
	if !foundOrders {
		t.Error("expected orders expression to be compiled")
	}
}

func TestCompileConfig_CircularDependency(t *testing.T) {
	yaml := `
upstreams:
  api:
    url: "http://localhost:8080"

compositions:
  test:
    path: "/test"
    method: GET
    steps:
      - name: a
        upstream: api
        path: "/a/{{ steps.b.body.id }}"
        method: GET
      - name: b
        upstream: api
        path: "/b/{{ steps.a.body.id }}"
        method: GET
    response:
      status: 200
      body:
        result: ok
      content_type: application/json
`

	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	_, err = CompileConfig(cfg)
	if err == nil {
		t.Fatal("expected error for circular dependency, got nil")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("expected 'circular dependency' error, got: %v", err)
	}
}

func TestCompileConfig_MissingStepReference(t *testing.T) {
	yaml := `
upstreams:
  api:
    url: "http://localhost:8080"

compositions:
  test:
    path: "/test"
    method: GET
    steps:
      - name: a
        upstream: api
        path: "/a/{{ steps.nonexistent.body.id }}"
        method: GET
    response:
      status: 200
      body:
        result: ok
      content_type: application/json
`

	cfg, err := ParseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	_, err = CompileConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing step reference, got nil")
	}
	if !strings.Contains(err.Error(), "non-existent") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'non-existent' or 'not found' error, got: %v", err)
	}
}
