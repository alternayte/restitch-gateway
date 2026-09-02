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
