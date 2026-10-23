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
	"sort"
	"time"

	"github.com/alternayte/auth-all/humaauth"
	"github.com/danielgtaylor/huma/v2"

	"github.com/alternayte/restitch-gateway/v2/internal/admin"
)

// defaultUsageRetention is how long Studio keeps usage of an environment
// unless its usage_retention_days says otherwise.
const defaultUsageRetention = 7 * 24 * time.Hour

// UsagePush is one push of one gateway instance: the minute buckets it
// accumulated and the state of each upstream's circuit breaker.
type UsagePush struct {
	InstanceID string            `json:"instance_id"`
	Buckets    []admin.Bucket    `json:"buckets"`
	Breakers   map[string]string `json:"breakers,omitempty"`
}

func (s *Server) serveUsage(w http.ResponseWriter, r *http.Request) {
	env, ok := s.requireGatewayToken(w, r)
	if !ok {
		return
	}
	var push UsagePush
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&push); err != nil || push.InstanceID == "" {
		http.Error(w, "a usage push needs an instance_id and buckets", http.StatusBadRequest)
		return
	}
	if err := s.storeUsage(r.Context(), env, push); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) storeUsage(ctx context.Context, env string, push UsagePush) error {
	tx, err := s.db.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, b := range push.Buckets {
		if b.Requests == 0 {
			continue
		}
		latency, _ := json.Marshal(b.LatencyBuckets)
		steps, _ := json.Marshal(b.StepMetrics)
		// A gateway that retries a push sends the same minute again; the
		// newer row replaces the older one.
		if _, err := tx.ExecContext(ctx, s.db.rebind(`
			INSERT INTO studio_usage (environment, instance_id, minute, composition, requests, errors, partials, latency_buckets, steps)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (environment, instance_id, minute, composition) DO UPDATE SET
				requests = excluded.requests, errors = excluded.errors, partials = excluded.partials,
				latency_buckets = excluded.latency_buckets, steps = excluded.steps`),
			env, push.InstanceID, b.Timestamp.UTC().Truncate(time.Minute), b.Composition,
			b.Requests, b.Errors, b.Partials, string(latency), string(steps)); err != nil {
			return err
		}
	}
	if push.Breakers != nil {
		breakers, _ := json.Marshal(push.Breakers)
		now := s.now().UTC()
		if _, err := tx.ExecContext(ctx, s.db.rebind(`
			INSERT INTO studio_gateway_instances
				(environment, instance_id, binary_version, applied_version, last_pull_error, first_seen_at, last_seen_at, breakers)
			VALUES (?, ?, '', 0, '', ?, ?, ?)
			ON CONFLICT (environment, instance_id) DO UPDATE SET breakers = excluded.breakers`),
			env, push.InstanceID, now, now, string(breakers)); err != nil {
			return err
		}
	}

	var days int
	if err := tx.QueryRowContext(ctx, s.db.rebind(
		`SELECT usage_retention_days FROM studio_environments WHERE name = ?`), env).Scan(&days); err != nil {
		return err
	}
	cutoff := s.now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	if _, err := tx.ExecContext(ctx, s.db.rebind(
		`DELETE FROM studio_usage WHERE environment = ? AND minute < ?`), env, cutoff); err != nil {
		return err
	}
	return tx.Commit()
}

type usageStep struct {
	Upstream  string  `json:"upstream"`
	Requests  int64   `json:"requests"`
	Errors    int64   `json:"errors"`
	CacheHits int64   `json:"cache_hits"`
	AvgMS     float64 `json:"avg_ms" doc:"Mean across instances, weighted by requests."`
}

type usageBucket struct {
	Timestamp      time.Time            `json:"timestamp"`
	Requests       int64                `json:"requests"`
	Errors         int64                `json:"errors"`
	Partials       int64                `json:"partials"`
	LatencyBuckets []int64              `json:"latency_buckets" doc:"Request counts per latency bound in latency_bounds_ms, plus one overflow bucket."`
	P95MS          float64              `json:"p95_ms" doc:"Upper bound of the latency bucket that holds the 95th percentile."`
	Steps          map[string]usageStep `json:"steps"`
}

type usageUpstream struct {
	Name     string            `json:"name"`
	Requests int64             `json:"requests"`
	Errors   int64             `json:"errors"`
	Breakers map[string]string `json:"breakers" doc:"Breaker state per gateway instance."`
}

type usageBody struct {
	LatencyBoundsMS []float64       `json:"latency_bounds_ms"`
	Buckets         []usageBucket   `json:"buckets"`
	Upstreams       []usageUpstream `json:"upstreams"`
}

