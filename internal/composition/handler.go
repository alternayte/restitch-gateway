package composition

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/restitch/restitch-gateway/internal/auth"
	"github.com/restitch/restitch-gateway/internal/server"
)

// Handler handles HTTP requests for compositions.
// It matches incoming requests to compositions, executes them, and returns
// the merged response.
type Handler struct {
	executor *Executor
	config   *CompiledConfig
	routes   map[string]string // path+method -> composition name
}

// NewHandler creates a new composition handler.
// The httpClient should be the Phase 1 client with optimized connection pooling.
func NewHandler(config *CompiledConfig, httpClient *http.Client) *Handler {
	executor := NewExecutor(config, httpClient)

	return &Handler{
		executor: executor,
		config:   config,
		routes:   make(map[string]string),
	}
}

// RegisterRoutes registers all composition routes with the router.
// Each composition is registered based on its path and method from the config.
func (h *Handler) RegisterRoutes(router *server.Router) {
	for compName, comp := range h.config.Config.Compositions {
		// Store route mapping
		routeKey := h.routeKey(comp.Path, comp.Method)
		h.routes[routeKey] = compName

		// Register route with router
		router.Handle(comp.Method, comp.Path, h.ServeHTTP)

		slog.Info("registered composition route",
			"composition", compName,
			"method", comp.Method,
			"path", comp.Path)
	}
}

// ServeHTTP handles an HTTP request by executing the appropriate composition.
// It matches the request to a composition, executes it, builds the response,
// and writes it back to the client.
//
// Error handling:
//   - 401 Unauthorized: Passthrough auth but client didn't provide Authorization header
//   - 404 Not Found: No composition matches this path
//   - 502 Bad Gateway: Upstream step failure (network error, auth failure, context cancellation)
//   - 500 Internal Server Error: Response template evaluation error
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Find composition for this path and method
	compositionName := h.matchComposition(r.URL.Path, r.Method)
	if compositionName == "" {
		slog.Warn("no composition found for request",
			"method", r.Method,
			"path", r.URL.Path)
		http.NotFound(w, r)
		return
	}

	slog.Info("executing composition",
		"composition", compositionName,
		"method", r.Method,
		"path", r.URL.Path,
		"request_id", r.Header.Get("X-Request-ID"))

	// Execute composition
	ctx := r.Context()
	result, err := h.executor.Execute(ctx, compositionName, r)
	if err != nil {
		// Check for passthrough auth missing header - return 401 Unauthorized
		// Per CONTEXT.md/RESEARCH.md: Don't forward unauthenticated requests
		if auth.IsMissingAuthHeaderError(err) {
			slog.Warn("passthrough auth missing client authorization",
				"composition", compositionName,
				"path", r.URL.Path)
			w.Header().Set("WWW-Authenticate", "Bearer")
			h.writeError(w, http.StatusUnauthorized, fmt.Errorf("authorization header required"))
			return
		}

		// Other execution errors (network failure, OAuth2 token failure, context cancellation)
		// Per CONTEXT.md: "Gateway auth failures (token fetch fails, network error) return 502"
		slog.Error("composition execution failed",
			"composition", compositionName,
			"error", err)
		h.writeError(w, http.StatusBadGateway, err)
		return
	}

	// Build response from template
	comp := h.config.Compositions[compositionName]
	response, err := BuildResponse(comp.Response, result.Steps, r, result.StepErrors)
	if err != nil {
		slog.Error("response template evaluation failed",
			"composition", compositionName,
			"error", err)
		h.writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Write response
	w.Header().Set("Content-Type", response.ContentType)
	// Set X-Partial-Response header if any errors occurred
	// Per CONTEXT.md: "HTTP 200 with `X-Partial-Response: true` header when any step fails"
	if result.IsPartial {
		w.Header().Set("X-Partial-Response", "true")
	}
	w.WriteHeader(response.Status)

	if err := json.NewEncoder(w).Encode(response.Body); err != nil {
		slog.Error("failed to encode response body",
			"composition", compositionName,
			"error", err)
		return
	}

	slog.Info("composition complete",
		"composition", compositionName,
		"status", response.Status,
		"partial", result.IsPartial,
		"errors", len(result.StepErrors))
}

// matchComposition finds the composition name for a given path and method.
// Returns empty string if no composition matches.
func (h *Handler) matchComposition(path, method string) string {
	// Normalize path (remove trailing slash unless it's root)
	normalizedPath := path
	if len(normalizedPath) > 1 && strings.HasSuffix(normalizedPath, "/") {
		normalizedPath = strings.TrimSuffix(normalizedPath, "/")
	}

	// Try exact match first
	routeKey := h.routeKey(normalizedPath, method)
	if compName, exists := h.routes[routeKey]; exists {
		return compName
	}

	// No match found
	return ""
}

// routeKey creates a unique key for route mapping.
func (h *Handler) routeKey(path, method string) string {
	return fmt.Sprintf("%s:%s", method, path)
}

// writeError writes an error response in JSON format.
func (h *Handler) writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	errorResponse := map[string]string{
		"error": err.Error(),
	}

	json.NewEncoder(w).Encode(errorResponse)
}
