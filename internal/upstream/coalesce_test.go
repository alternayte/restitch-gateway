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

package upstream

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCoalescer_DeduplicatesConcurrent(t *testing.T) {
	c := NewCoalescer()
	var calls atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, _, err := c.Do("key1", func() (any, error) {
				calls.Add(1)
				time.Sleep(10 * time.Millisecond)
				return "result", nil
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if v != "result" {
				t.Errorf("got %v, want result", v)
			}
		}()
	}
	wg.Wait()

	if calls.Load() != 1 {
		t.Errorf("expected 1 call, got %d", calls.Load())
	}
}

func TestCoalescer_DifferentKeysNotCoalesced(t *testing.T) {
	c := NewCoalescer()
	var calls atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		key := "key" + string(rune('a'+i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = c.Do(key, func() (any, error) {
				calls.Add(1)
				time.Sleep(10 * time.Millisecond)
				return "result", nil
			})
		}()
	}
	wg.Wait()

	if calls.Load() != 2 {
		t.Errorf("expected 2 calls for different keys, got %d", calls.Load())
	}
}

func TestCoalesceKey(t *testing.T) {
	k1 := CoalesceKey("GET", "http://example.com/users/1", "Bearer abc")
	k2 := CoalesceKey("GET", "http://example.com/users/1", "Bearer abc")
	k3 := CoalesceKey("GET", "http://example.com/users/1", "Bearer xyz")

	if k1 != k2 {
		t.Error("same inputs should produce same key")
	}
	if k1 == k3 {
		t.Error("different auth should produce different key")
	}
}
