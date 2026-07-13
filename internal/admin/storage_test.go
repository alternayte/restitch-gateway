package admin

import (
	"context"
	"testing"
	"time"

	"github.com/restitch/restitch-gateway/internal/reqlog"
)

func TestAccumulator_Flush(t *testing.T) {
	acc := NewAccumulator()

	acc.Record("comp1", 42.0, false, false, []StepSample{
		{Name: "s1", Upstream: "api", DurationMS: 30.0, IsError: false},
		{Name: "s2", Upstream: "api", DurationMS: 12.0, IsError: false},
	})
	acc.Record("comp1", 150.0, true, false, []StepSample{
		{Name: "s1", Upstream: "api", DurationMS: 150.0, IsError: true},
	})
	acc.Record("comp2", 5.0, false, true, nil)

	buckets := acc.Flush()

	if len(buckets) != 3 {
		t.Fatalf("expected 3 buckets (global + 2 compositions), got %d", len(buckets))
	}

	var global *Bucket
	for i := range buckets {
		if buckets[i].Composition == "" {
			global = &buckets[i]
		}
	}
	if global == nil {
		t.Fatal("no global bucket")
	}
	if global.Requests != 3 {
		t.Errorf("global requests = %d, want 3", global.Requests)
	}
	if global.Errors != 1 {
		t.Errorf("global errors = %d, want 1", global.Errors)
	}
	if global.Partials != 1 {
		t.Errorf("global partials = %d, want 1", global.Partials)
	}
}

func TestAccumulator_LatencyBuckets(t *testing.T) {
	acc := NewAccumulator()
	acc.Record("c", 5.0, false, false, nil)   // bucket 0 (0-10ms)
	acc.Record("c", 75.0, false, false, nil)  // bucket 2 (50-100ms)
	acc.Record("c", 300.0, false, false, nil) // bucket 4 (250-500ms)

	buckets := acc.Flush()
	for _, b := range buckets {
		if b.Composition == "c" {
			if len(b.LatencyBuckets) != 8 {
				t.Fatalf("latency buckets length = %d, want 8", len(b.LatencyBuckets))
			}
			if b.LatencyBuckets[0] != 1 {
				t.Errorf("bucket[0] = %d, want 1", b.LatencyBuckets[0])
			}
			if b.LatencyBuckets[2] != 1 {
				t.Errorf("bucket[2] = %d, want 1", b.LatencyBuckets[2])
			}
			if b.LatencyBuckets[4] != 1 {
				t.Errorf("bucket[4] = %d, want 1", b.LatencyBuckets[4])
			}
			return
		}
	}
	t.Fatal("comp bucket not found")
}

func TestMemoryStorage_QueryTimeSeries(t *testing.T) {
	s := NewMemoryStorage(time.Hour)
	defer s.Close()

	now := time.Now().Truncate(time.Minute)
	for i := 0; i < 5; i++ {
		_ = s.RecordBucket(context.Background(), Bucket{
			Timestamp:   now.Add(time.Duration(i) * time.Minute),
			Composition: "",
			Requests:    int64(i + 1),
		})
	}

	results, err := s.QueryTimeSeries(context.Background(), now, now.Add(5*time.Minute), time.Minute, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 5 {
		t.Errorf("results length = %d, want 5", len(results))
	}
}

func TestMemoryStorage_Compact(t *testing.T) {
	s := NewMemoryStorage(time.Minute)
	defer s.Close()

	old := time.Now().Add(-2 * time.Minute).Truncate(time.Minute)
	recent := time.Now().Truncate(time.Minute)

	_ = s.RecordBucket(context.Background(), Bucket{Timestamp: old, Composition: "", Requests: 1})
	_ = s.RecordBucket(context.Background(), Bucket{Timestamp: recent, Composition: "", Requests: 2})

	_ = s.Compact(context.Background(), time.Minute)

	results, _ := s.QueryTimeSeries(context.Background(), old, recent.Add(time.Minute), time.Minute, "")
	if len(results) != 1 {
		t.Errorf("after compact, results = %d, want 1 (only recent)", len(results))
	}
}

func TestMemoryStorage_GetRequestByID(t *testing.T) {
	s := NewMemoryStorage(time.Hour)
	defer s.Close()

	_ = s.RecordRequest(context.Background(), reqlog.Record{ID: "req-1", Composition: "comp1", Time: time.Now()})
	_ = s.RecordRequest(context.Background(), reqlog.Record{ID: "req-2", Composition: "comp2", Time: time.Now()})

	rec, err := s.GetRequestByID(context.Background(), "req-2")
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil {
		t.Fatal("expected record, got nil")
	}
	if rec.Composition != "comp2" {
		t.Errorf("composition = %q, want comp2", rec.Composition)
	}

	missing, err := s.GetRequestByID(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if missing != nil {
		t.Errorf("expected nil for missing id, got %+v", missing)
	}
}
