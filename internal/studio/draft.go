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
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/alternayte/auth-all/humaauth"
	"github.com/danielgtaylor/huma/v2"
	"gopkg.in/yaml.v3"

	"github.com/alternayte/restitch-gateway/v2/internal/composition"
	"github.com/alternayte/restitch-gateway/v2/internal/gwconfig"
)

// The resource kinds. auth, rate_limit and settings are singletons named
// "default"; upstreams and compositions carry their own names.
const (
	KindUpstream    = "upstream"
	KindComposition = "composition"
	KindAuth        = "auth"
	KindRateLimit   = "rate_limit"
	KindSettings    = "settings"
	singletonName   = "default"
)

var resourceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// settingsResource holds the artifact keys that belong to no other kind.
type settingsResource struct {
	BlockPrivateUpstreams bool `yaml:"block_private_upstreams"`
}

// checkResource decodes body strictly into the kind's artifact type and
// runs the checks that apply to the resource alone. Checks that span
// resources (upstream references, the auth policy for non-public routes)
// run when the draft is published.
func checkResource(kind, name string, body map[string]any) error {
	singleton := kind == KindAuth || kind == KindRateLimit || kind == KindSettings
	switch {
	case singleton && name != singletonName:
		return fmt.Errorf("%s is a single resource named %q", kind, singletonName)
	case !singleton && !resourceName.MatchString(name):
		return fmt.Errorf("name %q must match %s", name, resourceName)
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	if err := checkVarNames(string(data)); err != nil {
		return err
	}
	switch kind {
	case KindUpstream:
		return parseArtifact(map[string]any{"upstreams": map[string]any{name: body}})
	case KindComposition:
		var c composition.Composition
		return strictDecode(body, &c)
	case KindAuth:
		return parseArtifact(map[string]any{"auth": body})
	case KindRateLimit:
		return parseArtifact(map[string]any{"rate_limit": body})
	case KindSettings:
		var st settingsResource
		return strictDecode(body, &st)
	default:
		return fmt.Errorf("unknown resource kind %q", kind)
	}
}

// parseArtifact runs composition.ParseConfig on a one-resource artifact, so
// a resource meets the same rules the gateway applies.
func parseArtifact(doc map[string]any) error {
	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	expanded, err := gwconfig.ExpandWith(string(data), func(string) (string, bool) { return secretPlaceholder, true })
	if err != nil {
		return err
	}
	_, err = composition.ParseConfig([]byte(expanded))
	return err
}

func strictDecode(body map[string]any, out any) error {
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(out)
}

type environment struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
}

type resource struct {
	Kind      string         `json:"kind" enum:"upstream,composition,auth,rate_limit,settings"`
	Name      string         `json:"name"`
	Revision  int            `json:"revision" doc:"Increases by one on every save."`
	Body      map[string]any `json:"body"`
	UpdatedAt time.Time      `json:"updated_at"`
	UpdatedBy string         `json:"updated_by"`
}

// resourcePath addresses one resource. huma reads path parameters only
// from fields declared on the input struct itself, not from an embedded
// struct, so write operations repeat these fields.
type resourcePath struct {
	Environment string `path:"env"`
	Kind        string `path:"kind" enum:"upstream,composition,auth,rate_limit,settings"`
	Name        string `path:"name"`
}

