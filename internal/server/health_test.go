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

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	srv := New(Config{Port: 0, TLSPort: 0})
	handler := HealthHandler(srv)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
}

func TestReadyHandler(t *testing.T) {
	srv := New(Config{Port: 0, TLSPort: 0})
	handler := ReadyHandler(srv)

	// Not ready yet
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/ready", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("before ready: expected 503, got %d", rec.Code)
	}

	// Set ready
	srv.SetReady(true)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/ready", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("after ready: expected 200, got %d", rec.Code)
	}
}

func TestServer_ReadyAfterListen(t *testing.T) {
	srv := New(Config{Port: 0, TLSPort: 0})

	if srv.Ready() {
		t.Error("server should not be ready before Listen")
	}
}
