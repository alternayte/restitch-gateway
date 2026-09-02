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
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyRewrite(t *testing.T) {
	var gotPath, gotKey string
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-Admin-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"test"}`))
	}))
	defer admin.Close()

	adminKey := "test-key-123"
	mux := buildMux(muxDeps{gatewayAdminURL: admin.URL, adminKey: adminKey})

	req := httptest.NewRequest("GET", "/api/info", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if gotPath != "/admin/api/info" {
		t.Errorf("admin received path %q, want /admin/api/info", gotPath)
	}
	if gotKey != adminKey {
		t.Errorf("admin received key %q, want %q", gotKey, adminKey)
	}
}

// TestProxyStripsCORSHeaders covers finding M12: CORS headers from the
// gateway admin server must not reach the browser through the Studio proxy.
func TestProxyStripsCORSHeaders(t *testing.T) {
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Vary", "Origin")
		_, _ = w.Write([]byte(`{"version":"test"}`))
	}))
	defer admin.Close()

	mux := buildMux(muxDeps{gatewayAdminURL: admin.URL})

	req := httptest.NewRequest("GET", "/api/info", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin reached the browser: %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Errorf("Access-Control-Allow-Methods reached the browser: %q", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestSPAFallback(t *testing.T) {
	mux := buildMux(muxDeps{gatewayAdminURL: "http://localhost:9999"})

	t.Run("unknown path returns index.html", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/compositions", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		body, _ := io.ReadAll(rec.Result().Body)
		if !strings.Contains(string(body), "html") && !strings.Contains(string(body), "Restitch") {
			t.Errorf("expected index.html content for unknown path, got %q", string(body)[:100])
		}
	})

	t.Run("root returns index.html", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != 200 {
			t.Errorf("root status = %d, want 200", rec.Code)
		}
	})
}

// TestMissingAssetReturns404 guards the diagnosability of a build that skipped
// the frontend step. The SPA fallback previously served index.html for a
// missing /assets/*.js with a 200, so the browser got HTML where it expected a
// module: a blank page with no 404 anywhere to point at the real cause.
func TestMissingAssetReturns404(t *testing.T) {
	mux := buildMux(muxDeps{gatewayAdminURL: "http://localhost:9999"})

	t.Run("missing asset does not fall back to index.html", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/assets/index-doesnotexist.js", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404; body starts %.40q", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "<!doctype html>") {
			t.Error("missing asset served index.html; a blank page with no 404 is the result")
		}
	})

	t.Run("unknown client-side route still falls back", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/compositions", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200 — SPA routing must still work", rec.Code)
		}
	})
}
