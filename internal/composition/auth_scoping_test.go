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

package composition

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/alternayte/restitch-gateway/internal/server"
)

func TestHandler_AuthorizationScoping_D8(t *testing.T) {
	var mu sync.Mutex
	passthroughHeaders := make(http.Header)
	noAuthHeaders := make(http.Header)

	passthroughUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		passthroughHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"from": "passthrough"})
	}))
	defer passthroughUpstream.Close()

	noAuthUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		noAuthHeaders = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"from": "noauth"})
	}))
	defer noAuthUpstream.Close()

	configYAML := `
upstreams:
  passthrough-up:
    url: ` + passthroughUpstream.URL + `
    auth:
      passthrough: {}
  noauth-up:
    url: ` + noAuthUpstream.URL + `

compositions:
  scoped:
    path: /scoped
    method: GET
    steps:
      - name: a
        upstream: passthrough-up
        path: /data
      - name: b
        upstream: noauth-up
        path: /data
    response:
      body:
        a: "{{ steps.a.body }}"
        b: "{{ steps.b.body }}"
`

	cfg, err := ParseConfig([]byte(configYAML))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	compiled, err := CompileConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CompileConfig: %v", err)
	}

	handler := NewHandler(compiled, nil)
	router := server.NewRouter()
	handler.RegisterRoutes(router)
	router.Finalize()

	req := httptest.NewRequest("GET", "/scoped", nil)
	req.Header.Set("Authorization", "Bearer SECRET")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()

	if got := passthroughHeaders.Get("Authorization"); got != "Bearer SECRET" {
		t.Errorf("passthrough upstream should receive Authorization header, got %q", got)
	}

	if got := noAuthHeaders.Get("Authorization"); got != "" {
		t.Errorf("no-auth upstream should NOT receive Authorization header, got %q", got)
	}
}
