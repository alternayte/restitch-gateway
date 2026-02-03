package composition

import (
	"testing"
)

func TestExtractDependencies(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		want     []string
		wantErr  bool
	}{
		{
			name: "simple reference",
			expr: "steps.user.body",
			want: []string{"user"},
		},
		{
			name: "multiple references",
			expr: "steps.user.id + steps.orders.total",
			want: []string{"user", "orders"},
		},
		{
			name: "nested access",
			expr: "steps.user.body.profile.name",
			want: []string{"user"},
		},
		{
			name: "no references",
			expr: "req.query.id",
			want: []string{},
		},
		{
			name: "bracket notation",
			expr: `steps["user-service"].body`,
			want: []string{"user-service"},
		},
		{
			name: "mixed notation",
			expr: `steps.user.id + steps["order-service"].total`,
			want: []string{"user", "order-service"},
		},
		{
			name: "array access",
			expr: "steps.orders.body[0]",
			want: []string{"orders"},
		},
		{
			name: "complex expression",
			expr: "steps.user.body.id + steps.orders.body | filter(.status == 'active')",
			want: []string{"user", "orders"},
		},
		{
			name: "duplicate references",
			expr: "steps.user.id + steps.user.name",
			want: []string{"user"},
		},
		{
			name: "invalid syntax",
			expr: "steps.user + (",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractDependencies(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractDependencies() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Convert to map for comparison (order doesn't matter)
			gotMap := make(map[string]bool)
			for _, dep := range got {
				gotMap[dep] = true
			}
			wantMap := make(map[string]bool)
			for _, dep := range tt.want {
				wantMap[dep] = true
			}

			if len(gotMap) != len(wantMap) {
				t.Errorf("ExtractDependencies() = %v, want %v", got, tt.want)
				return
			}

			for dep := range wantMap {
				if !gotMap[dep] {
					t.Errorf("ExtractDependencies() missing dependency %q, got %v", dep, got)
				}
			}
		})
	}
}

func TestExtractAllDependencies(t *testing.T) {
	tests := []struct {
		name    string
		exprs   []string
		want    []string
		wantErr bool
	}{
		{
			name:  "multiple expressions",
			exprs: []string{"steps.user.body", "steps.orders.body", "steps.profile.body"},
			want:  []string{"user", "orders", "profile"},
		},
		{
			name:  "overlapping dependencies",
			exprs: []string{"steps.user.id", "steps.user.name + steps.orders.total"},
			want:  []string{"user", "orders"},
		},
		{
			name:  "empty list",
			exprs: []string{},
			want:  []string{},
		},
		{
			name:  "no dependencies",
			exprs: []string{"req.query.id", "req.headers.auth"},
			want:  []string{},
		},
		{
			name:    "invalid expression",
			exprs:   []string{"steps.user.id", "invalid ("},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractAllDependencies(tt.exprs...)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractAllDependencies() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Convert to map for comparison (order doesn't matter)
			gotMap := make(map[string]bool)
			for _, dep := range got {
				gotMap[dep] = true
			}
			wantMap := make(map[string]bool)
			for _, dep := range tt.want {
				wantMap[dep] = true
			}

			if len(gotMap) != len(wantMap) {
				t.Errorf("ExtractAllDependencies() = %v, want %v", got, tt.want)
				return
			}

			for dep := range wantMap {
				if !gotMap[dep] {
					t.Errorf("ExtractAllDependencies() missing dependency %q, got %v", dep, got)
				}
			}
		})
	}
}

func TestMergeDependencies(t *testing.T) {
	tests := []struct {
		name     string
		explicit []string
		inferred []string
		want     []string
	}{
		{
			name:     "both explicit and inferred",
			explicit: []string{"user"},
			inferred: []string{"orders"},
			want:     []string{"user", "orders"},
		},
		{
			name:     "overlapping dependencies",
			explicit: []string{"user", "orders"},
			inferred: []string{"orders", "profile"},
			want:     []string{"user", "orders", "profile"},
		},
		{
			name:     "only explicit",
			explicit: []string{"user", "orders"},
			inferred: []string{},
			want:     []string{"user", "orders"},
		},
		{
			name:     "only inferred",
			explicit: []string{},
			inferred: []string{"user", "orders"},
			want:     []string{"user", "orders"},
		},
		{
			name:     "empty both",
			explicit: []string{},
			inferred: []string{},
			want:     []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeDependencies(tt.explicit, tt.inferred)

			// Convert to map for comparison (order doesn't matter)
			gotMap := make(map[string]bool)
			for _, dep := range got {
				gotMap[dep] = true
			}
			wantMap := make(map[string]bool)
			for _, dep := range tt.want {
				wantMap[dep] = true
			}

			if len(gotMap) != len(wantMap) {
				t.Errorf("MergeDependencies() = %v, want %v", got, tt.want)
				return
			}

			for dep := range wantMap {
				if !gotMap[dep] {
					t.Errorf("MergeDependencies() missing dependency %q, got %v", dep, got)
				}
			}
		})
	}
}
