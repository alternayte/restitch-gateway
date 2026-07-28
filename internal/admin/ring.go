package admin

import (
	"sync"

	"github.com/restitch/restitch-gateway/internal/reqlog"
)

// RingBuffer is a fixed-size circular buffer for request records.
type RingBuffer struct {
	mu      sync.Mutex
	entries []reqlog.Record
	size    int
	idx     int
	count   int
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer(size int) *RingBuffer {
	if size <= 0 {
		size = 500
	}
	return &RingBuffer{
		entries: make([]reqlog.Record, size),
		size:    size,
	}
}

// Record adds a request record to the ring buffer.
func (rb *RingBuffer) Record(rec reqlog.Record) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.entries[rb.idx] = rec
	rb.idx = (rb.idx + 1) % rb.size
	if rb.count < rb.size {
		rb.count++
	}
}

// List returns the newest-first entries, up to limit.
func (rb *RingBuffer) List(limit int) []reqlog.Record {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if limit <= 0 || limit > rb.count {
		limit = rb.count
	}

	result := make([]reqlog.Record, limit)
	for i := 0; i < limit; i++ {
		pos := (rb.idx - 1 - i + rb.size) % rb.size
		result[i] = rb.entries[pos]
	}
	return result
}
