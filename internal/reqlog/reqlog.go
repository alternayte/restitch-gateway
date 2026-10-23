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

package reqlog

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Redacted replaces a secret value in a record.
const Redacted = "[redacted]"

// Record represents a single request record shared between the composition
// handler and telemetry. It never holds a request or response body.
type Record struct {
	ID          string    `json:"id"`
	TraceID     string    `json:"trace_id,omitempty"`
	Time        time.Time `json:"time"`
	Composition string    `json:"composition"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	Status      int       `json:"status"`
	DurationMS  float64   `json:"duration_ms"`
	Partial     bool      `json:"partial"`
	// Headers are the inbound request headers after RedactHeaders.
	Headers map[string]string `json:"headers,omitempty"`
	Steps   []StepRecord      `json:"steps"`
}

// StepRecord represents a single step's outcome within a request.
type StepRecord struct {
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	Wave          int     `json:"wave"`
	DurationMS    float64 `json:"duration_ms"`
	HTTPStatus    int     `json:"http_status"`
	Upstream      string  `json:"upstream"`
	URL           string  `json:"url"`
	StartOffsetMS float64 `json:"start_offset_ms"`
	BodySize      int64   `json:"body_size"`
	Error         string  `json:"error,omitempty"`
	Cached        bool    `json:"cached"`
	Retries       int     `json:"retries"`
}

// Recorder is the interface composition uses to report request records.
type Recorder interface {
	Record(rec Record)
}

// secretHeader reports whether a header name carries a credential.
func secretHeader(name string) bool {
	n := strings.ToLower(name)
	switch n {
	case "authorization", "proxy-authorization", "cookie", "set-cookie":
		return true
	}
	for _, part := range []string{"token", "secret", "key", "password", "auth", "session"} {
		if strings.Contains(n, part) {
			return true
		}
	}
	return false
}

// RedactHeaders flattens h and replaces every credential value with
// Redacted.
func RedactHeaders(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for name, values := range h {
		if secretHeader(name) {
			out[name] = Redacted
			continue
		}
		out[name] = strings.Join(values, ", ")
	}
	return out
}

// RedactURL keeps the scheme, host and path of raw and replaces every query
// value and any user info with Redacted, because templated query strings
// often carry tokens.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return Redacted
	}
	if u.User != nil {
		u.User = url.User(Redacted)
	}
	q := u.Query()
	if len(q) == 0 {
		return u.String()
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = url.QueryEscape(k) + "=" + Redacted
	}
	u.RawQuery = strings.Join(parts, "&")
	return u.String()
}

// Redact applies RedactHeaders' rule and RedactURL to a record that has
// already been built, for a receiver that cannot trust the sender.
func (r Record) Redact() Record {
	if r.Headers != nil {
		h := make(map[string]string, len(r.Headers))
		for k, v := range r.Headers {
			if secretHeader(k) {
				v = Redacted
			}
			h[k] = v
		}
		r.Headers = h
	}
	steps := make([]StepRecord, len(r.Steps))
	for i, s := range r.Steps {
		s.URL = RedactURL(s.URL)
		steps[i] = s
	}
	r.Steps = steps
	return r
}
