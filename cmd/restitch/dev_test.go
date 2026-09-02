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

package main

import (
	"reflect"
	"testing"
)

func TestBuildGatewayArgs(t *testing.T) {
	tests := []struct {
		name       string
		configFile string
		logFormat  string
		extra      string
		port       int
		want       []string
	}{
		{
			name:       "empty extra string yields exactly the fixed args",
			configFile: "restitch.yaml",
			logFormat:  "text",
			extra:      "",
			port:       8080,
			want: []string{
				"run",
				"--config=restitch.yaml",
				"--log-format=text",
				"--port=8080",
			},
		},
		{
			name:       "whitespace-only extra string yields exactly the fixed args",
			configFile: "restitch.yaml",
			logFormat:  "text",
			extra:      "   ",
			port:       8080,
			want: []string{
				"run",
				"--config=restitch.yaml",
				"--log-format=text",
				"--port=8080",
			},
		},
		{
			name:       "single extra flag appended once, last",
			configFile: "restitch.yaml",
			logFormat:  "text",
			extra:      "--verbose",
			port:       8080,
			want: []string{
				"run",
				"--config=restitch.yaml",
				"--log-format=text",
				"--port=8080",
				"--verbose",
			},
		},
		{
			name:       "multiple extra flags appended in order, last",
			configFile: "restitch.yaml",
			logFormat:  "text",
			extra:      "--verbose --trace-sample=1.0",
			port:       8080,
			want: []string{
				"run",
				"--config=restitch.yaml",
				"--log-format=text",
				"--port=8080",
				"--verbose",
				"--trace-sample=1.0",
			},
		},
		{
			name:       "extra flag duplicating a fixed one appears after it",
			configFile: "restitch.yaml",
			logFormat:  "text",
			extra:      "--port=9999",
			port:       8080,
			want: []string{
				"run",
				"--config=restitch.yaml",
				"--log-format=text",
				"--port=8080",
				"--port=9999",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGatewayArgs(tt.configFile, tt.logFormat, tt.extra, tt.port)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildGatewayArgs(%q, %q, %q, %d) = %#v, want %#v",
					tt.configFile, tt.logFormat, tt.extra, tt.port, got, tt.want)
			}
		})
	}
}

func TestBuildStudioArgs(t *testing.T) {
	tests := []struct {
		name       string
		extra      string
		studioPort int
		adminPort  int
		want       []string
	}{
		{
			name:       "empty extra string yields exactly the fixed args",
			extra:      "",
			studioPort: 3080,
			adminPort:  9090,
			want: []string{
				"--port=3080",
				"--gateway-admin-url=http://localhost:9090",
			},
		},
		{
			name:       "whitespace-only extra string yields exactly the fixed args",
			extra:      "   ",
			studioPort: 3080,
			adminPort:  9090,
			want: []string{
				"--port=3080",
				"--gateway-admin-url=http://localhost:9090",
			},
		},
		{
			name:       "single extra flag appended once, last",
			extra:      "--verbose",
			studioPort: 3080,
			adminPort:  9090,
			want: []string{
				"--port=3080",
				"--gateway-admin-url=http://localhost:9090",
				"--verbose",
			},
		},
		{
			name:       "multiple extra flags appended in order, last",
			extra:      "--verbose --theme=dark",
			studioPort: 3080,
			adminPort:  9090,
			want: []string{
				"--port=3080",
				"--gateway-admin-url=http://localhost:9090",
				"--verbose",
				"--theme=dark",
			},
		},
		{
			name:       "extra flag duplicating a fixed one appears after it",
			extra:      "--port=4000",
			studioPort: 3080,
			adminPort:  9090,
			want: []string{
				"--port=3080",
				"--gateway-admin-url=http://localhost:9090",
				"--port=4000",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildStudioArgs(tt.extra, tt.studioPort, tt.adminPort)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildStudioArgs(%q, %d, %d) = %#v, want %#v",
					tt.extra, tt.studioPort, tt.adminPort, got, tt.want)
			}
		})
	}
}
