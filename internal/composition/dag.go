// Copyright 2026 Restitch maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package composition

import (
	"fmt"
)

// ExecutionPlan represents the order in which steps should execute.
type ExecutionPlan struct {
	Waves [][]string
	Deps  map[string][]string // step → resolved deps (inferred+explicit)
}

// BuildDAG analyzes a composition and produces an execution plan.
func BuildDAG(comp *CompiledComposition) (*ExecutionPlan, error) {
	stepDeps := make(map[string][]string)
	for name, step := range comp.Steps {
		stepDeps[name] = step.Deps
	}

	if err := validateDependencies(comp.Steps, stepDeps); err != nil {
		return nil, err
	}

	plan, err := buildExecutionPlan(stepDeps)
	if err != nil {
		return nil, err
	}

	plan.Deps = stepDeps
	return plan, nil
}

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

func buildExecutionPlan(deps map[string][]string) (*ExecutionPlan, error) {
	inDegree := make(map[string]int)
	edges := make(map[string][]string)

	for step := range deps {
		if _, exists := inDegree[step]; !exists {
			inDegree[step] = 0
		}
	}

	for step, dependencies := range deps {
		inDegree[step] = len(dependencies)
		for _, dep := range dependencies {
			edges[dep] = append(edges[dep], step)
		}
	}

	var waves [][]string
	processed := 0

	var queue []string
	for step, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, step)
		}
	}

	for len(queue) > 0 {
		wave := make([]string, len(queue))
		copy(wave, queue)
		waves = append(waves, wave)
		processed += len(wave)

		var nextQueue []string
		for _, step := range queue {
			for _, dependent := range edges[step] {
				inDegree[dependent]--
				if inDegree[dependent] == 0 {
					nextQueue = append(nextQueue, dependent)
				}
			}
		}

		queue = nextQueue
	}

	if processed != len(inDegree) {
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
