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
	"fmt"
	"net/http"

	"github.com/alternayte/auth-all/humaauth"
	"github.com/danielgtaylor/huma/v2"
	"github.com/pmezard/go-difflib/difflib"

	"github.com/alternayte/restitch-gateway/v2/internal/composition"
)

func (s *Server) registerImportExport() {
	viewer := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleViewer)}
	editor := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleEditor)}
	tags := []string{"draft"}

	huma.Register(s.api, huma.Operation{
		OperationID: "exportDraft", Method: http.MethodGet, Path: "/api/v1/environments/{env}/export",
		Summary:     "The draft as artifact YAML",
		Description: "The same format that standalone mode reads and that import accepts. A version's YAML is on getVersion.",
		Tags:        tags, Middlewares: viewer,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
	}) (*struct {
		Body struct {
			YAML string `json:"yaml"`
		}
	}, error) {
		if err := s.requireEnvironment(ctx, in.Environment); err != nil {
			return nil, err
		}
		resources, err := s.draftResources(ctx, in.Environment)
		if err != nil {
			return nil, err
		}
		doc, err := assembleArtifact(resources)
		if err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				YAML string `json:"yaml"`
			}
		}{}
		out.Body.YAML = string(doc)
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "importDraft", Method: http.MethodPost, Path: "/api/v1/environments/{env}/import",
		Summary:     "Replace the draft with an artifact YAML file",
		Description: "The file must pass the same validation as publish; a refused file leaves the draft unchanged. Returns the diff from the old draft.",
		Tags:        tags, Middlewares: editor,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
		Body        struct {
			YAML string `json:"yaml" maxLength:"10485760"`
		}
	}) (*struct {
		Body struct {
			Diff string `json:"diff"`
		}
	}, error) {
		p, err := principal(ctx)
		if err != nil {
			return nil, err
		}
		diff, err := s.importDraft(ctx, in.Environment, []byte(in.Body.YAML), p.User.Email)
		if err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				Diff string `json:"diff"`
			}
		}{}
		out.Body.Diff = diff
		return out, nil
	})
}

// importDraft validates doc as a whole artifact and makes it the draft.
func (s *Server) importDraft(ctx context.Context, env string, doc []byte, by string) (string, error) {
	if err := s.requireEnvironment(ctx, env); err != nil {
		return "", err
	}
	if len(doc) > composition.MaxArtifactBytes {
		return "", huma.Error422UnprocessableEntity(fmt.Sprintf("the file is larger than %d bytes", composition.MaxArtifactBytes))
	}
	if err := validateArtifact(ctx, doc); err != nil {
		return "", huma.Error422UnprocessableEntity(err.Error())
	}
	resources, err := artifactResources(doc)
	if err != nil {
		return "", huma.Error422UnprocessableEntity(err.Error())
	}
	before, err := s.draftResources(ctx, env)
	if err != nil {
		return "", err
	}
	beforeDoc, err := assembleArtifact(before)
	if err != nil {
		return "", err
	}
	if err := s.replaceDraft(ctx, env, resources, by); err != nil {
		return "", err
	}
	afterDoc, err := assembleArtifact(resources)
	if err != nil {
		return "", err
	}
	return difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A: difflib.SplitLines(string(beforeDoc)), B: difflib.SplitLines(string(afterDoc)),
		FromFile: env + " draft", ToFile: "imported file", Context: 3,
	})
}
