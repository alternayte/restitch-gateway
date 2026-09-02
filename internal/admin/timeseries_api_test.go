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

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alternayte/restitch-gateway/internal/reqlog"
)

func TestHandleTimeSeries(t *testing.T) {
	store := NewMemoryStorage(time.Hour)
	now := time.Now().Truncate(time.Minute)
	_ = store.RecordBucket(context.Background(), Bucket{
		Timestamp:   now,
		Composition: "",
		Requests:    10,
		Errors:      1,
	})

	s := &Server{
		deps: Deps{Storage: store},
	}

	req := httptest.NewRequest("GET", "/admin/api/stats/timeseries?range=1h&resolution=1m", nil)
	w := httptest.NewRecorder()
	s.handleTimeSeries(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var buckets []Bucket
	_ = json.NewDecoder(w.Body).Decode(&buckets)
	if len(buckets) == 0 {
		t.Error("expected at least one bucket")
	}
}

func TestHandleRequestByID(t *testing.T) {
	store := NewMemoryStorage(time.Hour)
	_ = store.RecordRequest(context.Background(), reqlog.Record{
		ID:          "test-123",
		Composition: "comp1",
		Status:      200,
		DurationMS:  42.5,
		Time:        time.Now(),
	})

	s := &Server{
		deps: Deps{Storage: store},
	}

	req := httptest.NewRequest("GET", "/admin/api/requests/test-123", nil)
	req.SetPathValue("id", "test-123")
	w := httptest.NewRecorder()
	s.handleRequestByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}
