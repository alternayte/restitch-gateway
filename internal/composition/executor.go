package composition

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/restitch/restitch-gateway/internal/auth"
	"github.com/restitch/restitch-gateway/internal/observability"
	upstreampkg "github.com/restitch/restitch-gateway/internal/upstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// StepStatus represents the outcome of a step execution.
type StepStatus = string

const (
	StepSuccess StepStatus = "success"
	StepFailed  StepStatus = "failed"
	StepSkipped StepStatus = "skipped"
)

// StepTiming records execution timing for a single step.
type StepTiming struct {
	Name          string  `json:"name"`
	Wave          int     `json:"wave"`
	DurationMS    float64 `json:"duration_ms"`
	Status        string  `json:"status"`
	Optional      bool    `json:"optional"`
	Upstream      string  `json:"upstream"`
	URL           string  `json:"url"`
	StartOffsetMS float64 `json:"start_offset_ms"`
	BodySize      int64   `json:"body_size"`
	Error         string  `json:"error,omitempty"`
	Cached        bool    `json:"cached"`
	Retries       int     `json:"retries"`
}

// CompositionResult holds results from all steps in a composition.
type CompositionResult struct {
	Steps       map[string]*StepResult
	StepErrors  []StepErrorDetail
	IsPartial   bool
	StepTimings []StepTiming
}

// Executor runs compositions according to their DAG execution plans.
type Executor struct {
	config    *CompiledConfig
	coalescer *upstreampkg.Coalescer
	cache     *upstreampkg.StepCache
}

// NewExecutor creates a new composition executor.
func NewExecutor(config *CompiledConfig) *Executor {
	return &Executor{
		config:    config,
		coalescer: upstreampkg.NewCoalescer(),
		cache:     upstreampkg.NewStepCache(),
	}
}

// Close cleans up executor resources (cache janitor).
func (e *Executor) Close() {
	if e.cache != nil {
		e.cache.Close()
	}
}

// Execute runs a composition for an incoming request.
func (e *Executor) Execute(ctx context.Context, compositionName string, rd *RequestData) (*CompositionResult, error) {
	comp, exists := e.config.Compositions[compositionName]
	if !exists {
		return nil, fmt.Errorf("composition %q not found", compositionName)
	}

	plan := comp.ExecutionPlan
	requestStart := time.Now()

	slog.InfoContext(ctx, "executing composition",
		"composition", compositionName,
		"waves", len(plan.Waves),
		"total_steps", countSteps(plan.Waves),
		"execution_order", plan.Waves)

	results := make(map[string]*StepResult)
	resultsMutex := sync.Mutex{}
	var allErrors []stepError

	var stepTimings []StepTiming
	var stepTimingsMu sync.Mutex

	for waveIdx, wave := range plan.Waves {
		slog.DebugContext(ctx, "starting wave",
			"wave", waveIdx,
			"steps", wave)

		waveStart := time.Now()
		waveNum := waveIdx + 1

		var wg sync.WaitGroup
		var waveErrors []stepError
		var errorsMu sync.Mutex

		for _, stepName := range wave {
			stepName := stepName
			wg.Add(1)
			go func() {
				defer wg.Done()

				stepErr, timing := e.executeStepWithErrorHandling(ctx, compositionName, stepName, comp, plan, rd, results, &resultsMutex, waveNum, requestStart)
				if timing != nil {
					stepTimingsMu.Lock()
					stepTimings = append(stepTimings, *timing)
					stepTimingsMu.Unlock()
				}
				if stepErr != nil {
					errorsMu.Lock()
					waveErrors = append(waveErrors, *stepErr)
					errorsMu.Unlock()
				}
			}()
		}

		wg.Wait()

		if HasRequiredFailure(waveErrors) {
			allErrors = append(allErrors, waveErrors...)

			for _, stepErr := range waveErrors {
				if !stepErr.optional {
					slog.ErrorContext(ctx, "required step failed in wave", "wave", waveIdx, "step", stepErr.stepName, "error", stepErr.err)
					return nil, &RequiredStepError{Step: stepErr.stepName, Err: stepErr.err}
				}
			}
		}

		allErrors = append(allErrors, waveErrors...)

		slog.DebugContext(ctx, "wave complete",
			"wave", waveIdx,
			"duration", time.Since(waveStart))
	}

	return &CompositionResult{
		Steps:       results,
		StepErrors:  BuildErrorsArray(allErrors),
		IsPartial:   len(allErrors) > 0,
		StepTimings: stepTimings,
	}, nil
}

