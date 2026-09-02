package admin

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/alternayte/restitch-gateway/internal/reqlog"
)

type MemoryStorage struct {
	mu        sync.RWMutex
	buckets   []Bucket
	requests  []reqlog.Record
	retention time.Duration
	maxReqs   int
}

func NewMemoryStorage(retention time.Duration) *MemoryStorage {
	if retention == 0 {
		retention = 24 * time.Hour
	}
	return &MemoryStorage{
		retention: retention,
		maxReqs:   10000,
	}
}

func (m *MemoryStorage) RecordBucket(_ context.Context, bucket Bucket) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buckets = append(m.buckets, bucket)
	return nil
}

func (m *MemoryStorage) QueryTimeSeries(_ context.Context, from, to time.Time, resolution time.Duration, composition string) ([]Bucket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make([]Bucket, 0)
	for _, b := range m.buckets {
		if b.Timestamp.Before(from) || !b.Timestamp.Before(to) {
			continue
		}
		if composition != "" && b.Composition != composition {
			continue
		}
		if composition == "" && b.Composition != "" {
			continue
		}
		results = append(results, b)
	}

	if resolution > time.Minute {
		results = aggregateBuckets(results, resolution)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.Before(results[j].Timestamp)
	})

	return results, nil
}

func (m *MemoryStorage) RecordRequest(_ context.Context, record reqlog.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, record)
	if len(m.requests) > m.maxReqs {
		m.requests = m.requests[len(m.requests)-m.maxReqs:]
	}
	return nil
}

func (m *MemoryStorage) QueryRequests(_ context.Context, opts RequestQuery) ([]reqlog.Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}

	filtered := make([]reqlog.Record, 0, limit)
	for i := len(m.requests) - 1; i >= 0 && len(filtered) < limit; i-- {
		r := m.requests[i]
		if opts.Composition != "" && r.Composition != opts.Composition {
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
		filtered = append(filtered, r)
	}
	return filtered, nil
}

// GetRequestByID performs a linear scan over the in-memory request ring
// (bounded by maxReqs, so this is cheap) searching newest-first.
func (m *MemoryStorage) GetRequestByID(_ context.Context, id string) (*reqlog.Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := len(m.requests) - 1; i >= 0; i-- {
		if m.requests[i].ID == id {
			r := m.requests[i]
			return &r, nil
		}
	}
	return nil, nil
}

func (m *MemoryStorage) QueryStepMetrics(_ context.Context, composition string, from, to time.Time) ([]StepAggregate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stepAcc := make(map[string]*stepAggAcc)
	for _, b := range m.buckets {
		if b.Composition != composition {
			continue
		}
		if b.Timestamp.Before(from) || !b.Timestamp.Before(to) {
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

func (m *MemoryStorage) Compact(_ context.Context, retention time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-retention)
	kept := make([]Bucket, 0, len(m.buckets))
	for _, b := range m.buckets {
		if !b.Timestamp.Before(cutoff) {
			kept = append(kept, b)
		}
	}
	m.buckets = kept

	keptReqs := make([]reqlog.Record, 0, len(m.requests))
	for _, r := range m.requests {
		if !r.Time.Before(cutoff) {
			keptReqs = append(keptReqs, r)
		}
	}
	m.requests = keptReqs
	return nil
}

func (m *MemoryStorage) Close() error {
	return nil
}

type stepAggAcc struct {
	requests   int64
	errors     int64
	avgSum     float64
	p95Samples []float64
	upstream   string
}

func aggregateBuckets(buckets []Bucket, resolution time.Duration) []Bucket {
	groups := make(map[int64]*Bucket)
	for _, b := range buckets {
		key := b.Timestamp.Truncate(resolution).Unix()
		if existing, ok := groups[key]; ok {
			existing.Requests += b.Requests
			existing.Errors += b.Errors
			existing.Partials += b.Partials
		} else {
			cp := b
			cp.Timestamp = b.Timestamp.Truncate(resolution)
			groups[key] = &cp
		}
	}
	result := make([]Bucket, 0, len(groups))
	for _, b := range groups {
		result = append(result, *b)
	}
	return result
}