func (s *Server) registerUsage() {
	viewer := huma.Middlewares{humaauth.RequireRole(s.api, s.auth, RoleViewer)}
	huma.Register(s.api, huma.Operation{
		OperationID: "getUsage", Method: http.MethodGet, Path: "/api/v1/environments/{env}/usage",
		Summary: "Usage per minute, summed across the environment's gateway instances",
		Tags:    []string{"usage"}, Middlewares: viewer,
	}, func(ctx context.Context, in *struct {
		Environment string    `path:"env"`
		Composition string    `query:"composition" doc:"Empty selects the gateway-wide total."`
		From        time.Time `query:"from" doc:"Default: one hour ago."`
		To          time.Time `query:"to" doc:"Default: now."`
	}) (*struct{ Body usageBody }, error) {
		if err := s.requireEnvironment(ctx, in.Environment); err != nil {
			return nil, err
		}
		to, from := in.To, in.From
		if to.IsZero() {
			to = s.now()
		}
		if from.IsZero() {
			from = to.Add(-time.Hour)
		}
		body, err := s.usage(ctx, in.Environment, in.Composition, from.UTC(), to.UTC())
		if err != nil {
			return nil, err
		}
		return &struct{ Body usageBody }{Body: *body}, nil
	})
}

func (s *Server) usage(ctx context.Context, env, comp string, from, to time.Time) (*usageBody, error) {
	rows, err := s.db.db.QueryContext(ctx, s.db.rebind(`
		SELECT minute, requests, errors, partials, latency_buckets, steps FROM studio_usage
		WHERE environment = ? AND composition = ? AND minute >= ? AND minute <= ? ORDER BY minute`),
		env, comp, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byMinute := map[time.Time]*usageBucket{}
	upstreams := map[string]*usageUpstream{}
	for rows.Next() {
		var minute time.Time
		var requests, errors, partials int64
		var latencyJSON, stepsJSON string
		if err := rows.Scan(&minute, &requests, &errors, &partials, &latencyJSON, &stepsJSON); err != nil {
			return nil, err
		}
		minute = minute.UTC()
		b, ok := byMinute[minute]
		if !ok {
			b = &usageBucket{Timestamp: minute, LatencyBuckets: make([]int64, len(admin.LatencyBucketBounds)+1), Steps: map[string]usageStep{}}
			byMinute[minute] = b
		}
		b.Requests += requests
		b.Errors += errors
		b.Partials += partials
		var latency []int64
		_ = json.Unmarshal([]byte(latencyJSON), &latency)
		for i := range latency {
			if i < len(b.LatencyBuckets) {
				b.LatencyBuckets[i] += latency[i]
			}
		}
		var steps map[string]*admin.StepBucket
		_ = json.Unmarshal([]byte(stepsJSON), &steps)
		for name, st := range steps {
			cur := b.Steps[name]
			total := cur.Requests + st.Requests
			if total > 0 {
				cur.AvgMS = (cur.AvgMS*float64(cur.Requests) + st.AvgMS*float64(st.Requests)) / float64(total)
			}
			cur.Upstream, cur.Requests, cur.Errors, cur.CacheHits = st.Upstream, total, cur.Errors+st.Errors, cur.CacheHits+st.CacheHits
			b.Steps[name] = cur

			u, ok := upstreams[st.Upstream]
			if !ok {
				u = &usageUpstream{Name: st.Upstream, Breakers: map[string]string{}}
				upstreams[st.Upstream] = u
			}
			u.Requests += st.Requests
			u.Errors += st.Errors
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := s.attachBreakers(ctx, env, upstreams); err != nil {
		return nil, err
	}
	out := &usageBody{LatencyBoundsMS: admin.LatencyBucketBounds, Buckets: []usageBucket{}, Upstreams: []usageUpstream{}}
	for _, b := range byMinute {
		b.P95MS = histogramPercentile(b.LatencyBuckets, 0.95)
		out.Buckets = append(out.Buckets, *b)
	}
	sort.Slice(out.Buckets, func(i, j int) bool { return out.Buckets[i].Timestamp.Before(out.Buckets[j].Timestamp) })
	for _, u := range upstreams {
		out.Upstreams = append(out.Upstreams, *u)
	}
	sort.Slice(out.Upstreams, func(i, j int) bool { return out.Upstreams[i].Name < out.Upstreams[j].Name })
	return out, nil
}

// attachBreakers adds each live instance's breaker state to the upstreams.
func (s *Server) attachBreakers(ctx context.Context, env string, upstreams map[string]*usageUpstream) error {
	rows, err := s.db.db.QueryContext(ctx, s.db.rebind(
		`SELECT instance_id, breakers, last_seen_at FROM studio_gateway_instances WHERE environment = ?`), env)
	if err != nil {
		return err
	}
	defer rows.Close()
	now := s.now()
	for rows.Next() {
		var id, raw string
		var seen time.Time
		if err := rows.Scan(&id, &raw, &seen); err != nil {
			return err
		}
		if now.Sub(seen) >= staleAfter {
			continue
		}
		var states map[string]string
		_ = json.Unmarshal([]byte(raw), &states)
		for name, state := range states {
			if u, ok := upstreams[name]; ok {
				u.Breakers[id] = state
			}
		}
	}
	return rows.Err()
}

// histogramPercentile returns the upper bound of the bucket that holds the
// p-th quantile; the overflow bucket reports the last bound.
func histogramPercentile(counts []int64, p float64) float64 {
	var total int64
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		return 0
	}
	target := int64(float64(total)*p + 0.5)
	var seen int64
	for i, c := range counts {
		seen += c
		if seen >= target {
			if i < len(admin.LatencyBucketBounds) {
				return admin.LatencyBucketBounds[i]
			}
			break
		}
	}
	return admin.LatencyBucketBounds[len(admin.LatencyBucketBounds)-1]
}
