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
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestHelperDevProcess runs devCmd when the parent test starts the test
// binary as a child process.
func TestHelperDevProcess(t *testing.T) {
	if os.Getenv("RESTITCH_HELPER_DEV") != "1" {
		t.Skip("helper process only")
	}
	os.Exit(devCmd([]string{"-port", os.Getenv("DEV_PORT"), "-studio-port", os.Getenv("DEV_STUDIO_PORT"), "-data-dir", os.Getenv("DEV_DATA")}))
}

func TestDevServesPublishedComposition(t *testing.T) {
	upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hello":"dev"}`))
	}))
	defer upstreamSrv.Close()

	port, studioPort := freePort(t), freePort(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperDevProcess$")
	cmd.Env = append(os.Environ(), "RESTITCH_HELPER_DEV=1",
		"DEV_PORT="+strconv.Itoa(port), "DEV_STUDIO_PORT="+strconv.Itoa(studioPort), "DEV_DATA="+t.TempDir())
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Signal(syscall.SIGTERM); _ = cmd.Wait() })

	studioBase := "http://localhost:" + strconv.Itoa(studioPort)
	gw := "http://127.0.0.1:" + strconv.Itoa(port)
	waitStatus(t, gw+"/health", http.StatusOK, 10*time.Second)

	jar, _ := cookiejar.New(nil)
	sc := studioClient{t: t, base: studioBase, c: &http.Client{Jar: jar}}
	sc.call(http.MethodPost, "/api/auth/sign-in/email", map[string]string{"email": devEmail, "password": devPassword})
	sc.call(http.MethodPut, "/api/v1/environments/dev/resources/upstream/hello", map[string]any{
		"base_revision": 0, "body": map[string]any{"url": upstreamSrv.URL},
	})
	sc.call(http.MethodPut, "/api/v1/environments/dev/resources/composition/hello", map[string]any{
		"base_revision": 0, "body": map[string]any{
			"path": "/hello", "public": true,
			"steps":    []any{map[string]any{"name": "h", "upstream": "hello", "path": "/"}},
			"response": map[string]any{"body": map[string]any{"msg": "{{ steps.h.body.hello }}"}},
		},
	})
	sc.call(http.MethodPost, "/api/v1/environments/dev/publish", map[string]string{"message": "first"})

	waitStatus(t, gw+"/ready", http.StatusOK, 10*time.Second)
	resp, err := http.Get(gw + "/hello")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "dev") {
		t.Fatalf("GET /hello: %d %s", resp.StatusCode, body)
	}
}
