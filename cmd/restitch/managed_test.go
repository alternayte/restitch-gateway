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
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alternayte/restitch-gateway/v2/internal/studio"
)

// studioOn serves a Studio on a fixed port, so a gateway started before it
// reaches it once it is up.
func studioOn(t *testing.T, dbPath string, port int) (*studio.Server, func()) {
	t.Helper()
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	s, err := studio.New(context.Background(), studio.Config{
		DatabaseURL: "sqlite:" + dbPath, PublicURL: base,
		BootstrapEmail: "owner@example.com", BootstrapPassword: "correct horse battery staple",
	})
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	hs := &http.Server{Handler: s.Handler()}
	go func() { _ = hs.Serve(l) }()
	return s, func() { _ = hs.Close(); _ = s.Close() }
}

type studioClient struct {
	t    *testing.T
	base string
	c    *http.Client
}

func (sc studioClient) call(method, path string, body any) map[string]any {
	sc.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, sc.base+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", sc.base)
	resp, err := sc.c.Do(req)
	if err != nil {
		sc.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		sc.t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, data)
	}
	out := map[string]any{}
	_ = json.Unmarshal(data, &out)
	return out
}

func waitStatus(t *testing.T, url string, want int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	last := 0
	for time.Now().Before(deadline) {
		if resp, err := http.Get(url); err == nil {
			last = resp.StatusCode
			resp.Body.Close()
			if last == want {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("GET %s: last status %d, want %d", url, last, want)
}

func TestManagedGatewaySurvivesStudioOutage(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "studio.db")
	studioPort, gwPort := freePort(t), freePort(t)
	studioBase := "http://127.0.0.1:" + strconv.Itoa(studioPort)

	upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"ada"}`))
	}))
	defer upstreamSrv.Close()

	// Set up the environment and its token, then stop Studio.
	_, stop := studioOn(t, dbPath, studioPort)
	jar, _ := cookiejar.New(nil)
	sc := studioClient{t: t, base: studioBase, c: &http.Client{Jar: jar}}
	sc.call(http.MethodPost, "/api/auth/sign-in/email", map[string]string{"email": "owner@example.com", "password": "correct horse battery staple"})
	sc.call(http.MethodPost, "/api/v1/environments", map[string]string{"name": "prod"})
	token := sc.call(http.MethodPost, "/api/v1/environments/prod/gateway-tokens", map[string]string{"name": "gw"})["token"].(string)
	stop()

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperGatewayProcess$")
	cmd.Env = append(os.Environ(),
		"RESTITCH_HELPER_GATEWAY=1",
		"RESTITCH_STUDIO_URL="+studioBase,
		"RESTITCH_GATEWAY_TOKEN="+token,
		"RESTITCH_POLL_INTERVAL=100ms",
		"RESTITCH_PORT="+strconv.Itoa(gwPort),
		"RESTITCH_LOG_LEVEL=error",
	)
	var logs bytes.Buffer
	cmd.Stderr = &logs
	cmd.Stdout = &logs
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("gateway log:\n%s", logs.String())
		}
	})
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Signal(syscall.SIGTERM); _ = cmd.Wait() })

	gw := "http://127.0.0.1:" + strconv.Itoa(gwPort)
	waitStatus(t, gw+"/health", http.StatusOK, 10*time.Second)
	waitStatus(t, gw+"/ready", http.StatusServiceUnavailable, time.Second)

	// Studio comes back and an editor publishes the first version.
	_, stop = studioOn(t, dbPath, studioPort)
	defer stop()
	sc.call(http.MethodPut, "/api/v1/environments/prod/resources/upstream/users", map[string]any{
		"base_revision": 0, "body": map[string]any{"url": upstreamSrv.URL},
	})
	sc.call(http.MethodPut, "/api/v1/environments/prod/resources/composition/user", map[string]any{
		"base_revision": 0, "body": map[string]any{
			"path": "/user", "public": true,
			"steps":    []any{map[string]any{"name": "u", "upstream": "users", "path": "/u"}},
			"response": map[string]any{"body": map[string]any{"name": "{{ steps.u.body.name }}"}},
		},
	})
	sc.call(http.MethodPost, "/api/v1/environments/prod/publish", map[string]string{"message": "first"})

	waitStatus(t, gw+"/ready", http.StatusOK, 10*time.Second)
	resp, err := http.Get(gw + "/user")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "ada") {
		t.Fatalf("GET /user: %d %s", resp.StatusCode, body)
	}
}
