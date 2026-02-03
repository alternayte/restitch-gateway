package composition

import (
	"os"
	"testing"
)

// TestIntegration_EndToEnd verifies the complete flow from YAML to compiled config.
func TestIntegration_EndToEnd(t *testing.T) {
	// Create test config file
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

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(configYAML)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	// Test 1: Load and parse config file
	cfg, err := LoadConfigFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfigFile failed: %v", err)
	}

	// Verify parsed structure
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

	// Test 2: Compile all expressions
	compiled, err := CompileConfig(cfg)
	if err != nil {
		t.Fatalf("CompileConfig failed: %v", err)
	}

	// Verify compiled structure
	compiledComp, exists := compiled.Compositions["user-with-orders"]
	if !exists {
		t.Fatal("compiled composition user-with-orders not found")
	}

	if len(compiledComp.Steps) != 2 {
		t.Errorf("expected 2 compiled steps, got %d", len(compiledComp.Steps))
	}

	// Verify user step compiled
	userStep, exists := compiledComp.Steps["user"]
	if !exists {
		t.Fatal("compiled user step not found")
	}

	if userStep.PathExpr == nil {
		t.Error("user step path expression should be compiled")
	}

	if userStep.PathExpr.Raw == "" {
		t.Error("user step path expression should have raw template")
	}

	// Verify orders step compiled
	ordersStep, exists := compiledComp.Steps["orders"]
	if !exists {
		t.Fatal("compiled orders step not found")
	}

	if ordersStep.PathExpr == nil {
		t.Error("orders step path expression should be compiled")
	}

	// Verify response compiled
	if compiledComp.Response == nil {
		t.Fatal("compiled response should exist")
	}

	if len(compiledComp.Response.BodyExprs) == 0 {
		t.Error("response body expressions should be compiled")
	}

	// Verify specific body expressions exist
	foundUser := false
	foundOrders := false
	foundTotal := false
	for path := range compiledComp.Response.BodyExprs {
		if path == "user" {
			foundUser = true
		}
		if path == "orders" {
			foundOrders = true
		}
		if path == "total" {
			foundTotal = true
		}
	}

	if !foundUser {
		t.Error("expected user body expression to be compiled")
	}
	if !foundOrders {
		t.Error("expected orders body expression to be compiled")
	}
	if !foundTotal {
		t.Error("expected total body expression to be compiled")
	}
}

// TestIntegration_InvalidExpressionFailsAtParseTime verifies that invalid
// expressions cause startup failure (not request-time failure).
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

	// Parse YAML - should succeed (syntax validation is separate)
	cfg, err := ParseConfig([]byte(configYAML))
	if err != nil {
		t.Fatalf("ParseConfig should succeed, expression validation comes later: %v", err)
	}

	// Compile expressions - should FAIL at parse time
	_, err = CompileConfig(cfg)
	if err == nil {
		t.Fatal("CompileConfig should fail for invalid expression syntax")
	}

	// Error should identify the problematic composition and step
	if err.Error() == "" {
		t.Error("error message should be non-empty")
	}
}

// TestIntegration_MissingUpstreamFailsValidation verifies that references
// to non-existent upstreams are caught during parsing.
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
