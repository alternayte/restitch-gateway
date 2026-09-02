//go:build e2e

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/alternayte/restitch-gateway/internal/composition"
	"github.com/alternayte/restitch-gateway/internal/server"
)

func parseAndCompile(yamlBytes []byte) (*composition.CompiledConfig, error) {
	cfg, err := composition.ParseConfig(yamlBytes)
	if err != nil {
		return nil, err
	}
	return composition.CompileConfig(context.Background(), cfg, composition.CompileOptions{SkipAuthInit: true})
}

func buildHandler(t *testing.T, yamlBytes []byte) http.Handler {
	t.Helper()

	compiled, err := parseAndCompile(yamlBytes)
	if err != nil {
		t.Fatalf("compile config: %v", err)
	}

	handler := composition.NewHandler(compiled, nil)

	router := server.NewRouter()

	router.Handle("GET", "/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	handler.RegisterRoutes(router)
	router.Finalize()

	return router
}
