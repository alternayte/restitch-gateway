package composition

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// CompositionResult holds results from all steps in a composition.
type CompositionResult struct {
	Steps map[string]*StepResult
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
//   - First error cancels all remaining steps (fail-fast)
//
// Per CONTEXT.md execution behavior:
//   - Fail fast on required step failure
//   - Upstream error passthrough (handled by ExecuteStep)
//   - Logs show step start/complete events
//
// Returns CompositionResult with all step results, or error if any step fails.
func (e *Executor) Execute(ctx context.Context, compositionName string, req *http.Request) (*CompositionResult, error) {
	// Look up composition
	comp, exists := e.config.Compositions[compositionName]
	if !exists {
		return nil, fmt.Errorf("composition %q not found", compositionName)
	}

	// Use pre-built execution plan (validated at config parse time)
	plan := comp.ExecutionPlan

	// Log execution plan at debug level (optional for Phase 2)
	slog.Debug("executing composition",
		"composition", compositionName,
		"waves", len(plan.Waves),
		"total_steps", countSteps(plan.Waves))

	// Execute waves sequentially
	results := make(map[string]*StepResult)
	resultsMutex := sync.Mutex{}

	for waveIdx, wave := range plan.Waves {
		slog.Debug("starting wave",
			"wave", waveIdx,
			"steps", wave)

		waveStart := time.Now()

		// Execute all steps in this wave in parallel using errgroup
		g, waveCtx := errgroup.WithContext(ctx)

		for _, stepName := range wave {
			stepName := stepName // Capture loop variable

			g.Go(func() error {
				return e.executeStep(waveCtx, compositionName, stepName, comp, req, results, &resultsMutex)
			})
		}

		// Wait for wave to complete or first error
		if err := g.Wait(); err != nil {
			slog.Error("wave failed",
				"wave", waveIdx,
				"error", err)
			return nil, err // Fail fast - context cancels remaining goroutines
		}

		waveDuration := time.Since(waveStart)
		slog.Debug("wave complete",
			"wave", waveIdx,
			"duration", waveDuration)
	}

	slog.Info("composition complete",
		"composition", compositionName,
		"steps", len(results))

	return &CompositionResult{
		Steps: results,
	}, nil
}

// executeStep executes a single step with proper locking and error handling.
func (e *Executor) executeStep(
	ctx context.Context,
	compositionName string,
	stepName string,
	comp *CompiledComposition,
	req *http.Request,
	results map[string]*StepResult,
	resultsMutex *sync.Mutex,
) error {
	step, exists := comp.Steps[stepName]
	if !exists {
		return fmt.Errorf("step %q not found in composition", stepName)
	}

	// Get upstream configuration
	upstream, exists := e.config.Config.Upstreams[step.Step.Upstream]
	if !exists {
		return fmt.Errorf("upstream %q not found for step %q", step.Step.Upstream, stepName)
	}

	// Build environment with completed step results
	resultsMutex.Lock()
	env := buildRequestEnv(req, results)
	resultsMutex.Unlock()

	// Log step start
	stepStart := time.Now()
	slog.Info("step starting",
		"composition", compositionName,
		"step", stepName,
		"upstream", upstream.URL)

	// Execute step
	result, err := ExecuteStep(ctx, step, &upstream, env, e.httpClient)
	if err != nil {
		return fmt.Errorf("step %s: %w", stepName, err)
	}

	stepDuration := time.Since(stepStart)

	// Store result
	resultsMutex.Lock()
	results[stepName] = result
	resultsMutex.Unlock()

	// Log step complete
	slog.Info("step complete",
		"composition", compositionName,
		"step", stepName,
		"status", result.Status,
		"duration", stepDuration)

	return nil
}

// countSteps counts total steps across all waves.
func countSteps(waves [][]string) int {
	total := 0
	for _, wave := range waves {
		total += len(wave)
	}
	return total
}
