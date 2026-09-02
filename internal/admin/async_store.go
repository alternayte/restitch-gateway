package admin

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/alternayte/restitch-gateway/internal/reqlog"
)

// asyncQueueSize bounds the number of request records waiting to be written.
// When the queue is full, new records are dropped rather than blocking the
// hot path; the ring buffer and stats remain complete either way.
const asyncQueueSize = 1024

// AsyncStore decorates a Storage so request records are written through a
// bounded queue by a background writer. A slow or locked database can no
// longer stall request completion, which ran a synchronous 5-second-timeout
// write per request (finding M6). Bucket writes, compactions, and queries
// pass through to the underlying storage directly.
type AsyncStore struct {
	inner Storage
	queue chan reqlog.Record
	wg    sync.WaitGroup

	closeOnce sync.Once
}

// NewAsyncStore wraps inner with an asynchronous request-record writer.
func NewAsyncStore(inner Storage) *AsyncStore {
	as := &AsyncStore{
		inner: inner,
		queue: make(chan reqlog.Record, asyncQueueSize),
	}
	as.wg.Add(1)
	go as.run()
	return as
}

func (as *AsyncStore) run() {
	defer as.wg.Done()
	for rec := range as.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := as.inner.RecordRequest(ctx, rec); err != nil {
			slog.Error("failed to persist request record", "error", err)
		}
		cancel()
	}
}

// RecordRequest enqueues the record without blocking. If the queue is full
// the record is dropped with a log line instead of stalling the caller.
func (as *AsyncStore) RecordRequest(_ context.Context, record reqlog.Record) error {
	select {
	case as.queue <- record:
		return nil
	default:
		slog.Warn("request record queue full; dropping record")
		return nil
	}
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

// Close drains the queue and then closes the underlying storage. Callers
// must stop producing records first; at process shutdown the HTTP server is
// drained before deferred closes run.
func (as *AsyncStore) Close() error {
	var err error
	as.closeOnce.Do(func() {
		close(as.queue)
		as.wg.Wait()
		err = as.inner.Close()
	})
	return err
}
