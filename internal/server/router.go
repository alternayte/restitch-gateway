// Package server provides HTTP/HTTPS server implementation for the restitch gateway.
package server

import (
	"net/http"
	"strings"
	"sync"
)

// route represents a registered route with method, pattern, and handler.
type route struct {
	method  string           // HTTP method (GET, POST, etc.) or "" for any method
	pattern string           // Path pattern (exact or prefix ending with /)
	handler http.HandlerFunc // Handler function
}

// Router handles HTTP request routing by path pattern and method.
type Router struct {
	mu     sync.RWMutex
	routes []route
}

// NewRouter creates a new Router instance.
func NewRouter() *Router {
	return &Router{
		routes: make([]route, 0),
	}
}

// Handle registers a handler for the given method and pattern.
// Method can be empty string to match any method.
// Pattern ending with / matches as a prefix, otherwise exact match.
func (r *Router) Handle(method string, pattern string, handler http.HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.routes = append(r.routes, route{
		method:  method,
		pattern: pattern,
		handler: handler,
	})
}

// ServeHTTP implements http.Handler interface.
// Routes are matched in order: exact matches first, then prefix matches.
// Returns 404 for unmatched paths, 405 for wrong method on matched path.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	path := req.URL.Path
	method := req.Method

	var matchedPath bool
	var exactMatch *route
	var prefixMatch *route

	// First pass: find best matching route
	for i := range r.routes {
		rt := &r.routes[i]

		// Check if pattern matches path
		if rt.pattern == path {
			// Exact match
			matchedPath = true
			if rt.method == "" || rt.method == method {
				exactMatch = rt
				break // Exact match with correct method, use it
			}
		} else if strings.HasSuffix(rt.pattern, "/") && strings.HasPrefix(path, rt.pattern) {
			// Prefix match
			matchedPath = true
			if rt.method == "" || rt.method == method {
				// Keep the longest prefix match
				if prefixMatch == nil || len(rt.pattern) > len(prefixMatch.pattern) {
					prefixMatch = rt
				}
			}
		}
	}

	// Use exact match if found, otherwise prefix match
	if exactMatch != nil {
		exactMatch.handler(w, req)
		return
	}

	if prefixMatch != nil {
		prefixMatch.handler(w, req)
		return
	}

	// Path matched but method didn't
	if matchedPath {
		w.Header().Set("Allow", r.allowedMethods(path))
		http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// No match at all
	http.NotFound(w, req)
}

// allowedMethods returns a comma-separated list of allowed methods for a path.
func (r *Router) allowedMethods(path string) string {
	var methods []string
	seen := make(map[string]bool)

	for i := range r.routes {
		rt := &r.routes[i]

		matches := false
		if rt.pattern == path {
			matches = true
		} else if strings.HasSuffix(rt.pattern, "/") && strings.HasPrefix(path, rt.pattern) {
			matches = true
		}

		if matches && rt.method != "" && !seen[rt.method] {
			methods = append(methods, rt.method)
			seen[rt.method] = true
		}
	}

	if len(methods) == 0 {
		return ""
	}

	return strings.Join(methods, ", ")
}
