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

package ratelimit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLimiter_AllowWithinBurst(t *testing.T) {
	l := New(Config{RequestsPerSecond: 1, Burst: 5, Key: "ip"})

	for i := 0; i < 5; i++ {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "192.168.1.1:1234"
		if !l.Allow(r) {
			t.Fatalf("request %d should be allowed within burst", i+1)
		}
	}
}

func TestLimiter_BlockAfterBurst(t *testing.T) {
	l := New(Config{RequestsPerSecond: 1, Burst: 3, Key: "ip"})

	for i := 0; i < 3; i++ {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "10.0.0.1:5678"
		l.Allow(r)
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:5678"
	if l.Allow(r) {
		t.Fatal("request should be blocked after burst is exhausted")
	}
}

func TestLimiter_DifferentKeysGetSeparateLimits(t *testing.T) {
	l := New(Config{RequestsPerSecond: 1, Burst: 2, Key: "ip"})

	// Exhaust burst for first IP
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "10.0.0.1:1111"
		l.Allow(r)
	}

	// First IP should be blocked
	r1 := httptest.NewRequest("GET", "/", nil)
	r1.RemoteAddr = "10.0.0.1:2222"
	if l.Allow(r1) {
		t.Fatal("first IP should be blocked")
	}

	// Second IP should still be allowed
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.RemoteAddr = "10.0.0.2:1111"
	if !l.Allow(r2) {
		t.Fatal("second IP should be allowed (separate bucket)")
	}
}

func TestLimiter_HeaderKey(t *testing.T) {
	l := New(Config{RequestsPerSecond: 1, Burst: 1, Key: "header:X-Client-ID"})

	r1 := httptest.NewRequest("GET", "/", nil)
	r1.Header.Set("X-Client-ID", "client-a")
	if !l.Allow(r1) {
		t.Fatal("first request for client-a should be allowed")
	}

	r2 := httptest.NewRequest("GET", "/", nil)
	r2.Header.Set("X-Client-ID", "client-a")
	if l.Allow(r2) {
		t.Fatal("second request for client-a should be blocked")
	}

	r3 := httptest.NewRequest("GET", "/", nil)
	r3.Header.Set("X-Client-ID", "client-b")
	if !l.Allow(r3) {
		t.Fatal("first request for client-b should be allowed")
	}
}

func TestLimiter_APIKeyKey(t *testing.T) {
	l := New(Config{RequestsPerSecond: 1, Burst: 1, Key: "api-key"})

	r1 := httptest.NewRequest("GET", "/", nil)
	r1.Header.Set("X-API-Key", "key-1")
	if !l.Allow(r1) {
		t.Fatal("first request for key-1 should be allowed")
	}

	r2 := httptest.NewRequest("GET", "/", nil)
	r2.Header.Set("X-API-Key", "key-1")
	if l.Allow(r2) {
		t.Fatal("second request for key-1 should be blocked")
	}

	r3 := httptest.NewRequest("GET", "/", nil)
	r3.Header.Set("X-API-Key", "key-2")
	if !l.Allow(r3) {
		t.Fatal("first request for key-2 should be allowed")
	}
}

func TestLimiter_DefaultKeyIsIP(t *testing.T) {
	l := New(Config{RequestsPerSecond: 1, Burst: 1})

	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:9999"
	if !l.Allow(r) {
		t.Fatal("first request should be allowed")
	}

	r2 := httptest.NewRequest("GET", "/", nil)
	r2.RemoteAddr = "10.0.0.1:8888"
	if l.Allow(r2) {
		t.Fatal("second request from same IP should be blocked")
	}
}

func TestLimiter_Middleware429(t *testing.T) {
	l := New(Config{RequestsPerSecond: 1, Burst: 1, Key: "ip"})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := l.Middleware(next)

	// First request: allowed
	r1 := httptest.NewRequest("GET", "/", nil)
	r1.RemoteAddr = "10.0.0.1:1234"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w1.Code)
	}

	// Second request: blocked
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.RemoteAddr = "10.0.0.1:1234"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", w2.Code)
	}

	if w2.Header().Get("Retry-After") != "1" {
		t.Errorf("Retry-After = %q, want \"1\"", w2.Header().Get("Retry-After"))
	}

	var body map[string]string
	if err := json.NewDecoder(w2.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "rate limit exceeded" {
		t.Errorf("error = %q, want \"rate limit exceeded\"", body["error"])
	}
}

func TestLimiter_EmptyKey(t *testing.T) {
	l := New(Config{RequestsPerSecond: 1, Burst: 1, Key: "header:X-Missing"})

	r := httptest.NewRequest("GET", "/", nil)
	// No X-Missing header => empty key => uses "__empty__"
	if !l.Allow(r) {
		t.Fatal("first request with empty key should be allowed")
	}
	r2 := httptest.NewRequest("GET", "/", nil)
	if l.Allow(r2) {
		t.Fatal("second request with empty key should be blocked (same bucket)")
	}
}

// TestLimiter_BoundedEntries covers finding M3: the per-key table must not
// grow without bound under an attacker minting distinct header keys. After
// maxDistinctKeys distinct keys, the oldest entries are evicted (LRU).
func TestLimiter_BoundedEntries(t *testing.T) {
	l := New(Config{RequestsPerSecond: 1000, Burst: 10, Key: "header:X-Client-ID"})

	// Saturate the table.
	for i := 0; i < maxDistinctKeys+500; i++ {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("X-Client-ID", fmt.Sprintf("attacker-%d", i))
		if !l.Allow(r) {
			t.Fatalf("request %d should be allowed (burst)", i)
		}
	}

	l.mu.Lock()
	size := len(l.entries)
	l.mu.Unlock()
	if size > maxDistinctKeys {
		t.Fatalf("entry count = %d, want <= %d", size, maxDistinctKeys)
	}

	// LRU eviction: the earliest keys are gone, so a fresh request for one
	// creates a new bucket instead of sharing the saturated oldest bucket.
	old := httptest.NewRequest("GET", "/", nil)
	old.Header.Set("X-Client-ID", "attacker-0")
	if !l.Allow(old) {
		t.Fatal("evicted key should get a fresh bucket")
	}
}
