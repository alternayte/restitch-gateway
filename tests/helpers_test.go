//go:build e2e

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
