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
	Status     string  `json:"status"` // "success", "failed", "skipped"
	Optional   bool    `json:"optional"`
}

// CompositionResult holds results from all steps in a composition.
type CompositionResult struct {
	Steps       map[string]*StepResult
	StepErrors  []StepErrorDetail // Errors from failed optional steps
	IsPartial   bool              // True if any step failed
	StepTimings []StepTiming      // Timing data for all steps
}

// Executor runs compositions according to their DAG execution plans.
// It handles parallel execution within waves and fail-fast error propagation.
type Executor struct {
	config     *CompiledConfig
	httpClient *http.Client
}

// NewExecutor creates a new composition executor.
// The httpClient should be the Phase 1 client with optimized connection pooling.
func NewExecutor(config *CompiledConfig, httpClient *http.Client) *Executor {
	return &Executor{
		config:     config,
		httpClient: httpClient,
	}
}

// Execute runs a composition for an incoming request.
// It executes steps according to the DAG execution plan:
//   - Independent steps (same wave) execute in parallel
//   - Dependent steps wait for their dependencies to complete
//   - Optional step failures are collected, composition continues
//   - Required step failures fail-fast and cancel remaining steps
//
// Per CONTEXT.md execution behavior:
//   - Fail fast on required step failure only
//   - Optional step failures collected in StepErrors
//   - Failed optional steps have nil result (available to dependents)
//   - Dependent steps skip when their dependency failed
//   - Upstream error passthrough (handled by ExecuteStep)
//   - Logs show step start/complete events with wave numbers and timing
//
// Returns CompositionResult with step results, errors, and timing data.
func (e *Executor) Execute(ctx context.Context, compositionName string, req *http.Request) (*CompositionResult, error) {
	// Look up composition
	comp, exists := e.config.Compositions[compositionName]
	if !exists {
		return nil, fmt.Errorf("composition %q not found", compositionName)
	}

	// Use pre-built execution plan (validated at config parse time)
	plan := comp.ExecutionPlan

	// Log execution plan at INFO level (per OBS-04: DAG execution order for debugging)
	slog.Info("executing composition",
		"composition", compositionName,
		"waves", len(plan.Waves),
		"total_steps", countSteps(plan.Waves),
		"execution_order", plan.Waves)

	// Execute waves sequentially
	results := make(map[string]*StepResult)
	resultsMutex := sync.Mutex{}
	var allErrors []stepError // Collects errors across all waves

	// Step timing collection (per OBS-03)
	var stepTimings []StepTiming
	var stepTimingsMu sync.Mutex

	for waveIdx, wave := range plan.Waves {
		slog.Debug("starting wave",
			"wave", waveIdx,
			"steps", wave)

		waveStart := time.Now()
		waveNum := waveIdx + 1 // 1-indexed for human readability

		var wg sync.WaitGroup
		var waveErrors []stepError
		var errorsMu sync.Mutex

		for _, stepName := range wave {
			stepName := stepName // Capture loop variable
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

		// Check for required failures - fail fast
		if HasRequiredFailure(waveErrors) {
			// Collect all errors for logging but only fail on required
			allErrors = append(allErrors, waveErrors...)

			// Find the first required error to return
			for _, stepErr := range waveErrors {
				if !stepErr.optional {
					slog.Error("required step failed in wave", "wave", waveIdx, "step", stepErr.stepName, "error", stepErr.err)
					return nil, fmt.Errorf("required step %q failed: %w", stepErr.stepName, stepErr.err)
				}
			}
		}

		// Aggregate wave errors into allErrors for partial response
		allErrors = append(allErrors, waveErrors...)

		waveDuration := time.Since(waveStart)
		slog.Debug("wave complete",
			"wave", waveIdx,
			"duration", waveDuration)
	}

	slog.Info("composition complete",
		"composition", compositionName,
		"steps", len(results))

	return &CompositionResult{
		Steps:       results,
		StepErrors:  BuildErrorsArray(allErrors),
		IsPartial:   len(allErrors) > 0,
		StepTimings: stepTimings,
	}, nil
}

// executeStepWithErrorHandling executes a single step with optional step error handling.
// Returns stepError if step failed (nil if succeeded), and StepTiming for all outcomes.
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

	// Check if dependencies failed (result is nil)
	// Per CONTEXT.md: "If a step with dependents fails, dependent steps are skipped"
	resultsMutex.Lock()
	depFailed := checkDependenciesFailed(step, results)
	resultsMutex.Unlock()

	if depFailed {
		durationMS := float64(time.Since(stepStart).Microseconds()) / 1000.0
		// Mark as skipped, add nil result
		resultsMutex.Lock()
		results[stepName] = nil
		resultsMutex.Unlock()
		return &stepError{
				stepName: stepName,
				err:      fmt.Errorf("dependency_failed"),
				optional: true, // Skipped steps don't fail composition
			},
			&StepTiming{Name: stepName, Wave: waveNum, DurationMS: durationMS, Status: "skipped", Optional: step.Optional}
	}

	// Get compiled upstream with auth strategy
	compiledUpstream, exists := e.config.Upstreams[step.Step.Upstream]
	if !exists {
		durationMS := float64(time.Since(stepStart).Microseconds()) / 1000.0
		return &stepError{stepName: stepName, err: fmt.Errorf("upstream not found"), optional: step.Optional},
			&StepTiming{Name: stepName, Wave: waveNum, DurationMS: durationMS, Status: "failed", Optional: step.Optional}
	}

	// Build environment with completed step results
	resultsMutex.Lock()
	env := buildRequestEnv(req, results)
	resultsMutex.Unlock()

	slog.Info("step starting",
		"composition", compositionName,
		"step", stepName,
		"wave", waveNum,
		"upstream", compiledUpstream.Upstream.URL,
		"optional", step.Optional)

	// Execute step
	result, err := ExecuteStep(ctx, step, compiledUpstream, env, e.httpClient)
	durationMS := float64(time.Since(stepStart).Microseconds()) / 1000.0

	if err != nil {
		slog.Warn("step failed",
			"composition", compositionName,
			"step", stepName,
			"wave", waveNum,
			"optional", step.Optional,
			"duration_ms", durationMS,
			"error", err)

		// Store nil result for failed step (optional steps return null per CONTEXT.md)
		resultsMutex.Lock()
		results[stepName] = nil
		resultsMutex.Unlock()

		return &stepError{stepName: stepName, err: err, optional: step.Optional},
			&StepTiming{Name: stepName, Wave: waveNum, DurationMS: durationMS, Status: "failed", Optional: step.Optional}
	}

	// Store successful result
	resultsMutex.Lock()
	results[stepName] = result
	resultsMutex.Unlock()

	slog.Info("step complete",
		"composition", compositionName,
		"step", stepName,
		"wave", waveNum,
		"status", result.Status,
		"duration_ms", durationMS)

	// Check if error rule was applied - record in errors for transparency
	// Per RESEARCH.md Pitfall 5: "Always add matched error rule to `_errors` array"
	if result != nil && result.ErrorRuleMatched {
		return &stepError{
				stepName: stepName,
				err:      NewErrorRuleMatchedError(result.Status),
				optional: true, // Error rule matches don't fail composition
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
