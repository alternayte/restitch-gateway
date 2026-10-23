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
	"net/http/httptest"
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

func TestRequestKey(t *testing.T) {
	key := func(url, auth string, headers map[string]string) string {
		req := httptest.NewRequest("GET", url, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		k, err := RequestKey(req, auth)
		if err != nil {
			t.Fatalf("RequestKey: %v", err)
		}
		return k
	}

	base := key("http://example.com/users/1", "Bearer abc", map[string]string{"Accept": "application/json"})
	if k := key("http://example.com/users/1", "Bearer abc", map[string]string{"Accept": "application/json", "X-Request-ID": "r2", "traceparent": "00-x"}); k != base {
		t.Error("same request with different per-request IDs should produce same key")
	}
	if k := key("http://example.com/users/1", "Bearer xyz", map[string]string{"Accept": "application/json"}); k == base {
		t.Error("different auth should produce different key")
	}
	if k := key("http://example.com/users/1", "Bearer abc", map[string]string{"Accept": "text/html"}); k == base {
		t.Error("different header value should produce different key")
	}
	if key("http://example.com/x%2Fy/z", "", nil) == key("http://example.com/x/y%2Fz", "", nil) {
		t.Error("different escaped URLs should produce different keys")
	}
}
