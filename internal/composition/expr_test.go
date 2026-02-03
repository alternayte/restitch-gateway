package composition

import (
	"strings"
	"testing"
)

func TestCompileExpression_Valid(t *testing.T) {
	env := BuildBaseEnvironment([]string{"user"})

	tests := []struct {
		name string
		expr string
	}{
		{"simple variable", "req.query.user_id"},
		{"step reference", "steps.user.body.id"},
		{"arithmetic", "steps.user.body.count + 1"},
		{"string concat", `"User: " + steps.user.body.name`},
		{"array index", "steps.user.body.items[0]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled, err := CompileExpression(tt.expr, env)
			if err != nil {
				t.Fatalf("CompileExpression failed: %v", err)
			}

			if compiled.Raw != tt.expr {
				t.Errorf("expected Raw=%q, got %q", tt.expr, compiled.Raw)
			}

			if compiled.Program == nil {
				t.Error("Program should not be nil")
			}
		})
	}
}

func TestCompileExpression_InvalidSyntax(t *testing.T) {
	env := BuildBaseEnvironment(nil)

	tests := []struct {
		name string
		expr string
	}{
		{"unclosed paren", "req.query.id + (1"},
		{"invalid syntax", "req.query.id @ 1"}, // @ is not a valid operator
		{"unclosed string", `"unclosed`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CompileExpression(tt.expr, env)
			if err == nil {
				t.Error("expected compilation error for invalid syntax")
			}

			if err != nil && !strings.Contains(err.Error(), "compilation failed") {
				t.Errorf("error should mention compilation failed, got: %v", err)
			}
		})
	}
}

func TestEvaluateExpression(t *testing.T) {
	env := BuildBaseEnvironment([]string{"user"})

	// Compile expression
	compiled, err := CompileExpression("req.query.user_id", env)
	if err != nil {
		t.Fatalf("CompileExpression failed: %v", err)
	}

	// Evaluate with runtime data
	runtimeEnv := map[string]interface{}{
		"req": map[string]interface{}{
			"query": map[string]string{"user_id": "123"},
		},
	}

	result, err := EvaluateExpression(compiled, runtimeEnv)
	if err != nil {
		t.Fatalf("EvaluateExpression failed: %v", err)
	}

	if result != "123" {
		t.Errorf("expected result 123, got %v", result)
	}
}

func TestEvaluateExpression_Arithmetic(t *testing.T) {
	env := BuildBaseEnvironment([]string{"data"})

	compiled, err := CompileExpression("steps.data.body.count + 10", env)
	if err != nil {
		t.Fatalf("CompileExpression failed: %v", err)
	}

	runtimeEnv := map[string]interface{}{
		"steps": map[string]interface{}{
			"data": map[string]interface{}{
				"body": map[string]interface{}{
					"count": 5,
				},
			},
		},
	}

	result, err := EvaluateExpression(compiled, runtimeEnv)
	if err != nil {
		t.Fatalf("EvaluateExpression failed: %v", err)
	}

	if result != 15 {
		t.Errorf("expected result 15, got %v", result)
	}
}

func TestIsExpression(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"{{ req.query.user_id }}", true},
		{"/users/{{ req.query.id }}", true},
		{"{{ step.user.body }}", true},
		{"/static/path", false},
		{"plain text", false},
		{"", false},
		{"{not an expression}", false},
		{"{{ }}", true}, // Malformed but has delimiters
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := IsExpression(tt.input)
			if result != tt.expected {
				t.Errorf("IsExpression(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractExpressions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single expression",
			input:    "{{ req.query.user_id }}",
			expected: []string{"req.query.user_id"},
		},
		{
			name:     "multiple expressions",
			input:    "/users/{{ req.query.id }}/orders/{{ req.query.order }}",
			expected: []string{"req.query.id", "req.query.order"},
		},
		{
			name:     "with whitespace",
			input:    "{{  req.query.id  }}",
			expected: []string{"req.query.id"},
		},
		{
			name:     "no expressions",
			input:    "/static/path",
			expected: nil,
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "complex expression",
			input:    "{{ steps.user.body.name + ' ' + steps.user.body.email }}",
			expected: []string{"steps.user.body.name + ' ' + steps.user.body.email"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractExpressions(tt.input)

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d expressions, got %d", len(tt.expected), len(result))
			}

			for i, expr := range result {
				if expr != tt.expected[i] {
					t.Errorf("expression[%d] = %q, expected %q", i, expr, tt.expected[i])
				}
			}
		})
	}
}

func TestBuildBaseEnvironment(t *testing.T) {
	env := BuildBaseEnvironment([]string{"user", "orders"})

	// Check req structure
	req, ok := env["req"].(map[string]interface{})
	if !ok {
		t.Fatal("req should be map[string]interface{}")
	}

	if _, ok := req["path"]; !ok {
		t.Error("req should have path")
	}
	if _, ok := req["query"]; !ok {
		t.Error("req should have query")
	}
	if _, ok := req["headers"]; !ok {
		t.Error("req should have headers")
	}
	if _, ok := req["body"]; !ok {
		t.Error("req should have body")
	}

	// Check steps structure
	steps, ok := env["steps"].(map[string]interface{})
	if !ok {
		t.Fatal("steps should be map[string]interface{}")
	}

	if len(steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(steps))
	}

	// Check user step structure
	user, ok := steps["user"].(map[string]interface{})
	if !ok {
		t.Fatal("user step should be map[string]interface{}")
	}

	if _, ok := user["status"]; !ok {
		t.Error("user step should have status")
	}
	if _, ok := user["headers"]; !ok {
		t.Error("user step should have headers")
	}
	if _, ok := user["body"]; !ok {
		t.Error("user step should have body")
	}
}

func TestBuildBaseEnvironment_NoSteps(t *testing.T) {
	env := BuildBaseEnvironment(nil)

	if _, ok := env["steps"]; ok {
		t.Error("steps should not be present when no step names provided")
	}
}
