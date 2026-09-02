// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gwconfig

import (
	"testing"
)

func TestExpandEnvStrict(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{
			name:  "literal dollar escape",
			input: "pa$$word",
			want:  "pa$word",
		},
		{
			name:    "bare dollar is error",
			input:   "pa$sword",
			wantErr: true,
		},
		{
			name:    "missing var no default",
			input:   "${MISSING_VAR_XYZ}",
			wantErr: true,
		},
		{
			name:  "missing var with default",
			input: "${MISSING_VAR_XYZ:fallback}",
			want:  "fallback",
		},
		{
			name:  "set var",
			input: "${GWCONFIG_TEST_VAR}",
			env:   map[string]string{"GWCONFIG_TEST_VAR": "hello"},
			want:  "hello",
		},
		{
			name:  "set var with default ignored",
			input: "${GWCONFIG_TEST_VAR:unused}",
			env:   map[string]string{"GWCONFIG_TEST_VAR": "hello"},
			want:  "hello",
		},
		{
			name:  "no expansion needed",
			input: "plain text",
			want:  "plain text",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "multiple vars",
			input: "${GWCONFIG_A}:${GWCONFIG_B}",
			env:   map[string]string{"GWCONFIG_A": "x", "GWCONFIG_B": "y"},
			want:  "x:y",
		},
		{
			name:    "unclosed brace",
			input:   "${OOPS",
			wantErr: true,
		},
		{
			name:    "dollar at end",
			input:   "end$",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, err := ExpandEnvStrict(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
