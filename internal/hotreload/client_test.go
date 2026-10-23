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

package hotreload

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/alternayte/restitch-gateway/v2/internal/admin"
	"github.com/alternayte/restitch-gateway/v2/internal/composition"
)

func TestClient_FetchSendsTokenAndReadsVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gateway/v1/artifact" || r.Header.Get("Authorization") != "Bearer rgt_abc" {
			http.Error(w, "nope", http.StatusUnauthorized)
			return
		}
		bundleHandler("upstreams: {}\n", `"staging-3"`, 3)(w, r)
	}))
	defer srv.Close()

	c := NewClient(srv.URL+"/", "rgt_abc")
	res, err := c.Fetch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if string(res.YAML) != "upstreams: {}\n" || res.Version != 3 || res.ETag != `"staging-3"` {
		t.Fatalf("result = %+v", res)
	}
	res, err = c.Fetch(context.Background(), `"staging-3"`)
	if err != nil || !res.NotModified {
		t.Fatalf("same etag: %+v %v", res, err)
	}
}

func TestClient_FetchClassifiesStatus(t *testing.T) {
	for status, want := range map[int]error{
		http.StatusNotFound:     ErrNoVersion,
		http.StatusUnauthorized: ErrTokenRejected,
		http.StatusForbidden:    ErrTokenRejected,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		_, err := NewClient(srv.URL, "rgt_x").Fetch(context.Background(), "")
		srv.Close()
		if !errors.Is(err, want) {
			t.Errorf("status %d: err = %v, want %v", status, err, want)
		}
	}
}

func TestClient_FetchServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL, "rgt_x").Fetch(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_FetchConnectionRefused(t *testing.T) {
	if _, err := NewClient("http://127.0.0.1:1", "rgt_x").Fetch(context.Background(), ""); err == nil {
		t.Fatal("expected an error")
	}
}

func TestClient_FetchRefusesOversizedArtifact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", composition.MaxArtifactBytes+1)))
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL, "rgt_x").Fetch(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunHeartbeatReportsStatus(t *testing.T) {
	got := make(chan Heartbeat, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gateway/v1/heartbeat" || r.Header.Get("Authorization") != "Bearer rgt_abc" {
			http.Error(w, "nope", http.StatusUnauthorized)
			return
		}
		var hb Heartbeat
		_ = json.NewDecoder(r.Body).Decode(&hb)
		got <- hb
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	now := make(chan struct{}, 1)
	version := 1
	go RunHeartbeat(ctx, NewClient(srv.URL, "rgt_abc"), time.Hour, now, func() Heartbeat {
		return Heartbeat{InstanceID: "gw-1", BinaryVersion: "v2", AppliedVersion: version}
	})
	if hb := <-got; hb.InstanceID != "gw-1" || hb.AppliedVersion != 1 {
		t.Fatalf("first heartbeat = %+v", hb)
	}
	version = 2
	now <- struct{}{}
	select {
	case hb := <-got:
		if hb.AppliedVersion != 2 {
			t.Fatalf("second heartbeat = %+v", hb)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no heartbeat after the trigger")
	}
}

func TestUsagePusherRetriesOnceThenDrops(t *testing.T) {
	var fail atomic.Bool
	got := make(chan UsagePush, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		var p UsagePush
		_ = json.NewDecoder(r.Body).Decode(&p)
		got <- p
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	dropped := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "d"}, []string{"kind"})
	acc := admin.NewAccumulator()
	p := &UsagePusher{Client: NewClient(srv.URL, "rgt_x"), InstanceID: "gw-a", Acc: acc, Dropped: dropped,
		Breakers: func() map[string]string { return map[string]string{"users": "closed"} }}

	fail.Store(true)
	acc.Record("orders", 5, false, false, nil)
	p.Push(context.Background()) // fails, kept
	acc.Record("orders", 5, false, false, nil)
	p.Push(context.Background()) // fails again: the first window drops
	if v := testutil.ToFloat64(dropped.WithLabelValues("usage")); v == 0 {
		t.Fatal("the second failure must count the dropped buckets")
	}

	fail.Store(false)
	acc.Record("orders", 5, false, false, nil)
	p.Push(context.Background())
	push := <-got
	var requests int64
	for _, b := range push.Buckets {
		if b.Composition == "orders" {
			requests += b.Requests
		}
	}
	if requests != 2 || push.Breakers["users"] != "closed" || push.InstanceID != "gw-a" {
		t.Fatalf("push = %+v (orders requests %d, want 2: the kept window plus the new one)", push, requests)
	}
}
