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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alternayte/restitch-gateway/v2/internal/composition"
)

func TestClient_FetchSendsTokenAndReadsVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gateway/v1/artifact" || r.Header.Get("Authorization") != "Bearer rgt_abc" {
			http.Error(w, "nope", http.StatusUnauthorized)
			return
		}
		bundleHandler("upstreams: {}\n", `"staging-3"`, 3)(w, r)
	}))
	defer srv.Close()

	c := NewClient(srv.URL+"/", "rgt_abc")
	res, err := c.Fetch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if string(res.YAML) != "upstreams: {}\n" || res.Version != 3 || res.ETag != `"staging-3"` {
		t.Fatalf("result = %+v", res)
	}
	res, err = c.Fetch(context.Background(), `"staging-3"`)
	if err != nil || !res.NotModified {
		t.Fatalf("same etag: %+v %v", res, err)
	}
}

func TestClient_FetchClassifiesStatus(t *testing.T) {
	for status, want := range map[int]error{
		http.StatusNotFound:     ErrNoVersion,
		http.StatusUnauthorized: ErrTokenRejected,
		http.StatusForbidden:    ErrTokenRejected,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		_, err := NewClient(srv.URL, "rgt_x").Fetch(context.Background(), "")
		srv.Close()
		if !errors.Is(err, want) {
			t.Errorf("status %d: err = %v, want %v", status, err, want)
		}
	}
}

func TestClient_FetchServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL, "rgt_x").Fetch(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_FetchConnectionRefused(t *testing.T) {
	if _, err := NewClient("http://127.0.0.1:1", "rgt_x").Fetch(context.Background(), ""); err == nil {
		t.Fatal("expected an error")
	}
}

func TestClient_FetchRefusesOversizedArtifact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", composition.MaxArtifactBytes+1)))
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL, "rgt_x").Fetch(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v", err)
	}
}
