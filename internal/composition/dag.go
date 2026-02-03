package composition

import (
	"fmt"
)

// ExecutionPlan represents the order in which steps should execute.
// Steps in the same wave can run in parallel.
// Waves are executed sequentially - wave N+1 starts only after wave N completes.
type ExecutionPlan struct {
	Waves [][]string // Each wave contains step names that can run in parallel
}

// BuildDAG analyzes a composition and produces an execution plan.
// It uses Kahn's algorithm for topological sorting with level detection
// to identify which steps can run in parallel.
//
// Dependencies are determined by:
//  1. Analyzing expressions in step path, body, and headers for steps.X references
//  2. Merging with explicit depends_on from configuration
//
// Returns an error if:
//   - Circular dependency detected
//   - Step references non-existent step
//   - Expression parsing fails
func BuildDAG(comp *CompiledComposition) (*ExecutionPlan, error) {
	// Extract dependencies for each step
	stepDeps := make(map[string][]string)
	for name, step := range comp.Steps {
		deps, err := analyzeDependencies(step)
		if err != nil {
			return nil, fmt.Errorf("step %s: %w", name, err)
		}
		stepDeps[name] = deps
	}

	// Validate all dependencies reference existing steps
	if err := validateDependencies(comp.Steps, stepDeps); err != nil {
		return nil, err
	}

	// Build execution plan using topological sort
	plan, err := buildExecutionPlan(stepDeps)
	if err != nil {
		return nil, err
	}

	return plan, nil
}

// analyzeDependencies extracts all dependencies for a step from its expressions.
func analyzeDependencies(step *CompiledStep) ([]string, error) {
	var exprs []string

	// Extract expressions from path
	if step.PathExpr != nil && step.PathExpr.Raw != "" {
		pathExprs := ExtractExpressions(step.PathExpr.Raw)
		exprs = append(exprs, pathExprs...)
	}

	// Extract expressions from body
	if step.BodyExpr != nil && step.BodyExpr.Raw != "" {
		bodyExprs := ExtractExpressions(step.BodyExpr.Raw)
		exprs = append(exprs, bodyExprs...)
	}

	// Extract expressions from headers
	for _, headerExpr := range step.HeaderExprs {
		if headerExpr != nil && headerExpr.Raw != "" {
			hdrExprs := ExtractExpressions(headerExpr.Raw)
			exprs = append(exprs, hdrExprs...)
		}
	}

	// Extract dependencies from all expressions
	inferred, err := ExtractAllDependencies(exprs...)
	if err != nil {
		return nil, err
	}

	// Merge with explicit depends_on
	merged := MergeDependencies(step.Step.DependsOn, inferred)

	return merged, nil
}

// validateDependencies checks that all referenced steps exist in the composition.
func validateDependencies(steps map[string]*CompiledStep, deps map[string][]string) error {
	for stepName, stepDeps := range deps {
		for _, dep := range stepDeps {
			if _, exists := steps[dep]; !exists {
				return fmt.Errorf("step %s references non-existent step %q", stepName, dep)
			}
		}
	}
	return nil
}

// buildExecutionPlan uses Kahn's algorithm for topological sorting with level detection.
// Steps ready at the same level form a wave and can execute in parallel.
func buildExecutionPlan(deps map[string][]string) (*ExecutionPlan, error) {
	// Build dependency graph
	inDegree := make(map[string]int)
	edges := make(map[string][]string)

	// Initialize in-degree for all steps
	for step := range deps {
		if _, exists := inDegree[step]; !exists {
			inDegree[step] = 0
		}
	}

	// Build edges and calculate in-degrees
	for step, dependencies := range deps {
		inDegree[step] = len(dependencies)
		for _, dep := range dependencies {
			edges[dep] = append(edges[dep], step)
		}
	}

	// Kahn's algorithm with level detection
	var waves [][]string
	processed := 0

	// Find all steps with no dependencies (wave 0)
	var queue []string
	for step, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, step)
		}
	}

	// Process steps level by level
	for len(queue) > 0 {
		// All steps in current queue form a wave (can run in parallel)
		wave := make([]string, len(queue))
		copy(wave, queue)
		waves = append(waves, wave)
		processed += len(wave)

		// Process this wave and find next wave
		var nextQueue []string
		for _, step := range queue {
			// For each step that depends on this one, decrement in-degree
			for _, dependent := range edges[step] {
				inDegree[dependent]--
				if inDegree[dependent] == 0 {
					nextQueue = append(nextQueue, dependent)
				}
			}
		}

		queue = nextQueue
	}

	// Check for cycles: if not all steps were processed, there's a cycle
	if processed != len(inDegree) {
		// Find steps still in cycle
		cycleSteps := []string{}
		for step, degree := range inDegree {
			if degree > 0 {
				cycleSteps = append(cycleSteps, step)
			}
		}
		return nil, fmt.Errorf("circular dependency detected involving steps: %v", cycleSteps)
	}

	return &ExecutionPlan{Waves: waves}, nil
}
