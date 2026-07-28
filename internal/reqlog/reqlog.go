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
