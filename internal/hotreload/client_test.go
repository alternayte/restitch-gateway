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

package hotreload

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistryClient_Fetch_Success(t *testing.T) {
	bundle := map[string]any{
		"yaml_content":      "upstreams: {}\ncompositions: {}\n",
		"etag":              "abc123",
		"composition_count": 2,
		"composition_names": []string{"comp1", "comp2"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/registry/bundle" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("If-None-Match") != "" {
			t.Errorf("unexpected If-None-Match on first request: %s", r.Header.Get("If-None-Match"))
		}
		w.Header().Set("ETag", "abc123")
		_ = json.NewEncoder(w).Encode(bundle)
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	result, err := client.Fetch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if result.NotModified {
		t.Error("expected modified")
	}
	if result.ETag != "abc123" {
		t.Errorf("etag = %q, want %q", result.ETag, "abc123")
	}
	if string(result.YAML) != "upstreams: {}\ncompositions: {}\n" {
		t.Errorf("yaml = %q", string(result.YAML))
	}
	if result.CompositionCount != 2 {
		t.Errorf("count = %d, want 2", result.CompositionCount)
	}
}

func TestRegistryClient_Fetch_NotModified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == "abc123" {
			w.Header().Set("ETag", "abc123")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		t.Errorf("expected If-None-Match: abc123, got %q", r.Header.Get("If-None-Match"))
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	result, err := client.Fetch(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !result.NotModified {
		t.Error("expected NotModified")
	}
}

func TestRegistryClient_Fetch_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	_, err := client.Fetch(context.Background(), "")
	if err == nil {
		t.Error("expected error on 500")
	}
}

func TestRegistryClient_Fetch_ConnectionRefused(t *testing.T) {
	client := NewRegistryClient("http://127.0.0.1:1")
	_, err := client.Fetch(context.Background(), "")
	if err == nil {
		t.Error("expected error on connection refused")
	}
}

func TestRegistryClient_Fetch_AdminKey(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Admin-Key")
		w.Header().Set("ETag", "x")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"yaml_content": "", "etag": "x", "composition_count": 0, "composition_names": []string{},
		})
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL, WithAdminKey("secret"))
	_, err := client.Fetch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "secret" {
		t.Errorf("X-Admin-Key = %q, want %q", gotKey, "secret")
	}
}

func TestRegistryClient_Fetch_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewRegistryClient(srv.URL)
	_, err := client.Fetch(ctx, "")
	if err == nil {
		t.Error("expected error on canceled context")
	}
}

// TestRegistryClient_Fetch_OversizedBundle covers finding H4: a bundle
// larger than the limit must be rejected, not decoded.
func TestRegistryClient_Fetch_OversizedBundle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"yaml_content": "` + strings.Repeat("a", maxBundleBytes) + `"}`))
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	_, err := client.Fetch(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for oversized bundle")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error = %v, want a size-limit error", err)
	}
}
