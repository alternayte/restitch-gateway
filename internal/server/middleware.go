package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/restitch/restitch-gateway/internal/observability"
)

// LogFormat specifies the format for request logging.
type LogFormat string

const (
	// LogFormatJSON outputs structured JSON logs.
	LogFormatJSON LogFormat = "json"
	// LogFormatText outputs human-readable text logs.
	LogFormatText LogFormat = "text"
)

// logEntry represents a single request log entry.
// Field names use snake_case per CONTEXT.md requirements.
type logEntry struct {
	Time       string  `json:"time"`
	RequestID  string  `json:"request_id"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	StatusCode int     `json:"status_code"`
	DurationMS float64 `json:"duration_ms"`
	RemoteAddr string  `json:"remote_addr"`
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// newResponseWriter creates a new responseWriter wrapping the given http.ResponseWriter.
func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // default status
	}
}

// WriteHeader captures the status code before writing it.
func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

// Write calls WriteHeader with 200 if not already called, then writes the body.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// NewLoggingMiddleware creates a new logging middleware with the specified format.
func NewLoggingMiddleware(format LogFormat) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			rw := newResponseWriter(w)

			// Call the next handler
			next.ServeHTTP(rw, r)

			// Calculate duration
			duration := time.Since(start)
			durationMS := float64(duration.Nanoseconds()) / 1e6

			// Extract request ID from context (requires RequestIDMiddleware to run first)
			requestID := observability.GetRequestID(r.Context())

			// Create log entry
			entry := logEntry{
				Time:       start.UTC().Format(time.RFC3339),
				RequestID:  requestID,
				Method:     r.Method,
				Path:       r.URL.Path,
				StatusCode: rw.statusCode,
				DurationMS: durationMS,
				RemoteAddr: r.RemoteAddr,
			}

			// Output log based on format
			switch format {
			case LogFormatText:
				fmt.Fprintf(os.Stdout, "%s %s %s %s %d %.2fms %s\n",
					entry.Time, entry.RequestID, entry.Method, entry.Path, entry.StatusCode, entry.DurationMS, entry.RemoteAddr)
			default:
				// JSON format (default)
				json.NewEncoder(os.Stdout).Encode(entry)
			}
		})
	}
}
