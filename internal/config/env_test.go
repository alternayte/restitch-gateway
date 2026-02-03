package config

import (
	"os"
	"strings"
	"testing"
)

func TestExpandEnvWithValidation(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range originalEnv {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	tests := []struct {
		name      string
		input     string
		envSetup  map[string]string
		want      string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "valid single variable",
			input:    "${FOO}",
			envSetup: map[string]string{"FOO": "bar"},
			want:     "bar",
			wantErr:  false,
		},
		{
			name:      "missing variable",
			input:     "${MISSING}",
			envSetup:  map[string]string{},
			want:      "",
			wantErr:   true,
			errSubstr: "environment variable MISSING is not set",
		},
		{
			name:      "empty variable",
			input:     "${EMPTY}",
			envSetup:  map[string]string{"EMPTY": ""},
			want:      "",
			wantErr:   true,
			errSubstr: "environment variable EMPTY is empty",
		},
		{
			name:     "multiple variables",
			input:    "${A}/${B}",
			envSetup: map[string]string{"A": "x", "B": "y"},
			want:     "x/y",
			wantErr:  false,
		},
		{
			name:     "no variables (literal string)",
			input:    "literal",
			envSetup: map[string]string{},
			want:     "literal",
			wantErr:  false,
		},
		{
			name:     "mixed literal and variable",
			input:    "prefix-${VAR}-suffix",
			envSetup: map[string]string{"VAR": "value"},
			want:     "prefix-value-suffix",
			wantErr:  false,
		},
		{
			name:     "variable with underscores and numbers",
			input:    "${MY_VAR_123}",
			envSetup: map[string]string{"MY_VAR_123": "test"},
			want:     "test",
			wantErr:  false,
		},
		{
			name:      "first variable missing among multiple",
			input:     "${MISSING}/${EXISTS}",
			envSetup:  map[string]string{"EXISTS": "val"},
			want:      "",
			wantErr:   true,
			errSubstr: "environment variable MISSING is not set",
		},
		{
			name:      "second variable missing among multiple",
			input:     "${EXISTS}/${MISSING}",
			envSetup:  map[string]string{"EXISTS": "val"},
			want:      "",
			wantErr:   true,
			errSubstr: "environment variable MISSING is not set",
		},
		{
			name:     "same variable used multiple times",
			input:    "${VAR}-${VAR}",
			envSetup: map[string]string{"VAR": "dup"},
			want:     "dup-dup",
			wantErr:  false,
		},
		{
			name:     "empty input string",
			input:    "",
			envSetup: map[string]string{},
			want:     "",
			wantErr:  false,
		},
		{
			name:     "URL with variable",
			input:    "https://api.example.com?key=${API_KEY}",
			envSetup: map[string]string{"API_KEY": "secret123"},
			want:     "https://api.example.com?key=secret123",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear env and set up test-specific variables
			os.Clearenv()
			for k, v := range tt.envSetup {
				os.Setenv(k, v)
			}

			got, err := ExpandEnvWithValidation(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ExpandEnvWithValidation() expected error, got nil")
					return
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("ExpandEnvWithValidation() error = %v, want substring %q", err, tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Errorf("ExpandEnvWithValidation() unexpected error: %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("ExpandEnvWithValidation() = %q, want %q", got, tt.want)
			}
		})
	}
}
