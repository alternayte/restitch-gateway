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

package studio

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/alternayte/restitch-gateway/v2/internal/admin"
)

func pushUsage(t *testing.T, s *Server, token string, push UsagePush) int {
	t.Helper()
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(push)
	req := httptest.NewRequest(http.MethodPost, testPublicURL+"/gateway/v1/usage", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec.Code
}

func bucket(ts time.Time, comp string, requests, errors int64, cacheHits int64) admin.Bucket {
	return admin.Bucket{
		Timestamp: ts, Composition: comp, Requests: requests, Errors: errors,
		LatencyBuckets: []int64{requests, 0, 0, 0, 0, 0, 0, 0},
		StepMetrics: map[string]*admin.StepBucket{
			"u": {Requests: requests, Errors: errors, CacheHits: cacheHits, AvgMS: 10, Upstream: "users"},
		},
	}
}

type usageView struct {
	Buckets []struct {
		Timestamp time.Time `json:"timestamp"`
		Requests  int64     `json:"requests"`
		Errors    int64     `json:"errors"`
		Steps     map[string]struct {
			Requests  int64 `json:"requests"`
			CacheHits int64 `json:"cache_hits"`
		} `json:"steps"`
	} `json:"buckets"`
	Upstreams []struct {
		Name     string            `json:"name"`
		Requests int64             `json:"requests"`
		Errors   int64             `json:"errors"`
		Breakers map[string]string `json:"breakers"`
	} `json:"upstreams"`
}

func readUsage(t *testing.T, s *Server, c []*http.Cookie, from time.Time) usageView {
	t.Helper()
	q := url.Values{"composition": {"user"}, "from": {from.Format(time.RFC3339)}}
	rec := do(t, s, http.MethodGet, "/api/v1/environments/prod/usage?"+q.Encode(), "", c, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("usage: %d %s", rec.Code, rec.Body)
	}
	var v usageView
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	return v
}

func TestAggregatesSumAcrossInstances(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		createEnvironment(t, s, c, "prod")
		tok := issueToken(t, s, c, "prod")
		minute := time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Minute)

		if code := pushUsage(t, s, tok.Token, UsagePush{InstanceID: "gw-a",
			Buckets:  []admin.Bucket{bucket(minute, "user", 10, 1, 4)},
			Breakers: map[string]string{"users": "closed"}}); code != http.StatusNoContent {
			t.Fatalf("push a: %d", code)
		}
		pushUsage(t, s, tok.Token, UsagePush{InstanceID: "gw-b",
			Buckets:  []admin.Bucket{bucket(minute, "user", 5, 2, 1)},
			Breakers: map[string]string{"users": "open"}})

		v := readUsage(t, s, c, minute.Add(-time.Minute))
		if len(v.Buckets) != 1 || v.Buckets[0].Requests != 15 || v.Buckets[0].Errors != 3 ||
			v.Buckets[0].Steps["u"].CacheHits != 5 {
			t.Fatalf("buckets = %+v", v.Buckets)
		}
		if len(v.Upstreams) != 1 || v.Upstreams[0].Requests != 15 || v.Upstreams[0].Breakers["gw-b"] != "open" {
			t.Fatalf("upstreams = %+v", v.Upstreams)
		}
	})
}

func TestUsageOlderThanRetentionIsDeleted(t *testing.T) {
	s := newTestServer(t, "sqlite:"+t.TempDir()+"/studio.db")
	c := signIn(t, s)
	createEnvironment(t, s, c, "prod")
	tok := issueToken(t, s, c, "prod")
	old := time.Now().UTC().Truncate(time.Minute).Add(-(defaultUsageRetention + time.Hour))
	pushUsage(t, s, tok.Token, UsagePush{InstanceID: "gw-a", Buckets: []admin.Bucket{bucket(old, "user", 3, 0, 0)}})
	pushUsage(t, s, tok.Token, UsagePush{InstanceID: "gw-a", Buckets: []admin.Bucket{bucket(time.Now().UTC().Truncate(time.Minute), "user", 1, 0, 0)}})

	v := readUsage(t, s, c, old.Add(-time.Hour))
	if len(v.Buckets) != 1 || v.Buckets[0].Requests != 1 {
		t.Fatalf("buckets past the 7-day retention must be gone: %+v", v.Buckets)
	}
}
