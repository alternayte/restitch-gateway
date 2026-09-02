//go:build e2e

package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alternayte/restitch-gateway/internal/mockupstream"
)

type Spec struct {
	Name string  `json:"name"`
	In   SpecIn  `json:"in"`
	Out  SpecOut `json:"out"`
}

type SpecIn struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
}

type SpecOut struct {
	Status       int               `json:"status"`
	Headers      map[string]string `json:"headers"`
	BodyContains []string          `json:"body_contains"`
}

func TestSpecs(t *testing.T) {
	mock := httptest.NewServer(mockupstream.Handler())
	defer mock.Close()

	specFiles, err := filepath.Glob("specs/*.json")
	if err != nil {
		t.Fatalf("glob specs: %v", err)
	}
	if len(specFiles) == 0 {
		t.Fatal("no spec files found in specs/")
	}

	configBytes, err := os.ReadFile("fixtures/gateway.yaml")
	if err != nil {
		t.Fatalf("read fixture config: %v", err)
	}
	configStr := strings.ReplaceAll(string(configBytes), "${MOCK_URL}", mock.URL)

	handler := buildHandler(t, []byte(configStr))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	for _, specFile := range specFiles {
		raw, err := os.ReadFile(specFile)
		if err != nil {
			t.Fatalf("read spec %s: %v", specFile, err)
		}

		var spec Spec
		if err := json.Unmarshal(raw, &spec); err != nil {
			t.Fatalf("parse spec %s: %v", specFile, err)
		}

		t.Run(spec.Name, func(t *testing.T) {
			req, err := http.NewRequest(spec.In.Method, srv.URL+spec.In.Path, nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			for k, v := range spec.In.Headers {
				req.Header.Set(k, v)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != spec.Out.Status {
				t.Errorf("status = %d, want %d (body: %s)", resp.StatusCode, spec.Out.Status, string(body))
			}

			for k, v := range spec.Out.Headers {
				got := resp.Header.Get(k)
				if got != v {
					t.Errorf("header %s = %q, want %q", k, got, v)
				}
			}

			bodyStr := string(body)
			for _, substr := range spec.Out.BodyContains {
				if !strings.Contains(bodyStr, substr) {
					t.Errorf("body missing %q\nbody: %s", substr, bodyStr)
				}
			}
		})
	}
}
