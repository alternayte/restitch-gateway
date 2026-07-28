# M22 — Dev Mode Orchestrator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `restitch dev` command that runs gateway + studio together with colored logs, auto-restart, and health-check sequencing.

**Architecture:** New `internal/devmode/` package with ProcessManager (child process lifecycle) and PrefixWriter (colored line-prefixed output). CLI glue in `cmd/restitch/dev.go` discovers the studio binary, spawns gateway first, waits for health, then spawns studio. Bonus: consolidate hand-rolled backoff in `internal/hotreload/` onto `cenkalti/backoff/v5`.

**Tech Stack:** Go stdlib (`os/exec`, `flag`, `sync`), `cenkalti/backoff/v5` (already indirect dep)

**Spec:** `docs/superpowers/specs/2026-07-16-m22-dev-mode-orchestrator-design.md`

## Global Constraints

- Go module: `github.com/restitch/restitch-gateway`, Go 1.25.6
- No new deps beyond promoting `cenkalti/backoff/v5` from indirect to direct
- No cobra, no `fatih/color` — stdlib flag + raw ANSI codes
- Commit messages: `feat(M22): <task title>` or `fix:/test:/docs:` as appropriate
- After every task: run the Accept command, paste output into commit
- After every task: append a row to `docs/plan-progress/LEDGER.md`
- Do not edit `scripts/verify.sh`, `scripts/check-ledger.sh`, `scripts/lib/`, or existing gate scripts

---

### Task 0: Write M22 gate script

**Files:**
- Modify: `scripts/gates/m22.sh`

**Interfaces:**
- Consumes: harness helpers from `scripts/lib/harness.sh` (`h_init`, `h_task`, `h_run`, `h_build`, `h_grep_repo`, `h_finish`)
- Produces: gate script that validates all M22 tasks

- [ ] **Step 1: Write the gate script**

Replace the placeholder in `scripts/gates/m22.sh`:

```bash
#!/usr/bin/env bash
# Gate M22 — Dev Mode Orchestrator
set -euo pipefail
source "$(dirname "$0")/../lib/harness.sh"
h_init M22

# ── T22.1: PrefixWriter + WaitForHealth ─────────────────────────────
h_task T22.1
h_run "writer.go exists" -- test -f internal/devmode/writer.go
h_run "writer_test.go exists" -- test -f internal/devmode/writer_test.go
h_run "PrefixWriter defined" -- grep -q 'type PrefixWriter struct' internal/devmode/writer.go
h_run "WaitForHealth defined" -- grep -q 'func WaitForHealth' internal/devmode/writer.go
h_run "NO_COLOR support" -- grep -q 'NO_COLOR' internal/devmode/writer.go

# ── T22.2: ProcessManager ───────────────────────────────────────────
h_task T22.2
h_run "manager.go exists" -- test -f internal/devmode/manager.go
h_run "manager_test.go exists" -- test -f internal/devmode/manager_test.go
h_run "ProcessConfig defined" -- grep -q 'type ProcessConfig struct' internal/devmode/manager.go
h_run "ProcessManager defined" -- grep -q 'type ProcessManager struct' internal/devmode/manager.go
h_run "uses cenkalti/backoff" -- grep -q 'cenkalti/backoff' internal/devmode/manager.go

# ── T22.3: Backoff consolidation ────────────────────────────────────
h_task T22.3
h_run "backoff.go deleted" -- test ! -f internal/hotreload/backoff.go
h_run "backoff_test.go deleted" -- test ! -f internal/hotreload/backoff_test.go
h_run "no backoffDuration references" -- \
  bash -c '! grep -rn "backoffDuration" internal/'
h_run "poller uses cenkalti" -- grep -q 'cenkalti/backoff' internal/hotreload/poller.go

# ── T22.4: CLI dev command ──────────────────────────────────────────
h_task T22.4
h_run "dev.go exists" -- test -f cmd/restitch/dev.go
h_run "devCmd function" -- grep -q 'func devCmd' cmd/restitch/dev.go
h_run "findStudioBinary function" -- grep -q 'func findStudioBinary' cmd/restitch/dev.go
h_run "dev case in main.go" -- grep -q '"dev"' cmd/restitch/main.go
h_run "usage updated" -- grep -q 'dev' cmd/restitch/main.go

# ── Unit tests ──────────────────────────────────────────────────────
h_task M22.unit
h_run "go vet" -- go vet ./...
h_run "devmode tests" -- go test -race -count=1 ./internal/devmode/...
h_run "hotreload tests" -- go test -race -count=1 ./internal/hotreload/...
h_run "full test suite" -- go test -race -count=1 ./...

# ── Build + help smoke ──────────────────────────────────────────────
h_task M22.gate
h_build
h_run "restitch dev --help exits 0" -- "${REPO_ROOT}/bin/restitch" dev --help

# MANUAL: live smoke test (spawn gateway + studio, verify colored output, Ctrl+C)
h_manual "Run 'make build-all && bin/restitch dev' — verify colored output, Ctrl+C shutdown"

h_finish
```

