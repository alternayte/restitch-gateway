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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alternayte/restitch-gateway/v2/internal/server"
)

const validateUpstreams = `
upstreams:
  api:
    url: http://localhost:8081
`

func TestValidateRejectsNonPublicWithoutAuthPolicy(t *testing.T) {
	err := Validate(context.Background(), []byte(validateUpstreams+`
compositions:
  orders:
    path: /orders
    steps: [{name: s, upstream: api, path: /orders}]
    response: {body: "{{ steps.s.body }}"}
`))
	if err == nil || !strings.Contains(err.Error(), `composition orders is not public and the artifact has no auth policy`) {
		t.Fatalf("expected the fail-open route to be refused, got %v", err)
	}
}

func TestValidateAcceptsPublicOrAuthenticatedRoutes(t *testing.T) {
	for name, doc := range map[string]string{
		"public": validateUpstreams + `
compositions:
  orders:
    path: /orders
    public: true
    steps: [{name: s, upstream: api, path: /orders}]
    response: {body: "{{ steps.s.body }}"}
`,
		"auth policy": `
auth:
  api_keys: [k]
` + validateUpstreams + `
compositions:
  orders:
    path: /orders
    steps: [{name: s, upstream: api, path: /orders}]
    response: {body: "{{ steps.s.body }}"}
`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := Validate(context.Background(), []byte(doc)); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestValidateRejectsReservedProbePaths(t *testing.T) {
	for _, p := range []string{"/health", "/ready"} {
		err := Validate(context.Background(), []byte(validateUpstreams+`
compositions:
  probe:
    path: `+p+`
    public: true
    steps: [{name: s, upstream: api, path: /x}]
    response: {body: "{{ steps.s.body }}"}
`))
		if err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("%s: expected a reserved-path error, got %v", p, err)
		}
	}
}

func TestValidateRejectsUnknownNestedKey(t *testing.T) {
	err := Validate(context.Background(), []byte(`
upstreams:
  api:
    url: http://localhost:8081
    timeout_ms: 500
`))
	if err == nil || !strings.Contains(err.Error(), "timeout_ms") {
		t.Fatalf("expected the unknown key to be refused, got %v", err)
	}
}

func TestValidateRejectsV1Blocks(t *testing.T) {
	err := Validate(context.Background(), []byte("server:\n  port: 1\n"+validateUpstreams))
	if err == nil || !strings.Contains(err.Error(), "server.port moved to RESTITCH_PORT") {
		t.Fatalf("expected the v1 block to be refused, got %v", err)
	}
}

func TestNonPublicRouteWithoutAuthenticatorFailsClosed(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
auth:
  api_keys: [k]
` + validateUpstreams + `
compositions:
  orders:
    path: /orders
    steps: [{name: s, upstream: api, path: /orders}]
    response: {body: "{{ steps.s.body }}"}
`))
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileConfig(context.Background(), cfg, CompileOptions{SkipAuthInit: true})
	if err != nil {
		t.Fatal(err)
	}
	router := server.NewRouter()
	NewHandler(compiled, nil).RegisterRoutes(router)
	router.Finalize()

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/orders", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("a non-public route with no authenticator must fail closed, got %d", rec.Code)
	}
}

func TestValidateRejectsUnknownReferences(t *testing.T) {
	base := `
upstreams:
  a:
    url: http://localhost:8081
compositions:
  c:
    path: /c/{id}
    public: true
    steps:
      - name: s
        upstream: a
        path: "%s"
    response: {body: "{{ steps.s.body }}"}
`
	for expr, want := range map[string]string{
		"/u/{{ req.path.id }}":      "req.path is a string",
		"/u/{{ request.method.x }}": "request.method is a string",
		"/u/{{ req.nope }}":         `req has no field "nope"`,
		"/u/{{ foo }}":              `unknown name "foo"`,
	} {
		err := Validate(context.Background(), []byte(fmt.Sprintf(base, expr)))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want an error containing %q", expr, err, want)
		}
	}
	resp := strings.Replace(base, `response: {body: "{{ steps.s.body }}"}`, `response: {body: "{{ steps.s.status.code }} {{ steps.s.bogus }}"}`, 1)
	err := Validate(context.Background(), []byte(fmt.Sprintf(resp, "/u")))
	if err == nil || !strings.Contains(err.Error(), "steps.s.status is a number") {
		t.Errorf("steps.s.status.code: got %v", err)
	}
	for _, ok := range []string{
		"/u/{{ req.params.id }}",
		"/u/{{ request.query.page }}",
		"/u/{{ len(req.headers) }}",
		"/u/{{ upper(req.method) }}",
		"/u/{{ req.body.items[0].id }}",
	} {
		if err := Validate(context.Background(), []byte(fmt.Sprintf(base, ok))); err != nil {
			t.Errorf("%s: unexpected error %v", ok, err)
		}
	}
}
