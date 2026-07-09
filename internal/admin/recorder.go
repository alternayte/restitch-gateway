package admin

import (
	"context"
	"time"

	"github.com/restitch/restitch-gateway/internal/reqlog"
)

// MultiRecorder feeds request records to the ring buffer, stats, the
// time-series accumulator, and durable storage.
type MultiRecorder struct {
	Ring        *RingBuffer
	Stats       *Stats
	Accumulator *Accumulator
	Storage     Storage
}

func (mr *MultiRecorder) Record(rec reqlog.Record) {
	mr.Ring.Record(rec)
	mr.Stats.Record(rec.Composition, rec.DurationMS, rec.Status >= 500, rec.Partial)

	if mr.Accumulator != nil {
		var stepSamples []StepSample
		for _, s := range rec.Steps {
			stepSamples = append(stepSamples, StepSample{
				Name:       s.Name,
				Upstream:   s.Upstream,
				DurationMS: s.DurationMS,
				IsError:    s.Status == "failed",
			})
		}
		mr.Accumulator.Record(rec.Composition, rec.DurationMS, rec.Status >= 500, rec.Partial, stepSamples)
	}

	if mr.Storage != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		mr.Storage.RecordRequest(ctx, rec)
	}
}
