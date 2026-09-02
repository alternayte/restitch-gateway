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
	"testing"
	"time"
)

func TestStepCache_GetSet(t *testing.T) {
	c := NewStepCache()
	defer c.Close()

	resp := &CachedResponse{Status: 200, Headers: http.Header{"X-Test": {"val"}}, Body: []byte(`{"ok":true}`)}
	c.Set("key1", resp, 1*time.Second)

	got, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Status != 200 {
		t.Errorf("status = %d, want 200", got.Status)
	}
}

func TestStepCache_Expiry(t *testing.T) {
	c := NewStepCache()
	defer c.Close()

	c.Set("key1", &CachedResponse{Status: 200}, 10*time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	_, ok := c.Get("key1")
	if ok {
		t.Error("expected cache miss after expiry")
	}
}

func TestStepCache_Miss(t *testing.T) {
	c := NewStepCache()
	defer c.Close()

	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected cache miss for nonexistent key")
	}
}

func TestStepCache_DifferentKeys(t *testing.T) {
	c := NewStepCache()
	defer c.Close()

	c.Set("a", &CachedResponse{Status: 200}, 1*time.Second)
	c.Set("b", &CachedResponse{Status: 201}, 1*time.Second)

	a, ok := c.Get("a")
	if !ok || a.Status != 200 {
		t.Errorf("key a: got %v, want 200", a)
	}

	b, ok := c.Get("b")
	if !ok || b.Status != 201 {
		t.Errorf("key b: got %v, want 201", b)
	}
}
