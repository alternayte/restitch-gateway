package composition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/alternayte/restitch-gateway/internal/auth"
	"github.com/alternayte/restitch-gateway/internal/inbound"
	"github.com/alternayte/restitch-gateway/internal/observability"
	"github.com/alternayte/restitch-gateway/internal/ratelimit"
	"github.com/alternayte/restitch-gateway/internal/reqlog"
	"github.com/alternayte/restitch-gateway/internal/server"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var paramPattern = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// Handler handles HTTP requests for compositions.
type Handler struct {
	executor      *Executor
	config        *CompiledConfig
	authenticator *inbound.Authenticator
	recorder      reqlog.Recorder
}

// SetRecorder sets the request recorder for the handler.
func (h *Handler) SetRecorder(r reqlog.Recorder) {
	h.recorder = r
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

		if comp.RateLimit != nil {
			limiter := ratelimit.New(*comp.RateLimit)
			handler = limiter.Middleware(handler)
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

	tracer := otel.Tracer("restitch")
	ctx, span := tracer.Start(ctx, "composition:"+compositionName)
	defer span.End()
	span.SetAttributes(
		attribute.String("restitch.composition", compositionName),
		attribute.String("http.method", r.Method),
		attribute.String("http.route", r.URL.Path),
	)

	ctx = auth.WithClientAuthorization(ctx, r.Header.Get("Authorization"))

	handlerStart := time.Now()

	slog.InfoContext(ctx, "executing composition",
		"composition", compositionName,
		"method", r.Method,
		"path", r.URL.Path)

	// Determine max request body size (default 1 MiB).
	maxBytes := int64(1 << 20)
	if comp := h.config.Config.Compositions[compositionName]; comp.MaxRequestBytes > 0 {
		maxBytes = comp.MaxRequestBytes
	}

	// Read and parse request body once
	var body any
	if r.Body != nil && r.ContentLength != 0 {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				h.writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body too large"})
				return
			}
			// other read error; continue with nil body
		} else if len(raw) > 0 && strings.Contains(r.Header.Get("Content-Type"), "json") {
			if err := json.Unmarshal(raw, &body); err != nil {
				// Malformed JSON must not slip past schema validation with a
				// nil body and execute with empty input (finding H11).
				h.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
				return
			}
		}
	}

	// Validate request body against JSON schema if configured.
	compiledComp := h.config.Compositions[compositionName]
	if compiledComp.RequestSchema != nil && body != nil {
		if err := compiledComp.RequestSchema.Validate(body); err != nil {
			details := extractValidationErrors(err)
			h.writeJSON(w, http.StatusBadRequest, map[string]any{
				"error":   "request validation failed",
				"details": details,
			})
			return
		}
	}

	rd := NewRequestData(r, params, body)
	executeStart := time.Now()
	result, err := h.executor.Execute(ctx, compositionName, rd)
	executeDuration := time.Since(executeStart)
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

	response, err := BuildResponse(ctx, comp.Response, result.Steps, rd, result.StepErrors, failedSteps)
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

	durationMS := float64(time.Since(handlerStart).Nanoseconds()) / 1e6
	durationSec := durationMS / 1000.0
	executeDurationMS := float64(executeDuration.Nanoseconds()) / 1e6
	gatewayOverheadMS := durationMS - executeDurationMS

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
		"slowest_step", findSlowestStep(result.StepTimings),
		"gateway_overhead_ms", gatewayOverheadMS)

	// Prometheus metrics
	if m := observability.DefaultMetrics(); m != nil {
		statusStr := observability.StatusStr(response.Status)
		m.RequestsTotal.WithLabelValues(compositionName, r.Method, statusStr).Inc()
		m.RequestDuration.WithLabelValues(compositionName).Observe(durationSec)
		if result.IsPartial {
			m.PartialResponsesTotal.WithLabelValues(compositionName).Inc()
		}
		for _, t := range result.StepTimings {
			upstream := ""
			if step, ok := h.config.Compositions[compositionName]; ok {
				if cs, ok := step.Steps[t.Name]; ok {
					upstream = cs.Step.Upstream
				}
			}
			m.StepDuration.WithLabelValues(compositionName, t.Name, upstream, t.Status).Observe(t.DurationMS / 1000.0)
		}
	}

	// Request recording (ring buffer + stats)
	if h.recorder != nil {
		steps := make([]reqlog.StepRecord, len(result.StepTimings))
		for i, t := range result.StepTimings {
			httpStatus := 0
			if sr, ok := result.Steps[t.Name]; ok && sr != nil {
				httpStatus = sr.Status
			}
			steps[i] = reqlog.StepRecord{
				Name:          t.Name,
				Status:        t.Status,
				Wave:          t.Wave,
				DurationMS:    t.DurationMS,
				HTTPStatus:    httpStatus,
				Upstream:      t.Upstream,
				URL:           t.URL,
				StartOffsetMS: t.StartOffsetMS,
				BodySize:      t.BodySize,
				Error:         t.Error,
				Cached:        t.Cached,
				Retries:       t.Retries,
			}
		}
		rec := reqlog.Record{
			ID:          observability.GetRequestID(ctx),
			Time:        handlerStart,
			Composition: compositionName,
			Method:      r.Method,
			Path:        r.URL.Path,
			Status:      response.Status,
			DurationMS:  durationMS,
			Partial:     result.IsPartial,
			Steps:       steps,
		}
		if sc := trace.SpanFromContext(ctx).SpanContext(); sc.HasTraceID() {
			rec.TraceID = sc.TraceID().String()
		}
		h.recorder.Record(rec)
	}
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
	_ = json.NewEncoder(w).Encode(v)
}

// extractValidationErrors converts jsonschema validation errors into a list
// of human-readable strings.
func extractValidationErrors(err error) []string {
	var ve *jsonschema.ValidationError
	if errors.As(err, &ve) {
		causes := ve.BasicOutput().Errors
		if len(causes) > 0 {
			details := make([]string, 0, len(causes))
			for _, c := range causes {
				if c.Error == nil {
					continue
				}
				msg := c.Error.String()
				if c.InstanceLocation != "" {
					msg = fmt.Sprintf("%s: %s", c.InstanceLocation, c.Error.String())
				}
				if msg != "" {
					details = append(details, msg)
				}
			}
			if len(details) > 0 {
				return details
			}
		}
	}
	return []string{err.Error()}
}
