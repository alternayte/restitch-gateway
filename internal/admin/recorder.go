package admin

import "github.com/restitch/restitch-gateway/internal/reqlog"

// MultiRecorder feeds request records to both the ring buffer and stats.
type MultiRecorder struct {
	Ring  *RingBuffer
	Stats *Stats
}

func (mr *MultiRecorder) Record(rec reqlog.Record) {
	mr.Ring.Record(rec)
	mr.Stats.Record(rec.Composition, rec.DurationMS, rec.Status >= 500, rec.Partial)
}