- [ ] **Step 2: Verify the gate script fails (expected — nothing implemented yet)**

Run: `bash scripts/gates/m22.sh`
Expected: FAIL on T22.1 ("writer.go exists")

- [ ] **Step 3: Commit**

```bash
git add scripts/gates/m22.sh
git commit -m "$(cat <<'EOF'
gate: M22 dev mode orchestrator gate script

Replace placeholder with checks for all M22 tasks: PrefixWriter,
ProcessManager, backoff consolidation, CLI dev command, unit tests,
and build smoke.
EOF
)"
```

**Accept:** `bash scripts/gates/m22.sh 2>&1 | head -5` shows FAIL at T22.1 (expected).

---

### Task 1: PrefixWriter + WaitForHealth (`internal/devmode/writer.go`)

**Files:**
- Create: `internal/devmode/writer.go`
- Create: `internal/devmode/writer_test.go`

**Interfaces:**
- Consumes: nothing (leaf package)
- Produces:
  - `NewPrefixWriter(dest io.Writer, prefix, colorCode string) *PrefixWriter`
  - `(*PrefixWriter).Write(p []byte) (int, error)`
  - `WaitForHealth(ctx context.Context, url string, timeout time.Duration) error`
  - Constants: `ColorCyan`, `ColorMagenta`, `ColorYellow`, `ColorReset`

- [ ] **Step 1: Write the failing tests**

Create `internal/devmode/writer_test.go`:

```go
package devmode

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
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
	pw.Write([]byte("line1\nline2\nline3"))
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
				pw.Write([]byte("test line"))
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
	pw.Write([]byte("plain"))
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devmode/ -v -count=1`
Expected: compilation error — package doesn't exist yet

- [ ] **Step 3: Implement writer.go**

Create `internal/devmode/writer.go`:

```go
package devmode

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
)

const (
	ColorCyan    = "\033[36m"
	ColorMagenta = "\033[35m"
	ColorYellow  = "\033[33m"
	ColorReset   = "\033[0m"
)

type PrefixWriter struct {
	mu        sync.Mutex
	dest      io.Writer
	prefix    string
	colorCode string
	noColor   bool
}

func NewPrefixWriter(dest io.Writer, prefix, colorCode string) *PrefixWriter {
	_, noColor := os.LookupEnv("NO_COLOR")
	return &PrefixWriter{
		dest:      dest,
		prefix:    prefix,
		colorCode: colorCode,
		noColor:   noColor,
	}
}

func (pw *PrefixWriter) Write(p []byte) (int, error) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	var tag string
	if pw.noColor {
		tag = fmt.Sprintf("[%s]", pw.prefix)
	} else {
		tag = fmt.Sprintf("%s[%s]%s", pw.colorCode, pw.prefix, ColorReset)
	}

	scanner := bufio.NewScanner(bytes.NewReader(p))
	for scanner.Scan() {
		fmt.Fprintf(pw.dest, "%s %s\n", tag, scanner.Text())
	}
	return len(p), scanner.Err()
}

func WaitForHealth(ctx context.Context, url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 200 * time.Millisecond
	bo.MaxInterval = 2 * time.Second

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := backoff.Retry(ctx, func() (struct{}, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return struct{}{}, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return struct{}{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return struct{}{}, fmt.Errorf("health check returned %d", resp.StatusCode)
		}
		return struct{}{}, nil
	}, backoff.WithBackOff(bo))
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/devmode/ -v -count=1 -race`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/devmode/writer.go internal/devmode/writer_test.go
git commit -m "$(cat <<'EOF'
feat(M22): PrefixWriter and WaitForHealth (T22.1)

