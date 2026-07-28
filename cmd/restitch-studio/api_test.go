package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (registers as "sqlite")

	"github.com/restitch/restitch-gateway/internal/registry"
)

const sampleYAML = `
upstreams:
  mock:
    url: "http://localhost:8081"
compositions:
  test-comp:
    path: "/test"
    method: GET
    steps:
      - name: s1
        upstream: mock
        path: "/users/1"
    response:
      body:
        result: "{{ steps.s1.body }}"
`

const sampleYAML2 = `
upstreams:
  mock2:
    url: "http://localhost:8082"
compositions:
  test-comp-2:
    path: "/test2"
    method: GET
    steps:
      - name: s1
        upstream: mock2
        path: "/users/2"
    response:
      body:
        result: "{{ steps.s1.body }}"
`

const invalidYAML = `
upstreams:
  mock
    url: "http://localhost:8081"
compositions: [this is not valid
`

func testMux(t *testing.T) *http.ServeMux {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if err := registry.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	store := registry.NewStore(db)
	api := NewRegistryAPI(store)
	return buildMux(muxDeps{gatewayAdminURL: "http://localhost:9999", registryAPI: api})
}

func doJSON(t *testing.T, mux *http.ServeMux, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func createTestConfig(t *testing.T, mux *http.ServeMux, name, yamlContent string) map[string]any {
	t.Helper()
	rec := doJSON(t, mux, "POST", "/api/v1/configs", map[string]any{
		"name":         name,
		"yaml_content": yamlContent,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create config: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAPI_CreateConfig_201(t *testing.T) {
	mux := testMux(t)

	rec := doJSON(t, mux, "POST", "/api/v1/configs", map[string]any{
		"name":         "test-config",
		"yaml_content": sampleYAML,
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["id"] == nil || got["id"] == "" {
		t.Errorf("expected non-empty id, got %v", got["id"])
	}
	if vn, ok := got["version_number"].(float64); !ok || vn != 1 {
		t.Errorf("version_number = %v, want 1", got["version_number"])
	}
}

func TestAPI_CreateConfig_422(t *testing.T) {
	mux := testMux(t)

	rec := doJSON(t, mux, "POST", "/api/v1/configs", map[string]any{
		"name":         "bad-config",
		"yaml_content": invalidYAML,
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", rec.Code, rec.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if valid, ok := got["valid"].(bool); !ok || valid {
		t.Errorf("valid = %v, want false", got["valid"])
	}
}

func TestAPI_GetConfig_200(t *testing.T) {
	mux := testMux(t)
	created := createTestConfig(t, mux, "test-config", sampleYAML)
	id := created["id"].(string)

	rec := doJSON(t, mux, "GET", "/api/v1/configs/"+id, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	yamlContent, _ := got["yaml_content"].(string)
	if yamlContent == "" {
		t.Error("expected non-empty yaml_content")
	}
}

func TestAPI_GetConfig_404(t *testing.T) {
	mux := testMux(t)

	rec := doJSON(t, mux, "GET", "/api/v1/configs/nonexistent-id", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAPI_ListConfigs(t *testing.T) {
	mux := testMux(t)
	createTestConfig(t, mux, "config-1", sampleYAML)
	createTestConfig(t, mux, "config-2", sampleYAMLWithComp("config-2-comp"))

	rec := doJSON(t, mux, "GET", "/api/v1/configs", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var got struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(got.Items))
	}
}

func TestAPI_UpdateContent(t *testing.T) {
	mux := testMux(t)
	created := createTestConfig(t, mux, "test-config", sampleYAML)
	id := created["id"].(string)

	rec := doJSON(t, mux, "PUT", "/api/v1/configs/"+id, map[string]any{
		"yaml_content": sampleYAMLWithComp("test-comp-v2"),
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if vn, ok := got["version_number"].(float64); !ok || vn != 2 {
		t.Errorf("version_number = %v, want 2", got["version_number"])
	}
}

func TestAPI_DeleteConfig(t *testing.T) {
	mux := testMux(t)
	created := createTestConfig(t, mux, "test-config", sampleYAML)
	id := created["id"].(string)

	rec := doJSON(t, mux, "DELETE", "/api/v1/configs/"+id, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}

	rec2 := doJSON(t, mux, "GET", "/api/v1/configs/"+id, nil)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", rec2.Code)
	}
}

func TestAPI_ValidateConfig_Valid(t *testing.T) {
	mux := testMux(t)

	rec := doJSON(t, mux, "POST", "/api/v1/configs/validate", map[string]any{
		"yaml_content": sampleYAML,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if valid, ok := got["valid"].(bool); !ok || !valid {
		t.Errorf("valid = %v, want true", got["valid"])
	}
}

func TestAPI_ValidateConfig_Invalid(t *testing.T) {
	mux := testMux(t)

	rec := doJSON(t, mux, "POST", "/api/v1/configs/validate", map[string]any{
		"yaml_content": invalidYAML,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if valid, ok := got["valid"].(bool); !ok || valid {
		t.Errorf("valid = %v, want false", got["valid"])
	}
}

func TestAPI_Bundle(t *testing.T) {
	mux := testMux(t)
	createTestConfig(t, mux, "config-1", sampleYAML)
	createTestConfig(t, mux, "config-2", sampleYAML2)

	rec := doJSON(t, mux, "GET", "/api/v1/registry/bundle", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Error("expected non-empty ETag header")
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	yamlContent, _ := got["yaml_content"].(string)
	if yamlContent == "" {
		t.Error("expected non-empty merged yaml_content")
	}

	// Conditional request with matching If-None-Match -> 304.
	req := httptest.NewRequest("GET", "/api/v1/registry/bundle", nil)
	req.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusNotModified {
		t.Errorf("conditional request status = %d, want 304", rec2.Code)
	}
}

func TestAPI_ListVersions(t *testing.T) {
	mux := testMux(t)
	created := createTestConfig(t, mux, "test-config", sampleYAML)
	id := created["id"].(string)

	doJSON(t, mux, "PUT", "/api/v1/configs/"+id, map[string]any{
		"yaml_content": sampleYAMLWithComp("test-comp-v2"),
	})
	doJSON(t, mux, "PUT", "/api/v1/configs/"+id, map[string]any{
		"yaml_content": sampleYAMLWithComp("test-comp-v3"),
	})

	rec := doJSON(t, mux, "GET", "/api/v1/configs/"+id+"/versions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	var got struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 3 {
		t.Errorf("len(items) = %d, want 3", len(got.Items))
	}
}

func TestAPI_ActivateVersion(t *testing.T) {
	mux := testMux(t)
	created := createTestConfig(t, mux, "test-config", sampleYAML)
	id := created["id"].(string)

	doJSON(t, mux, "PUT", "/api/v1/configs/"+id, map[string]any{
		"yaml_content": sampleYAMLWithComp("test-comp-v2"),
	})

	rec := doJSON(t, mux, "POST", "/api/v1/configs/"+id+"/versions/1/activate", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	rec2 := doJSON(t, mux, "GET", "/api/v1/configs/"+id, nil)
	var got map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	yamlContent, _ := got["yaml_content"].(string)
	if !bytes.Contains([]byte(yamlContent), []byte("test-comp")) {
		t.Errorf("expected v1 content to be active, got %q", yamlContent)
	}
	if vn, ok := got["version_number"].(float64); !ok || vn != 1 {
		t.Errorf("version_number = %v, want 1", got["version_number"])
	}
}

// sampleYAMLWithComp returns a valid config YAML with a distinct
// composition name, used to avoid upstream/composition name collisions
// when creating multiple configs in the same test.
func sampleYAMLWithComp(compName string) string {
	return `
upstreams:
  mock:
    url: "http://localhost:8081"
compositions:
  ` + compName + `:
    path: "/test"
    method: GET
    steps:
      - name: s1
        upstream: mock
        path: "/users/1"
    response:
      body:
        result: "{{ steps.s1.body }}"
`
}
