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
	"crypto/sha256"
	"fmt"

	"golang.org/x/sync/singleflight"
)

// Coalescer deduplicates concurrent identical requests using singleflight.
type Coalescer struct {
	g singleflight.Group
}

// NewCoalescer creates a new Coalescer.
func NewCoalescer() *Coalescer {
	return &Coalescer{}
}

// CoalesceKey builds a cache/coalesce key from method, URL, and auth identity.
func CoalesceKey(method, url, authIdentity string) string {
	h := sha256.Sum256([]byte(authIdentity))
	return fmt.Sprintf("%s %s %x", method, url, h[:8])
}

// Do executes fn once per key among concurrent callers.
// Returns the result and whether it was shared from another caller.
func (c *Coalescer) Do(key string, fn func() (any, error)) (any, bool, error) {
	v, err, shared := c.g.Do(key, func() (any, error) {
		return fn()
	})
	return v, shared, err
}