Thread-safe line-prefixed io.Writer with ANSI color support
(NO_COLOR respected). WaitForHealth polls a URL with cenkalti/backoff
exponential backoff under a context timeout.
EOF
)"
```

**Accept:** `go test ./internal/devmode/ -v -count=1 -race` — all PASS.

---

### Task 2: ProcessManager (`internal/devmode/manager.go`)

**Files:**
- Create: `internal/devmode/manager.go`
- Create: `internal/devmode/manager_test.go`

**Interfaces:**
- Consumes: `cenkalti/backoff/v5`
- Produces:
  - `ProcessConfig{Name, Executable, Args, ExtraEnv string/[]string}`
  - `NewProcessManager(cfg ProcessConfig, stdout, stderr io.Writer) *ProcessManager`
  - `(*ProcessManager).Run(ctx context.Context) error`

- [ ] **Step 1: Write the failing tests**

Create `internal/devmode/manager_test.go`:

```go
package devmode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is not a real test — it's a child process helper.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)
	switch os.Getenv("GO_HELPER_CMD") {
	case "sleep":
		time.Sleep(time.Minute)
	case "exit1":
		fmt.Fprintln(os.Stdout, "exiting with error")
		os.Exit(1)
	case "quick":
		fmt.Fprintln(os.Stdout, "quick process done")
	default:
		fmt.Fprintf(os.Stderr, "unknown helper command: %s\n", os.Getenv("GO_HELPER_CMD"))
		os.Exit(2)
	}
}

func TestProcessManager_ContextCancel(t *testing.T) {
	stdout := &safeBuffer{}
	stderr := &safeBuffer{}

	pm := NewProcessManager(ProcessConfig{Name: "test"}, stdout, stderr)
	pm.cmdModifier = func(cmd *exec.Cmd) {
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "GO_HELPER_CMD=sleep")
	}
	pm.initialInterval = 10 * time.Millisecond
	pm.maxInterval = 50 * time.Millisecond
	pm.stableAfter = time.Second

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- pm.Run(ctx) }()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("got %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	if !strings.Contains(stdout.String(), "started (PID") {
		t.Errorf("missing 'started' message: %s", stdout.String())
	}
}

func TestProcessManager_RestartOnCrash(t *testing.T) {
	stdout := &safeBuffer{}
	stderr := &safeBuffer{}

	pm := NewProcessManager(ProcessConfig{Name: "crasher"}, stdout, stderr)
	pm.cmdModifier = func(cmd *exec.Cmd) {
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "GO_HELPER_CMD=exit1")
	}
	pm.initialInterval = 10 * time.Millisecond
	pm.maxInterval = 50 * time.Millisecond
	pm.stableAfter = time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- pm.Run(ctx) }()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}

	if !strings.Contains(stdout.String(), "restarting in") {
		t.Errorf("missing 'restarting in' in stdout: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "exited after") {
		t.Errorf("missing 'exited after' in stderr: %s", stderr.String())
	}
	startCount := strings.Count(stdout.String(), "started (PID")
	if startCount < 2 {
		t.Errorf("expected >= 2 starts, got %d", startCount)
	}
}

func TestProcessManager_ExtraEnv(t *testing.T) {
	stdout := &safeBuffer{}
	stderr := &safeBuffer{}

	pm := NewProcessManager(ProcessConfig{
		Name:     "env",
		ExtraEnv: []string{"TEST_EXTRA_VAR=hello"},
	}, stdout, stderr)

	var capturedEnv []string
	pm.cmdModifier = func(cmd *exec.Cmd) {
		capturedEnv = cmd.Env
		cmd.Env = append(cmd.Env, "GO_WANT_HELPER_PROCESS=1", "GO_HELPER_CMD=quick")
	}
	pm.initialInterval = 10 * time.Millisecond
	pm.maxInterval = 50 * time.Millisecond
	pm.stableAfter = time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- pm.Run(ctx) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not complete")
	}

	found := false
	for _, env := range capturedEnv {
		if env == "TEST_EXTRA_VAR=hello" {
			found = true
			break
		}
	}
	if !found {
		t.Error("TEST_EXTRA_VAR=hello not found in cmd.Env")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devmode/ -run TestProcessManager -v -count=1`
Expected: compilation error — ProcessManager/ProcessConfig not defined

- [ ] **Step 3: Implement manager.go**

Create `internal/devmode/manager.go`:

```go
package devmode

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v5"
)

type ProcessConfig struct {
	Name       string
	Executable string
	Args       []string
	ExtraEnv   []string
}

