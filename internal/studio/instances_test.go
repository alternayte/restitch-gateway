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
	"testing"
	"time"
)

func heartbeat(s *Server, token string, hb map[string]any) int {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(hb)
	req := httptest.NewRequest(http.MethodPost, testPublicURL+"/gateway/v1/heartbeat", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec.Code
}

type instanceView struct {
	InstanceID     string `json:"instance_id"`
	BinaryVersion  string `json:"binary_version"`
	AppliedVersion int    `json:"applied_version"`
	LastPullError  string `json:"last_pull_error"`
	Stale          bool   `json:"stale"`
}

func listInstances(t *testing.T, s *Server, c []*http.Cookie, env string) []instanceView {
	t.Helper()
	rec := do(t, s, http.MethodGet, "/api/v1/environments/"+env+"/instances", "", c, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("instances: %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Instances []instanceView `json:"instances"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Instances
}

func TestHeartbeatRecordsAppliedVersion(t *testing.T) {
	forEachStore(t, func(t *testing.T, s *Server) {
		c := signIn(t, s)
		createEnvironment(t, s, c, "prod")
		tok := issueToken(t, s, c, "prod")

		for _, hb := range []map[string]any{
			{"instance_id": "gw-a", "binary_version": "v2.0.0", "applied_version": 3},
			{"instance_id": "gw-b", "binary_version": "v2.0.0", "applied_version": 2, "last_pull_error": "studio returned status 502"},
			{"instance_id": "gw-a", "binary_version": "v2.0.0", "applied_version": 4},
		} {
			if code := heartbeat(s, tok.Token, hb); code != http.StatusNoContent {
				t.Fatalf("heartbeat: %d", code)
			}
		}
		got := listInstances(t, s, c, "prod")
		if len(got) != 2 {
			t.Fatalf("instances = %+v, want 2", got)
		}
		byID := map[string]instanceView{}
		for _, i := range got {
			byID[i.InstanceID] = i
		}
		if byID["gw-a"].AppliedVersion != 4 || byID["gw-b"].LastPullError == "" || byID["gw-a"].Stale {
			t.Fatalf("instances = %+v", got)
		}
	})
}

func TestSilentInstanceIsStale(t *testing.T) {
	s := newTestServer(t, "sqlite:"+t.TempDir()+"/studio.db")
	c := signIn(t, s)
	createEnvironment(t, s, c, "prod")
	tok := issueToken(t, s, c, "prod")
	heartbeat(s, tok.Token, map[string]any{"instance_id": "gw-a", "binary_version": "v2", "applied_version": 1})

	s.now = func() time.Time { return time.Now().Add(staleAfter + time.Second) }
	if got := listInstances(t, s, c, "prod"); len(got) != 1 || !got[0].Stale {
		t.Fatalf("instances = %+v, want one stale instance", got)
	}
}

func TestHeartbeatNeedsAGatewayToken(t *testing.T) {
	s := newTestServer(t, "sqlite:"+t.TempDir()+"/studio.db")
	if code := heartbeat(s, "rgt_unknown", map[string]any{"instance_id": "x"}); code != http.StatusUnauthorized {
		t.Fatalf("unknown token: got %d, want 401", code)
	}
}

func TestInstanceSilentForADayLeavesTheList(t *testing.T) {
	s := newTestServer(t, "sqlite:"+t.TempDir()+"/studio.db")
	c := signIn(t, s)
	createEnvironment(t, s, c, "prod")
	tok := issueToken(t, s, c, "prod")
	heartbeat(s, tok.Token, map[string]any{"instance_id": "gw-old", "binary_version": "v2", "applied_version": 1})

	s.now = func() time.Time { return time.Now().Add(forgetAfter + time.Minute) }
	if got := listInstances(t, s, c, "prod"); len(got) != 0 {
		t.Fatalf("instances = %+v, want none after a day of silence", got)
	}
}
