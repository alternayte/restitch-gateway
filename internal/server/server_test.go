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

func TestNew(t *testing.T) {
	srv := New(Config{Port: 0, TLSPort: 0})
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.Router() == nil {
		t.Fatal("expected non-nil router")
	}
	if srv.Ready() {
		t.Error("server should not be ready before listen")
	}
}

func TestSetHandler(t *testing.T) {
	srv := New(Config{Port: 0, TLSPort: 0})
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})
	srv.SetHandler(h)

	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))
	if !called {
		t.Error("custom handler not called")
	}
}

func TestNewLoggingMiddleware(t *testing.T) {
	mw := NewLoggingMiddleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))

	if rec.Code != 200 {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected 'ok', got %q", rec.Body.String())
	}
}

func TestResponseWriter_Flush(t *testing.T) {
	rw := newResponseWriter(httptest.NewRecorder())
	rw.Flush()
}

func TestResponseWriter_BytesWritten(t *testing.T) {
	rw := newResponseWriter(httptest.NewRecorder())
	_, _ = rw.Write([]byte("hello"))
	if rw.bytesWritten != 5 {
		t.Errorf("bytesWritten = %d, want 5", rw.bytesWritten)
	}
}
