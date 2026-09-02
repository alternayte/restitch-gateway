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

package devmode

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPrefixWriter_SingleLine(t *testing.T) {
	var buf bytes.Buffer
	pw := NewPrefixWriter(&buf, "TEST", ColorCyan)
	n, err := pw.Write([]byte("hello world"))
	if err != nil {
		t.Fatal(err)
	}
	if n != len("hello world") {
		t.Errorf("n = %d, want %d", n, len("hello world"))
	}
	got := buf.String()
	want := ColorCyan + "[TEST]" + ColorReset + " hello world\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrefixWriter_MultiLine(t *testing.T) {
	var buf bytes.Buffer
	pw := NewPrefixWriter(&buf, "M", ColorMagenta)
	if _, err := pw.Write([]byte("line1\nline2\nline3")); err != nil {
		t.Fatalf("write: %v", err)
	}
	lines := strings.Split(buf.String(), "\n")
	// 3 prefixed lines + trailing empty
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4: %v", len(lines), lines)
	}
	for i, want := range []string{"line1", "line2", "line3"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want it to contain %q", i, lines[i], want)
		}
		if !strings.HasPrefix(lines[i], ColorMagenta+"[M]"+ColorReset) {
			t.Errorf("line %d missing prefix: %q", i, lines[i])
		}
	}
}

func TestPrefixWriter_ConcurrentWrites(t *testing.T) {
	buf := &safeBuffer{}
	pw := NewPrefixWriter(buf, "C", ColorCyan)
	var wg sync.WaitGroup
	const goroutines = 10
	const lines = 10
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < lines; j++ {
				// t.Fatal is not safe from a goroutine; a bytes.Buffer write cannot fail.
				_, _ = pw.Write([]byte("test line"))
			}
		}()
	}
	wg.Wait()
	count := strings.Count(buf.String(), "[C]")
	if count != goroutines*lines {
		t.Errorf("got %d prefixed lines, want %d", count, goroutines*lines)
	}
}

func TestPrefixWriter_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var buf bytes.Buffer
	pw := NewPrefixWriter(&buf, "X", ColorCyan)
	if _, err := pw.Write([]byte("plain")); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "\033") {
		t.Errorf("ANSI codes present with NO_COLOR: %q", got)
	}
	if !strings.HasPrefix(got, "[X] plain") {
		t.Errorf("got %q, want prefix [X]", got)
	}
}

func TestPrefixWriter_ReturnsByteCount(t *testing.T) {
	var buf bytes.Buffer
	pw := NewPrefixWriter(&buf, "B", ColorCyan)
	inputs := []string{"short", "a longer line", "multi\nline\ninput"}
	for _, in := range inputs {
		n, err := pw.Write([]byte(in))
		if err != nil {
			t.Fatal(err)
		}
		if n != len(in) {
			t.Errorf("Write(%q) = %d, want %d", in, n, len(in))
		}
	}
}

func TestWaitForHealth_Healthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	start := time.Now()
	err := WaitForHealth(context.Background(), srv.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Error("took too long for healthy server")
	}
}

func TestWaitForHealth_UnhealthyThenHealthy(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if count.Add(1) <= 3 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	err := WaitForHealth(context.Background(), srv.URL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if count.Load() < 4 {
		t.Errorf("expected >= 4 attempts, got %d", count.Load())
	}
}

func TestWaitForHealth_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()
	err := WaitForHealth(context.Background(), srv.URL, 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestWaitForHealth_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := WaitForHealth(ctx, srv.URL, 5*time.Second)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Error("should return immediately with canceled context")
	}
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
