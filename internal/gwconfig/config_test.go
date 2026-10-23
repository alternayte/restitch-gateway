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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDuration_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"10s", 10 * time.Second, false},
		{"250ms", 250 * time.Millisecond, false},
		{"5m", 5 * time.Minute, false},
		{"1h30m", 90 * time.Minute, false},
		{"bad", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var d Duration
			err := yaml.Unmarshal([]byte(tt.input), &d)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d.Duration != tt.expected {
				t.Errorf("got %v, want %v", d.Duration, tt.expected)
			}
		})
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRejectsV1ServerBlock(t *testing.T) {
	path := writeConfig(t, `
server:
  port: 9000
  tls_cert: /tls/cert.pem
  shutdown_timeout: 20s
  auth:
    api_keys: ["k"]
admin:
  api_key: k
  storage:
    type: sqlite
upstreams:
  test:
    url: "http://localhost:8081"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected a v1 file to be refused")
	}
	msg := err.Error()
	for _, want := range []string{
		"server.port moved to RESTITCH_PORT",
		"server.tls_cert moved to RESTITCH_TLS_CERT",
		"server.shutdown_timeout moved to RESTITCH_SHUTDOWN_TIMEOUT",
		"server.auth moved to the top-level auth key",
		"admin.api_key removed in v2: the gateway has no admin API",
		"admin.storage removed in v2",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error lacks %q:\n%s", want, msg)
		}
	}
}

func TestLoadRejectsUnknownTopLevelKey(t *testing.T) {
	_, err := Load(writeConfig(t, "upstream:\n  a:\n    url: http://x\n"))
	if err == nil || !strings.Contains(err.Error(), `unknown top-level key "upstream"`) {
		t.Fatalf("expected an unknown-key error, got %v", err)
	}
}

func TestLoad_Defaults(t *testing.T) {
	yaml := `
upstreams:
  test:
    url: "http://localhost"
compositions:
  x:
    path: /x
    steps: []
    response:
      body: {}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(path, []byte(yaml), 0644)

	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if f.Server.Port != 8080 {
		t.Errorf("default port = %d, want 8080", f.Server.Port)
	}
	if f.Server.LogFormat != "json" {
		t.Errorf("default log_format = %s, want json", f.Server.LogFormat)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	f := &File{
		Server: ServerConfig{
			Port:      99999,
			LogFormat: "invalid",
			LogLevel:  "bad",
		},
	}

	err := f.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}

	s := err.Error()
	if !strings.Contains(s, "RESTITCH_PORT") {
		t.Error("should report the port error")
	}
	if !strings.Contains(s, "RESTITCH_LOG_FORMAT") {
		t.Error("should report the log format error")
	}
	if !strings.Contains(s, "RESTITCH_LOG_LEVEL") {
		t.Error("should report the log level error")
	}
}
