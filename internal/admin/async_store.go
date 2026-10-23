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
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/alternayte/restitch-gateway/internal/reqlog"
)

// asyncQueueSize bounds the number of request records waiting to be written.
// When the queue is full, new records are dropped rather than blocking the
// hot path; the ring buffer and stats remain complete either way.
const asyncQueueSize = 1024

// recordWriteTimeout bounds one request-record write in normal operation.
const recordWriteTimeout = 5 * time.Second

// dropLogInterval is the minimum gap between "records dropped" log lines. A
// database stall drops every request; one line per record flooded the log.
const dropLogInterval = 10 * time.Second

// defaultDrainTimeout bounds the drain when Close is called without a prior
// Drain. The gateway calls Drain with server.shutdown_timeout instead.
const defaultDrainTimeout = 5 * time.Second

// Drop reasons, used as the "reason" label of the dropped-records counter.
const (
	dropQueueFull    = "queue_full"
	dropClosed       = "closed"
	dropDrainTimeout = "drain_timeout"
)

// AsyncStore decorates a Storage so request records are written through a
// bounded queue by a background writer. A slow or locked database can no
// longer stall request completion, which ran a synchronous 5-second-timeout
// write per request (finding M6). Bucket writes, compactions, and queries
// pass through to the underlying storage directly.
type AsyncStore struct {
	inner   Storage
	queue   chan reqlog.Record
	dropped *prometheus.CounterVec // nil disables the metric

	// mu guards closed and the close of queue. Senders hold the read lock
	// across a non-blocking send, so no send can reach a closed channel.
	mu     sync.RWMutex
	closed bool

	// writeCtx is cancelled when the drain deadline passes; the writer then
	// discards what is left instead of writing it.
	writeCtx    context.Context
	cancelWrite context.CancelFunc
	writerDone  chan struct{}

	unloggedDrops atomic.Int64
	lastDropLog   atomic.Int64 // unix nanoseconds; 0 means never logged

	closeOnce sync.Once
	closeErr  error
}

// NewAsyncStore wraps inner with an asynchronous request-record writer.
// dropped, when non-nil, counts records that never reach inner, labelled by
// reason.
func NewAsyncStore(inner Storage, dropped *prometheus.CounterVec) *AsyncStore {
	ctx, cancel := context.WithCancel(context.Background())
	as := &AsyncStore{
		inner:       inner,
		queue:       make(chan reqlog.Record, asyncQueueSize),
		dropped:     dropped,
		writeCtx:    ctx,
		cancelWrite: cancel,
		writerDone:  make(chan struct{}),
	}
	go as.run()
	return as
}

func (as *AsyncStore) run() {
	defer close(as.writerDone)
	abandoned := 0
	for rec := range as.queue {
		if as.writeCtx.Err() != nil {
			abandoned++
			continue
		}
		ctx, cancel := context.WithTimeout(as.writeCtx, recordWriteTimeout)
		if err := as.inner.RecordRequest(ctx, rec); err != nil {
			slog.Error("failed to persist request record", "error", err)
		}
		cancel()
	}
	if abandoned > 0 {
		as.noteDrop(dropDrainTimeout, abandoned)
	}
}

// RecordRequest enqueues the record without blocking. If the queue is full,
// or the store is closed, the record is dropped and counted instead of
// stalling the caller.
func (as *AsyncStore) RecordRequest(_ context.Context, record reqlog.Record) error {
	as.mu.RLock()
	if as.closed {
		as.mu.RUnlock()
		as.noteDrop(dropClosed, 1)
		return nil
	}
	select {
	case as.queue <- record:
		as.mu.RUnlock()
	default:
		as.mu.RUnlock()
		as.noteDrop(dropQueueFull, 1)
	}
	return nil
}

// noteDrop counts n dropped records and logs at most once per
// dropLogInterval: the first drop logs at once, later drops are summed into
// the next line.
func (as *AsyncStore) noteDrop(reason string, n int) {
	if as.dropped != nil {
		as.dropped.WithLabelValues(reason).Add(float64(n))
	}
	as.unloggedDrops.Add(int64(n))
	now := time.Now().UnixNano()
	last := as.lastDropLog.Load()
	if last != 0 && now-last < int64(dropLogInterval) {
		return
	}
	if !as.lastDropLog.CompareAndSwap(last, now) {
		return
	}
	as.logDrops(reason)
}

// logDrops writes one line for the drops counted since the last line. An
// empty reason marks the final summary written by Drain.
func (as *AsyncStore) logDrops(reason string) {
	count := as.unloggedDrops.Swap(0)
	if count == 0 {
		return
	}
	if reason == "" {
		slog.Warn("request records dropped", "count", count)
		return
	}
	slog.Warn("request records dropped",
		"count", count, "reason", reason, "next_log_after", dropLogInterval.String())
}

func (as *AsyncStore) RecordBucket(ctx context.Context, bucket Bucket) error {
	return as.inner.RecordBucket(ctx, bucket)
}

func (as *AsyncStore) QueryTimeSeries(ctx context.Context, from, to time.Time, resolution time.Duration, composition string) ([]Bucket, error) {
	return as.inner.QueryTimeSeries(ctx, from, to, resolution, composition)
}

func (as *AsyncStore) QueryRequests(ctx context.Context, opts RequestQuery) ([]reqlog.Record, error) {
	return as.inner.QueryRequests(ctx, opts)
}

func (as *AsyncStore) GetRequestByID(ctx context.Context, id string) (*reqlog.Record, error) {
	return as.inner.GetRequestByID(ctx, id)
}

func (as *AsyncStore) QueryStepMetrics(ctx context.Context, composition string, from, to time.Time) ([]StepAggregate, error) {
	return as.inner.QueryStepMetrics(ctx, composition, from, to)
}

func (as *AsyncStore) Compact(ctx context.Context, retention time.Duration) error {
	return as.inner.Compact(ctx, retention)
}

// Drain stops accepting records and waits until the writer has persisted
// the queue, or until ctx is done. ctx is one deadline for the whole queue,
// not per record. Records still queued at the deadline are dropped and
// counted. Drain does not close the underlying storage; Close does.
func (as *AsyncStore) Drain(ctx context.Context) error {
	as.mu.Lock()
	if !as.closed {
		as.closed = true
		close(as.queue)
	}
	as.mu.Unlock()

	var err error
	select {
	case <-as.writerDone:
	case <-ctx.Done():
		as.cancelWrite()
		<-as.writerDone
		err = fmt.Errorf("drain request records: %w", ctx.Err())
	}
	as.logDrops("")
	return err
}

// Close drains the queue (bounded by defaultDrainTimeout if Drain was not
// called first) and then closes the underlying storage. Records sent after
// Close are dropped, never sent on a closed channel.
func (as *AsyncStore) Close() error {
	as.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultDrainTimeout)
		if err := as.Drain(ctx); err != nil {
			slog.Error("request record queue not fully drained", "error", err)
		}
		cancel()
		as.cancelWrite()
		as.closeErr = as.inner.Close()
	})
	return as.closeErr
}
