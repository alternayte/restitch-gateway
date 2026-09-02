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

// Package ratelimit provides per-key token-bucket rate limiting for HTTP requests.
package ratelimit

import (
	"container/list"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

// Config configures a rate limiter.
type Config struct {
	RequestsPerSecond float64 `yaml:"requests_per_second" json:"requests_per_second"`
	Burst             int     `yaml:"burst" json:"burst"`
	Key               string  `yaml:"key" json:"key"` // "ip", "header:X-Client-ID", "api-key"
}

// maxDistinctKeys bounds the per-key limiter table. In header:<name> mode an
// attacker can mint unlimited distinct keys; without a bound the table grows
// without limit (finding M3). The bound is generous for real traffic and the
// eviction is LRU, so the most active keys survive.
const maxDistinctKeys = 10_000

// Limiter is a bounded per-key token-bucket rate limiter.
type Limiter struct {
	rate    rate.Limit
	burst   int
	keyFunc func(*http.Request) string

	mu      sync.Mutex
	entries map[string]*list.Element
	lru     *list.List // front = most recently used
}

type lruItem struct {
	key     string
	limiter *rate.Limiter
}

// New creates a new Limiter from the given config.
func New(cfg Config) *Limiter {
	l := &Limiter{
		rate:    rate.Limit(cfg.RequestsPerSecond),
		burst:   cfg.Burst,
		entries: make(map[string]*list.Element),
		lru:     list.New(),
	}

	key := cfg.Key
	if key == "" {
		key = "ip"
	}

	switch {
	case key == "ip":
		l.keyFunc = extractIP
	case key == "api-key":
		l.keyFunc = func(r *http.Request) string {
			return r.Header.Get("X-API-Key")
		}
	case strings.HasPrefix(key, "header:"):
		header := strings.TrimPrefix(key, "header:")
		l.keyFunc = func(r *http.Request) string {
			return r.Header.Get(header)
		}
	default:
		l.keyFunc = extractIP
	}

	return l
}

// Allow reports whether the request is allowed under the rate limit.
func (l *Limiter) Allow(r *http.Request) bool {
	key := l.keyFunc(r)
	if key == "" {
		key = "__empty__"
	}

	l.mu.Lock()
	lim := l.limiterFor(key)
	l.mu.Unlock()
	return lim.Allow()
}

// limiterFor returns the token bucket for key, creating it if needed. The
// caller must hold l.mu. When the table is full, the least-recently-used
// entry is evicted first.
func (l *Limiter) limiterFor(key string) *rate.Limiter {
	if el, ok := l.entries[key]; ok {
		l.lru.MoveToFront(el)
		return el.Value.(*lruItem).limiter
	}
	if l.lru.Len() >= maxDistinctKeys {
		if back := l.lru.Back(); back != nil {
			item := back.Value.(*lruItem)
			delete(l.entries, item.key)
			l.lru.Remove(back)
		}
	}
	item := &lruItem{key: key, limiter: rate.NewLimiter(l.rate, l.burst)}
	l.entries[key] = l.lru.PushFront(item)
	return item.limiter
}

// Middleware returns an http.Handler that wraps next with rate limiting.
// Rejected requests receive a 429 response with Retry-After header.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.Allow(r) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractIP returns the IP address from the request, stripping the port.
func extractIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
