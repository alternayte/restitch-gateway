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

package upstream

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// HealthStatus represents the health of a single upstream.
type HealthStatus struct {
	Status    string    `json:"status"`
	LatencyMS float64   `json:"latency_ms"`
	CheckedAt time.Time `json:"checked_at"`
	Error     string    `json:"error,omitempty"`
}

// Checker probes each upstream through its own client (auth applied),
// caches results for ttl, and single-flights concurrent probes.
type Checker struct {
	upstreams map[string]*Upstream
	ttl       time.Duration
	mu        sync.RWMutex
	cache     map[string]cachedStatus
	group     singleflight.Group
}

type cachedStatus struct {
	status  HealthStatus
	expires time.Time
}

// NewChecker creates a health checker.
func NewChecker(ups map[string]*Upstream, ttl time.Duration) *Checker {
	if ttl == 0 {
		ttl = 10 * time.Second
	}
	return &Checker{
		upstreams: ups,
		ttl:       ttl,
		cache:     make(map[string]cachedStatus),
	}
}

// Check probes all upstreams and returns their health status.
func (c *Checker) Check(ctx context.Context) map[string]HealthStatus {
	result := make(map[string]HealthStatus, len(c.upstreams))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for name := range c.upstreams {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			status := c.checkOne(ctx, name)
			mu.Lock()
			result[name] = status
			mu.Unlock()
		}()
	}

	wg.Wait()
	return result
}

func (c *Checker) checkOne(ctx context.Context, name string) HealthStatus {
	c.mu.RLock()
	if cs, ok := c.cache[name]; ok && time.Now().Before(cs.expires) {
		c.mu.RUnlock()
		return cs.status
	}
	c.mu.RUnlock()

	val, _, _ := c.group.Do(name, func() (any, error) {
		up := c.upstreams[name]
		// A nil upstream means the checker's map is stale (for example after
		// a reload added an upstream before the checker was rebuilt). Return
		// "unknown" instead of panicking (finding H3).
		if up == nil {
			return HealthStatus{Status: "unknown", CheckedAt: time.Now().UTC()}, nil
		}
		status := probeUpstream(ctx, up)

		c.mu.Lock()
		c.cache[name] = cachedStatus{status: status, expires: time.Now().Add(c.ttl)}
		c.mu.Unlock()

		return status, nil
	})

	return val.(HealthStatus)
}

func probeUpstream(ctx context.Context, up *Upstream) HealthStatus {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	healthURL := up.BaseURL
	method := http.MethodHead
	if up.HealthPath != "" {
		healthURL = strings.TrimSuffix(up.BaseURL, "/") + up.HealthPath
		method = http.MethodGet
	}

	start := time.Now()

	req, err := http.NewRequestWithContext(probeCtx, method, healthURL, nil)
	if err != nil {
		return HealthStatus{
			Status:    "unhealthy",
			LatencyMS: float64(time.Since(start).Nanoseconds()) / 1e6,
			CheckedAt: time.Now().UTC(),
			Error:     err.Error(),
		}
	}

	resp, err := up.Client.Do(req)
	latency := float64(time.Since(start).Nanoseconds()) / 1e6

	if err != nil {
		return HealthStatus{
			Status:    "unhealthy",
			LatencyMS: latency,
			CheckedAt: time.Now().UTC(),
			Error:     err.Error(),
		}
	}
	defer DrainAndClose(resp.Body)

	status := "healthy"
	var errMsg string
	if resp.StatusCode >= 400 {
		status = "unhealthy"
		errMsg = fmt.Sprintf("status %d", resp.StatusCode)
	}

	return HealthStatus{
		Status:    status,
		LatencyMS: latency,
		CheckedAt: time.Now().UTC(),
		Error:     errMsg,
	}
}
