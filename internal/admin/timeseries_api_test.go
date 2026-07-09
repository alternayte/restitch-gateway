package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/restitch/restitch-gateway/internal/reqlog"
)

func TestHandleTimeSeries(t *testing.T) {
	store := NewMemoryStorage(time.Hour)
	now := time.Now().Truncate(time.Minute)
	store.RecordBucket(nil, Bucket{
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
	json.NewDecoder(w.Body).Decode(&buckets)
	if len(buckets) == 0 {
		t.Error("expected at least one bucket")
	}
}

func TestHandleRequestByID(t *testing.T) {
	store := NewMemoryStorage(time.Hour)
	store.RecordRequest(nil, reqlog.Record{
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
