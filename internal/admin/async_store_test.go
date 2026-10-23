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
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/alternayte/restitch-gateway/v2/internal/reqlog"
)

func newDropCounter() *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "test_request_records_dropped_total",
	}, []string{"reason"})
}

// dropCount reads the counter value for reason.
func dropCount(t *testing.T, c *prometheus.CounterVec, reason string) float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "reason" && l.GetValue() == reason {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// TestAsyncStore_RecordAfterCloseDoesNotPanic pins the shutdown race: a
// handler still running after srv.Shutdown times out may record a request
// after the store is closed. That must drop the record, not panic on a send
// to a closed channel.
func TestAsyncStore_RecordAfterCloseDoesNotPanic(t *testing.T) {
	as := NewAsyncStore(NewMemoryStorage(time.Hour), nil)
	if err := as.Close(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RecordRequest after Close panicked: %v", r)
		}
	}()
	if err := as.RecordRequest(context.Background(), reqlog.Record{ID: "late"}); err != nil {
		t.Fatalf("RecordRequest after Close: %v", err)
	}
}

// TestAsyncStore_FullQueueCountsAndLogsOnce pins finding (c): a database
// stall drops every record. Each drop is counted, but only the first one
// within dropLogInterval writes a log line.
func TestAsyncStore_FullQueueCountsAndLogsOnce(t *testing.T) {
	var logBuf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))
	defer slog.SetDefault(prev)

	blocker := &blockingStorage{done: make(chan struct{})}
	dropped := newDropCounter()
	as := NewAsyncStore(blocker, dropped)
	defer func() {
		close(blocker.done)
		_ = as.Close()
	}()

	ctx := context.Background()
	// The writer takes one record and blocks on it; asyncQueueSize more fill
	// the queue. Everything after that is dropped.
	const overflow = 200
	for i := 0; i < asyncQueueSize+1+overflow; i++ {
		_ = as.RecordRequest(ctx, reqlog.Record{ID: fmt.Sprintf("r-%d", i)})
	}

	got := dropCount(t, dropped, dropQueueFull)
	if got < overflow {
		t.Errorf("queue_full drops = %v, want >= %d", got, overflow)
	}
	if lines := strings.Count(logBuf.String(), "request records dropped"); lines != 1 {
		t.Errorf("drop log lines = %d, want 1:\n%s", lines, logBuf.String())
	}
}

// TestAsyncStore_DrainHasOneDeadline pins finding (d): Drain gives up at its
// context deadline for the whole queue, instead of allowing up to
// recordWriteTimeout per queued record, and counts what it abandons.
func TestAsyncStore_DrainHasOneDeadline(t *testing.T) {
	blocker := &blockingStorage{done: make(chan struct{})}
	defer close(blocker.done)
	dropped := newDropCounter()
	as := NewAsyncStore(blocker, dropped)

	ctx := context.Background()
	for i := 0; i < 100; i++ {
		if err := as.RecordRequest(ctx, reqlog.Record{ID: fmt.Sprintf("d-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}

	drainCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := as.Drain(drainCtx)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Drain took %v; the deadline was 100ms", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Drain error = %v, want deadline exceeded", err)
	}
	if got := dropCount(t, dropped, dropDrainTimeout); got < 98 {
		t.Errorf("drain_timeout drops = %v, want >= 98", got)
	}

	// After Drain, sends are dropped rather than panicking.
	if err := as.RecordRequest(ctx, reqlog.Record{ID: "late"}); err != nil {
		t.Fatal(err)
	}
	if got := dropCount(t, dropped, dropClosed); got != 1 {
		t.Errorf("closed drops = %v, want 1", got)
	}
	if err := as.Close(); err != nil {
		t.Fatal(err)
	}
}
