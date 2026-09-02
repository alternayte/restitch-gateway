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

// Package observability provides request tracing and logging utilities.
package observability

import (
	"context"
	"crypto/rand"
	"net/http"
	"time"

	"github.com/oklog/ulid/v2"
)

// contextKey is a type-safe key for context values.
type contextKey string

const (
	// RequestIDHeader is the HTTP header name for request ID.
	RequestIDHeader = "X-Request-ID"

	// requestIDKey is the context key for storing request ID.
	requestIDKey contextKey = "request_id"
)

// NewRequestID generates a new ULID-formatted request ID.
// Uses crypto/rand for entropy source to ensure collision-safe IDs.
func NewRequestID() string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

// GetRequestID extracts the request ID from the context.
// Returns an empty string if no request ID is present.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// withRequestID returns a new context with the request ID stored.
func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDMiddleware is HTTP middleware that extracts or generates a request ID.
// If an X-Request-ID header is present in the request, it is used.
// Otherwise, a new ULID is generated.
// The request ID is stored in the request context and set as a response header.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for existing request ID in header
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			// Generate new ULID if no header present
			requestID = NewRequestID()
		}

		// Store in context
		ctx := withRequestID(r.Context(), requestID)

		// Set response header
		w.Header().Set(RequestIDHeader, requestID)

		// Call next handler with enriched context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
