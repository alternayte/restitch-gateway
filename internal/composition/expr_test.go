package composition

import (
	"testing"
)

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
		{"{{ }}", true},
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

	steps, ok := env["steps"].(map[string]interface{})
	if !ok {
		t.Fatal("steps should be map[string]interface{}")
	}

	if len(steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(steps))
	}

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
