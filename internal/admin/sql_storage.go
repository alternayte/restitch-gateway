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

package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (registers as "sqlite")

	"github.com/alternayte/restitch-gateway/internal/reqlog"
)

// SQLStorage is a Storage implementation backed by SQLite (or Turso, which
// speaks the same SQL over libSQL). It uses database/sql with the pure-Go
// modernc.org/sqlite driver, storing Bucket and reqlog.Record values as JSON
// blobs in TEXT columns for simplicity.
type SQLStorage struct {
	db        *sql.DB
	retention time.Duration
}

// NewSQLStorage opens (or creates) a SQLite database at dsn and ensures the
// schema exists. authToken is accepted for interface parity with Turso-style
// remote connections but is unused by the local sqlite driver.
func NewSQLStorage(dsn string, authToken string, retention time.Duration) (*SQLStorage, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS timeseries_buckets (
			timestamp   INTEGER NOT NULL,
			composition TEXT NOT NULL DEFAULT '',
			data        TEXT NOT NULL,
			PRIMARY KEY (timestamp, composition)
		);
		CREATE TABLE IF NOT EXISTS request_log (
			id          TEXT PRIMARY KEY,
			timestamp   INTEGER NOT NULL,
			composition TEXT NOT NULL,
			data        TEXT NOT NULL,
			created_at  INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_requests_ts ON request_log(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_requests_comp ON request_log(composition, timestamp DESC);
	`); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLStorage{db: db, retention: retention}, nil
}

func (s *SQLStorage) RecordBucket(ctx context.Context, bucket Bucket) error {
	data, err := json.Marshal(bucket)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO timeseries_buckets (timestamp, composition, data) VALUES (?, ?, ?)`,
		bucket.Timestamp.Unix(), bucket.Composition, string(data))
	return err
}

func (s *SQLStorage) QueryTimeSeries(ctx context.Context, from, to time.Time, resolution time.Duration, composition string) ([]Bucket, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data FROM timeseries_buckets WHERE timestamp >= ? AND timestamp < ? AND composition = ? ORDER BY timestamp`,
		from.Unix(), to.Unix(), composition)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]Bucket, 0)
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var b Bucket
		if err := json.Unmarshal([]byte(data), &b); err != nil {
			continue
		}
		results = append(results, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if resolution > time.Minute {
		results = aggregateBuckets(results, resolution)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.Before(results[j].Timestamp)
	})

	return results, nil
}

func (s *SQLStorage) RecordRequest(ctx context.Context, record reqlog.Record) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	// DO NOTHING, not INSERT OR REPLACE: a client-supplied X-Request-ID must
	// not be able to overwrite a stored request record (finding M5). The
	// first record for an ID wins; collisions on generated ULIDs do not
	// occur in practice.
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO request_log (id, timestamp, composition, data, created_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO NOTHING`,
		record.ID, record.Time.Unix(), record.Composition, string(data), time.Now().Unix())
	return err
}

func (s *SQLStorage) QueryRequests(ctx context.Context, opts RequestQuery) ([]reqlog.Record, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}

	query := `SELECT data FROM request_log`
	var args []any
	var conditions []string

	if opts.Composition != "" {
		conditions = append(conditions, `composition = ?`)
		args = append(args, opts.Composition)
	}

	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}

	// Fetch more than limit to allow for Go-side filtering on JSON fields.
	// Cap the SQL scan at 10x limit to bound memory usage.
	sqlLimit := limit * 10
	if sqlLimit > 10000 {
		sqlLimit = 10000
	}
	query += ` ORDER BY timestamp DESC LIMIT ?`
	args = append(args, sqlLimit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]reqlog.Record, 0, limit)
	for rows.Next() && len(results) < limit {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var r reqlog.Record
		if err := json.Unmarshal([]byte(data), &r); err != nil {
			continue
		}
		if opts.StatusMin > 0 && r.Status < opts.StatusMin {
			continue
		}
		if opts.StatusMax > 0 && r.Status > opts.StatusMax {
			continue
		}
		if opts.MinDuration > 0 && r.DurationMS < opts.MinDuration {
			continue
		}
		if opts.Partial != nil && r.Partial != *opts.Partial {
			continue
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// GetRequestByID performs a direct indexed lookup by primary key rather than
// scanning the whole request_log table.
func (s *SQLStorage) GetRequestByID(ctx context.Context, id string) (*reqlog.Record, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM request_log WHERE id = ?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r reqlog.Record
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *SQLStorage) QueryStepMetrics(ctx context.Context, composition string, from, to time.Time) ([]StepAggregate, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data FROM timeseries_buckets WHERE composition = ? AND timestamp >= ? AND timestamp < ?`,
		composition, from.Unix(), to.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stepAcc := make(map[string]*stepAggAcc)
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var b Bucket
		if err := json.Unmarshal([]byte(data), &b); err != nil {
			continue
		}
		for name, sm := range b.StepMetrics {
			acc, ok := stepAcc[name]
			if !ok {
				acc = &stepAggAcc{upstream: sm.Upstream}
				stepAcc[name] = acc
			}
			acc.requests += sm.Requests
			acc.errors += sm.Errors
			acc.avgSum += sm.AvgMS * float64(sm.Requests)
			acc.p95Samples = append(acc.p95Samples, sm.P95MS)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	results := make([]StepAggregate, 0, len(stepAcc))
	for name, acc := range stepAcc {
		sa := StepAggregate{
			Name:     name,
			Upstream: acc.upstream,
			Requests: acc.requests,
			Errors:   acc.errors,
		}
		if acc.requests > 0 {
			sa.AvgMS = acc.avgSum / float64(acc.requests)
		}
		if len(acc.p95Samples) > 0 {
			sa.P95MS = percentile(acc.p95Samples, 0.95)
			sa.P99MS = percentile(acc.p95Samples, 0.99)
		}
		results = append(results, sa)
	}
	return results, nil
}

func (s *SQLStorage) Compact(ctx context.Context, retention time.Duration) error {
	cutoff := time.Now().Add(-retention).Unix()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM timeseries_buckets WHERE timestamp < ?`, cutoff); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM request_log WHERE timestamp < ?`, cutoff)
	return err
}

func (s *SQLStorage) Close() error {
	return s.db.Close()
}