type ProcessManager struct {
	cfg    ProcessConfig
	stdout io.Writer
	stderr io.Writer

	initialInterval time.Duration
	maxInterval     time.Duration
	stableAfter     time.Duration

	cmdModifier func(*exec.Cmd)
}

func NewProcessManager(cfg ProcessConfig, stdout, stderr io.Writer) *ProcessManager {
	return &ProcessManager{
		cfg:             cfg,
		stdout:          stdout,
		stderr:          stderr,
		initialInterval: time.Second,
		maxInterval:     15 * time.Second,
		stableAfter:     30 * time.Second,
	}
}

func (pm *ProcessManager) Run(ctx context.Context) error {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = pm.initialInterval
	bo.MaxInterval = pm.maxInterval

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		startTime := time.Now()
		err := pm.runOnce(ctx)
		uptime := time.Since(startTime)

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			fmt.Fprintf(pm.stderr, "[%s] exited after %s: %v\n", pm.cfg.Name, uptime.Truncate(time.Millisecond), err)
		} else {
			fmt.Fprintf(pm.stderr, "[%s] exited cleanly after %s\n", pm.cfg.Name, uptime.Truncate(time.Millisecond))
		}

		if uptime >= pm.stableAfter {
			bo.Reset()
		}

		delay := bo.NextBackOff()
		fmt.Fprintf(pm.stdout, "[%s] restarting in %s...\n", pm.cfg.Name, delay.Truncate(time.Millisecond))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

func (pm *ProcessManager) runOnce(ctx context.Context) error {
	executable := pm.cfg.Executable
	if executable == "" {
		var err error
		executable, err = os.Executable()
		if err != nil {
			return fmt.Errorf("cannot determine executable path: %w", err)
		}
	}

	cmd := exec.CommandContext(ctx, executable, pm.cfg.Args...)
	cmd.Stdout = pm.stdout
	cmd.Stderr = pm.stderr

	if len(pm.cfg.ExtraEnv) > 0 {
		cmd.Env = append(os.Environ(), pm.cfg.ExtraEnv...)
	}

	if pm.cmdModifier != nil {
		pm.cmdModifier(cmd)
	}

	cmd.Cancel = func() error {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = 5 * time.Second

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}

	fmt.Fprintf(pm.stdout, "[%s] started (PID %d)\n", pm.cfg.Name, cmd.Process.Pid)

	return cmd.Wait()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/devmode/ -v -count=1 -race`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/devmode/manager.go internal/devmode/manager_test.go
git commit -m "$(cat <<'EOF'
feat(M22): ProcessManager with auto-restart and backoff (T22.2)

Spawns a child process, monitors it, and restarts on crash with
cenkalti/backoff exponential backoff (initial 1s, max 15s). Resets
backoff after 30s of stable uptime. SIGTERM on context cancellation
with 5s grace before SIGKILL.
EOF
)"
```

**Accept:** `go test ./internal/devmode/ -v -count=1 -race` — all PASS.

---

### Task 3: Backoff consolidation (`internal/hotreload/`)

**Files:**
- Delete: `internal/hotreload/backoff.go`
- Delete: `internal/hotreload/backoff_test.go`
- Modify: `internal/hotreload/poller.go:23-47` (Poller struct + NewPoller)
- Modify: `internal/hotreload/poller.go:64-81` (error branch in Run)
- Modify: `internal/hotreload/poller.go:84` (success reset)

**Interfaces:**
- Consumes: `cenkalti/backoff/v5`
- Produces: same `Poller` API — `NewPoller`, `Run`, `Trigger`, `Status` signatures unchanged

- [ ] **Step 1: Run existing hotreload tests to establish baseline**

Run: `go test ./internal/hotreload/ -v -count=1 -race`
Expected: all PASS

- [ ] **Step 2: Modify poller.go to use cenkalti/backoff**

In `internal/hotreload/poller.go`, add the import:

```go
import (
	// ... existing imports ...
	"github.com/cenkalti/backoff/v5"
)
```

Add `bo` field to the `Poller` struct:

```go
type Poller struct {
	client    *RegistryClient
	interval  time.Duration
	reloadFn  func(yaml []byte) (string, error)
	triggerCh chan struct{}
	status    atomic.Pointer[PollStatus]
	metrics   *observability.Metrics
	bo        *backoff.ExponentialBackOff
}
```

Initialize it in `NewPoller`, before the return:

```go
bo := backoff.NewExponentialBackOff()
bo.InitialInterval = interval
bo.MaxInterval = 5 * time.Minute
p.bo = bo
```

In `Run`, replace the error branch backoff line (`wait := backoffDuration(...)`) with:

```go
wait := p.bo.NextBackOff()
```

In `Run`, after the success block that resets `consecutiveErrors`, add:

```go
p.bo.Reset()
```

- [ ] **Step 3: Delete the old backoff files**

Delete `internal/hotreload/backoff.go` and `internal/hotreload/backoff_test.go`.

- [ ] **Step 4: Run hotreload tests to verify nothing broke**

Run: `go test ./internal/hotreload/ -v -count=1 -race`
Expected: all PASS (5 poller tests, 2 client tests)

- [ ] **Step 5: Verify no references to the deleted function remain**

Run: `grep -rn "backoffDuration" internal/`
Expected: no output

- [ ] **Step 6: Run the full test suite**

Run: `go test ./... -count=1 -race`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/hotreload/poller.go
git rm internal/hotreload/backoff.go internal/hotreload/backoff_test.go
git commit -m "$(cat <<'EOF'
feat(M22): consolidate hotreload backoff onto cenkalti/backoff (T22.3)

Replace hand-rolled backoffDuration() with cenkalti/backoff/v5
ExponentialBackOff in the registry poller. Delete backoff.go and its
test. upstream/retry.go backoff is intentionally left untouched (its
HTTP-specific control flow doesn't map onto cenkalti's API).
EOF
)"
```

**Accept:** `go test ./internal/hotreload/ -v -count=1 -race` — all PASS. `grep -rn "backoffDuration" internal/` — empty.

---

### Task 4: CLI dev command + main.go wiring (`cmd/restitch/dev.go`)

**Files:**
- Create: `cmd/restitch/dev.go`
- Modify: `cmd/restitch/main.go:18-30` (switch block)

**Interfaces:**
- Consumes:
  - `devmode.NewProcessManager(cfg ProcessConfig, stdout, stderr io.Writer) *ProcessManager`
  - `devmode.NewPrefixWriter(dest io.Writer, prefix, colorCode string) *PrefixWriter`
  - `devmode.WaitForHealth(ctx context.Context, url string, timeout time.Duration) error`
  - `devmode.ColorCyan`, `devmode.ColorMagenta`, `devmode.ColorYellow`, `devmode.ColorReset`
- Produces: `devCmd(args []string) int`, `findStudioBinary(override string) (string, error)`

- [ ] **Step 1: Implement dev.go**

Create `cmd/restitch/dev.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/restitch/restitch-gateway/internal/devmode"
)

