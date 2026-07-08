package composition

import (
	"net/http"
	"testing"
)

func mustCompileBodyNode(t *testing.T, body any) *CompiledBodyNode {
	t.Helper()
	env := BuildBaseEnvironment([]string{"user", "posts", "item1", "item2"})
	node, err := compileBodyNode(body, env)
	if err != nil {
		t.Fatalf("compileBodyNode failed: %v", err)
	}
	return node
}

func TestBuildResponse(t *testing.T) {
	tests := []struct {
		name        string
		template    *CompiledResponse
		stepResults map[string]*StepResult
		wantStatus  int
		wantErr     bool
	}{
		{
			name: "simple static response",
			template: &CompiledResponse{
				Body: mustCompileBodyNode(t, map[string]any{
					"message": "hello",
					"count":   42,
				}),
				ContentType: "application/json",
			},
			stepResults: map[string]*StepResult{},
			wantStatus:  200,
		},
		{
			name: "response with step results",
			template: &CompiledResponse{
				Body: mustCompileBodyNode(t, map[string]any{
					"user":  "{{ steps.user.body }}",
					"posts": "{{ steps.posts.body }}",
				}),
				ContentType: "application/json",
			},
			stepResults: map[string]*StepResult{
				"user": {
					Status: 200,
					Body: map[string]interface{}{
						"id":   float64(1),
						"name": "Alice",
					},
				},
				"posts": {
					Status: 200,
					Body: []interface{}{
						map[string]interface{}{"id": float64(101)},
						map[string]interface{}{"id": float64(102)},
					},
				},
			},
			wantStatus: 200,
		},
		{
			name: "nested template structure",
			template: &CompiledResponse{
				Body: mustCompileBodyNode(t, map[string]any{
					"data": map[string]any{
						"user":  "{{ steps.user.body }}",
						"total": "{{ len(steps.posts.body) }}",
					},
					"meta": map[string]any{
						"status": "ok",
					},
				}),
				ContentType: "application/json",
			},
			stepResults: map[string]*StepResult{
				"user": {
					Status: 200,
					Body: map[string]interface{}{
						"id":   float64(1),
						"name": "Alice",
					},
				},
				"posts": {
					Status: 200,
					Body: []interface{}{
						map[string]interface{}{"id": float64(101)},
						map[string]interface{}{"id": float64(102)},
					},
				},
			},
			wantStatus: 200,
		},
		{
			name: "array in template",
			template: &CompiledResponse{
				Body: mustCompileBodyNode(t, map[string]any{
					"items": []any{
						"{{ steps.item1.body }}",
						"{{ steps.item2.body }}",
					},
				}),
				ContentType: "application/json",
			},
			stepResults: map[string]*StepResult{
				"item1": {
					Status: 200,
					Body:   map[string]interface{}{"name": "first"},
				},
				"item2": {
					Status: 200,
					Body:   map[string]interface{}{"name": "second"},
				},
			},
			wantStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "http://example.com/test", nil)

			resp, err := BuildResponse(tt.template, tt.stepResults, req, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if resp.Status != tt.wantStatus {
				t.Errorf("BuildResponse() status = %d, want %d", resp.Status, tt.wantStatus)
			}

			if resp.Body == nil {
				t.Errorf("BuildResponse() body is nil")
			}
		})
	}
}

func TestEvaluateBodyNode(t *testing.T) {
	tests := []struct {
		name     string
		body     any
		env      map[string]any
		wantNil  bool
		wantErr  bool
	}{
		{
			name: "literal string",
			body: "hello",
			env:  map[string]any{},
		},
		{
			name: "literal number",
			body: 42,
			env:  map[string]any{},
		},
		{
			name: "simple expression",
			body: "{{ name }}",
			env: map[string]any{
				"name": "Alice",
			},
		},
		{
			name: "nested map",
			body: map[string]any{
				"user": "{{ user.name }}",
				"age":  "{{ user.age }}",
			},
			env: map[string]any{
				"user": map[string]any{
					"name": "Alice",
					"age":  float64(30),
				},
			},
		},
		{
			name: "array of expressions",
			body: []any{
				"{{ items[0] }}",
				"{{ items[1] }}",
			},
			env: map[string]any{
				"items": []any{"first", "second"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := compileBodyNode(tt.body, tt.env)
			if err != nil {
				t.Fatalf("compileBodyNode failed: %v", err)
			}

			got, err := evaluateBodyNode(node, tt.env)
			if (err != nil) != tt.wantErr {
				t.Errorf("evaluateBodyNode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if got == nil && !tt.wantNil {
				t.Errorf("evaluateBodyNode() = nil, want non-nil")
			}
		})
	}
}
