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
	"math"
	"sort"
	"sync"
)

// Stats tracks per-composition request statistics.
type Stats struct {
	mu    sync.Mutex
	total int64
	errs  int64
	parts int64
	comps map[string]*compStats
}

type compStats struct {
	count   int64
	errors  int64
	latency []float64
}

// NewStats creates a new Stats instance.
func NewStats() *Stats {
	return &Stats{comps: make(map[string]*compStats)}
}

// Record records a request for stats.
func (s *Stats) Record(composition string, durationMS float64, isError bool, isPartial bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.total++
	if isError {
		s.errs++
	}
	if isPartial {
		s.parts++
	}

	cs, ok := s.comps[composition]
	if !ok {
		cs = &compStats{}
		s.comps[composition] = cs
	}
	cs.count++
	if isError {
		cs.errors++
	}
	if len(cs.latency) >= 512 {
		cs.latency = cs.latency[1:]
	}
	cs.latency = append(cs.latency, durationMS)
}

// StatsResponse is the JSON response for /admin/api/stats.
type StatsResponse struct {
	TotalRequests    int64                       `json:"total_requests"`
	TotalErrors      int64                       `json:"total_errors"`
	PartialResponses int64                       `json:"partial_responses"`
	PerComposition   map[string]CompositionStats `json:"per_composition"`
}

// CompositionStats holds per-composition statistics.
type CompositionStats struct {
	Count  int64   `json:"count"`
	Errors int64   `json:"errors"`
	AvgMS  float64 `json:"avg_ms"`
	P95MS  float64 `json:"p95_ms"`
}

// Snapshot returns the current stats.
func (s *Stats) Snapshot() StatsResponse {
	s.mu.Lock()
	defer s.mu.Unlock()

	perComp := make(map[string]CompositionStats, len(s.comps))
	for name, cs := range s.comps {
		var avg, p95 float64
		if len(cs.latency) > 0 {
			var sum float64
			for _, v := range cs.latency {
				sum += v
			}
			avg = math.Round(sum/float64(len(cs.latency))*100) / 100

			sorted := make([]float64, len(cs.latency))
			copy(sorted, cs.latency)
			sort.Float64s(sorted)
			idx := int(float64(len(sorted)) * 0.95)
			if idx >= len(sorted) {
				idx = len(sorted) - 1
			}
			p95 = math.Round(sorted[idx]*100) / 100
		}
		perComp[name] = CompositionStats{
			Count:  cs.count,
			Errors: cs.errors,
			AvgMS:  avg,
			P95MS:  p95,
		}
	}

	return StatsResponse{
		TotalRequests:    s.total,
		TotalErrors:      s.errs,
		PartialResponses: s.parts,
		PerComposition:   perComp,
	}
}
