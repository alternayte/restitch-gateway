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
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBreakerTripper_TripsAfterMaxFailures(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(503)
	}))
	defer srv.Close()

	rt := newBreakerTripper(http.DefaultTransport, BreakerConfig{
		MaxFailures: 3,
		Interval:    60 * time.Second,
		Timeout:     1 * time.Second,
	}, "test")

	// First 3 requests go through (but count as failures due to 503)
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", srv.URL+"/test", nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", i+1, err)
		}
		if resp.StatusCode != 503 {
			t.Fatalf("attempt %d: expected 503, got %d", i+1, resp.StatusCode)
		}
	}

	if hits.Load() != 3 {
		t.Fatalf("expected 3 hits before trip, got %d", hits.Load())
	}

	// 4th request should fail without hitting the server (circuit open)
	req, _ := http.NewRequest("GET", srv.URL+"/test", nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected circuit open error")
	}
	if !strings.Contains(err.Error(), "circuit open") {
		t.Errorf("expected circuit open error, got: %v", err)
	}

	if hits.Load() != 3 {
		t.Errorf("expected no additional hits after trip, got %d total", hits.Load())
	}
}

func TestBreakerTripper_2xxDoesNotCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	rt := newBreakerTripper(http.DefaultTransport, BreakerConfig{
		MaxFailures: 2,
		Interval:    60 * time.Second,
		Timeout:     1 * time.Second,
	}, "test")

	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest("GET", srv.URL+"/test", nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", i+1, err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("attempt %d: expected 200, got %d", i+1, resp.StatusCode)
		}
	}
}
