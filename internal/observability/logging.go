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

package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Setup configures the global slog default.
// format: "json"|"text"; level: "debug"|"info"|"warn"|"error".
func Setup(format, level string) error {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", level)
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var base slog.Handler
	switch strings.ToLower(format) {
	case "json":
		base = slog.NewJSONHandler(os.Stdout, opts)
	case "text":
		base = slog.NewTextHandler(os.Stdout, opts)
	default:
		return fmt.Errorf("invalid log format: %s (must be json or text)", format)
	}

	slog.SetDefault(slog.New(ContextHandler{base}))
	return nil
}

// ContextHandler wraps a slog.Handler and injects request_id from ctx.
type ContextHandler struct {
	slog.Handler
}

// Handle injects request_id from context into every log record.
func (h ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := GetRequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs returns a new ContextHandler wrapping the inner handler with the given attrs.
func (h ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ContextHandler{h.Handler.WithAttrs(attrs)}
}

// WithGroup returns a new ContextHandler wrapping the inner handler with the given group.
func (h ContextHandler) WithGroup(name string) slog.Handler {
	return ContextHandler{h.Handler.WithGroup(name)}
}
