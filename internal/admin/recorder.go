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
	"log/slog"
	"time"

	"github.com/alternayte/restitch-gateway/internal/reqlog"
)

// MultiRecorder feeds request records to the ring buffer, stats, the
// time-series accumulator, and durable storage.
type MultiRecorder struct {
	Ring        *RingBuffer
	Stats       *Stats
	Accumulator *Accumulator
	Storage     Storage
}

func (mr *MultiRecorder) Record(rec reqlog.Record) {
	mr.Ring.Record(rec)
	mr.Stats.Record(rec.Composition, rec.DurationMS, rec.Status >= 500, rec.Partial)

	if mr.Accumulator != nil {
		var stepSamples []StepSample
		for _, s := range rec.Steps {
			stepSamples = append(stepSamples, StepSample{
				Name:       s.Name,
				Upstream:   s.Upstream,
				DurationMS: s.DurationMS,
				IsError:    s.Status == "failed",
			})
		}
		mr.Accumulator.Record(rec.Composition, rec.DurationMS, rec.Status >= 500, rec.Partial, stepSamples)
	}

	if mr.Storage != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mr.Storage.RecordRequest(ctx, rec); err != nil {
			slog.Error("failed to persist request record", "error", err)
		}
	}
}