func (e *Executor) executeStepWithErrorHandling(
	ctx context.Context,
	compositionName string,
	stepName string,
	comp *CompiledComposition,
	plan *ExecutionPlan,
	rd *RequestData,
	results map[string]*StepResult,
	resultsMutex *sync.Mutex,
	waveNum int,
	requestStart time.Time,
) (*stepError, *StepTiming) {
	ctx, stepSpan := otel.Tracer("restitch").Start(ctx, "step:"+stepName)
	defer stepSpan.End()
	stepSpan.SetAttributes(
		attribute.String("restitch.step", stepName),
		attribute.Int("restitch.wave", waveNum),
	)

	stepStart := time.Now()
	startOffsetMS := float64(stepStart.Sub(requestStart).Microseconds()) / 1000.0

	step, exists := comp.Steps[stepName]
	if !exists {
		durationMS := float64(time.Since(stepStart).Microseconds()) / 1000.0
		stepSpan.SetStatus(codes.Error, "step not found")
		return &stepError{stepName: stepName, err: fmt.Errorf("step not found"), optional: false},
			&StepTiming{Name: stepName, Wave: waveNum, DurationMS: durationMS, Status: StepFailed, Optional: false, StartOffsetMS: startOffsetMS, Error: "step not found"}
	}

	resultsMutex.Lock()
	depFailed := checkDependenciesFailed(plan.Deps[stepName], results)
	resultsMutex.Unlock()

	stepSpan.SetAttributes(attribute.String("restitch.upstream", step.Step.Upstream))

	if depFailed {
		durationMS := float64(time.Since(stepStart).Microseconds()) / 1000.0
		stepSpan.SetStatus(codes.Error, "dependency_failed")
		resultsMutex.Lock()
		results[stepName] = nil
		resultsMutex.Unlock()
		return &stepError{
				stepName: stepName,
				err:      fmt.Errorf("dependency_failed"),
				optional: true,
			},
			&StepTiming{Name: stepName, Wave: waveNum, DurationMS: durationMS, Status: StepSkipped, Optional: step.Optional, Upstream: step.Step.Upstream, StartOffsetMS: startOffsetMS, Error: "dependency_failed"}
	}

	up, exists := e.config.Upstreams[step.Step.Upstream]
	if !exists {
		durationMS := float64(time.Since(stepStart).Microseconds()) / 1000.0
		stepSpan.SetStatus(codes.Error, "upstream not found")
		return &stepError{stepName: stepName, err: fmt.Errorf("upstream not found"), optional: step.Optional},
			&StepTiming{Name: stepName, Wave: waveNum, DurationMS: durationMS, Status: StepFailed, Optional: step.Optional, Upstream: step.Step.Upstream, StartOffsetMS: startOffsetMS, Error: "upstream not found"}
	}

	resultsMutex.Lock()
	env := buildRequestEnv(ctx, rd, results)
	resultsMutex.Unlock()

	slog.InfoContext(ctx, "step starting",
		"composition", compositionName,
		"step", stepName,
		"wave", waveNum,
		"upstream", up.BaseURL,
		"optional", step.Optional)

	// Compute the full request URL for observability (best-effort; evaluation
	// errors are surfaced later when the step actually executes).
	stepURL := ""
	if evalPath, pathErr := step.PathPart.EvalString(env, EscapePath); pathErr == nil {
		if step.QueryPart != nil {
			if q, queryErr := step.QueryPart.EvalString(env, EscapeQuery); queryErr == nil {
				evalPath += "?" + q
			}
		}
		stepURL = strings.TrimRight(up.BaseURL, "/") + "/" + strings.TrimLeft(evalPath, "/")
	}

	// Build cache/coalesce key from evaluated URL + auth identity
	var cacheKey string
	if step.Step.Cache != nil || step.Step.Coalesce {
		evalPath, _ := step.PathPart.EvalString(env, EscapeNone)
		if step.QueryPart != nil {
			q, _ := step.QueryPart.EvalString(env, EscapeNone)
			evalPath += "?" + q
		}
		fullURL := strings.TrimRight(up.BaseURL, "/") + "/" + strings.TrimLeft(evalPath, "/")
		authID := auth.ClientAuthorization(ctx)
		cacheKey = upstreampkg.CoalesceKey(step.Step.Method, fullURL, authID)
	}

	// Cache check (before coalesce and execute)
	if step.Step.Cache != nil && step.Step.Method == "GET" {
		if cached, ok := e.cache.Get(cacheKey); ok {
			if m := observability.DefaultMetrics(); m != nil {
				m.CacheHitsTotal.WithLabelValues(compositionName, stepName).Inc()
			}
			parsedBody, _ := parseJSONBody(cached.Body, cached.Headers.Get("Content-Type"))
			if parsedBody == nil {
				parsedBody = string(cached.Body)
			}
			result := &StepResult{
				Status:  cached.Status,
				Headers: cached.Headers,
				Body:    parsedBody,
				RawBody: cached.Body,
			}
			durationMS := float64(time.Since(stepStart).Microseconds()) / 1000.0
			resultsMutex.Lock()
			results[stepName] = result
			resultsMutex.Unlock()
			slog.InfoContext(ctx, "step complete (cached)",
				"composition", compositionName,
				"step", stepName,
				"wave", waveNum,
				"status", result.Status,
				"duration_ms", durationMS)
			return nil, &StepTiming{
				Name:          stepName,
				Wave:          waveNum,
				DurationMS:    durationMS,
				Status:        StepSuccess,
				Optional:      step.Optional,
				Upstream:      step.Step.Upstream,
				URL:           stepURL,
				StartOffsetMS: startOffsetMS,
				BodySize:      int64(len(result.RawBody)),
				Cached:        true,
			}
		} else {
			if m := observability.DefaultMetrics(); m != nil {
				m.CacheMissesTotal.WithLabelValues(compositionName, stepName).Inc()
			}
		}
	}

	// Execute step (with optional coalescing)
	var result *StepResult
	var err error

	executeStep := func() (*StepResult, error) {
		return ExecuteStep(ctx, step, up, env)
	}

	if step.Step.Coalesce && step.Step.Method == "GET" {
		v, shared, coalesceErr := e.coalescer.Do(cacheKey, func() (any, error) {
			return executeStep()
		})
		if coalesceErr != nil {
			err = coalesceErr
		} else if v != nil {
			result = v.(*StepResult)
		}
		if shared {
			if m := observability.DefaultMetrics(); m != nil {
				m.CoalescedTotal.WithLabelValues(compositionName, stepName).Inc()
			}
		}
	} else {
		result, err = executeStep()
	}

	durationMS := float64(time.Since(stepStart).Microseconds()) / 1000.0

	if err != nil {
		stepSpan.SetStatus(codes.Error, err.Error())
		stepSpan.RecordError(err)

		slog.WarnContext(ctx, "step failed",
			"composition", compositionName,
			"step", stepName,
			"wave", waveNum,
			"optional", step.Optional,
			"duration_ms", durationMS,
			"error", err)

		resultsMutex.Lock()
		results[stepName] = nil
		resultsMutex.Unlock()

		return &stepError{stepName: stepName, err: err, optional: step.Optional},
			&StepTiming{
				Name:          stepName,
				Wave:          waveNum,
				DurationMS:    durationMS,
				Status:        StepFailed,
				Optional:      step.Optional,
				Upstream:      step.Step.Upstream,
				URL:           stepURL,
				StartOffsetMS: startOffsetMS,
				Error:         err.Error(),
			}
	}

	// Cache fill (status < 500, not error-rule-matched)
	if step.Step.Cache != nil && step.Step.Method == "GET" && result.Status < 500 && !result.ErrorRuleMatched {
		body := result.RawBody
		if body == nil {
			body, _ = json.Marshal(result.Body)
		}
		e.cache.Set(cacheKey, &upstreampkg.CachedResponse{
			Status:  result.Status,
			Headers: result.Headers.Clone(),
			Body:    body,
		}, step.Step.Cache.TTL)
	}

	resultsMutex.Lock()
	results[stepName] = result
	resultsMutex.Unlock()

	stepSpan.SetStatus(codes.Ok, "")

	slog.InfoContext(ctx, "step complete",
		"composition", compositionName,
		"step", stepName,
		"wave", waveNum,
		"status", result.Status,
		"duration_ms", durationMS)

	if result != nil && result.ErrorRuleMatched {
		errRule := NewErrorRuleMatchedError(result.Status)
		return &stepError{
				stepName: stepName,
				err:      errRule,
				optional: true,
			},
			&StepTiming{
				Name:          stepName,
				Wave:          waveNum,
				DurationMS:    durationMS,
				Status:        StepSuccess,
				Optional:      step.Optional,
				Upstream:      step.Step.Upstream,
				URL:           stepURL,
				StartOffsetMS: startOffsetMS,
				BodySize:      int64(len(result.RawBody)),
				Error:         errRule.Error(),
			}
	}

	return nil, &StepTiming{
		Name:          stepName,
		Wave:          waveNum,
		DurationMS:    durationMS,
		Status:        StepSuccess,
		Optional:      step.Optional,
		Upstream:      step.Step.Upstream,
		URL:           stepURL,
		StartOffsetMS: startOffsetMS,
		BodySize:      int64(len(result.RawBody)),
	}
}

func checkDependenciesFailed(deps []string, results map[string]*StepResult) bool {
	for _, depName := range deps {
		if result, exists := results[depName]; !exists || result == nil {
			return true
		}
	}
	return false
}

func countSteps(waves [][]string) int {
	total := 0
	for _, wave := range waves {
		total += len(wave)
	}
	return total
}
