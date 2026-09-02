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

// Package server provides HTTP/HTTPS server implementation for the restitch gateway.
package server

import (
	"net/http"
)

// Router wraps http.ServeMux with middleware support.
type Router struct {
	mux         *http.ServeMux
	middlewares []func(http.Handler) http.Handler
	handler     http.Handler
	finalized   bool
}

// NewRouter creates a new Router instance.
func NewRouter() *Router {
	return &Router{
		mux: http.NewServeMux(),
	}
}

// Use adds a middleware to the router. Must be called before Finalize.
func (r *Router) Use(mw func(http.Handler) http.Handler) {
	if r.finalized {
		panic("router: Use called after Finalize")
	}
	r.middlewares = append(r.middlewares, mw)
}

// Handle registers a handler for the given method and pattern.
// When method is non-empty, it registers "METHOD pattern".
// When method is empty, it registers the bare pattern.
func (r *Router) Handle(method, pattern string, h http.HandlerFunc) {
	if method != "" {
		r.mux.HandleFunc(method+" "+pattern, h)
	} else {
		r.mux.HandleFunc(pattern, h)
	}
}

// Finalize composes the middleware chain once. Must be called after all
// registrations and before serving requests.
func (r *Router) Finalize() {
	if r.finalized {
		return
	}
	r.finalized = true

	var handler http.Handler = r.mux
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		handler = r.middlewares[i](handler)
	}
	r.handler = handler
}

// ServeHTTP implements http.Handler. Panics if Finalize was not called.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if !r.finalized {
		panic("router: ServeHTTP called before Finalize")
	}
	r.handler.ServeHTTP(w, req)
}