func (s *Server) registerDraft() {
	viewer := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleViewer)}
	editor := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleEditor)}
	owner := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleOwner)}
	tags := []string{"draft"}

	huma.Register(s.api, huma.Operation{
		OperationID: "createEnvironment", Method: http.MethodPost, Path: "/api/v1/environments",
		Summary: "Create an environment with an empty draft", Tags: tags, Middlewares: owner,
	}, func(ctx context.Context, in *struct {
		Body struct {
			Name string `json:"name" pattern:"^[a-z][a-z0-9-]{0,62}$"`
		}
	}) (*struct{ Body environment }, error) {
		p, err := principal(ctx)
		if err != nil {
			return nil, err
		}
		env := environment{Name: in.Body.Name, CreatedAt: time.Now().UTC(), CreatedBy: p.User.Email}
		res, err := s.db.db.ExecContext(ctx, s.db.rebind(
			`INSERT INTO studio_environments (name, created_at, created_by) VALUES (?, ?, ?) ON CONFLICT (name) DO NOTHING`),
			env.Name, env.CreatedAt, env.CreatedBy)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, huma.Error409Conflict(fmt.Sprintf("environment %q exists", env.Name))
		}
		return &struct{ Body environment }{Body: env}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "listEnvironments", Method: http.MethodGet, Path: "/api/v1/environments",
		Summary: "List environments", Tags: tags, Middlewares: viewer,
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Environments []environment `json:"environments"`
		}
	}, error) {
		rows, err := s.db.db.QueryContext(ctx, `SELECT name, created_at, created_by FROM studio_environments ORDER BY name`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := &struct {
			Body struct {
				Environments []environment `json:"environments"`
			}
		}{}
		out.Body.Environments = []environment{}
		for rows.Next() {
			var e environment
			if err := rows.Scan(&e.Name, &e.CreatedAt, &e.CreatedBy); err != nil {
				return nil, err
			}
			out.Body.Environments = append(out.Body.Environments, e)
		}
		return out, rows.Err()
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "listResources", Method: http.MethodGet, Path: "/api/v1/environments/{env}/resources",
		Summary: "List the resources in an environment's draft", Tags: tags, Middlewares: viewer,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
	}) (*struct {
		Body struct {
			Resources []resource `json:"resources"`
		}
	}, error) {
		if err := s.requireEnvironment(ctx, in.Environment); err != nil {
			return nil, err
		}
		list, err := s.draftResources(ctx, in.Environment)
		if err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				Resources []resource `json:"resources"`
			}
		}{}
		out.Body.Resources = list
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "getResource", Method: http.MethodGet, Path: "/api/v1/environments/{env}/resources/{kind}/{name}",
		Summary: "Read one resource", Tags: tags, Middlewares: viewer,
	}, func(ctx context.Context, in *resourcePath) (*struct{ Body resource }, error) {
		r, err := s.getResource(ctx, in.Environment, in.Kind, in.Name)
		if err != nil {
			return nil, err
		}
		return &struct{ Body resource }{Body: *r}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "saveResource", Method: http.MethodPut, Path: "/api/v1/environments/{env}/resources/{kind}/{name}",
		Summary: "Create or update one resource",
		Description: "base_revision 0 creates the resource. Any other value updates it, and a value " +
			"that is not the current revision gets 409, so a stale copy never overwrites a newer save.",
		Tags: tags, Middlewares: editor,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
		Kind        string `path:"kind" enum:"upstream,composition,auth,rate_limit,settings"`
		Name        string `path:"name"`
		Body        struct {
			BaseRevision int            `json:"base_revision" minimum:"0"`
			Body         map[string]any `json:"body"`
		}
	}) (*struct{ Body resource }, error) {
		p, err := principal(ctx)
		if err != nil {
			return nil, err
		}
		if err := s.requireEnvironment(ctx, in.Environment); err != nil {
			return nil, err
		}
		if err := checkResource(in.Kind, in.Name, in.Body.Body); err != nil {
			return nil, huma.Error422UnprocessableEntity(err.Error())
		}
		r, err := s.saveResource(ctx, in.Environment, in.Kind, in.Name, in.Body.BaseRevision, in.Body.Body, p.User.Email)
		if err != nil {
			return nil, err
		}
		return &struct{ Body resource }{Body: *r}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteResource", Method: http.MethodDelete, Path: "/api/v1/environments/{env}/resources/{kind}/{name}",
		Summary: "Delete one resource", DefaultStatus: http.StatusNoContent, Tags: tags, Middlewares: editor,
	}, func(ctx context.Context, in *struct {
		Environment  string `path:"env"`
		Kind         string `path:"kind" enum:"upstream,composition,auth,rate_limit,settings"`
		Name         string `path:"name"`
		BaseRevision int    `query:"base_revision" required:"true" minimum:"1"`
	}) (*struct{}, error) {
		res, err := s.db.db.ExecContext(ctx, s.db.rebind(
			`DELETE FROM studio_resources WHERE environment = ? AND kind = ? AND name = ? AND revision = ?`),
			in.Environment, in.Kind, in.Name, in.BaseRevision)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, s.missingOrStale(ctx, in.Environment, in.Kind, in.Name)
		}
		return nil, nil
	})
}

