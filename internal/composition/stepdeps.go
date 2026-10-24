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
	"slices"
)

// StepDeps is what one step waits for. Explicit lists the names in its
// depends_on. Inferred lists the steps its templates reference that are not
// already explicit. The gateway waits for both.
type StepDeps struct {
	Explicit []string
	Inferred []string
}

// StepDependencies compiles the steps of comp the way the gateway does and
// returns the dependencies of each step by name. A step whose templates do not
// compile keeps its explicit dependencies only. The error is the first step
// that does not compile, or else the error the execution plan raises for an
// unknown step or a cycle. The map is complete even when the error is not nil.
func StepDependencies(comp *Composition) (map[string]StepDeps, error) {
	names := make([]string, len(comp.Steps))
	for i, step := range comp.Steps {
		names[i] = step.Name
	}
	env := BuildBaseEnvironment(names)

	out := make(map[string]StepDeps, len(comp.Steps))
	compiled := &CompiledComposition{Steps: make(map[string]*CompiledStep, len(comp.Steps))}
	var firstErr error
	for i := range comp.Steps {
		step := &comp.Steps[i]
		explicit := uniqueSorted(step.DependsOn, nil)
		cs, err := compileStep(step, env)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("step %s: %w", step.Name, err)
			}
			out[step.Name] = StepDeps{Explicit: explicit, Inferred: []string{}}
			continue
		}
		compiled.Steps[step.Name] = cs
		out[step.Name] = StepDeps{Explicit: explicit, Inferred: uniqueSorted(templateDeps(cs), explicit)}
	}
	if firstErr != nil {
		return out, firstErr
	}
	if _, err := BuildDAG(compiled); err != nil {
		return out, err
	}
	return out, nil
}

// uniqueSorted returns the names in sorted order, once each, without the
// names in skip. It never returns nil.
func uniqueSorted(names, skip []string) []string {
	out := []string{}
	for _, n := range names {
		if !slices.Contains(skip, n) && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}
