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

package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alternayte/restitch-gateway/v2/internal/admin"
	"github.com/alternayte/restitch-gateway/v2/internal/reqlog"
)

// eventStorage records the order of writes and the close, and rejects
// writes after Close the way a closed database does.
type eventStorage struct {
	*admin.MemoryStorage
	mu     sync.Mutex
	closed bool
	events []string
}

func (s *eventStorage) log(ev string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		s.events = append(s.events, "after-close:"+ev)
		return errors.New("storage is closed")
	}
	s.events = append(s.events, ev)
	return nil
}

func (s *eventStorage) RecordBucket(_ context.Context, b admin.Bucket) error {
	return s.log("bucket:" + b.Composition)
}

func (s *eventStorage) RecordRequest(_ context.Context, r reqlog.Record) error {
	return s.log("request:" + r.ID)
}

func (s *eventStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.events = append(s.events, "close")
	return nil
}

func (s *eventStorage) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.events)
}

// TestGatewayShutdown_FinalFlushReachesStore pins the shutdown order: the
// last accumulator window and every queued request record reach the store
// before it closes. Deferred closes ran LIFO, so the store closed first and
// the final time-series buckets were written to a closed database.
func TestGatewayShutdown_FinalFlushReachesStore(t *testing.T) {
	inner := &eventStorage{MemoryStorage: admin.NewMemoryStorage(time.Hour)}
	store := admin.NewAsyncStore(inner, nil)

	acc := admin.NewAccumulator()
	acc.Record("orders", 12, false, false, nil)
	if err := store.RecordRequest(context.Background(), reqlog.Record{ID: "r1"}); err != nil {
		t.Fatal(err)
	}

	var stepsMu sync.Mutex
	var steps []string
	step := func(s string) {
		stepsMu.Lock()
		defer stepsMu.Unlock()
		steps = append(steps, s)
	}

	sd := &gatewayShutdown{
		servers: []func(context.Context) error{
			func(context.Context) error { step("gateway-server"); return nil },
			func(context.Context) error { step("admin-server"); return nil },
		},
		stopBackground: func() {
			step("background")
			if got := inner.snapshot(); slices.Contains(got, "close") {
				t.Errorf("store closed before background work stopped: %v", got)
			}
		},
		acc:     acc,
		store:   store,
		timeout: 5 * time.Second,
	}
	if err := sd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	if want := []string{"gateway-server", "admin-server", "background"}; !slices.Equal(steps, want) {
		t.Errorf("steps = %v, want %v", steps, want)
	}

	events := inner.snapshot()
	for _, ev := range events {
		if strings.HasPrefix(ev, "after-close:") {
			t.Errorf("write after close: %q (events %v)", ev, events)
		}
	}
	closeAt := slices.Index(events, "close")
	if closeAt != len(events)-1 {
		t.Fatalf("close is not the last event: %v", events)
	}
	for _, want := range []string{"bucket:orders", "request:r1"} {
		if i := slices.Index(events, want); i < 0 || i > closeAt {
			t.Errorf("%s did not reach the store before close: %v", want, events)
		}
	}
}
