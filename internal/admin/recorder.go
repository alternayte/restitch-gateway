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

import "github.com/alternayte/restitch-gateway/v2/internal/reqlog"

// MultiRecorder feeds request records to the time-series accumulator and
// to the request sampler. The gateway pushes both to Studio.
type MultiRecorder struct {
	Accumulator *Accumulator
	Samples     reqlog.Recorder
}

func (mr *MultiRecorder) Record(rec reqlog.Record) {
	if mr.Samples != nil {
		mr.Samples.Record(rec)
	}
	if mr.Accumulator == nil {
		return
	}
	var stepSamples []StepSample
	for _, s := range rec.Steps {
		stepSamples = append(stepSamples, StepSample{
			Name:       s.Name,
			Upstream:   s.Upstream,
			DurationMS: s.DurationMS,
			IsError:    s.Status == "failed",
			Cached:     s.Cached,
		})
	}
	mr.Accumulator.Record(rec.Composition, rec.DurationMS, rec.Status >= 500, rec.Partial, stepSamples)
}