const (
	devGatewayPort = 8080
	devAdminPort   = 9090
	devStudioPort  = 3080
)

func devCmd(args []string) int {
	flags := flag.NewFlagSet("dev", flag.ExitOnError)
	configFile := flags.String("config", "restitch.yaml", "path to composition config file")
	logFormat := flags.String("log-format", "text", "log format: json or text")
	studioBin := flags.String("studio-bin", "", "path to restitch-studio binary (auto-discovered if empty)")
	_ = flags.Parse(args)

	studioBinary, err := findStudioBinary(*studioBin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	gatewayBinary, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine executable path: %v\n", err)
		return 1
	}

	noColor := os.Getenv("NO_COLOR") != ""
	banner := func(msg string) {
		if noColor {
			fmt.Println(msg)
		} else {
			fmt.Printf("%s%s%s\n", devmode.ColorYellow, msg, devmode.ColorReset)
		}
	}

	banner(fmt.Sprintf("restitch dev %s", version))
	banner(fmt.Sprintf("  Gateway:  http://localhost:%d", devGatewayPort))
	banner(fmt.Sprintf("  Admin:    http://localhost:%d", devAdminPort))
	banner(fmt.Sprintf("  Studio:   http://localhost:%d", devStudioPort))
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	gwOut := devmode.NewPrefixWriter(os.Stdout, "gateway", devmode.ColorCyan)
	gwErr := devmode.NewPrefixWriter(os.Stderr, "gateway", devmode.ColorCyan)
	stOut := devmode.NewPrefixWriter(os.Stdout, "studio", devmode.ColorMagenta)
	stErr := devmode.NewPrefixWriter(os.Stderr, "studio", devmode.ColorMagenta)

	gwManager := devmode.NewProcessManager(devmode.ProcessConfig{
		Name:       "gateway",
		Executable: gatewayBinary,
		Args: []string{
			"run",
			"--config=" + *configFile,
			"--log-format=" + *logFormat,
			fmt.Sprintf("--port=%d", devGatewayPort),
		},
	}, gwOut, gwErr)

	stManager := devmode.NewProcessManager(devmode.ProcessConfig{
		Name:       "studio",
		Executable: studioBinary,
		Args: []string{
			fmt.Sprintf("--port=%d", devStudioPort),
			fmt.Sprintf("--gateway-admin-url=http://localhost:%d", devAdminPort),
		},
	}, stOut, stErr)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = gwManager.Run(ctx)
	}()

	healthURL := fmt.Sprintf("http://localhost:%d/health", devGatewayPort)
	banner(fmt.Sprintf("Waiting for gateway health at %s ...", healthURL))

	if err := devmode.WaitForHealth(ctx, healthURL, 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "error: gateway did not become healthy: %v\n", err)
		cancel()
		wg.Wait()
		return 1
	}
	banner("Gateway is healthy, starting studio...")

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = stManager.Run(ctx)
	}()

	sig := <-sigChan
	fmt.Println()
	banner(fmt.Sprintf("Received %v, shutting down...", sig))

	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		banner("Forced exit after timeout")
	}

	return 0
}