func (s *Server) requireEnvironment(ctx context.Context, name string) error {
	var n int
	err := s.db.db.QueryRowContext(ctx, s.db.rebind(`SELECT COUNT(*) FROM studio_environments WHERE name = ?`), name).Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		return huma.Error404NotFound(fmt.Sprintf("environment %q not found", name))
	}
	return nil
}

func (s *Server) draftResources(ctx context.Context, env string) ([]resource, error) {
	rows, err := s.db.db.QueryContext(ctx, s.db.rebind(
		`SELECT kind, name, revision, body, updated_at, updated_by FROM studio_resources WHERE environment = ? ORDER BY kind, name`), env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []resource{}
	for rows.Next() {
		r, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *r)
	}
	return list, rows.Err()
}

func (s *Server) getResource(ctx context.Context, env, kind, name string) (*resource, error) {
	row := s.db.db.QueryRowContext(ctx, s.db.rebind(
		`SELECT kind, name, revision, body, updated_at, updated_by FROM studio_resources WHERE environment = ? AND kind = ? AND name = ?`),
		env, kind, name)
	r, err := scanResource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(fmt.Sprintf("%s %q not found in %q", kind, name, env))
	}
	return r, err
}

func (s *Server) saveResource(ctx context.Context, env, kind, name string, base int, body map[string]any, by string) (*resource, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var res sql.Result
	if base == 0 {
		// The primary key decides a concurrent create: one insert wins.
		res, err = s.db.db.ExecContext(ctx, s.db.rebind(`
			INSERT INTO studio_resources (environment, kind, name, revision, body, updated_at, updated_by)
			VALUES (?, ?, ?, 1, ?, ?, ?) ON CONFLICT (environment, kind, name) DO NOTHING`),
			env, kind, name, string(data), now, by)
	} else {
		res, err = s.db.db.ExecContext(ctx, s.db.rebind(`
			UPDATE studio_resources SET revision = revision + 1, body = ?, updated_at = ?, updated_by = ?
			WHERE environment = ? AND kind = ? AND name = ? AND revision = ?`),
			string(data), now, by, env, kind, name, base)
	}
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if base == 0 {
			return nil, huma.Error409Conflict(fmt.Sprintf("%s %q exists: reload it and save with its revision", kind, name))
		}
		return nil, s.missingOrStale(ctx, env, kind, name)
	}
	return s.getResource(ctx, env, kind, name)
}

// missingOrStale explains a write that matched no row.
func (s *Server) missingOrStale(ctx context.Context, env, kind, name string) error {
	if _, err := s.getResource(ctx, env, kind, name); err != nil {
		return err
	}
	return huma.Error409Conflict(fmt.Sprintf("%s %q changed since you loaded it: reload it and save again", kind, name))
}

type rowScanner interface{ Scan(dest ...any) error }

func scanResource(row rowScanner) (*resource, error) {
	var r resource
	var body string
	if err := row.Scan(&r.Kind, &r.Name, &r.Revision, &body, &r.UpdatedAt, &r.UpdatedBy); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(body), &r.Body); err != nil {
		return nil, err
	}
	return &r, nil
}
