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

	"github.com/alternayte/restitch-gateway/v2/internal/reqlog"
)

// RequestsPush is one push of sampled requests from one instance.
type RequestsPush struct {
	InstanceID string          `json:"instance_id"`
	Requests   []reqlog.Record `json:"requests"`
}

func failedRequest(r reqlog.Record) bool { return r.Status >= 500 || r.Partial }

func (s *Server) serveRequests(w http.ResponseWriter, r *http.Request) {
	env, ok := s.requireGatewayToken(w, r)
	if !ok {
		return
	}
	var push RequestsPush
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&push); err != nil || push.InstanceID == "" {
		http.Error(w, "a requests push needs an instance_id", http.StatusBadRequest)
		return
	}
	if err := s.storeRequests(r.Context(), env, push); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) storeRequests(ctx context.Context, env string, push RequestsPush) error {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var capPerMinute, retentionDays int
	if err := tx.QueryRowContext(ctx, s.db.rebind(
		`SELECT request_samples_per_minute, usage_retention_days FROM studio_environments WHERE name = ?`), env).
		Scan(&capPerMinute, &retentionDays); err != nil {
		return err
	}
	for _, rec := range push.Requests {
		if rec.ID == "" {
			continue
		}
		// Redact again: Studio does not trust a sender to have done it.
		rec = rec.Redact()
		at := rec.Time.UTC()
		minute := at.Truncate(time.Minute)
		failed := failedRequest(rec)
		if !failed {
			var kept int
			if err := tx.QueryRowContext(ctx, s.db.rebind(
				`SELECT COUNT(*) FROM studio_requests WHERE environment = ? AND minute = ? AND failed = 0`),
				env, minute).Scan(&kept); err != nil {
				return err
			}
			if kept >= capPerMinute {
				continue
			}
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.rebind(`
			INSERT INTO studio_requests (environment, instance_id, request_id, at, minute, composition, status, failed, duration_ms, record)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (environment, instance_id, request_id) DO NOTHING`),
			env, push.InstanceID, rec.ID, at, minute, rec.Composition, rec.Status, boolInt(failed), rec.DurationMS, string(data)); err != nil {
			return err
		}
	}
	cutoff := s.now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	if _, err := tx.ExecContext(ctx, s.db.rebind(`DELETE FROM studio_requests WHERE environment = ? AND at < ?`), env, cutoff); err != nil {
		return err
	}
	return tx.Commit()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

type sampledRequest struct {
	reqlog.Record
	InstanceID string `json:"instance_id"`
}

type environmentSettings struct {
	UsageRetentionDays      int `json:"usage_retention_days" minimum:"1" maximum:"365"`
	RequestSamplesPerMinute int `json:"request_samples_per_minute" minimum:"0" maximum:"1000" doc:"Successful requests kept per minute; failed ones are always kept."`
}

func (s *Server) registerRequests() {
	viewer := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleViewer)}
	owner := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleOwner)}

	huma.Register(s.api, huma.Operation{
		OperationID: "listRequests", Method: http.MethodGet, Path: "/api/v1/environments/{env}/requests",
		Summary: "Sampled requests, newest first", Tags: []string{"usage"}, Middlewares: viewer,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
		Composition string `query:"composition"`
		Failed      bool   `query:"failed" doc:"Only failed and partial requests."`
		Limit       int    `query:"limit" minimum:"1" maximum:"500" default:"50"`
	}) (*struct {
		Body struct {
			Requests []sampledRequest `json:"requests"`
		}
	}, error) {
		if err := s.requireEnvironment(ctx, in.Environment); err != nil {
			return nil, err
		}
		q := `SELECT instance_id, record FROM studio_requests WHERE environment = ?`
		args := []any{in.Environment}
		if in.Composition != "" {
			q += ` AND composition = ?`
			args = append(args, in.Composition)
		}
		if in.Failed {
			q += ` AND failed = 1`
		}
		q += ` ORDER BY at DESC LIMIT ?`
		args = append(args, in.Limit)
		rows, err := s.db.db.QueryContext(ctx, s.db.rebind(q), args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := &struct {
			Body struct {
				Requests []sampledRequest `json:"requests"`
			}
		}{}
		out.Body.Requests = []sampledRequest{}
		for rows.Next() {
			var sr sampledRequest
			var raw string
			if err := rows.Scan(&sr.InstanceID, &raw); err != nil {
				return nil, err
			}
			if err := json.Unmarshal([]byte(raw), &sr.Record); err != nil {
				return nil, err
			}
			out.Body.Requests = append(out.Body.Requests, sr)
		}
		return out, rows.Err()
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "getEnvironmentSettings", Method: http.MethodGet, Path: "/api/v1/environments/{env}",
		Summary: "An environment's telemetry settings", Tags: []string{"draft"}, Middlewares: viewer,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
	}) (*struct{ Body environmentSettings }, error) {
		st, err := s.settings(ctx, in.Environment)
		if err != nil {
			return nil, err
		}
		return &struct{ Body environmentSettings }{Body: *st}, nil
	})

	huma.Register(s.api, huma.Operation{
		OperationID: "updateEnvironmentSettings", Method: http.MethodPatch, Path: "/api/v1/environments/{env}",
		Summary: "Change an environment's telemetry settings", Tags: []string{"draft"}, Middlewares: owner,
	}, func(ctx context.Context, in *struct {
		Environment string `path:"env"`
		Body        struct {
			UsageRetentionDays      *int `json:"usage_retention_days,omitempty" minimum:"1" maximum:"365"`
			RequestSamplesPerMinute *int `json:"request_samples_per_minute,omitempty" minimum:"0" maximum:"1000"`
		}
	}) (*struct{ Body environmentSettings }, error) {
		st, err := s.settings(ctx, in.Environment)
		if err != nil {
			return nil, err
		}
		if in.Body.UsageRetentionDays != nil {
			st.UsageRetentionDays = *in.Body.UsageRetentionDays
		}
		if in.Body.RequestSamplesPerMinute != nil {
			st.RequestSamplesPerMinute = *in.Body.RequestSamplesPerMinute
		}
		if _, err := s.db.db.ExecContext(ctx, s.db.rebind(`
			UPDATE studio_environments SET usage_retention_days = ?, request_samples_per_minute = ? WHERE name = ?`),
			st.UsageRetentionDays, st.RequestSamplesPerMinute, in.Environment); err != nil {
			return nil, err
		}
		return &struct{ Body environmentSettings }{Body: *st}, nil
	})
}

func (s *Server) settings(ctx context.Context, env string) (*environmentSettings, error) {
	var st environmentSettings
	err := s.db.db.QueryRowContext(ctx, s.db.rebind(
		`SELECT usage_retention_days, request_samples_per_minute FROM studio_environments WHERE name = ?`), env).
		Scan(&st.UsageRetentionDays, &st.RequestSamplesPerMinute)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, huma.Error404NotFound("environment not found")
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}
