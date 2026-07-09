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

func TestLoad_FullSchema(t *testing.T) {
	yaml := `
server:
  port: 9000
  log_format: text
  log_level: debug
  read_timeout: 15s
  write_timeout: 45s
  shutdown_timeout: 20s
admin:
  port: 9091
  api_key: testkey
  request_log_size: 200
upstreams:
  test:
    url: "http://localhost:8081"
compositions:
  hello:
    path: /hello
    steps: []
    response:
      status: 200
      body: {}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if f.Server.Port != 9000 {
		t.Errorf("server.port = %d, want 9000", f.Server.Port)
	}
	if f.Server.LogFormat != "text" {
		t.Errorf("log_format = %s, want text", f.Server.LogFormat)
	}
	if f.Server.ReadTimeout.Duration != 15*time.Second {
		t.Errorf("read_timeout = %v, want 15s", f.Server.ReadTimeout.Duration)
	}
	if f.Admin.Port != 9091 {
		t.Errorf("admin.port = %d, want 9091", f.Admin.Port)
	}
	if f.Admin.APIKey != "testkey" {
		t.Errorf("admin.api_key = %s, want testkey", f.Admin.APIKey)
	}
	if f.Hash() == "" {
		t.Error("hash should not be empty")
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
	os.WriteFile(path, []byte(yaml), 0644)

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
	if f.Admin.Port != 9090 {
		t.Errorf("default admin.port = %d, want 9090", f.Admin.Port)
	}
	if f.Admin.RequestLogSize != 500 {
		t.Errorf("default request_log_size = %d, want 500", f.Admin.RequestLogSize)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	f := &File{
		Server: ServerConfig{
			Port:      99999,
			LogFormat: "invalid",
			LogLevel:  "bad",
		},
		Admin: AdminConfig{
			Port: -1,
		},
	}

	err := f.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}

	s := err.Error()
	if !strings.Contains(s, "server.port") {
		t.Error("should report server.port error")
	}
	if !strings.Contains(s, "log_format") {
		t.Error("should report log_format error")
	}
	if !strings.Contains(s, "log_level") {
		t.Error("should report log_level error")
	}
	if !strings.Contains(s, "admin.port") {
		t.Error("should report admin.port error")
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	f := &File{}
	applyDefaults(f)

	t.Setenv("RESTITCH_PORT", "9999")
	t.Setenv("RESTITCH_LOG_FORMAT", "text")
	t.Setenv("RESTITCH_ADMIN_PORT", "9191")

	ApplyEnvOverrides(f)

	if f.Server.Port != 9999 {
		t.Errorf("port = %d, want 9999", f.Server.Port)
	}
	if f.Server.LogFormat != "text" {
		t.Errorf("log_format = %s, want text", f.Server.LogFormat)
	}
	if f.Admin.Port != 9191 {
		t.Errorf("admin.port = %d, want 9191", f.Admin.Port)
	}
}

func TestAdminConfig_IsEnabled(t *testing.T) {
	a := AdminConfig{}
	if !a.IsEnabled() {
		t.Error("default should be enabled")
	}

	f := false
	a.Enabled = &f
	if a.IsEnabled() {
		t.Error("should be disabled")
	}
}
