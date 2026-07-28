package hotreload

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
