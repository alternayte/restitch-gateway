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
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"sort"

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

// perRequestHeaders are set on every outgoing request with a value unique to
// that request (request IDs and trace context). They carry no request
// semantics; keying on them would give every request its own cache entry.
var perRequestHeaders = map[string]bool{
	"X-Request-Id":     true,
	"X-Correlation-Id": true,
	"Traceparent":      true,
	"Tracestate":       true,
	"Baggage":          true,
}

// RequestKey derives the cache and coalesce key from the outgoing upstream
// request: method, final URL as sent, every header except perRequestHeaders,
// the body, and the inbound client Authorization. The client Authorization
// is a separate input because the passthrough auth RoundTripper adds it to
// the request after the key is taken. Fields are length-prefixed and hashed
// with the full SHA-256 digest (finding L4).
func RequestKey(req *http.Request, clientAuthorization string) (string, error) {
	h := sha256.New()
	writeField(h, req.Method)
	writeField(h, req.URL.String())

	names := make([]string, 0, len(req.Header))
	for name := range req.Header {
		canonical := http.CanonicalHeaderKey(name)
		if !perRequestHeaders[canonical] {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		return http.CanonicalHeaderKey(names[i]) < http.CanonicalHeaderKey(names[j])
	})
	writeLen(h, len(names))
	for _, name := range names {
		writeField(h, http.CanonicalHeaderKey(name))
		values := req.Header[name]
		writeLen(h, len(values))
		for _, v := range values {
			writeField(h, v)
		}
	}

	var body []byte
	if req.GetBody != nil {
		rc, err := req.GetBody()
		if err != nil {
			return "", fmt.Errorf("read request body for key: %w", err)
		}
		body, err = io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return "", fmt.Errorf("read request body for key: %w", err)
		}
	}
	writeField(h, string(body))
	writeField(h, clientAuthorization)

	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeLen(h hash.Hash, n int) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(n))
	h.Write(buf[:])
}

func writeField(h hash.Hash, s string) {
	writeLen(h, len(s))
	h.Write([]byte(s))
}

// Do executes fn once per key among concurrent callers.
// Returns the result and whether it was shared from another caller.
func (c *Coalescer) Do(key string, fn func() (any, error)) (any, bool, error) {
	v, err, shared := c.g.Do(key, func() (any, error) {
		return fn()
	})
	return v, shared, err
}
