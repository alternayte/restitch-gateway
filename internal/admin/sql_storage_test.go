package admin

import (
	"context"
	"testing"
	"time"

	"github.com/alternayte/restitch-gateway/internal/reqlog"
)

func TestSQLStorage_RecordAndQuery(t *testing.T) {
	s, err := NewSQLStorage("file::memory:?cache=shared", "", 24*time.Hour)
	if err != nil {
		t.Fatalf("NewSQLStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().Truncate(time.Minute)

	err = s.RecordBucket(ctx, Bucket{
		Timestamp:   now,
		Composition: "",
		Requests:    10,
		Errors:      2,
	})
	if err != nil {
		t.Fatal(err)
	}

	results, err := s.QueryTimeSeries(ctx, now.Add(-time.Minute), now.Add(time.Minute), time.Minute, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].Requests != 10 {
		t.Errorf("requests = %d, want 10", results[0].Requests)
	}
}

func TestSQLStorage_RecordRequest(t *testing.T) {
	s, err := NewSQLStorage("file::memory:?cache=shared", "", 24*time.Hour)
	if err != nil {
		t.Fatalf("NewSQLStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	rec := reqlog.Record{
		ID:          "req-1",
		Time:        time.Now(),
		Composition: "test",
		Method:      "GET",
		Path:        "/api/test",
		Status:      200,
		DurationMS:  42.5,
	}
	if err := s.RecordRequest(ctx, rec); err != nil {
		t.Fatal(err)
	}

	results, err := s.QueryRequests(ctx, RequestQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].ID != "req-1" {
		t.Errorf("id = %q, want %q", results[0].ID, "req-1")
	}
}

func TestSQLStorage_GetRequestByID(t *testing.T) {
	s, err := NewSQLStorage("file::memory:?cache=shared", "", 24*time.Hour)
	if err != nil {
		t.Fatalf("NewSQLStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	rec := reqlog.Record{
		ID:          "req-1",
		Time:        time.Now(),
		Composition: "test",
		Method:      "GET",
		Path:        "/api/test",
		Status:      200,
		DurationMS:  42.5,
	}
	if err := s.RecordRequest(ctx, rec); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetRequestByID(ctx, "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("got nil record, want req-1")
	}
	if got.ID != "req-1" {
		t.Errorf("id = %q, want %q", got.ID, "req-1")
	}

	missing, err := s.GetRequestByID(ctx, "does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if missing != nil {
		t.Errorf("missing = %+v, want nil", missing)
	}
}

func TestSQLStorage_QueryStepMetrics(t *testing.T) {
	s, err := NewSQLStorage("file::memory:?cache=shared", "", 24*time.Hour)
	if err != nil {
		t.Fatalf("NewSQLStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().Truncate(time.Minute)

	err = s.RecordBucket(ctx, Bucket{
		Timestamp:   now,
		Composition: "comp-a",
		Requests:    5,
		StepMetrics: map[string]*StepBucket{
			"step-1": {
				Requests: 5,
				Errors:   1,
				AvgMS:    20,
				P95MS:    50,
				Upstream: "upstream-a",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	results, err := s.QueryStepMetrics(ctx, "comp-a", now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].Name != "step-1" {
		t.Errorf("name = %q, want %q", results[0].Name, "step-1")
	}
	if results[0].Upstream != "upstream-a" {
		t.Errorf("upstream = %q, want %q", results[0].Upstream, "upstream-a")
	}
	if results[0].Requests != 5 {
		t.Errorf("requests = %d, want 5", results[0].Requests)
	}
}

func TestSQLStorage_QueryRequestsFiltering(t *testing.T) {
	s, err := NewSQLStorage("file::memory:?cache=shared", "", 24*time.Hour)
	if err != nil {
		t.Fatalf("NewSQLStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	base := time.Now()
	partialTrue := true

	records := []reqlog.Record{
		{ID: "1", Time: base, Composition: "comp-a", Status: 200, DurationMS: 10, Partial: false},
		{ID: "2", Time: base.Add(time.Second), Composition: "comp-b", Status: 500, DurationMS: 200, Partial: true},
		{ID: "3", Time: base.Add(2 * time.Second), Composition: "comp-a", Status: 404, DurationMS: 30, Partial: false},
	}
	for _, r := range records {
		if err := s.RecordRequest(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	results, err := s.QueryRequests(ctx, RequestQuery{Composition: "comp-a", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("comp-a results = %d, want 2", len(results))
	}

	results, err = s.QueryRequests(ctx, RequestQuery{StatusMin: 400, StatusMax: 599, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("status filter results = %d, want 2", len(results))
	}

	results, err = s.QueryRequests(ctx, RequestQuery{Partial: &partialTrue, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "2" {
		t.Fatalf("partial filter results = %+v, want [2]", results)
	}

	results, err = s.QueryRequests(ctx, RequestQuery{MinDuration: 100, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "2" {
		t.Fatalf("duration filter results = %+v, want [2]", results)
	}
}

func TestSQLStorage_Compact(t *testing.T) {
	s, err := NewSQLStorage("file::memory:?cache=shared", "", time.Minute)
	if err != nil {
		t.Fatalf("NewSQLStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	old := time.Now().Add(-2 * time.Minute).Truncate(time.Minute)
	recent := time.Now().Truncate(time.Minute)

	_ = s.RecordBucket(ctx, Bucket{Timestamp: old, Requests: 1})
	_ = s.RecordBucket(ctx, Bucket{Timestamp: recent, Requests: 2})

	_ = s.Compact(ctx, time.Minute)

	results, _ := s.QueryTimeSeries(ctx, old, recent.Add(time.Minute), time.Minute, "")
	if len(results) != 1 {
		t.Errorf("after compact = %d, want 1", len(results))
	}
}
