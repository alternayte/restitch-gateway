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
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouter_MethodRouting(t *testing.T) {
	r := NewRouter()
	r.Handle("GET", "/api/test", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("get"))
	})
	r.Handle("POST", "/api/test", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("post"))
	})
	r.Finalize()

	// GET
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/api/test", nil))
	if rec.Body.String() != "get" {
		t.Errorf("GET: got %q, want get", rec.Body.String())
	}

	// POST
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/test", nil))
	if rec.Body.String() != "post" {
		t.Errorf("POST: got %q, want post", rec.Body.String())
	}
}

func TestRouter_MethodNotAllowed(t *testing.T) {
	r := NewRouter()
	r.Handle("GET", "/api/test", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	r.Finalize()

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("DELETE", "/api/test", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
	allow := rec.Header().Get("Allow")
	if allow == "" {
		t.Error("expected Allow header on 405")
	}
}

func TestRouter_HeadServedByGet(t *testing.T) {
	r := NewRouter()
	r.Handle("GET", "/api/test", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	r.Finalize()

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("HEAD", "/api/test", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("HEAD should be served by GET handler, got %d", rec.Code)
	}
}

func TestRouter_PathParams(t *testing.T) {
	r := NewRouter()
	r.Handle("GET", "/api/users/{id}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(req.PathValue("id")))
	})
	r.Finalize()

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/api/users/42", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "42" {
		t.Errorf("got %q, want 42", rec.Body.String())
	}
}

func TestRouter_MiddlewareOrder(t *testing.T) {
	r := NewRouter()
	var order []string
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			order = append(order, "A")
			next.ServeHTTP(w, req)
		})
	})
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			order = append(order, "B")
			next.ServeHTTP(w, req)
		})
	})
	r.Handle("GET", "/test", func(w http.ResponseWriter, req *http.Request) {
		order = append(order, "handler")
	})
	r.Finalize()

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))

	if len(order) != 3 || order[0] != "A" || order[1] != "B" || order[2] != "handler" {
		t.Errorf("middleware order = %v, want [A B handler]", order)
	}
}

func TestRouter_FinalizePanic(t *testing.T) {
	r := NewRouter()
	r.Handle("GET", "/test", func(w http.ResponseWriter, req *http.Request) {})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when ServeHTTP called before Finalize")
		}
	}()

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))
}

func TestRouter_NotFound(t *testing.T) {
	r := NewRouter()
	r.Handle("GET", "/exists", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	r.Finalize()

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}
