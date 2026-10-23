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
	"encoding/json"
	"net/http"

	"github.com/alternayte/auth-all/humaauth"
	"github.com/danielgtaylor/huma/v2"

	"github.com/alternayte/restitch-gateway/v2/internal/composition"
)

// kindDefinitions names the schema definition of each resource kind's
// body. settings has no Go type of its own: it holds artifact top-level
// keys, so its schema is written here.
var kindDefinitions = map[string]string{
	KindUpstream:    "composition.Upstream",
	KindComposition: "composition.Composition",
	KindAuth:        "inbound.InboundAuthConfig",
	KindRateLimit:   "ratelimit.Config",
}

func (s *Server) registerSchema() {
	viewer := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleViewer)}
	raw, err := composition.ArtifactSchema()
	if err != nil {
		panic(err) // the schema comes from Go types; failing here is a build bug
	}
	huma.Register(s.api, huma.Operation{
		OperationID: "getArtifactSchema", Method: http.MethodGet, Path: "/api/v1/schema",
		Summary:     "The JSON Schema of the artifact and of each resource kind",
		Description: "Generated from the gateway's Go config types. definitions names the $defs entry of each kind's body.",
		Tags:        []string{"draft"}, Middlewares: viewer,
	}, func(context.Context, *struct{}) (*struct {
		Body struct {
			Artifact    json.RawMessage   `json:"artifact"`
			Definitions map[string]string `json:"definitions"`
			Settings    map[string]any    `json:"settings"`
		}
	}, error) {
		out := &struct {
			Body struct {
				Artifact    json.RawMessage   `json:"artifact"`
				Definitions map[string]string `json:"definitions"`
				Settings    map[string]any    `json:"settings"`
			}
		}{}
		out.Body.Artifact = raw
		out.Body.Definitions = kindDefinitions
		out.Body.Settings = map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{"block_private_upstreams": map[string]any{"type": "boolean"}},
		}
		return out, nil
	})
}
