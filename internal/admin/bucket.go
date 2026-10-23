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

import "time"

var LatencyBucketBounds = []float64{10, 50, 100, 250, 500, 1000, 5000}

type Bucket struct {
	Timestamp      time.Time              `json:"timestamp"`
	Composition    string                 `json:"composition"`
	Requests       int64                  `json:"requests"`
	Errors         int64                  `json:"errors"`
	Partials       int64                  `json:"partials"`
	LatencyP50     float64                `json:"latency_p50"`
	LatencyP95     float64                `json:"latency_p95"`
	LatencyP99     float64                `json:"latency_p99"`
	LatencyBuckets []int64                `json:"latency_buckets"`
	StepMetrics    map[string]*StepBucket `json:"step_metrics,omitempty"`
}

type StepBucket struct {
	Requests  int64   `json:"requests"`
	Errors    int64   `json:"errors"`
	CacheHits int64   `json:"cache_hits"`
	AvgMS     float64 `json:"avg_ms"`
	P95MS     float64 `json:"p95_ms"`
	Upstream  string  `json:"upstream"`
}

type StepSample struct {
	Name       string
	Upstream   string
	DurationMS float64
	IsError    bool
	Cached     bool
}
