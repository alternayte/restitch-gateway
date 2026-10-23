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
	"testing"
)

func TestAccumulator_Flush(t *testing.T) {
	acc := NewAccumulator()

	acc.Record("comp1", 42.0, false, false, []StepSample{
		{Name: "s1", Upstream: "api", DurationMS: 30.0, IsError: false},
		{Name: "s2", Upstream: "api", DurationMS: 12.0, IsError: false},
	})
	acc.Record("comp1", 150.0, true, false, []StepSample{
		{Name: "s1", Upstream: "api", DurationMS: 150.0, IsError: true},
	})
	acc.Record("comp2", 5.0, false, true, nil)

	buckets := acc.Flush()

	if len(buckets) != 3 {
		t.Fatalf("expected 3 buckets (global + 2 compositions), got %d", len(buckets))
	}

	var global *Bucket
	for i := range buckets {
		if buckets[i].Composition == "" {
			global = &buckets[i]
		}
	}
	if global == nil {
		t.Fatal("no global bucket")
	}
	if global.Requests != 3 {
		t.Errorf("global requests = %d, want 3", global.Requests)
	}
	if global.Errors != 1 {
		t.Errorf("global errors = %d, want 1", global.Errors)
	}
	if global.Partials != 1 {
		t.Errorf("global partials = %d, want 1", global.Partials)
	}
}

func TestAccumulator_LatencyBuckets(t *testing.T) {
	acc := NewAccumulator()
	acc.Record("c", 5.0, false, false, nil)   // bucket 0 (0-10ms)
	acc.Record("c", 75.0, false, false, nil)  // bucket 2 (50-100ms)
	acc.Record("c", 300.0, false, false, nil) // bucket 4 (250-500ms)

	buckets := acc.Flush()
	for _, b := range buckets {
		if b.Composition == "c" {
			if len(b.LatencyBuckets) != 8 {
				t.Fatalf("latency buckets length = %d, want 8", len(b.LatencyBuckets))
			}
			if b.LatencyBuckets[0] != 1 {
				t.Errorf("bucket[0] = %d, want 1", b.LatencyBuckets[0])
			}
			if b.LatencyBuckets[2] != 1 {
				t.Errorf("bucket[2] = %d, want 1", b.LatencyBuckets[2])
			}
			if b.LatencyBuckets[4] != 1 {
				t.Errorf("bucket[4] = %d, want 1", b.LatencyBuckets[4])
			}
			return
		}
	}
	t.Fatal("comp bucket not found")
}
