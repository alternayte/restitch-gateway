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
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPublishCommand(t *testing.T) {
	dir := t.TempDir()
	port := freePort(t)
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	_, stop := studioOn(t, filepath.Join(dir, "studio.db"), port)
	defer stop()

	jar, _ := cookiejar.New(nil)
	sc := studioClient{t: t, base: base, c: &http.Client{Jar: jar}}
	sc.call(http.MethodPost, "/api/auth/sign-in/email", map[string]string{"email": "owner@example.com", "password": "correct horse battery staple"})
	sc.call(http.MethodPost, "/api/v1/environments", map[string]string{"name": "prod"})

	body, _ := json.Marshal(map[string]string{"name": "ci"})
	req, _ := http.NewRequest(http.MethodPost, base+"/api/auth/api-keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", base)
	resp, err := sc.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		Plaintext string `json:"plaintext"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.Plaintext == "" {
		t.Fatalf("api key create: %d", resp.StatusCode)
	}

	env := map[string]string{"RESTITCH_STUDIO_URL": base, "RESTITCH_API_KEY": created.Plaintext}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	write := func(name, doc string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	bad := write("bad.yaml", "upstreams:\n  a:\n    url: https://a\ncompositions:\n  c:\n    path: /c\n    steps: [{name: s, upstream: a, path: /}]\n    response: {body: x}\n")
	var out, errOut bytes.Buffer
	if code := runPublish([]string{"-env", "prod", bad}, lookup, &out, &errOut); code != 1 ||
		!strings.Contains(errOut.String(), "no auth policy") {
		t.Fatalf("invalid file: exit %d, stderr %q", code, errOut.String())
	}

	good := write("good.yaml", "upstreams:\n  a:\n    url: https://a\ncompositions:\n  c:\n    path: /c\n    public: true\n    steps: [{name: s, upstream: a, path: /}]\n    response: {body: x}\n")
	out.Reset()
	errOut.Reset()
	if code := runPublish([]string{"-env", "prod", "-m", "from ci", good}, lookup, &out, &errOut); code != 0 ||
		!strings.Contains(out.String(), "Published prod version 1.") {
		t.Fatalf("valid file: exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
	versions := sc.call(http.MethodGet, "/api/v1/environments/prod/versions", nil)
	if raw, _ := json.Marshal(versions); !strings.Contains(string(raw), "from ci") {
		t.Fatalf("versions = %s", raw)
	}
}
