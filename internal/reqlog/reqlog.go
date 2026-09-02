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

package reqlog

import "time"

// Record represents a single request record shared between composition handler and admin.
type Record struct {
	ID          string       `json:"id"`
	TraceID     string       `json:"trace_id,omitempty"`
	Time        time.Time    `json:"time"`
	Composition string       `json:"composition"`
	Method      string       `json:"method"`
	Path        string       `json:"path"`
	Status      int          `json:"status"`
	DurationMS  float64      `json:"duration_ms"`
	Partial     bool         `json:"partial"`
	Steps       []StepRecord `json:"steps"`
}

// StepRecord represents a single step's outcome within a request.
type StepRecord struct {
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	Wave          int     `json:"wave"`
	DurationMS    float64 `json:"duration_ms"`
	HTTPStatus    int     `json:"http_status"`
	Upstream      string  `json:"upstream"`
	URL           string  `json:"url"`
	StartOffsetMS float64 `json:"start_offset_ms"`
	BodySize      int64   `json:"body_size"`
	Error         string  `json:"error,omitempty"`
	Cached        bool    `json:"cached"`
	Retries       int     `json:"retries"`
}

// Recorder is the interface composition uses to report request records.
type Recorder interface {
	Record(rec Record)
}
