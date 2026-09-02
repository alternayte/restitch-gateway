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
	"net/http"
	"sync"
	"time"
)

const numShards = 16

// CachedResponse holds a cached upstream response.
type CachedResponse struct {
	Status  int
	Headers http.Header
	Body    []byte
}

type cacheEntry struct {
	resp    *CachedResponse
	expires time.Time
}

type cacheShard struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}

// StepCache is a sharded in-memory TTL cache for step responses.
type StepCache struct {
	shards [numShards]*cacheShard
	done   chan struct{}
}

// NewStepCache creates a new cache with a background janitor.
func NewStepCache() *StepCache {
	c := &StepCache{done: make(chan struct{})}
	for i := range c.shards {
		c.shards[i] = &cacheShard{entries: make(map[string]cacheEntry)}
	}
	go c.janitor()
	return c
}

// Close stops the janitor goroutine.
func (c *StepCache) Close() {
	close(c.done)
}

func (c *StepCache) shard(key string) *cacheShard {
	h := uint32(0)
	for _, b := range []byte(key) {
		h = h*31 + uint32(b)
	}
	return c.shards[h%numShards]
}

// Get retrieves a cached response. Returns nil, false if not found or expired.
func (c *StepCache) Get(key string) (*CachedResponse, bool) {
	s := c.shard(key)
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.resp, true
}

// Set stores a response with the given TTL.
func (c *StepCache) Set(key string, v *CachedResponse, ttl time.Duration) {
	s := c.shard(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = cacheEntry{resp: v, expires: time.Now().Add(ttl)}
}

func (c *StepCache) janitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.sweep()
		}
	}
}

func (c *StepCache) sweep() {
	now := time.Now()
	for _, s := range c.shards {
		s.mu.Lock()
		for k, e := range s.entries {
			if now.After(e.expires) {
				delete(s.entries, k)
			}
		}
		s.mu.Unlock()
	}
}
