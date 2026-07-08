package testenv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/restitch/restitch-gateway/internal/composition"
	"github.com/restitch/restitch-gateway/internal/server"
)

// ScriptedResponse defines a canned response for a scripted upstream.
type ScriptedResponse struct {
	Status  int
	Body    string
	Headers map[string]string
	DelayMS int
}

// UpstreamSpec describes how to create a mock upstream server.
type UpstreamSpec struct {
	Handler      http.Handler
	Script       map[string]ScriptedResponse // "METHOD /path" → response
	CloseOnStart bool
}

// Config holds the test configuration.
type Config struct {
	YAML      string
	Upstreams map[string]UpstreamSpec
}

// Env provides access to the running gateway for tests.
type Env struct {
	GatewayURL string
	Client     *http.Client
	hits       map[string]*atomic.Int64
}

// Hits returns the number of requests received by the named upstream.
func (e *Env) Hits(upstream string) int {
	if c, ok := e.hits[upstream]; ok {
		return int(c.Load())
	}
	return 0
}

// Run boots a full gateway in-process with scripted upstreams and calls fn.
func Run(t *testing.T, cfg Config, fn func(t *testing.T, env *Env)) {
	t.Helper()

	env := &Env{
		Client: &http.Client{Timeout: 10 * time.Second},
		hits:   make(map[string]*atomic.Int64),
	}

	yamlContent := cfg.YAML

	// Start upstream servers
	for name, spec := range cfg.Upstreams {
		name := name
		counter := &atomic.Int64{}
		env.hits[name] = counter

		var handler http.Handler
		if spec.Handler != nil {
			handler = spec.Handler
		} else {
			handler = buildScriptedHandler(spec.Script)
		}

		wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			counter.Add(1)
			handler.ServeHTTP(w, r)
		})

		us := httptest.NewServer(wrapped)
		if spec.CloseOnStart {
			us.Close()
		} else {
			t.Cleanup(us.Close)
		}

		placeholder := fmt.Sprintf("@@UPSTREAM_%s@@", name)
		yamlContent = strings.ReplaceAll(yamlContent, placeholder, us.URL)
	}

	// Write config to temp file
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "restitch.yaml")
	if err := os.WriteFile(cfgPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Load and compile
	parsed, err := composition.LoadConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}

	compiled, err := composition.CompileConfig(context.Background(), parsed)
	if err != nil {
		t.Fatalf("CompileConfig: %v", err)
	}

	handler := composition.NewHandler(compiled, nil)

	router := server.NewRouter()
	handler.RegisterRoutes(router)

	srv := server.New(server.Config{Port: 0})
	router.Handle(http.MethodGet, "/health", server.HealthHandler(srv))
	router.Handle(http.MethodGet, "/ready", server.ReadyHandler(srv))
	router.Finalize()

	gw := httptest.NewServer(router)
	t.Cleanup(gw.Close)
	env.GatewayURL = gw.URL

	fn(t, env)
}

func buildScriptedHandler(script map[string]ScriptedResponse) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		resp, ok := script[key]
		if !ok {
			// Try without method
			resp, ok = script[r.URL.Path]
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not scripted: " + key})
			return
		}

		if resp.DelayMS > 0 {
			time.Sleep(time.Duration(resp.DelayMS) * time.Millisecond)
		}

		for k, v := range resp.Headers {
			w.Header().Set(k, v)
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}

		status := resp.Status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		if resp.Body != "" {
			w.Write([]byte(resp.Body))
		}
	})
}
