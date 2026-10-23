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
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alternayte/auth-all/humaauth"
	"github.com/danielgtaylor/huma/v2"
	"github.com/pmezard/go-difflib/difflib"
	"gopkg.in/yaml.v3"

	"github.com/alternayte/restitch-gateway/v2/internal/composition"
	"github.com/alternayte/restitch-gateway/v2/internal/gwconfig"
)

// secretPlaceholder stands in for every ${VAR} when Studio validates an
// artifact. Secrets resolve on the gateway, so Studio never sees them. A
// ${VAR} therefore belongs in a string field, and a URL writes its scheme
// out: https://${HOST} validates, a bare ${URL} cannot.
const secretPlaceholder = "x"

// validateArtifact runs the gateway validator on an unexpanded artifact.
func validateArtifact(ctx context.Context, doc []byte) error {
	expanded, err := gwconfig.ExpandWith(string(doc), func(string) (string, bool) { return secretPlaceholder, true })
	if err != nil {
		return err
	}
	if err := composition.Validate(ctx, []byte(expanded)); err != nil {
		if strings.Contains(err.Error(), `url scheme must be http or https, got ""`) {
			return fmt.Errorf("%w (a url that holds ${VAR} writes its scheme out, for example https://${HOST})", err)
		}
		return err
	}
	return nil
}

// assembleArtifact renders the draft as the artifact YAML that standalone
// mode reads. Map keys marshal in sorted order, so the same draft always
// renders the same bytes.
func assembleArtifact(resources []resource) ([]byte, error) {
	doc := map[string]any{}
	upstreams := map[string]any{}
	compositions := map[string]any{}
	for _, r := range resources {
		switch r.Kind {
		case KindUpstream:
			upstreams[r.Name] = r.Body
		case KindComposition:
			compositions[r.Name] = r.Body
		case KindAuth:
			doc["auth"] = r.Body
		case KindRateLimit:
			doc["rate_limit"] = r.Body
		case KindSettings:
			for k, v := range r.Body {
				doc[k] = v
			}
		}
	}
	if len(upstreams) > 0 {
		doc["upstreams"] = upstreams
	}
	if len(compositions) > 0 {
		doc["compositions"] = compositions
	}
	return yaml.Marshal(doc)
}

