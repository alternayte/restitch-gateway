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
	"sort"

	"github.com/alternayte/auth-all/humaauth"
	"github.com/danielgtaylor/huma/v2"
	"gopkg.in/yaml.v3"

	"github.com/alternayte/restitch-gateway/v2/internal/gwconfig"
)

// Secrets stay on the gateway host. The artifact carries only ${NAME}
// references, which the gateway resolves from its environment. Studio
// checks each name's syntax and lists the names an environment needs; it
// never stores a value.

// checkVarNames refuses a ${...} whose name the gateway environment cannot
// hold. The scan is the gateway's own (gwconfig.References), so Studio and
// the gateway agree on what a reference is.
func checkVarNames(doc string) error {
	refs, err := gwconfig.References(doc)
	if err != nil {
		return err
	}
	for _, r := range refs {
		if !gwconfig.VarName.MatchString(r.Name) {
			return fmt.Errorf("${%s} is not a valid name: use letters, digits and _, and do not start with a digit", r.Name)
		}
	}
	return nil
}

type requiredName struct {
	Name     string   `json:"name"`
	Optional bool     `json:"optional" doc:"True when every reference gives a default, as in ${NAME:default}."`
	UsedBy   []string `json:"used_by" doc:"kind/name of each resource that references the name, sorted."`
}

type requiredNamesBody struct {
	Names []requiredName `json:"names" doc:"Sorted by name."`
}

// requiredNames lists the ${NAME} references in resources.
func requiredNames(resources []resource) (*requiredNamesBody, error) {
	byName := map[string]*requiredName{}
	users := map[string]map[string]bool{}
	for _, r := range resources {
		data, err := yaml.Marshal(r.Body)
		if err != nil {
			return nil, err
		}
		refs, err := gwconfig.References(string(data))
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", r.Kind, r.Name, err)
		}
		for _, ref := range refs {
			n := byName[ref.Name]
			if n == nil {
				n = &requiredName{Name: ref.Name, Optional: true}
				byName[ref.Name] = n
				users[ref.Name] = map[string]bool{}
			}
			n.Optional = n.Optional && ref.HasDefault
			users[ref.Name][r.Kind+"/"+r.Name] = true
		}
	}
	out := &requiredNamesBody{Names: []requiredName{}}
	for name, n := range byName {
		for u := range users[name] {
			n.UsedBy = append(n.UsedBy, u)
		}
		sort.Strings(n.UsedBy)
		out.Names = append(out.Names, *n)
	}
	sort.Slice(out.Names, func(i, j int) bool { return out.Names[i].Name < out.Names[j].Name })
	return out, nil
}

func (s *Server) registerSecrets() {
	viewer := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleViewer)}
	tags := []string{"secrets"}
	const desc = "Lists each ${NAME} reference. The gateway resolves the names from its environment; Studio stores no value."

	huma.Register(s.api, huma.Operation{
		OperationID: "listDraftSecretNames", Method: http.MethodGet, Path: "/api/v1/environments/{env}/secrets",
		Summary: "List the ${NAME} references in the draft", Description: desc, Tags: tags, Middlewares: viewer,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
	}) (*struct{ Body requiredNamesBody }, error) {
		if err := s.requireEnvironment(ctx, in.Environment); err != nil {
			return nil, err
		}
		resources, err := s.draftResources(ctx, in.Environment)
		if err != nil {
			return nil, err
		}
		body, err := requiredNames(resources)
		if err != nil {
			return nil, err
		}
		return &struct{ Body requiredNamesBody }{Body: *body}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "listVersionSecretNames", Method: http.MethodGet, Path: "/api/v1/environments/{env}/versions/{number}/secrets",
		Summary: "List the ${NAME} references in an artifact version", Description: desc, Tags: tags, Middlewares: viewer,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
		Number      int    `path:"number" minimum:"1"`
	}) (*struct{ Body requiredNamesBody }, error) {
		v, err := s.version(ctx, in.Environment, in.Number)
		if err != nil {
			return nil, err
		}
		resources, err := splitArtifact([]byte(v.YAML))
		if err != nil {
			return nil, err
		}
		body, err := requiredNames(resources)
		if err != nil {
			return nil, err
		}
		return &struct{ Body requiredNamesBody }{Body: *body}, nil
	})
}
