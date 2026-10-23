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
	"net/http"
	"time"

	"github.com/alternayte/auth-all/humaauth"
	"github.com/danielgtaylor/huma/v2"
)

type meOutput struct {
	Body struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name,omitempty"`
		Role  string `json:"role" enum:"viewer,editor,owner"`
	}
}

type preferencesBody struct {
	Theme           string   `json:"theme,omitempty" enum:"light,dark,system"`
	PinnedNames     []string `json:"pinned_names,omitempty" maxItems:"100"`
	SidebarClosed   bool     `json:"sidebar_closed,omitempty"`
	DefaultRange    string   `json:"default_range,omitempty" maxLength:"16"`
	LastEnvironment string   `json:"last_environment,omitempty" maxLength:"128"`
}

type preferencesInput struct {
	Body preferencesBody
}

type preferencesOutput struct {
	Body preferencesBody
}

func (s *Server) registerMe() {
	viewer := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleViewer)}

	huma.Register(s.api, huma.Operation{
		OperationID: "getMe",
		Method:      http.MethodGet,
		Path:        "/api/v1/me",
		Summary:     "The signed-in user and role",
		Tags:        []string{"me"},
		Middlewares: viewer,
	}, func(ctx context.Context, _ *struct{}) (*meOutput, error) {
		p, err := principal(ctx)
		if err != nil {
			return nil, err
		}
		out := &meOutput{}
		out.Body.ID = p.User.ID
		out.Body.Email = p.User.Email
		out.Body.Name = p.User.DisplayName
		out.Body.Role = p.Role
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "getPreferences",
		Method:      http.MethodGet,
		Path:        "/api/v1/me/preferences",
		Summary:     "The signed-in user's UI preferences",
		Tags:        []string{"me"},
		Middlewares: viewer,
	}, func(ctx context.Context, _ *struct{}) (*preferencesOutput, error) {
		p, err := principal(ctx)
		if err != nil {
			return nil, err
		}
		var raw string
		err = s.db.db.QueryRowContext(ctx, s.db.rebind(
			`SELECT data FROM studio_user_preferences WHERE user_id = ?`), p.User.ID).Scan(&raw)
		out := &preferencesOutput{}
		if errors.Is(err, sql.ErrNoRows) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &out.Body); err != nil {
			return nil, err
		}
		return out, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "putPreferences",
		Method:      http.MethodPut,
		Path:        "/api/v1/me/preferences",
		Summary:     "Replace the signed-in user's UI preferences",
		Tags:        []string{"me"},
		Middlewares: viewer,
	}, func(ctx context.Context, in *preferencesInput) (*preferencesOutput, error) {
		p, err := principal(ctx)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(in.Body)
		if err != nil {
			return nil, err
		}
		_, err = s.db.db.ExecContext(ctx, s.db.rebind(`
			INSERT INTO studio_user_preferences (user_id, data, updated_at) VALUES (?, ?, ?)
			ON CONFLICT (user_id) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`),
			p.User.ID, string(data), time.Now().UTC())
		if err != nil {
			return nil, err
		}
		return &preferencesOutput{Body: in.Body}, nil
	})
}
