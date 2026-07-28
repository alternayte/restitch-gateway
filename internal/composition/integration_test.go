package composition

import (
	"context"
	"os"
	"testing"
)

func TestIntegration_EndToEnd(t *testing.T) {
	configYAML := `
upstreams:
  user-service:
    url: "https://users.example.com"
  order-service:
    url: "https://orders.example.com"

compositions:
  user-with-orders:
    path: "/api/user-orders"
    method: GET
    steps:
      - name: user
        upstream: user-service
        path: "/users/{{ req.query.user_id }}"
      - name: orders
        upstream: order-service
        path: "/orders?user_id={{ req.query.user_id }}"
    response:
      status: 200
      content_type: "application/json"
      body:
        user: "{{ steps.user.body }}"
        orders: "{{ steps.orders.body }}"
        total: "{{ len(steps.orders.body) }}"
`

	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(configYAML)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	cfg, err := LoadConfigFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfigFile failed: %v", err)
	}

	if len(cfg.Upstreams) != 2 {
		t.Errorf("expected 2 upstreams, got %d", len(cfg.Upstreams))
	}

	comp, exists := cfg.Compositions["user-with-orders"]
	if !exists {
		t.Fatal("composition user-with-orders not found")
	}

	if len(comp.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(comp.Steps))
	}

	compiled, err := CompileConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CompileConfig failed: %v", err)
	}

	compiledComp, exists := compiled.Compositions["user-with-orders"]
	if !exists {
		t.Fatal("compiled composition user-with-orders not found")
	}

	if len(compiledComp.Steps) != 2 {
		t.Errorf("expected 2 compiled steps, got %d", len(compiledComp.Steps))
	}

	userStep, exists := compiledComp.Steps["user"]
	if !exists {
		t.Fatal("compiled user step not found")
	}

	if userStep.PathPart == nil {
		t.Error("user step path template should be compiled")
	}

	if userStep.PathPart.Raw == "" {
		t.Error("user step path template should have raw value")
	}

	ordersStep, exists := compiledComp.Steps["orders"]
	if !exists {
		t.Fatal("compiled orders step not found")
	}

	if ordersStep.PathPart == nil {
		t.Error("orders step path template should be compiled")
	}

	if compiledComp.Response == nil {
		t.Fatal("compiled response should exist")
	}

	if compiledComp.Response.Body == nil {
		t.Error("response body should be compiled")
	}

	// Verify the body tree structure
	if compiledComp.Response.Body.Map == nil {
		t.Fatal("response body should be a map node")
	}

	if _, ok := compiledComp.Response.Body.Map["user"]; !ok {
		t.Error("expected user body expression in compiled body tree")
	}
	if _, ok := compiledComp.Response.Body.Map["orders"]; !ok {
		t.Error("expected orders body expression in compiled body tree")
	}
	if _, ok := compiledComp.Response.Body.Map["total"]; !ok {
		t.Error("expected total body expression in compiled body tree")
	}
}

func TestIntegration_InvalidExpressionFailsAtParseTime(t *testing.T) {
	configYAML := `
upstreams:
  test:
    url: "https://test.com"

compositions:
  test:
    path: "/test"
    steps:
      - name: step1
        upstream: test
        path: "/bad/{{ invalid @ syntax }}"
    response:
      status: 200
      body: {}
`

	cfg, err := ParseConfig([]byte(configYAML))
	if err != nil {
		t.Fatalf("ParseConfig should succeed, expression validation comes later: %v", err)
	}

	_, err = CompileConfig(context.Background(), cfg)
	if err == nil {
		t.Fatal("CompileConfig should fail for invalid expression syntax")
	}

	if err.Error() == "" {
		t.Error("error message should be non-empty")
	}
}

func TestIntegration_MissingUpstreamFailsValidation(t *testing.T) {
	configYAML := `
upstreams:
  test:
    url: "https://test.com"

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

	_, err := ParseConfig([]byte(configYAML))
	if err == nil {
		t.Fatal("ParseConfig should fail for missing upstream reference")
	}
}