func findStudioBinary(override string) (string, error) {
	if override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("studio binary not found at %s: %w", override, err)
		}
		return override, nil
	}

	self, err := os.Executable()
	if err == nil {
		sibling := filepath.Join(filepath.Dir(self), "restitch-studio")
		if _, err := os.Stat(sibling); err == nil {
			return sibling, nil
		}
	}

	path, err := exec.LookPath("restitch-studio")
	if err == nil {
		return path, nil
	}

	return "", fmt.Errorf("restitch-studio not found — run 'make build-all' or pass --studio-bin")
}
```

- [ ] **Step 2: Add the dev case to main.go**

In `cmd/restitch/main.go`, add the `dev` case and update the usage line:

```go
	case "dev":
		os.Exit(devCmd(os.Args[2:]))
```

Update the usage line:

```go
		fmt.Fprintf(os.Stderr, "usage: restitch [run|check|version|import|dev] [flags]\n")
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./cmd/restitch/`
Expected: success

- [ ] **Step 4: Verify --help works**

Run: `go run ./cmd/restitch dev --help`
Expected: prints flag usage (config, log-format, studio-bin) and exits 0

- [ ] **Step 5: Verify build-all + binary discovery**

Run: `make build-all && bin/restitch dev --help`
Expected: exits 0 with flag usage

- [ ] **Step 6: Commit**

```bash
git add cmd/restitch/dev.go cmd/restitch/main.go
git commit -m "$(cat <<'EOF'
feat(M22): restitch dev command with binary discovery (T22.4)

Adds 'restitch dev' subcommand that spawns gateway + studio with
colored log prefixes, health-check sequencing, and auto-restart.
Studio binary discovered via sibling path, PATH, or --studio-bin flag.
EOF
)"
```

**Accept:** `make build-all && bin/restitch dev --help` — exits 0 with flag usage. `go vet ./...` — clean.

---

### Task 5: Full verification

**Files:**
- Modify: `docs/plan-progress/LEDGER.md` (append rows for T22.1–T22.4)

**Interfaces:**
- Consumes: all prior tasks
- Produces: green gate, ledger rows

- [ ] **Step 1: Run the full test suite**

Run: `go test ./... -count=1 -race`
Expected: all PASS

- [ ] **Step 2: Run go vet**

Run: `go vet ./...`
Expected: clean

- [ ] **Step 3: Run the M22 gate**

Run: `bash scripts/gates/m22.sh`
Expected: all checks PASS except the MANUAL item

- [ ] **Step 4: Run verify.sh for M22**

Run: `scripts/verify.sh M22`
Expected: `RESULT M22: PASS` (or PASS with MANUAL items listed)

- [ ] **Step 5: Commit evidence + ledger rows**

Append ledger rows for T22.1, T22.2, T22.3, T22.4, M22.unit, M22.gate to `docs/plan-progress/LEDGER.md`. Commit the evidence file written by verify.sh.

```bash
git add docs/plan-progress/LEDGER.md docs/plan-progress/evidence/
git commit -m "$(cat <<'EOF'
docs: record M22.gate PASS at <commit-sha>
EOF
)"
```

**Accept:** `scripts/verify.sh M22` prints `RESULT M22: PASS`. `scripts/check-ledger.sh` exits 0.
