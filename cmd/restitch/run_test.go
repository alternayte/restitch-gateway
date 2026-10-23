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
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestGatewayCmd_MutualExclusivity(t *testing.T) {
	code := gatewayCmd([]string{"-config", "x.yaml", "-registry-url", "http://example.com"})
	if code != 2 {
		t.Errorf("expected exit 2 for mutual exclusivity, got %d", code)
	}
}

func TestGatewayFlagOverEnvOverDefault(t *testing.T) {
	env := map[string]string{
		"RESTITCH_PORT":      "9001",
		"RESTITCH_LOG_LEVEL": "debug",
		"RESTITCH_CONFIG":    "/etc/restitch/artifact.yaml",
	}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }

	opts, err := parseGatewayFlags([]string{"-port", "9002"}, lookup)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.port != 9002 {
		t.Errorf("port: flag must beat env, got %d", opts.port)
	}
	if opts.logLevel != "debug" {
		t.Errorf("log-level: env must beat default, got %q", opts.logLevel)
	}
	if opts.configFile != "/etc/restitch/artifact.yaml" {
		t.Errorf("config: RESTITCH_CONFIG must set the file, got %q", opts.configFile)
	}
	if opts.logFormat != "json" {
		t.Errorf("log-format: default must apply, got %q", opts.logFormat)
	}
	if !opts.set["port"] || !opts.set["log-level"] || opts.set["log-format"] {
		t.Errorf("set must hold flags given by flag or env only, got %v", opts.set)
	}
}

func TestGatewayEnvRejectsInvalidValue(t *testing.T) {
	lookup := func(k string) (string, bool) {
		if k == "RESTITCH_PORT" {
			return "eighty", true
		}
		return "", false
	}
	if _, err := parseGatewayFlags(nil, lookup); err == nil {
		t.Fatal("expected an error for RESTITCH_PORT=eighty")
	}
}

// TestHelperGatewayProcess runs gatewayCmd when the parent test starts the
// test binary as a child process; it does nothing in a normal run.
func TestHelperGatewayProcess(t *testing.T) {
	if os.Getenv("RESTITCH_HELPER_GATEWAY") != "1" {
		t.Skip("helper process only")
	}
	os.Exit(gatewayCmd(nil))
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestGatewayOpensNoAdminListener(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "artifact.yaml")
	if err := os.WriteFile(cfgPath, []byte(`
upstreams:
  api:
    url: http://127.0.0.1:1
compositions:
  hello:
    path: /hello
    public: true
    steps: [{name: s, upstream: api, path: /x, optional: true}]
    response: {body: {ok: true}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	port, opsPort := freePort(t), freePort(t)

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperGatewayProcess$")
	cmd.Env = append(os.Environ(),
		"RESTITCH_HELPER_GATEWAY=1",
		"RESTITCH_CONFIG="+cfgPath,
		"RESTITCH_PORT="+strconv.Itoa(port),
		"RESTITCH_OPS_PORT="+strconv.Itoa(opsPort),
		"RESTITCH_LOG_LEVEL=error",
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
	})

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	ops := "http://127.0.0.1:" + strconv.Itoa(opsPort)
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(base + "/ready")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("gateway never became ready")
		}
		time.Sleep(50 * time.Millisecond)
	}

	status := func(url string) int {
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := status(base + "/health"); got != http.StatusOK {
		t.Errorf("/health on the data port: %d", got)
	}
	if got := status(ops + "/metrics"); got != http.StatusOK {
		t.Errorf("/metrics on the ops port: %d", got)
	}
	for _, url := range []string{base + "/admin/api/info", ops + "/admin/api/info", ops + "/health"} {
		if got := status(url); got != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", url, got)
		}
	}
}
