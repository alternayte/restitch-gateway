package composition

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// StepTiming records execution timing for a single step.
type StepTiming struct {
	Name       string  `json:"name"`
	Wave       int     `json:"wave"`
	DurationMS float64 `json:"duration_ms"`
	Status     string  `json:"status"`
	Optional   bool    `json:"optional"`
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
	config     *CompiledConfig
	httpClient *http.Client
}

// NewExecutor creates a new composition executor.
func NewExecutor(config *CompiledConfig, httpClient *http.Client) *Executor {
	return &Executor{
		config:     config,
		httpClient: httpClient,
	}
}

// Execute runs a composition for an incoming request.
func (e *Executor) Execute(ctx context.Context, compositionName string, req *http.Request) (*CompositionResult, error) {
	comp, exists := e.config.Compositions[compositionName]
	if !exists {
		return nil, fmt.Errorf("composition %q not found", compositionName)
	}

	plan := comp.ExecutionPlan

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

				stepErr, timing := e.executeStepWithErrorHandling(ctx, compositionName, stepName, comp, req, results, &resultsMutex, waveNum)
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
					return nil, fmt.Errorf("required step %q failed: %w", stepErr.stepName, stepErr.err)
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

// executeStepWithErrorHandling executes a single step with optional step error handling.
func (e *Executor) executeStepWithErrorHandling(
	ctx context.Context,
	compositionName string,
	stepName string,
	comp *CompiledComposition,
	req *http.Request,
	results map[string]*StepResult,
	resultsMutex *sync.Mutex,
	waveNum int,
) (*stepError, *StepTiming) {
	stepStart := time.Now()

	step, exists := comp.Steps[stepName]
	if !exists {
		durationMS := float64(time.Since(stepStart).Microseconds()) / 1000.0
		return &stepError{stepName: stepName, err: fmt.Errorf("step not found"), optional: false},
			&StepTiming{Name: stepName, Wave: waveNum, DurationMS: durationMS, Status: "failed", Optional: false}
	}

	resultsMutex.Lock()
	depFailed := checkDependenciesFailed(step, results)
	resultsMutex.Unlock()

	if depFailed {
		durationMS := float64(time.Since(stepStart).Microseconds()) / 1000.0
		resultsMutex.Lock()
		results[stepName] = nil
		resultsMutex.Unlock()
		return &stepError{
				stepName: stepName,
				err:      fmt.Errorf("dependency_failed"),
				optional: true,
			},
			&StepTiming{Name: stepName, Wave: waveNum, DurationMS: durationMS, Status: "skipped", Optional: step.Optional}
	}

	compiledUpstream, exists := e.config.Upstreams[step.Step.Upstream]
	if !exists {
		durationMS := float64(time.Since(stepStart).Microseconds()) / 1000.0
		return &stepError{stepName: stepName, err: fmt.Errorf("upstream not found"), optional: step.Optional},
			&StepTiming{Name: stepName, Wave: waveNum, DurationMS: durationMS, Status: "failed", Optional: step.Optional}
	}

	resultsMutex.Lock()
	env := buildRequestEnv(req, nil, nil, results)
	resultsMutex.Unlock()

	slog.InfoContext(ctx, "step starting",
		"composition", compositionName,
		"step", stepName,
		"wave", waveNum,
		"upstream", compiledUpstream.Upstream.URL,
		"optional", step.Optional)

	result, err := ExecuteStep(ctx, step, compiledUpstream, env, e.httpClient)
	durationMS := float64(time.Since(stepStart).Microseconds()) / 1000.0

	if err != nil {
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
			&StepTiming{Name: stepName, Wave: waveNum, DurationMS: durationMS, Status: "failed", Optional: step.Optional}
	}

	resultsMutex.Lock()
	results[stepName] = result
	resultsMutex.Unlock()

	slog.InfoContext(ctx, "step complete",
		"composition", compositionName,
		"step", stepName,
		"wave", waveNum,
		"status", result.Status,
		"duration_ms", durationMS)

	if result != nil && result.ErrorRuleMatched {
		return &stepError{
				stepName: stepName,
				err:      NewErrorRuleMatchedError(result.Status),
				optional: true,
			},
			&StepTiming{Name: stepName, Wave: waveNum, DurationMS: durationMS, Status: "success", Optional: step.Optional}
	}

	return nil, &StepTiming{Name: stepName, Wave: waveNum, DurationMS: durationMS, Status: "success", Optional: step.Optional}
}

// checkDependenciesFailed returns true if any dependency has nil result.
func checkDependenciesFailed(step *CompiledStep, results map[string]*StepResult) bool {
	for _, depName := range step.Step.DependsOn {
		if result, exists := results[depName]; !exists || result == nil {
			return true
		}
	}
	return false
}

// countSteps counts total steps across all waves.
func countSteps(waves [][]string) int {
	total := 0
	for _, wave := range waves {
		total += len(wave)
	}
	return total
}
