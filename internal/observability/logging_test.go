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
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestSetup_JSON(t *testing.T) {
	if err := Setup("json", "info"); err != nil {
		t.Fatalf("Setup(json, info) = %v", err)
	}
}

func TestSetup_Text(t *testing.T) {
	if err := Setup("text", "debug"); err != nil {
		t.Fatalf("Setup(text, debug) = %v", err)
	}
}

func TestSetup_InvalidFormat(t *testing.T) {
	if err := Setup("xml", "info"); err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestSetup_InvalidLevel(t *testing.T) {
	if err := Setup("json", "verbose"); err == nil {
		t.Error("expected error for invalid level")
	}
}

func TestContextHandler_InjectsRequestID(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler := ContextHandler{base}

	logger := slog.New(handler)

	ctx := withRequestID(context.Background(), "test-id-123")
	logger.InfoContext(ctx, "test message")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log: %v", err)
	}

	if entry["request_id"] != "test-id-123" {
		t.Errorf("request_id = %v, want test-id-123", entry["request_id"])
	}
}

func TestContextHandler_NoRequestID(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler := ContextHandler{base}

	logger := slog.New(handler)
	logger.InfoContext(context.Background(), "no id")

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log: %v", err)
	}

	if _, ok := entry["request_id"]; ok {
		t.Error("request_id should not be present when not in context")
	}
}
