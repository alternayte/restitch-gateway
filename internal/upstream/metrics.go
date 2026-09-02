package upstream

import (
	"net/http"

	"github.com/alternayte/restitch-gateway/internal/observability"
)

type metricsTripper struct {
	next    http.RoundTripper
	name    string
	metrics *observability.Metrics
}

func newMetricsTripper(next http.RoundTripper, name string, m *observability.Metrics) http.RoundTripper {
	if m == nil {
		return next
	}
	return &metricsTripper{next: next, name: name, metrics: m}
}

func (mt *metricsTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := mt.next.RoundTrip(req)
	if err != nil {
		mt.metrics.UpstreamRequestsTotal.WithLabelValues(mt.name, "error").Inc()
		return nil, err
	}
	mt.metrics.UpstreamRequestsTotal.WithLabelValues(mt.name, observability.StatusClass(resp.StatusCode)).Inc()
	return resp, nil
}
