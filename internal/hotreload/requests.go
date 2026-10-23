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

package hotreload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/alternayte/restitch-gateway/v2/internal/reqlog"
	"github.com/alternayte/restitch-gateway/v2/internal/upstream"
)

const (
	// RequestsInterval is how often a managed gateway pushes samples.
	RequestsInterval = 10 * time.Second
	// successesPerMinute bounds the successful samples one instance sends;
	// Studio applies the environment's own, lower, cap on top.
	successesPerMinute = 60
	// maxQueuedSamples bounds memory while Studio is unreachable.
	maxQueuedSamples = 1000
)

// RequestsPush is one push of sampled requests.
type RequestsPush struct {
	InstanceID string          `json:"instance_id"`
	Requests   []reqlog.Record `json:"requests"`
}

// RequestSampler keeps every failed request and a capped share of the
// successful ones, and pushes them to Studio. It is a reqlog.Recorder.
type RequestSampler struct {
	Client     *Client
	InstanceID string
	Dropped    *prometheus.CounterVec

	mu        sync.Mutex
	queue     []reqlog.Record
	pending   []reqlog.Record
	minute    time.Time
	successes int
}

// Record queues rec when it is sampled. It never blocks the request.
func (s *RequestSampler) Record(rec reqlog.Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if failed := rec.Status >= 500 || rec.Partial; !failed {
		if m := rec.Time.Truncate(time.Minute); !m.Equal(s.minute) {
			s.minute, s.successes = m, 0
		}
		if s.successes >= successesPerMinute {
			return
		}
		s.successes++
	}
	if len(s.queue) >= maxQueuedSamples {
		s.drop(1)
		return
	}
	s.queue = append(s.queue, rec)
}

func (s *RequestSampler) drop(n int) {
	if s.Dropped != nil && n > 0 {
		s.Dropped.WithLabelValues("requests").Add(float64(n))
	}
}

// Push sends the queue with any samples left from the last failed push. A
// second failure drops the older ones.
func (s *RequestSampler) Push(ctx context.Context) {
	s.mu.Lock()
	fresh := s.queue
	s.queue = nil
	batch := append(s.pending, fresh...)
	s.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	if err := s.Client.sendRequests(ctx, RequestsPush{InstanceID: s.InstanceID, Requests: batch}); err != nil {
		s.mu.Lock()
		s.drop(len(s.pending))
		s.pending = fresh
		s.mu.Unlock()
		slog.Warn("request sample push failed; retrying with the next push", "error", err, "samples", len(fresh))
		return
	}
	s.mu.Lock()
	s.pending = nil
	s.mu.Unlock()
}

// Run pushes every interval until ctx ends.
func (s *RequestSampler) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pushCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			s.Push(pushCtx)
			cancel()
		}
	}
}

func (c *Client) sendRequests(ctx context.Context, push RequestsPush) error {
	body, err := json.Marshal(push)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/gateway/v1/requests", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("requests push: %w", err)
	}
	defer upstream.DrainAndClose(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("requests push: studio returned status %d", resp.StatusCode)
	}
	return nil
}
