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
	"sync"

	"github.com/alternayte/restitch-gateway/internal/reqlog"
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
