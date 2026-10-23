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
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/alternayte/restitch-gateway/v2/internal/admin"
	"github.com/alternayte/restitch-gateway/v2/internal/upstream"
)

// UsageInterval is how often a managed gateway pushes its aggregates.
const UsageInterval = time.Minute

// UsagePush is one push of one instance's minute buckets and the state of
// each upstream's circuit breaker.
type UsagePush struct {
	InstanceID string            `json:"instance_id"`
	Buckets    []admin.Bucket    `json:"buckets"`
	Breakers   map[string]string `json:"breakers,omitempty"`
}

// SendUsage posts one usage push to Studio.
func (c *Client) SendUsage(ctx context.Context, push UsagePush) error {
	body, err := json.Marshal(push)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/gateway/v1/usage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("usage push: %w", err)
	}
	defer upstream.DrainAndClose(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("usage push: studio returned status %d", resp.StatusCode)
	}
	return nil
}

// UsagePusher drains the accumulator into Studio. A push that fails is
// kept and sent again with the next one; a second failure drops it and
// counts the drop, so a Studio outage never grows memory without bound.
type UsagePusher struct {
	Client     *Client
	InstanceID string
	Acc        *admin.Accumulator
	Breakers   func() map[string]string
	Dropped    *prometheus.CounterVec

	pending []admin.Bucket
}

// Push flushes the accumulator and sends it with any bucket left from the
// last failed push.
func (p *UsagePusher) Push(ctx context.Context) {
	fresh := p.Acc.Flush()
	push := UsagePush{InstanceID: p.InstanceID, Buckets: append(p.pending, fresh...)}
	if p.Breakers != nil {
		push.Breakers = p.Breakers()
	}
	if err := p.Client.SendUsage(ctx, push); err != nil {
		if len(p.pending) > 0 && p.Dropped != nil {
			p.Dropped.WithLabelValues("usage").Add(float64(len(p.pending)))
		}
		p.pending = fresh
		slog.Warn("usage push failed; retrying with the next push", "error", err, "buckets", len(fresh))
		return
	}
	p.pending = nil
}

// Run pushes every interval until ctx ends.
func (p *UsagePusher) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pushCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			p.Push(pushCtx)
			cancel()
		}
	}
}
