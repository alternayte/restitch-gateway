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
type Handler struct {
	executor *Executor
	config   *CompiledConfig
	routes   map[string]string
}

// NewHandler creates a new composition handler.
func NewHandler(config *CompiledConfig, httpClient *http.Client) *Handler {
	executor := NewExecutor(config, httpClient)

	return &Handler{
		executor: executor,
		config:   config,
		routes:   make(map[string]string),
	}
}

// RegisterRoutes registers all composition routes with the router.
func (h *Handler) RegisterRoutes(router *server.Router) {
	for compName, comp := range h.config.Config.Compositions {
		routeKey := h.routeKey(comp.Path, comp.Method)
		h.routes[routeKey] = compName

		router.Handle(comp.Method, comp.Path, h.ServeHTTP)

		slog.Info("registered composition route",
			"composition", compName,
			"method", comp.Method,
			"path", comp.Path)
	}
}

// ServeHTTP handles an HTTP request by executing the appropriate composition.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	compositionName := h.matchComposition(r.URL.Path, r.Method)
	if compositionName == "" {
		slog.WarnContext(ctx, "no composition found for request",
			"method", r.Method,
			"path", r.URL.Path)
		http.NotFound(w, r)
		return
	}

	slog.InfoContext(ctx, "executing composition",
		"composition", compositionName,
		"method", r.Method,
		"path", r.URL.Path)

	result, err := h.executor.Execute(ctx, compositionName, r)
	if err != nil {
		if auth.IsMissingAuthHeaderError(err) {
			slog.WarnContext(ctx, "passthrough auth missing client authorization",
				"composition", compositionName,
				"path", r.URL.Path)
			w.Header().Set("WWW-Authenticate", "Bearer")
			h.writeError(w, http.StatusUnauthorized, fmt.Errorf("authorization header required"))
			return
		}

		slog.ErrorContext(ctx, "composition execution failed",
			"composition", compositionName,
			"error", err)
		h.writeError(w, http.StatusBadGateway, err)
		return
	}

	comp := h.config.Compositions[compositionName]
	response, err := BuildResponse(comp.Response, result.Steps, r, result.StepErrors)
	if err != nil {
		slog.ErrorContext(ctx, "response template evaluation failed",
			"composition", compositionName,
			"error", err)
		h.writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", response.ContentType)
	if result.IsPartial {
		w.Header().Set("X-Partial-Response", "true")
	}
	w.WriteHeader(response.Status)

	if err := json.NewEncoder(w).Encode(response.Body); err != nil {
		slog.ErrorContext(ctx, "failed to encode response body",
			"composition", compositionName,
			"error", err)
		return
	}

	timingSummary := make(map[string]float64)
	for _, t := range result.StepTimings {
		timingSummary[t.Name] = t.DurationMS
	}

	slog.InfoContext(ctx, "composition complete",
		"composition", compositionName,
		"status", response.Status,
		"partial", result.IsPartial,
		"errors", len(result.StepErrors),
		"step_timings", timingSummary,
		"total_steps", len(result.StepTimings),
		"slowest_step", findSlowestStep(result.StepTimings))
}

// findSlowestStep returns the name and duration of the slowest step.
func findSlowestStep(timings []StepTiming) map[string]interface{} {
	if len(timings) == 0 {
		return nil
	}
	slowest := timings[0]
	for _, t := range timings[1:] {
		if t.DurationMS > slowest.DurationMS {
			slowest = t
		}
	}
	return map[string]interface{}{
		"name":        slowest.Name,
		"duration_ms": slowest.DurationMS,
	}
}

// matchComposition finds the composition name for a given path and method.
func (h *Handler) matchComposition(path, method string) string {
	normalizedPath := path
	if len(normalizedPath) > 1 && strings.HasSuffix(normalizedPath, "/") {
		normalizedPath = strings.TrimSuffix(normalizedPath, "/")
	}

	routeKey := h.routeKey(normalizedPath, method)
	if compName, exists := h.routes[routeKey]; exists {
		return compName
	}

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
