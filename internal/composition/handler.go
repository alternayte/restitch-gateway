package composition

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/restitch/restitch-gateway/internal/auth"
	"github.com/restitch/restitch-gateway/internal/inbound"
	"github.com/restitch/restitch-gateway/internal/server"
)

var paramPattern = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// Handler handles HTTP requests for compositions.
type Handler struct {
	executor      *Executor
	config        *CompiledConfig
	authenticator *inbound.Authenticator
}

// Executor returns the handler's executor for lifecycle management.
func (h *Handler) Executor() *Executor {
	return h.executor
}

// NewHandler creates a new composition handler.
// authenticator may be nil when inbound auth is not configured.
func NewHandler(config *CompiledConfig, authenticator *inbound.Authenticator) *Handler {
	executor := NewExecutor(config)

	return &Handler{
		executor:      executor,
		config:        config,
		authenticator: authenticator,
	}
}

// RegisterRoutes registers all composition routes with the router.
// Each composition gets its own closure — no runtime route matching.
func (h *Handler) RegisterRoutes(router *server.Router) {
	for compName, comp := range h.config.Config.Compositions {
		name := compName

		matches := paramPattern.FindAllStringSubmatch(comp.Path, -1)
		paramNames := make([]string, 0, len(matches))
		for _, m := range matches {
			paramNames = append(paramNames, m[1])
		}

		var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			params := make(map[string]string, len(paramNames))
			for _, p := range paramNames {
				params[p] = r.PathValue(p)
			}
			h.serveComposition(w, r, name, params)
		})

		if !comp.Public && h.authenticator != nil {
			handler = h.authenticator.Middleware(handler)
		}

		router.Handle(comp.Method, comp.Path, handler.ServeHTTP)

		slog.Info("registered composition route",
			"composition", name,
			"method", comp.Method,
			"path", comp.Path,
			"public", comp.Public)
	}
}

// serveComposition handles a request for a specific composition.
func (h *Handler) serveComposition(w http.ResponseWriter, r *http.Request, compositionName string, params map[string]string) {
	ctx := r.Context()

	ctx = auth.WithClientAuthorization(ctx, r.Header.Get("Authorization"))

	slog.InfoContext(ctx, "executing composition",
		"composition", compositionName,
		"method", r.Method,
		"path", r.URL.Path)

	// Read and parse request body once
	var body any
	if r.Body != nil && r.ContentLength != 0 {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err == nil && len(raw) > 0 && strings.Contains(r.Header.Get("Content-Type"), "json") {
			json.Unmarshal(raw, &body)
		}
	}

	result, err := h.executor.Execute(ctx, compositionName, r, params, body)
	if err != nil {
		if r.Context().Err() != nil {
			slog.InfoContext(ctx, "client canceled",
				"composition", compositionName)
			return
		}

		if auth.IsMissingAuthHeaderError(err) {
			slog.WarnContext(ctx, "passthrough auth missing client authorization",
				"composition", compositionName,
				"path", r.URL.Path)
			w.Header().Set("WWW-Authenticate", "Bearer")
			h.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authorization header required"})
			return
		}

		slog.ErrorContext(ctx, "composition execution failed",
			"composition", compositionName,
			"error", err)

		var reqErr *RequiredStepError
		if errors.As(err, &reqErr) {
			if errors.Is(reqErr.Err, context.DeadlineExceeded) {
				h.writeJSON(w, http.StatusGatewayTimeout, map[string]string{
					"error": "upstream timeout",
					"step":  reqErr.Step,
				})
			} else {
				h.writeJSON(w, http.StatusBadGateway, map[string]string{
					"error": "upstream error",
					"step":  reqErr.Step,
				})
			}
		} else {
			h.writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream error"})
		}
		return
	}

	comp := h.config.Compositions[compositionName]

	failedSteps := make(map[string]bool)
	for name, res := range result.Steps {
		if res == nil {
			failedSteps[name] = true
		}
	}

	response, err := BuildResponse(comp.Response, result.Steps, r, result.StepErrors, failedSteps)
	if err != nil {
		slog.ErrorContext(ctx, "response template evaluation failed",
			"composition", compositionName,
			"error", err)
		h.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	w.Header().Set("Content-Type", response.ContentType)
	if result.IsPartial {
		w.Header().Set("X-Restitch-Complete", "false")
		w.Header().Set("X-Partial-Response", "true")
	} else {
		w.Header().Set("X-Restitch-Complete", "true")
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

// writeJSON writes a JSON response with the given status code.
func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
