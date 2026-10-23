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
