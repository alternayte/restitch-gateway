package composition

import (
	"testing"
)

func TestBuildDAG(t *testing.T) {
	tests := []struct {
		name      string
		comp      *CompiledComposition
		wantWaves [][]string
		wantErr   bool
		errMsg    string
	}{
		{
			name: "no dependencies - all in wave 1",
			comp: &CompiledComposition{
				Steps: map[string]*CompiledStep{
					"user":   {Step: &Step{Name: "user"}, Deps: nil},
					"orders": {Step: &Step{Name: "orders"}, Deps: nil},
				},
			},
			wantWaves: [][]string{
				{"user", "orders"},
			},
		},
		{
			name: "linear chain - sequential waves",
			comp: &CompiledComposition{
				Steps: map[string]*CompiledStep{
					"a": {Step: &Step{Name: "a"}, Deps: nil},
					"b": {Step: &Step{Name: "b"}, Deps: []string{"a"}},
					"c": {Step: &Step{Name: "c"}, Deps: []string{"b"}},
				},
			},
			wantWaves: [][]string{
				{"a"},
				{"b"},
				{"c"},
			},
		},
		{
			name: "diamond pattern - parallel then merge",
			comp: &CompiledComposition{
				Steps: map[string]*CompiledStep{
					"a": {Step: &Step{Name: "a"}, Deps: nil},
					"b": {Step: &Step{Name: "b"}, Deps: nil},
					"c": {Step: &Step{Name: "c"}, Deps: []string{"a", "b"}},
				},
			},
			wantWaves: [][]string{
				{"a", "b"},
				{"c"},
			},
		},
		{
			name: "explicit depends_on",
			comp: &CompiledComposition{
				Steps: map[string]*CompiledStep{
					"user":    {Step: &Step{Name: "user"}, Deps: nil},
					"profile": {Step: &Step{Name: "profile", DependsOn: []string{"user"}}, Deps: []string{"user"}},
				},
			},
			wantWaves: [][]string{
				{"user"},
				{"profile"},
			},
		},
		{
			name: "mixed explicit and inferred dependencies",
			comp: &CompiledComposition{
				Steps: map[string]*CompiledStep{
					"user":    {Step: &Step{Name: "user"}, Deps: nil},
					"orders":  {Step: &Step{Name: "orders"}, Deps: []string{"user"}},
					"profile": {Step: &Step{Name: "profile", DependsOn: []string{"user"}}, Deps: []string{"user"}},
				},
			},
			wantWaves: [][]string{
				{"user"},
				{"orders", "profile"},
			},
		},
		{
			name: "circular dependency - direct cycle",
			comp: &CompiledComposition{
				Steps: map[string]*CompiledStep{
					"a": {Step: &Step{Name: "a"}, Deps: []string{"b"}},
					"b": {Step: &Step{Name: "b"}, Deps: []string{"a"}},
				},
			},
			wantErr: true,
			errMsg:  "circular dependency",
		},
		{
			name: "circular dependency - indirect cycle",
			comp: &CompiledComposition{
				Steps: map[string]*CompiledStep{
					"a": {Step: &Step{Name: "a"}, Deps: []string{"c"}},
					"b": {Step: &Step{Name: "b"}, Deps: []string{"a"}},
					"c": {Step: &Step{Name: "c"}, Deps: []string{"b"}},
				},
			},
			wantErr: true,
			errMsg:  "circular dependency",
		},
		{
			name: "missing step reference",
			comp: &CompiledComposition{
				Steps: map[string]*CompiledStep{
					"user": {Step: &Step{Name: "user"}, Deps: []string{"nonexistent"}},
				},
			},
			wantErr: true,
			errMsg:  "non-existent step",
		},
		{
			name: "dependencies from body (pre-computed)",
			comp: &CompiledComposition{
				Steps: map[string]*CompiledStep{
					"user":   {Step: &Step{Name: "user"}, Deps: nil},
					"create": {Step: &Step{Name: "create"}, Deps: []string{"user"}},
				},
			},
			wantWaves: [][]string{
				{"user"},
				{"create"},
			},
		},
		{
			name: "dependencies from headers (pre-computed)",
			comp: &CompiledComposition{
				Steps: map[string]*CompiledStep{
					"user":       {Step: &Step{Name: "user"}, Deps: nil},
					"authorized": {Step: &Step{Name: "authorized"}, Deps: []string{"user"}},
				},
			},
			wantWaves: [][]string{
				{"user"},
				{"authorized"},
			},
		},
		{
			name: "complex DAG with multiple waves",
			comp: &CompiledComposition{
				Steps: map[string]*CompiledStep{
					"user":     {Step: &Step{Name: "user"}, Deps: nil},
					"profile":  {Step: &Step{Name: "profile"}, Deps: nil},
					"orders":   {Step: &Step{Name: "orders"}, Deps: []string{"user"}},
					"payments": {Step: &Step{Name: "payments"}, Deps: []string{"user"}},
					"summary":  {Step: &Step{Name: "summary"}, Deps: []string{"orders", "payments"}},
				},
			},
			wantWaves: [][]string{
				{"user", "profile"},
				{"orders", "payments"},
				{"summary"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildDAG(tt.comp)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildDAG() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err != nil && tt.errMsg != "" {
					errStr := err.Error()
					if !contains(errStr, tt.errMsg) {
						t.Errorf("BuildDAG() error = %v, expected to contain %q", err, tt.errMsg)
					}
				}
				return
			}

			if !equalWaves(got.Waves, tt.wantWaves) {
				t.Errorf("BuildDAG() waves = %v, want %v", got.Waves, tt.wantWaves)
			}
		})
	}
}

func TestBuildExecutionPlan(t *testing.T) {
	tests := []struct {
		name      string
		deps      map[string][]string
		wantWaves [][]string
		wantErr   bool
	}{
		{
			name: "no dependencies",
			deps: map[string][]string{
				"a": {},
				"b": {},
				"c": {},
			},
			wantWaves: [][]string{
				{"a", "b", "c"},
			},
		},
		{
			name: "linear dependencies",
			deps: map[string][]string{
				"a": {},
				"b": {"a"},
				"c": {"b"},
			},
			wantWaves: [][]string{
				{"a"},
				{"b"},
				{"c"},
			},
		},
		{
			name: "diamond dependencies",
			deps: map[string][]string{
				"a": {},
				"b": {},
				"c": {"a", "b"},
			},
			wantWaves: [][]string{
				{"a", "b"},
				{"c"},
			},
		},
		{
			name: "cycle detection",
			deps: map[string][]string{
				"a": {"b"},
				"b": {"a"},
			},
			wantErr: true,
		},
		{
			name: "indirect cycle",
			deps: map[string][]string{
				"a": {"c"},
				"b": {"a"},
				"c": {"b"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildExecutionPlan(tt.deps)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildExecutionPlan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if !equalWaves(got.Waves, tt.wantWaves) {
				t.Errorf("buildExecutionPlan() waves = %v, want %v", got.Waves, tt.wantWaves)
			}
		})
	}
}

// Helper functions

func equalWaves(got, want [][]string) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		if !equalWave(got[i], want[i]) {
			return false
		}
	}

	return true
}

func equalWave(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}

	gotMap := make(map[string]bool)
	for _, step := range got {
		gotMap[step] = true
	}

	for _, step := range want {
		if !gotMap[step] {
			return false
		}
	}

	return true
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
