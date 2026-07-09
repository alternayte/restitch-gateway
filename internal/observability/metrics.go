package observability

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metric collectors for the gateway.
type Metrics struct {
	registry *prometheus.Registry

	RequestsTotal          *prometheus.CounterVec
	RequestDuration        *prometheus.HistogramVec
	PartialResponsesTotal  *prometheus.CounterVec
	StepDuration           *prometheus.HistogramVec
	UpstreamRequestsTotal  *prometheus.CounterVec
	RetriesTotal           *prometheus.CounterVec
	BreakerState           *prometheus.GaugeVec
	CacheHitsTotal         *prometheus.CounterVec
	CacheMissesTotal       *prometheus.CounterVec
	CoalescedTotal         *prometheus.CounterVec
}

// NewMetrics creates a new Metrics instance with a private registry.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector())

	f := promauto.With(reg)

	return &Metrics{
		registry: reg,

		RequestsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Name: "restitch_requests_total",
			Help: "Total composition requests",
		}, []string{"composition", "method", "status"}),

		RequestDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "restitch_request_duration_seconds",
			Help:    "Composition request duration",
			Buckets: prometheus.DefBuckets,
		}, []string{"composition"}),

		PartialResponsesTotal: f.NewCounterVec(prometheus.CounterOpts{
			Name: "restitch_partial_responses_total",
			Help: "Total partial responses (at least one step failed)",
		}, []string{"composition"}),

		StepDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "restitch_step_duration_seconds",
			Help:    "Step execution duration",
			Buckets: prometheus.DefBuckets,
		}, []string{"composition", "step", "upstream", "status"}),

		UpstreamRequestsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Name: "restitch_upstream_requests_total",
			Help: "Total upstream requests by status class",
		}, []string{"upstream", "status_class"}),

		RetriesTotal: f.NewCounterVec(prometheus.CounterOpts{
			Name: "restitch_retries_total",
			Help: "Total retry attempts",
		}, []string{"upstream"}),

		BreakerState: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "restitch_breaker_state",
			Help: "Circuit breaker state (0=closed, 1=half-open, 2=open)",
		}, []string{"upstream"}),

		CacheHitsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Name: "restitch_cache_hits_total",
			Help: "Step cache hits",
		}, []string{"composition", "step"}),

		CacheMissesTotal: f.NewCounterVec(prometheus.CounterOpts{
			Name: "restitch_cache_misses_total",
			Help: "Step cache misses",
		}, []string{"composition", "step"}),

		CoalescedTotal: f.NewCounterVec(prometheus.CounterOpts{
			Name: "restitch_coalesced_total",
			Help: "Coalesced (deduplicated) step requests",
		}, []string{"composition", "step"}),
	}
}

// Handler returns an HTTP handler that serves the /metrics endpoint.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// StatusClass returns the status class string (2xx, 3xx, 4xx, 5xx, error).
func StatusClass(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500:
		return "5xx"
	default:
		return "error"
	}
}

// StatusStr converts an HTTP status code to a string.
func StatusStr(code int) string {
	return strconv.Itoa(code)
}

var defaultMetrics *Metrics

// SetDefaultMetrics sets the global metrics instance (called once from main).
func SetDefaultMetrics(m *Metrics) {
	defaultMetrics = m
}

// DefaultMetrics returns the global metrics instance, or nil if not set.
func DefaultMetrics() *Metrics {
	return defaultMetrics
}
