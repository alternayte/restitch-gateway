package admin

import (
	"context"
	"time"

	"github.com/restitch/restitch-gateway/internal/reqlog"
)

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
	Requests int64   `json:"requests"`
	Errors   int64   `json:"errors"`
	AvgMS    float64 `json:"avg_ms"`
	P95MS    float64 `json:"p95_ms"`
	Upstream string  `json:"upstream"`
}

type StepAggregate struct {
	Name     string  `json:"name"`
	Upstream string  `json:"upstream"`
	Requests int64   `json:"requests"`
	Errors   int64   `json:"errors"`
	AvgMS    float64 `json:"avg_ms"`
	P95MS    float64 `json:"p95_ms"`
	P99MS    float64 `json:"p99_ms"`
}

type RequestQuery struct {
	Limit       int
	Composition string
	StatusMin   int
	StatusMax   int
	MinDuration float64
	Partial     *bool
}

type Storage interface {
	RecordBucket(ctx context.Context, bucket Bucket) error
	QueryTimeSeries(ctx context.Context, from, to time.Time, resolution time.Duration, composition string) ([]Bucket, error)
	RecordRequest(ctx context.Context, record reqlog.Record) error
	QueryRequests(ctx context.Context, opts RequestQuery) ([]reqlog.Record, error)
	GetRequestByID(ctx context.Context, id string) (*reqlog.Record, error)
	QueryStepMetrics(ctx context.Context, composition string, from, to time.Time) ([]StepAggregate, error)
	Compact(ctx context.Context, retention time.Duration) error
	Close() error
}

type StepSample struct {
	Name       string
	Upstream   string
	DurationMS float64
	IsError    bool
}
