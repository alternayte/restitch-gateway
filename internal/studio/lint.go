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

package studio

import (
	"context"
	"net/http"

	"github.com/alternayte/auth-all/humaauth"
	"github.com/danielgtaylor/huma/v2"

	"github.com/alternayte/restitch-gateway/v2/internal/composition"
)

func (s *Server) registerLint() {
	viewer := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleViewer)}
	huma.Register(s.api, huma.Operation{
		OperationID: "lintTemplate", Method: http.MethodPost, Path: "/api/v1/lint",
		Summary:     "Check one template string the way publish does",
		Description: "Compiles the template against the steps of its composition, with the same checks as gateway load.",
		Tags:        []string{"draft"}, Middlewares: viewer,
	}, func(_ context.Context, in *struct {
		Body struct {
			Template string   `json:"template" maxLength:"65536"`
			Steps    []string `json:"steps,omitempty" maxItems:"500"`
		}
	}) (*struct {
		Body struct {
			Errors []string `json:"errors"`
		}
	}, error) {
		out := &struct {
			Body struct {
				Errors []string `json:"errors"`
			}
		}{}
		out.Body.Errors = []string{}
		if err := composition.LintTemplate(in.Body.Template, in.Body.Steps); err != nil {
			out.Body.Errors = append(out.Body.Errors, err.Error())
		}
		return out, nil
	})
}

type stepDependencies struct {
	Explicit []string `json:"explicit" doc:"Names in the step's depends_on."`
	Inferred []string `json:"inferred" doc:"Steps the step's templates reference that are not in depends_on."`
}

type stepDependenciesOutput struct {
	Body struct {
		Steps map[string]stepDependencies `json:"steps" doc:"Each step by name."`
		Error string                      `json:"error" doc:"Why the gateway would refuse the steps, or empty."`
	}
}

func (s *Server) registerStepDependencies() {
	viewer := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleViewer)}
	huma.Register(s.api, huma.Operation{
		OperationID: "stepDependencies", Method: http.MethodPost, Path: "/api/v1/step-dependencies",
		Summary:     "List what each step of a composition waits for",
		Description: "Compiles the steps of an unsaved composition the way the gateway does and returns, per step, its depends_on names and the steps its templates reference.",
		Tags:        []string{"draft"}, Middlewares: viewer,
	}, func(_ context.Context, in *struct {
		Body struct {
			Composition map[string]any `json:"composition"`
		}
	}) (*stepDependenciesOutput, error) {
		out := &stepDependenciesOutput{}
		out.Body.Steps = map[string]stepDependencies{}
		var comp composition.Composition
		if err := strictDecode(in.Body.Composition, &comp); err != nil {
			out.Body.Error = err.Error()
			return out, nil
		}
		deps, err := composition.StepDependencies(&comp)
		for name, d := range deps {
			out.Body.Steps[name] = stepDependencies{Explicit: d.Explicit, Inferred: d.Inferred}
		}
		if err != nil {
			out.Body.Error = err.Error()
		}
		return out, nil
	})
}