type artifactVersion struct {
	Number    int       `json:"number"`
	YAML      string    `json:"yaml,omitempty"`
	Author    string    `json:"author"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type previewBody struct {
	Valid          bool     `json:"valid"`
	Errors         []string `json:"errors"`
	YAML           string   `json:"yaml"`
	Diff           string   `json:"diff" doc:"Unified diff from the current version to the draft. Empty when they match."`
	CurrentVersion int      `json:"current_version" doc:"0 when the environment has no version yet."`
}

func (s *Server) registerPublish() {
	viewer := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleViewer)}
	editor := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleEditor)}
	tags := []string{"publish"}

	huma.Register(s.api, huma.Operation{
		OperationID: "previewPublish", Method: http.MethodPost, Path: "/api/v1/environments/{env}/publish/preview",
		Summary: "Validate the draft and diff it against the current version", Tags: tags, Middlewares: editor,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
	}) (*struct{ Body previewBody }, error) {
		pv, err := s.preview(ctx, in.Environment)
		if err != nil {
			return nil, err
		}
		return &struct{ Body previewBody }{Body: *pv}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "publish", Method: http.MethodPost, Path: "/api/v1/environments/{env}/publish",
		Summary: "Publish the draft as a new artifact version",
		Description: "Validates the whole draft with the gateway validator. An invalid draft gets 422 and writes " +
			"nothing. A valid draft becomes the next numbered version, which gateways pull.",
		Tags: tags, Middlewares: editor,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
		Body        struct {
			Message string `json:"message" minLength:"1" maxLength:"1000"`
		}
	}) (*struct{ Body artifactVersion }, error) {
		p, err := principal(ctx)
		if err != nil {
			return nil, err
		}
		pv, err := s.preview(ctx, in.Environment)
		if err != nil {
			return nil, err
		}
		if !pv.Valid {
			return nil, huma.Error422UnprocessableEntity("the draft is not valid: " + strings.Join(pv.Errors, "; "))
		}
		v, err := s.writeVersion(ctx, in.Environment, []byte(pv.YAML), p.User.Email, in.Body.Message)
		if err != nil {
			return nil, err
		}
		return &struct{ Body artifactVersion }{Body: *v}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "listVersions", Method: http.MethodGet, Path: "/api/v1/environments/{env}/versions",
		Summary: "List artifact versions, newest first", Tags: tags, Middlewares: viewer,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
	}) (*struct {
		Body struct {
			Versions []artifactVersion `json:"versions"`
		}
	}, error) {
		if err := s.requireEnvironment(ctx, in.Environment); err != nil {
			return nil, err
		}
		rows, err := s.db.db.QueryContext(ctx, s.db.rebind(
			`SELECT number, author, message, created_at FROM studio_artifact_versions WHERE environment = ? ORDER BY number DESC`),
			in.Environment)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := &struct {
			Body struct {
				Versions []artifactVersion `json:"versions"`
			}
		}{}
		out.Body.Versions = []artifactVersion{}
		for rows.Next() {
			var v artifactVersion
			if err := rows.Scan(&v.Number, &v.Author, &v.Message, &v.CreatedAt); err != nil {
				return nil, err
			}
			out.Body.Versions = append(out.Body.Versions, v)
		}
		return out, rows.Err()
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "getVersion", Method: http.MethodGet, Path: "/api/v1/environments/{env}/versions/{number}",
		Summary: "Read one artifact version with its YAML", Tags: tags, Middlewares: viewer,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
		Number      int    `path:"number" minimum:"1"`
	}) (*struct{ Body artifactVersion }, error) {
		v, err := s.version(ctx, in.Environment, in.Number)
		if err != nil {
			return nil, err
		}
		return &struct{ Body artifactVersion }{Body: *v}, nil
	})
}

func (s *Server) preview(ctx context.Context, env string) (*previewBody, error) {
	if err := s.requireEnvironment(ctx, env); err != nil {
		return nil, err
	}
	resources, err := s.draftResources(ctx, env)
	if err != nil {
		return nil, err
	}
	doc, err := assembleArtifact(resources)
	if err != nil {
		return nil, err
	}
	pv := &previewBody{Valid: true, Errors: []string{}, YAML: string(doc)}
	if len(doc) > composition.MaxArtifactBytes {
		pv.Valid = false
		pv.Errors = append(pv.Errors, fmt.Sprintf("the artifact is %d bytes; gateways read at most %d", len(doc), composition.MaxArtifactBytes))
	} else if err := validateArtifact(ctx, doc); err != nil {
		pv.Valid = false
		pv.Errors = append(pv.Errors, err.Error())
	}

	current := ""
	if latest, err := s.latestVersion(ctx, env); err != nil {
		return nil, err
	} else if latest != nil {
		pv.CurrentVersion = latest.Number
		current = latest.YAML
	}
	pv.Diff, err = difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A: difflib.SplitLines(current), B: difflib.SplitLines(pv.YAML),
		FromFile: fmt.Sprintf("version %d", pv.CurrentVersion), ToFile: "draft", Context: 3,
	})
	if err != nil {
		return nil, err
	}
	return pv, nil
}

// writeVersion appends the next numbered version. The primary key on
// (environment, number) refuses a concurrent publish that read the same
// latest number; the caller retries.
func (s *Server) writeVersion(ctx context.Context, env string, doc []byte, author, message string) (*artifactVersion, error) {
	for attempt := 0; attempt < 3; attempt++ {
		var next int
		if err := s.db.db.QueryRowContext(ctx, s.db.rebind(
			`SELECT COALESCE(MAX(number), 0) + 1 FROM studio_artifact_versions WHERE environment = ?`), env).Scan(&next); err != nil {
			return nil, err
		}
		v := artifactVersion{Number: next, YAML: string(doc), Author: author, Message: message, CreatedAt: time.Now().UTC()}
		res, err := s.db.db.ExecContext(ctx, s.db.rebind(`
			INSERT INTO studio_artifact_versions (environment, number, yaml, author, message, created_at)
			VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (environment, number) DO NOTHING`),
			env, v.Number, v.YAML, v.Author, v.Message, v.CreatedAt)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			return &v, nil
		}
	}
	return nil, huma.Error409Conflict("another publish to this environment ran at the same time: publish again")
}

func (s *Server) latestVersion(ctx context.Context, env string) (*artifactVersion, error) {
	var n sql.NullInt64
	if err := s.db.db.QueryRowContext(ctx, s.db.rebind(
		`SELECT MAX(number) FROM studio_artifact_versions WHERE environment = ?`), env).Scan(&n); err != nil {
		return nil, err
	}
	if !n.Valid {
		return nil, nil
	}
	return s.version(ctx, env, int(n.Int64))
}

func (s *Server) version(ctx context.Context, env string, number int) (*artifactVersion, error) {
	v := artifactVersion{Number: number}
	err := s.db.db.QueryRowContext(ctx, s.db.rebind(
		`SELECT yaml, author, message, created_at FROM studio_artifact_versions WHERE environment = ? AND number = ?`),
		env, number).Scan(&v.YAML, &v.Author, &v.Message, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound(fmt.Sprintf("version %d of %q not found", number, env))
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// artifactResources splits an artifact into draft resources, the inverse
// of assembleArtifact.
func artifactResources(doc []byte) ([]resource, error) {
	var top map[string]any
	if err := yaml.Unmarshal(doc, &top); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	var out []resource
	settings := map[string]any{}
	for key, val := range top {
		switch key {
		case "upstreams", "compositions":
			items, ok := val.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s must be a mapping", key)
			}
			kind := KindUpstream
			if key == "compositions" {
				kind = KindComposition
			}
			for name, body := range items {
				m, ok := body.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("%s.%s must be a mapping", key, name)
				}
				out = append(out, resource{Kind: kind, Name: name, Body: m})
			}
		case "auth", "rate_limit":
			m, ok := val.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s must be a mapping", key)
			}
			kind := KindAuth
			if key == "rate_limit" {
				kind = KindRateLimit
			}
			out = append(out, resource{Kind: kind, Name: singletonName, Body: m})
		default:
			settings[key] = val
		}
	}
	if len(settings) > 0 {
		out = append(out, resource{Kind: KindSettings, Name: singletonName, Body: settings})
	}
	for _, r := range out {
		if err := checkResource(r.Kind, r.Name, r.Body); err != nil {
			return nil, fmt.Errorf("%s %s: %w", r.Kind, r.Name, err)
		}
	}
	return out, nil
}

// replaceDraft makes resources the whole draft of env in one transaction.
// A resource that existed keeps counting revisions up, so a tab that holds
// an older revision still gets 409.
func (s *Server) replaceDraft(ctx context.Context, env string, resources []resource, by string) error {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	revisions := map[string]int{}
	rows, err := tx.QueryContext(ctx, s.db.rebind(`SELECT kind, name, revision FROM studio_resources WHERE environment = ?`), env)
	if err != nil {
		return err
	}
	for rows.Next() {
		var kind, name string
		var rev int
		if err := rows.Scan(&kind, &name, &rev); err != nil {
			rows.Close()
			return err
		}
		revisions[kind+"/"+name] = rev
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, s.db.rebind(`DELETE FROM studio_resources WHERE environment = ?`), env); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, r := range resources {
		data, err := json.Marshal(r.Body)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.rebind(`
			INSERT INTO studio_resources (environment, kind, name, revision, body, updated_at, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?)`),
			env, r.Kind, r.Name, revisions[r.Kind+"/"+r.Name]+1, string(data), now, by); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Server) registerHistory() {
	editor := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleEditor)}
	tags := []string{"publish"}

	huma.Register(s.api, huma.Operation{
		OperationID: "rollback", Method: http.MethodPost, Path: "/api/v1/environments/{env}/versions/{number}/rollback",
		Summary:     "Publish an old version again as a new version",
		Description: "History stays append-only: version N's YAML becomes the next version. The draft does not change.",
		Tags:        tags, Middlewares: editor,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
		Number      int    `path:"number" minimum:"1"`
	}) (*struct{ Body artifactVersion }, error) {
		p, err := principal(ctx)
		if err != nil {
			return nil, err
		}
		old, err := s.version(ctx, in.Environment, in.Number)
		if err != nil {
			return nil, err
		}
		if err := validateArtifact(ctx, []byte(old.YAML)); err != nil {
			return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("version %d no longer validates: %v", in.Number, err))
		}
		v, err := s.writeVersion(ctx, in.Environment, []byte(old.YAML), p.User.Email, fmt.Sprintf("Roll back to version %d", in.Number))
		if err != nil {
			return nil, err
		}
		return &struct{ Body artifactVersion }{Body: *v}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "promote", Method: http.MethodPost, Path: "/api/v1/environments/{env}/promote",
		Summary:     "Copy a published version of another environment into this draft",
		Description: "Replaces this environment's draft with the version's resources and returns the diff. Publish to apply it.",
		Tags:        tags, Middlewares: editor,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
		Body        struct {
			FromEnvironment string `json:"from_environment"`
			Version         int    `json:"version" minimum:"1"`
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
		if err := s.requireEnvironment(ctx, in.Environment); err != nil {
			return nil, err
		}
		src, err := s.version(ctx, in.Body.FromEnvironment, in.Body.Version)
		if err != nil {
			return nil, err
		}
		resources, err := artifactResources([]byte(src.YAML))
		if err != nil {
			return nil, huma.Error422UnprocessableEntity(err.Error())
		}
		before, err := s.draftResources(ctx, in.Environment)
		if err != nil {
			return nil, err
		}
		beforeDoc, err := assembleArtifact(before)
		if err != nil {
			return nil, err
		}
		if err := s.replaceDraft(ctx, in.Environment, resources, p.User.Email); err != nil {
			return nil, err
		}
		afterDoc, err := assembleArtifact(resources)
		if err != nil {
			return nil, err
		}
		diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
			A: difflib.SplitLines(string(beforeDoc)), B: difflib.SplitLines(string(afterDoc)),
			FromFile: in.Environment + " draft", ToFile: fmt.Sprintf("%s version %d", in.Body.FromEnvironment, in.Body.Version), Context: 3,
		})
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
