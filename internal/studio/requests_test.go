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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alternayte/restitch-gateway/v2/internal/reqlog"
)

func pushRequests(t *testing.T, s *Server, token string, recs []reqlog.Record) int {
	t.Helper()
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(RequestsPush{InstanceID: "gw-a", Requests: recs})
	req := httptest.NewRequest(http.MethodPost, testPublicURL+"/gateway/v1/requests", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec.Code
}

func record(id string, status int, at time.Time) reqlog.Record {
	return reqlog.Record{
		ID: id, Time: at, Composition: "user", Method: "GET", Path: "/user", Status: status, DurationMS: 12,
		Headers: map[string]string{"Authorization": "Bearer leaked", "Accept": "application/json"},
		Steps:   []reqlog.StepRecord{{Name: "u", Upstream: "users", URL: "https://users/u?token=leaked", Status: "success"}},
	}
}

func TestSampleNeverCarriesBodyOrAuthorization(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		createEnvironment(t, s, c, "prod")
		tok := issueToken(t, s, c, "prod")
		if code := pushRequests(t, s, tok.Token, []reqlog.Record{record("r1", 502, time.Now().UTC())}); code != http.StatusNoContent {
			t.Fatalf("push: %d", code)
		}
		rec := do(t, s, http.MethodGet, "/api/v1/environments/prod/requests", "", c, nil)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "r1") {
			t.Fatalf("list: %d %s", rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), "leaked") {
			t.Fatalf("a secret reached Studio storage: %s", rec.Body)
		}
	})
}

func TestSuccessSamplesAreCappedPerMinute(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		createEnvironment(t, s, c, "prod")
		tok := issueToken(t, s, c, "prod")
		rec := do(t, s, http.MethodPatch, "/api/v1/environments/prod", testPublicURL, c, map[string]int{"request_samples_per_minute": 2})
		if rec.Code != http.StatusOK {
			t.Fatalf("settings: %d %s", rec.Code, rec.Body)
		}
		now := time.Now().UTC().Truncate(time.Minute).Add(10 * time.Second)
		var recs []reqlog.Record
		for i := 0; i < 5; i++ {
			recs = append(recs, record(fmt.Sprintf("ok%d", i), 200, now))
		}
		recs = append(recs, record("fail1", 500, now), record("fail2", 503, now))
		pushRequests(t, s, tok.Token, recs)

		rec = do(t, s, http.MethodGet, "/api/v1/environments/prod/requests?limit=100", "", c, nil)
		var out struct {
			Requests []struct {
				ID     string `json:"id"`
				Status int    `json:"status"`
			} `json:"requests"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		ok, failed := 0, 0
		for _, r := range out.Requests {
			if r.Status >= 500 {
				failed++
			} else {
				ok++
			}
		}
		if ok != 2 || failed != 2 {
			t.Fatalf("kept %d successes and %d failures, want 2 and 2: %s", ok, failed, rec.Body)
		}
	})
}
