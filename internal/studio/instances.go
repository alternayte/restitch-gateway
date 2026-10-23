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
	"time"

	"github.com/alternayte/auth-all/humaauth"
	"github.com/danielgtaylor/huma/v2"
)

// staleAfter marks an instance that sent no heartbeat for this long.
// Gateways send one every 15 seconds.
const staleAfter = time.Minute

// forgetAfter drops an instance from the list: replicas come and go, and
// one silent for a day is gone, not stale.
const forgetAfter = 24 * time.Hour

// Heartbeat is what a gateway instance reports about itself.
type Heartbeat struct {
	InstanceID     string `json:"instance_id"`
	BinaryVersion  string `json:"binary_version"`
	AppliedVersion int    `json:"applied_version"`
	LastPullError  string `json:"last_pull_error,omitempty"`
}

type instance struct {
	Heartbeat
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	Stale       bool      `json:"stale" doc:"No heartbeat for a minute or more."`
}

// serveHeartbeat records one heartbeat for the token's environment.
func (s *Server) serveHeartbeat(w http.ResponseWriter, r *http.Request) {
	env, ok := s.requireGatewayToken(w, r)
	if !ok {
		return
	}
	var hb Heartbeat
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&hb); err != nil || hb.InstanceID == "" || len(hb.InstanceID) > 200 {
		http.Error(w, "a heartbeat needs an instance_id", http.StatusBadRequest)
		return
	}
	now := s.now().UTC()
	_, err := s.db.db.ExecContext(r.Context(), s.db.rebind(`
		INSERT INTO studio_gateway_instances
			(environment, instance_id, binary_version, applied_version, last_pull_error, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (environment, instance_id) DO UPDATE SET
			binary_version = excluded.binary_version, applied_version = excluded.applied_version,
			last_pull_error = excluded.last_pull_error, last_seen_at = excluded.last_seen_at`),
		env, hb.InstanceID, hb.BinaryVersion, hb.AppliedVersion, hb.LastPullError, now, now)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) registerInstances() {
	viewer := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleViewer)}
	huma.Register(s.api, huma.Operation{
		OperationID: "listInstances", Method: http.MethodGet, Path: "/api/v1/environments/{env}/instances",
		Summary: "List the gateway instances of an environment and the version each applied",
		Tags:    []string{"instances"}, Middlewares: viewer,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
	}) (*struct {
		Body struct {
			CurrentVersion int        `json:"current_version"`
			Instances      []instance `json:"instances"`
		}
	}, error) {
		if err := s.requireEnvironment(ctx, in.Environment); err != nil {
			return nil, err
		}
		out := &struct {
			Body struct {
				CurrentVersion int        `json:"current_version"`
				Instances      []instance `json:"instances"`
			}
		}{}
		if latest, err := s.latestVersion(ctx, in.Environment); err != nil {
			return nil, err
		} else if latest != nil {
			out.Body.CurrentVersion = latest.Number
		}
		rows, err := s.db.db.QueryContext(ctx, s.db.rebind(`
			SELECT instance_id, binary_version, applied_version, last_pull_error, first_seen_at, last_seen_at
			FROM studio_gateway_instances WHERE environment = ? AND last_seen_at >= ? ORDER BY instance_id`),
			in.Environment, s.now().UTC().Add(-forgetAfter))
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out.Body.Instances = []instance{}
		now := s.now()
		for rows.Next() {
			var i instance
			if err := rows.Scan(&i.InstanceID, &i.BinaryVersion, &i.AppliedVersion, &i.LastPullError, &i.FirstSeenAt, &i.LastSeenAt); err != nil {
				return nil, err
			}
			i.Stale = now.Sub(i.LastSeenAt) >= staleAfter
			out.Body.Instances = append(out.Body.Instances, i)
		}
		return out, rows.Err()
	})
}
