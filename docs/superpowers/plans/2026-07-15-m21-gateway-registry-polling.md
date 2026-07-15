# M21 — Gateway Registry Polling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a registry polling mode to the gateway so it can fetch config from Studio's registry endpoint as an alternative to file-watching.

**Architecture:** New `internal/hotreload/` package with a stateless `RegistryClient` (HTTP + ETag) and a `Poller` (interval loop + backoff + metrics). The existing `doReload` closure in `run.go` is refactored into `reloadFromBytes([]byte)` shared by both file mode and registry mode. CLI gains `--registry-url` and `--poll-interval` flags on the `run` subcommand, mutually exclusive with `--config`.

**Tech Stack:** Go stdlib (net/http, time, sync/atomic, math), existing prometheus client, existing gwconfig/composition packages.

**Spec:** `docs/superpowers/specs/2026-07-15-m21-gateway-registry-polling-design.md`

## Global Constraints

- Go module: `github.com/restitch/restitch-gateway`, Go 1.25.6
- No new third-party dependencies (stdlib backoff, no `cenkalti/backoff`)
- Commit messages: `feat(M21): <task title>`
- After every task: run the Accept command and paste output into commit
- After every task: append a row to `docs/plan-progress/LEDGER.md`
- Never edit `scripts/verify.sh`, `scripts/check-ledger.sh`, or `scripts/lib/`

---

### Task 0: M21 Gate Script

**Files:**
- Create: `scripts/gates/m21.sh`

**Interfaces:**
- Consumes: harness functions from `scripts/lib/harness.sh` (`h_init`, `h_task`, `h_run`, `h_start_mockupstream`, `h_start_studio`, `h_start_gateway`, `h_assert_status`, `h_assert_json_body`, `h_finish`)
- Produces: gate script used by `scripts/verify.sh M21`

**Requires user approval before proceeding to implementation tasks.**

- [ ] **Step 1: Read the existing m20.sh gate script for reference**

Run: `cat scripts/gates/m20.sh`

Study the pattern: `h_init`, per-task file/grep checks, `h_task M20.unit` for tests, `h_task M20.gate` for smoke.

- [ ] **Step 2: Write the gate script**

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../lib/harness.sh"
h_init M21

# ── Per-task file & grep checks ──────────────────────────────────────

h_task T21.1
h_run "client.go exists" -- test -f internal/hotreload/client.go
h_run "client_test.go exists" -- test -f internal/hotreload/client_test.go
h_run "FetchResult type defined" -- grep -q 'type FetchResult struct' internal/hotreload/client.go

h_task T21.2
h_run "poller.go exists" -- test -f internal/hotreload/poller.go
h_run "backoff.go exists" -- test -f internal/hotreload/backoff.go
h_run "PollStatus type defined" -- grep -q 'type PollStatus struct' internal/hotreload/poller.go
h_run "Trigger method exists" -- grep -q 'func.*Poller.*Trigger' internal/hotreload/poller.go

h_task T21.3
h_run "registry-url flag in run.go" -- grep -q 'registry-url' cmd/restitch/run.go
h_run "poll-interval flag in run.go" -- grep -q 'poll-interval' cmd/restitch/run.go
h_run "registry-key flag in run.go" -- grep -q 'registry-key' cmd/restitch/run.go
h_run "RESTITCH_REGISTRY_URL env" -- grep -q 'RESTITCH_REGISTRY_URL' cmd/restitch/run.go

h_task T21.4
h_run "RegistryStatus in admin Deps" -- grep -q 'RegistryStatus' internal/admin/server.go
h_run "registry/status endpoint" -- grep -q 'registry/status' internal/admin/server.go

h_task T21.5
h_run "triggerCh in poller" -- grep -q 'triggerCh' internal/hotreload/poller.go

# ── Unit tests ───────────────────────────────────────────────────────

h_task M21.unit
h_run "go vet hotreload" -- go vet ./internal/hotreload/...
h_run "go test hotreload" -- go test -race -count=1 ./internal/hotreload/...
h_run "go test admin" -- go test -race -count=1 ./internal/admin/...
h_run "go test full suite" -- go test -race -count=1 ./...

# ── Smoke: registry polling mode ─────────────────────────────────────

h_task M21.gate

h_start_mockupstream

h_start_studio

sleep 1

# Create a composition config in the registry via Studio API
COMP_YAML="upstreams:
  mock:
    url: \"http://localhost:${MOCK_PORT}\"
compositions:
  regtest:
    path: \"/regtest\"
    steps:
      - name: e
        upstream: mock
        path: \"/echo\"
    response:
      body:
        result: \"{{ steps.e.body }}\"
"
CREATE_RESP=$(curl -sf -X POST "http://localhost:${STUDIO_PORT}/api/v1/configs" \
  -H "Content-Type: application/json" \
  -d "$(python3 -c "import json,sys; print(json.dumps({'name':'regtest','yaml':sys.stdin.read()}))" <<< "$COMP_YAML")")
h_run "config created in registry" -- test -n "$CREATE_RESP"

# Start gateway in registry mode
GW_PID_FILE="$H_TMP/gw_reg.pid"
go run ./cmd/restitch run \
  -registry-url "http://localhost:${STUDIO_PORT}" \
  -poll-interval 2s \
  -port "$GW_PORT" \
  -log-format json \
  > "$H_TMP/gw_reg.log" 2>&1 &
echo $! > "$GW_PID_FILE"
h_wait_for "http://localhost:${GW_PORT}/health" 10

# Verify gateway serves the registry composition
h_assert_status "GET http://localhost:${GW_PORT}/regtest" 200

# Verify admin registry status endpoint
h_assert_status "GET http://localhost:${ADMIN_PORT}/admin/api/registry/status" 200
h_run "status shows registry mode" -- \
  curl -sf "http://localhost:${ADMIN_PORT}/admin/api/registry/status" \
  | python3 -c "import json,sys; d=json.load(sys.stdin); assert d['mode']=='registry', f'mode={d[\"mode\"]}'"

# Verify SIGHUP triggers immediate poll
kill -HUP "$(cat "$GW_PID_FILE")" 2>/dev/null || true
sleep 2
h_run "SIGHUP logged" -- grep -q "registry poll triggered" "$H_TMP/gw_reg.log"

h_finish
```

- [ ] **Step 3: Make executable and verify syntax**

Run: `chmod +x scripts/gates/m21.sh && bash -n scripts/gates/m21.sh`
Expected: no syntax errors.

- [ ] **Step 4: Commit and get user approval**

```bash
git add scripts/gates/m21.sh
git commit -m "$(cat <<'EOF'
feat(M21): add gate script for registry polling milestone
EOF
)"
```

**STOP: Present the gate script to the user for approval before proceeding.**

**Accept:** `bash -n scripts/gates/m21.sh` exits 0 (syntax valid). User approves the gate script.

---

### Task 1: T21.1 — Registry HTTP Client

**Files:**
- Create: `internal/hotreload/client.go`
- Create: `internal/hotreload/client_test.go`

**Interfaces:**
- Consumes: `internal/registry.BundledConfig` JSON shape (`yaml_content`, `etag`, `composition_count`, `composition_names`)
- Produces: `RegistryClient`, `FetchResult`, `NewRegistryClient()`, `Fetch()` — used by Task 2 (Poller)

- [ ] **Step 1: Write the failing tests**

Create `internal/hotreload/client_test.go`:

```go
package hotreload

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegistryClient_Fetch_Success(t *testing.T) {
	bundle := map[string]any{
		"yaml_content":      "upstreams: {}\ncompositions: {}\n",
		"etag":              "abc123",
		"composition_count": 2,
		"composition_names": []string{"comp1", "comp2"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/registry/bundle" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("If-None-Match") != "" {
			t.Errorf("unexpected If-None-Match on first request: %s", r.Header.Get("If-None-Match"))
		}
		w.Header().Set("ETag", "abc123")
		json.NewEncoder(w).Encode(bundle)
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	result, err := client.Fetch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if result.NotModified {
		t.Error("expected modified")
	}
	if result.ETag != "abc123" {
		t.Errorf("etag = %q, want %q", result.ETag, "abc123")
	}
	if string(result.YAML) != "upstreams: {}\ncompositions: {}\n" {
		t.Errorf("yaml = %q", string(result.YAML))
	}
	if result.CompositionCount != 2 {
		t.Errorf("count = %d, want 2", result.CompositionCount)
	}
}

func TestRegistryClient_Fetch_NotModified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == "abc123" {
			w.Header().Set("ETag", "abc123")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		t.Errorf("expected If-None-Match: abc123, got %q", r.Header.Get("If-None-Match"))
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	result, err := client.Fetch(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !result.NotModified {
		t.Error("expected NotModified")
	}
}

func TestRegistryClient_Fetch_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	_, err := client.Fetch(context.Background(), "")
	if err == nil {
		t.Error("expected error on 500")
	}
}

func TestRegistryClient_Fetch_ConnectionRefused(t *testing.T) {
	client := NewRegistryClient("http://127.0.0.1:1")
	_, err := client.Fetch(context.Background(), "")
	if err == nil {
		t.Error("expected error on connection refused")
	}
}

func TestRegistryClient_Fetch_AdminKey(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Admin-Key")
		w.Header().Set("ETag", "x")
		json.NewEncoder(w).Encode(map[string]any{
			"yaml_content": "", "etag": "x", "composition_count": 0, "composition_names": []string{},
		})
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL, WithAdminKey("secret"))
	_, err := client.Fetch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "secret" {
		t.Errorf("X-Admin-Key = %q, want %q", gotKey, "secret")
	}
}

func TestRegistryClient_Fetch_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := NewRegistryClient(srv.URL)
	_, err := client.Fetch(ctx, "")
	if err == nil {
		t.Error("expected error on canceled context")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/hotreload/ -v -count=1`
Expected: compilation error (package doesn't exist yet).

- [ ] **Step 3: Implement the client**

Create `internal/hotreload/client.go`:

```go
package hotreload

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type FetchResult struct {
	YAML             []byte
	ETag             string
	CompositionCount int
	NotModified      bool
}

type ClientOption func(*RegistryClient)

func WithAdminKey(key string) ClientOption {
	return func(c *RegistryClient) { c.adminKey = key }
}

func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *RegistryClient) { c.httpClient = hc }
}

type RegistryClient struct {
	url        string
	httpClient *http.Client
	adminKey   string
}

func NewRegistryClient(url string, opts ...ClientOption) *RegistryClient {
	c := &RegistryClient{
		url:        url,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *RegistryClient) Fetch(ctx context.Context, lastETag string) (*FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/api/v1/registry/bundle", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if lastETag != "" {
		req.Header.Set("If-None-Match", lastETag)
	}
	if c.adminKey != "" {
		req.Header.Set("X-Admin-Key", c.adminKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch bundle: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return &FetchResult{
			NotModified: true,
			ETag:        resp.Header.Get("ETag"),
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	var bundle struct {
		YAMLContent      string   `json:"yaml_content"`
		ETag             string   `json:"etag"`
		CompositionCount int      `json:"composition_count"`
		CompositionNames []string `json:"composition_names"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&bundle); err != nil {
		return nil, fmt.Errorf("decode bundle: %w", err)
	}

	etag := resp.Header.Get("ETag")
	if etag == "" {
		etag = bundle.ETag
	}

	return &FetchResult{
		YAML:             []byte(bundle.YAMLContent),
		ETag:             etag,
		CompositionCount: bundle.CompositionCount,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/hotreload/ -v -count=1 -race`
Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/hotreload/client.go internal/hotreload/client_test.go
git commit -m "$(cat <<'EOF'
feat(M21): registry HTTP client with ETag support (T21.1)
EOF
)"
```

**Accept:** `go test ./internal/hotreload/ -run TestRegistryClient -v -count=1` — all tests PASS.

---

### Task 2: T21.2 — Polling Engine + Backoff + Metrics

**Files:**
- Create: `internal/hotreload/backoff.go`
- Create: `internal/hotreload/backoff_test.go`
- Create: `internal/hotreload/poller.go`
- Create: `internal/hotreload/poller_test.go`
- Modify: `internal/observability/metrics.go` (add `RegisterRegistryMetrics`)

**Interfaces:**
- Consumes: `RegistryClient.Fetch()` from Task 1, `*observability.Metrics` from existing code
- Produces: `Poller`, `PollStatus`, `NewPoller()`, `Run()`, `Trigger()`, `Status()` — used by Tasks 3-5

- [ ] **Step 1: Write backoff tests**

Create `internal/hotreload/backoff_test.go`:

```go
package hotreload

import (
	"testing"
	"time"
)

func TestBackoffDuration(t *testing.T) {
	base := 10 * time.Second
	max := 5 * time.Minute

	tests := []struct {
		attempt int
		wantMin time.Duration
		wantMax time.Duration
	}{
		{0, 8 * time.Second, 12 * time.Second},       // 10s ± 20%
		{1, 16 * time.Second, 24 * time.Second},      // 20s ± 20%
		{2, 32 * time.Second, 48 * time.Second},      // 40s ± 20%
		{3, 64 * time.Second, 96 * time.Second},      // 80s ± 20%
		{4, 128 * time.Second, 192 * time.Second},     // 160s ± 20%
		{5, 240 * time.Second, 360 * time.Second},     // 300s (capped) ± 20%
		{10, 240 * time.Second, 360 * time.Second},    // still capped
	}

	for _, tt := range tests {
		for i := 0; i < 50; i++ {
			got := backoffDuration(tt.attempt, base, max)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("attempt=%d: got %v, want [%v, %v]", tt.attempt, got, tt.wantMin, tt.wantMax)
				break
			}
		}
	}
}
```

- [ ] **Step 2: Implement backoff**

Create `internal/hotreload/backoff.go`:

```go
package hotreload

import (
	"math"
	"math/rand/v2"
	"time"
)

func backoffDuration(attempt int, base, max time.Duration) time.Duration {
	d := time.Duration(float64(base) * math.Pow(2, float64(attempt)))
	if d > max {
		d = max
	}
	jitter := 0.8 + rand.Float64()*0.4 // ±20%
	return time.Duration(float64(d) * jitter)
}
```

- [ ] **Step 3: Run backoff tests**

Run: `go test ./internal/hotreload/ -run TestBackoff -v -count=1`
Expected: PASS.

- [ ] **Step 4: Add registry metrics to observability**

Read `internal/observability/metrics.go` to see the existing struct and `NewMetrics()` pattern. Then add three new nilable fields and a registration method.

Add to the `Metrics` struct:

```go
RegistryPollsTotal   *prometheus.CounterVec
RegistryPollDuration prometheus.Histogram
RegistryLastSuccess  prometheus.Gauge
```

Add method:

```go
func (m *Metrics) RegisterRegistryMetrics() {
	f := promauto.With(m.registry)
	m.RegistryPollsTotal = f.NewCounterVec(prometheus.CounterOpts{
		Name: "restitch_registry_polls_total",
		Help: "Total registry poll attempts.",
	}, []string{"result"})
	m.RegistryPollDuration = f.NewHistogram(prometheus.HistogramOpts{
		Name:    "restitch_registry_poll_duration_seconds",
		Help:    "Duration of registry poll cycles including reload.",
		Buckets: prometheus.DefBuckets,
	})
	m.RegistryLastSuccess = f.NewGauge(prometheus.GaugeOpts{
		Name: "restitch_registry_last_success_timestamp",
		Help: "Unix timestamp of last successful registry poll.",
	})
}
```

- [ ] **Step 5: Write poller tests**

Create `internal/hotreload/poller_test.go`:

```go
package hotreload

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func bundleHandler(yaml, etag string, count int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		json.NewEncoder(w).Encode(map[string]any{
			"yaml_content":      yaml,
			"etag":              etag,
			"composition_count": count,
			"composition_names": []string{},
		})
	}
}

func TestPoller_HappyPath(t *testing.T) {
	srv := httptest.NewServer(bundleHandler("upstreams: {}\n", "e1", 1))
	defer srv.Close()

	var reloadCalls atomic.Int32
	reloadFn := func(yaml []byte) (string, error) {
		reloadCalls.Add(1)
		return "hash1", nil
	}

	client := NewRegistryClient(srv.URL)
	p := NewPoller(client, 50*time.Millisecond, reloadFn, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	p.Run(ctx)

	if reloadCalls.Load() < 1 {
		t.Error("expected at least one reload call")
	}
	status := p.Status()
	if status.LastETag != "e1" {
		t.Errorf("etag = %q, want %q", status.LastETag, "e1")
	}
	if status.LastError != "" {
		t.Errorf("unexpected error: %s", status.LastError)
	}
}

func TestPoller_NotModified_NoReload(t *testing.T) {
	srv := httptest.NewServer(bundleHandler("x: y\n", "e1", 1))
	defer srv.Close()

	var reloadCalls atomic.Int32
	reloadFn := func(yaml []byte) (string, error) {
		reloadCalls.Add(1)
		return "hash1", nil
	}

	client := NewRegistryClient(srv.URL)
	p := NewPoller(client, 50*time.Millisecond, reloadFn, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	p.Run(ctx)

	if reloadCalls.Load() != 1 {
		t.Errorf("expected exactly 1 reload (initial), got %d", reloadCalls.Load())
	}
}

func TestPoller_FetchError_Backoff(t *testing.T) {
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	p := NewPoller(client, 50*time.Millisecond, func([]byte) (string, error) { return "", nil }, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	p.Run(ctx)

	status := p.Status()
	if status.ErrorType != "fetch" {
		t.Errorf("error_type = %q, want %q", status.ErrorType, "fetch")
	}
	if status.ConsecutiveErrors < 2 {
		t.Errorf("expected at least 2 consecutive errors, got %d", status.ConsecutiveErrors)
	}
}

func TestPoller_Trigger_ImmediatePoll(t *testing.T) {
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		w.Header().Set("ETag", "e1")
		json.NewEncoder(w).Encode(map[string]any{
			"yaml_content": "x: y\n", "etag": "e1",
			"composition_count": 0, "composition_names": []string{},
		})
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	p := NewPoller(client, 10*time.Second, func([]byte) (string, error) { return "", nil }, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		p.Trigger()
	}()

	p.Run(ctx)

	if reqCount.Load() < 2 {
		t.Errorf("expected >=2 requests (initial + triggered), got %d", reqCount.Load())
	}
}

func TestPoller_ContextCancel_CleansUp(t *testing.T) {
	srv := httptest.NewServer(bundleHandler("x: y\n", "e1", 0))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	p := NewPoller(client, time.Hour, func([]byte) (string, error) { return "", nil }, nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
}
```

- [ ] **Step 6: Implement the poller**

Create `internal/hotreload/poller.go`:

```go
package hotreload

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/restitch/restitch-gateway/internal/observability"
)

type PollStatus struct {
	LastPollTime      time.Time `json:"last_poll"`
	LastSuccessTime   time.Time `json:"last_success"`
	LastETag          string    `json:"etag"`
	CompositionCount  int       `json:"composition_count"`
	LastError         string    `json:"error"`
	ErrorType         string    `json:"error_type"`
	ConsecutiveErrors int       `json:"consecutive_errors"`
}

type Poller struct {
	client    *RegistryClient
	interval  time.Duration
	reloadFn  func(yaml []byte) (string, error)
	triggerCh chan struct{}
	status    atomic.Pointer[PollStatus]
	metrics   *observability.Metrics
}

func NewPoller(
	client *RegistryClient,
	interval time.Duration,
	reloadFn func(yaml []byte) (string, error),
	metrics *observability.Metrics,
) *Poller {
	p := &Poller{
		client:    client,
		interval:  interval,
		reloadFn:  reloadFn,
		triggerCh: make(chan struct{}, 1),
		metrics:   metrics,
	}
	p.status.Store(&PollStatus{})
	return p
}

func (p *Poller) Run(ctx context.Context) error {
	consecutiveErrors := 0
	lastETag := ""

	for {
		start := time.Now()
		result, pollErr := p.poll(ctx, lastETag)
		elapsed := time.Since(start)

		p.recordMetricsDuration(elapsed)

		now := time.Now()
		s := p.Status()
		s.LastPollTime = now

		if pollErr != nil {
			consecutiveErrors++
			errType := classifyError(pollErr)
			s.LastError = pollErr.Error()
			s.ErrorType = errType
			s.ConsecutiveErrors = consecutiveErrors
			p.status.Store(&s)
			p.recordMetricsResult("error")

			slog.Error("registry poll failed",
				"error", pollErr, "type", errType,
				"consecutive", consecutiveErrors)

			wait := backoffDuration(consecutiveErrors-1, p.interval, 5*time.Minute)
			if !p.sleepOrTrigger(ctx, wait) {
				return ctx.Err()
			}
			continue
		}

		consecutiveErrors = 0
		s.ConsecutiveErrors = 0
		s.LastError = ""
		s.ErrorType = ""

		if result.NotModified {
			s.LastSuccessTime = now
			p.status.Store(&s)
			p.recordMetricsResult("not_modified")
		} else {
			lastETag = result.ETag
			s.LastETag = result.ETag
			s.CompositionCount = result.CompositionCount
			s.LastSuccessTime = now
			p.status.Store(&s)
			p.recordMetricsResult("success")
		}

		if !p.sleepOrTrigger(ctx, p.interval) {
			return ctx.Err()
		}
	}
}

func (p *Poller) poll(ctx context.Context, lastETag string) (*FetchResult, error) {
	result, err := p.client.Fetch(ctx, lastETag)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	if result.NotModified {
		return result, nil
	}
	if _, err := p.reloadFn(result.YAML); err != nil {
		return nil, fmt.Errorf("reload: %w", err)
	}
	return result, nil
}

func (p *Poller) Trigger() {
	select {
	case p.triggerCh <- struct{}{}:
		slog.Info("registry poll triggered")
	default:
	}
}

func (p *Poller) Status() PollStatus {
	if s := p.status.Load(); s != nil {
		return *s
	}
	return PollStatus{}
}

func (p *Poller) sleepOrTrigger(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-p.triggerCh:
		return true
	case <-t.C:
		return true
	}
}

func classifyError(err error) string {
	msg := err.Error()
	if len(msg) >= 6 && msg[:6] == "fetch:" {
		return "fetch"
	}
	if len(msg) >= 7 && msg[:7] == "reload:" {
		return "reload"
	}
	return "fetch"
}

func (p *Poller) recordMetricsResult(result string) {
	if p.metrics == nil || p.metrics.RegistryPollsTotal == nil {
		return
	}
	p.metrics.RegistryPollsTotal.WithLabelValues(result).Inc()
	if result != "error" {
		p.metrics.RegistryLastSuccess.SetToCurrentTime()
	}
}

func (p *Poller) recordMetricsDuration(d time.Duration) {
	if p.metrics == nil || p.metrics.RegistryPollDuration == nil {
		return
	}
	p.metrics.RegistryPollDuration.Observe(d.Seconds())
}
```

- [ ] **Step 7: Run all tests**

Run: `go test ./internal/hotreload/ -v -count=1 -race`
Expected: all tests PASS (client + backoff + poller).

- [ ] **Step 8: Commit**

```bash
git add internal/hotreload/backoff.go internal/hotreload/backoff_test.go \
       internal/hotreload/poller.go internal/hotreload/poller_test.go \
       internal/observability/metrics.go
git commit -m "$(cat <<'EOF'
feat(M21): polling engine with backoff and registry metrics (T21.2)
EOF
)"
```

**Accept:** `go test ./internal/hotreload/ -v -count=1 -race` — all tests PASS. `go vet ./internal/hotreload/...` exits 0.

---

### Task 3: T21.4 — Admin Status Endpoint

**Files:**
- Modify: `internal/admin/server.go` (add `RegistryStatus` to `Deps`, register endpoint, write handler)

**Interfaces:**
- Consumes: `hotreload.PollStatus` from Task 2
- Produces: `Deps.RegistryStatus` field, `GET /admin/api/registry/status` endpoint — used by Task 4 (CLI wiring sets the field)

- [ ] **Step 1: Write the test**

Add to the existing admin test file (find it with `ls internal/admin/*_test.go`). Add a test for the new endpoint:

```go
func TestRegistryStatusEndpoint(t *testing.T) {
	now := time.Now()
	statusFn := func() any {
		return map[string]any{
			"mode":               "registry",
			"registry_url":       "http://studio:8090",
			"poll_interval_seconds": 10,
			"last_poll":          now.Format(time.RFC3339),
			"last_success":       now.Format(time.RFC3339),
			"etag":               "abc123",
			"composition_count":  5,
			"error":              nil,
			"error_type":         nil,
			"consecutive_errors": 0,
		}
	}

	deps := Deps{
		// ... minimal required fields for New() ...
		RegistryStatus: statusFn,
	}
	srv := New(gwconfig.AdminConfig{Port: 0}, deps)
	// ... httptest request to /admin/api/registry/status ...
	// assert 200, JSON contains "mode":"registry", "etag":"abc123"
}

func TestRegistryStatusEndpoint_NotRegistered(t *testing.T) {
	deps := Deps{
		// RegistryStatus is nil — file mode
	}
	srv := New(gwconfig.AdminConfig{Port: 0}, deps)
	// ... httptest request to /admin/api/registry/status ...
	// assert 404
}
```

Note: the implementer should adapt this test to match the existing admin test patterns (how `New()` is called, how httptest requests are made). The exact test setup depends on the existing test helpers.

- [ ] **Step 2: Add RegistryStatus to Deps**

In `internal/admin/server.go`, add to the `Deps` struct:

```go
RegistryStatus func() any // nil in file mode; returns registry poll status JSON
```

- [ ] **Step 3: Register the endpoint conditionally**

In `New()`, after the existing endpoint registrations, add:

```go
if deps.RegistryStatus != nil {
	mux.HandleFunc("GET /admin/api/registry/status", s.requireKey(s.handleRegistryStatus))
}
```

- [ ] **Step 4: Write the handler**

Add method to `*Server`:

```go
func (s *Server) handleRegistryStatus(w http.ResponseWriter, r *http.Request) {
	status := s.deps.RegistryStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/admin/ -v -count=1 -race`
Expected: all tests PASS (existing + new).

- [ ] **Step 6: Commit**

```bash
git add internal/admin/server.go internal/admin/*_test.go
git commit -m "$(cat <<'EOF'
feat(M21): admin registry status endpoint (T21.4)
EOF
)"
```

**Accept:** `go test ./internal/admin/ -v -count=1 -race` — all PASS. `grep -q 'RegistryStatus' internal/admin/server.go` exits 0.

---

### Task 4: T21.3 — CLI Wiring + Reload Refactor

**Files:**
- Modify: `cmd/restitch/run.go` (add flags, refactor `doReload` into `reloadFromBytes`, add registry mode startup)

**Interfaces:**
- Consumes: `hotreload.NewRegistryClient()`, `hotreload.NewPoller()` from Tasks 1-2; `admin.Deps.RegistryStatus` from Task 3; existing `gwconfig`, `composition`, `server` packages
- Produces: working `restitch run --registry-url` mode — consumed by Task 5 (SIGHUP test) and the gate script

- [ ] **Step 1: Add new flags and env var fallbacks**

In `runCmd()`, after the existing flag definitions, add:

```go
registryURL := fs.String("registry-url", "", "Registry URL for polling mode")
registryKey := fs.String("registry-key", "", "Admin API key for registry auth")
pollInterval := fs.Duration("poll-interval", 10*time.Second, "Registry poll interval")
```

After `fs.Parse(args)`, add env var fallbacks:

```go
if *registryURL == "" {
	if v := os.Getenv("RESTITCH_REGISTRY_URL"); v != "" {
		*registryURL = v
	}
}
if *registryKey == "" {
	if v := os.Getenv("RESTITCH_REGISTRY_KEY"); v != "" {
		*registryKey = v
	}
}
if v := os.Getenv("RESTITCH_POLL_INTERVAL"); v != "" && !isFlagSet(fs, "poll-interval") {
	if d, err := time.ParseDuration(v); err == nil {
		*pollInterval = d
	}
}
```

Add the `isFlagSet` helper (if not already present):

```go
func isFlagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}
```

- [ ] **Step 2: Add mutual exclusivity check**

After env var fallbacks:

```go
if *registryURL != "" && isFlagSet(fs, "config") {
	slog.Error("--config and --registry-url are mutually exclusive")
	return 1
}
```

- [ ] **Step 3: Refactor doReload into reloadFromBytes**

Extract the core of `doReload` into a `reloadFromBytes` closure that takes `[]byte` and returns `(string, error)`. The existing `doReload` becomes a thin wrapper that reads the file and calls `reloadFromBytes`.

The implementer should read the current `doReload` body (lines ~263-317 of run.go) and split it:

```go
// reloadFromBytes compiles raw YAML and hot-swaps the pipeline.
reloadFromBytes := func(rawYAML []byte) (string, error) {
	expanded, err := gwconfig.ExpandEnvStrict(string(rawYAML))
	if err != nil {
		return "", fmt.Errorf("env expansion: %w", err)
	}
	file, err := gwconfig.LoadBytes([]byte(expanded), rawYAML)
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	gwconfig.ApplyEnvOverrides(file)
	// ... existing flag overrides (port, tls, log format, etc.) ...
	newHash := file.Hash()
	if newHash == swapper.Current().Hash {
		slog.Info("config unchanged")
		return newHash, nil
	}
	// ... existing ParseConfig → CompileConfig → build auth → Pipeline → Swap ...
	return newHash, nil
}

// File-mode reload: read file then pass bytes.
doReload := func() (string, error) {
	raw, err := os.ReadFile(*configFile)
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}
	return reloadFromBytes(raw)
}
```

Verify the existing file-mode startup still works — the initial load path also needs to call `reloadFromBytes` (or use the same `gwconfig.ReadAndExpand` → `LoadBytes` flow it already uses). Only the hot-reload path is refactored.

- [ ] **Step 4: Add registry mode startup**

Add a branch after the initial pipeline build:

```go
if *registryURL != "" {
	// Registry mode
	var clientOpts []hotreload.ClientOption
	if *registryKey != "" {
		clientOpts = append(clientOpts, hotreload.WithAdminKey(*registryKey))
	}
	regClient := hotreload.NewRegistryClient(*registryURL, clientOpts...)

	// Blocking initial fetch
	result, err := regClient.Fetch(ctx, "")
	if err != nil {
		slog.Error("failed to fetch initial config from registry", "url", *registryURL, "error", err)
		return 1
	}
	if _, err := reloadFromBytes(result.YAML); err != nil {
		slog.Error("failed to load initial config from registry", "error", err)
		return 1
	}
	slog.Info("loaded initial config from registry",
		"url", *registryURL, "etag", result.ETag,
		"compositions", result.CompositionCount)

	// Register registry metrics
	if metrics != nil {
		metrics.RegisterRegistryMetrics()
	}

	poller := hotreload.NewPoller(regClient, *pollInterval, reloadFromBytes, metrics)

	// Wire admin status
	adminDeps.RegistryStatus = func() any {
		s := poller.Status()
		return map[string]any{
			"mode":                 "registry",
			"registry_url":        *registryURL,
			"poll_interval_seconds": pollInterval.Seconds(),
			"last_poll":           s.LastPollTime,
			"last_success":        s.LastSuccessTime,
			"etag":                s.LastETag,
			"composition_count":   s.CompositionCount,
			"error":               nilIfEmpty(s.LastError),
			"error_type":          nilIfEmpty(s.ErrorType),
			"consecutive_errors":  s.ConsecutiveErrors,
		}
	}

	// Wire admin reload to trigger immediate poll
	adminDeps.Reload = func() (string, error) {
		poller.Trigger()
		return swapper.Current().Hash, nil
	}

	// Start poller (SIGHUP also triggers)
	go poller.Run(ctx)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGHUP)
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigCh:
				poller.Trigger()
			}
		}
	}()
} else {
	// File mode — existing behavior
	rl := newReloader(*configFile, doReload)
	go rl.watchSignals()
	go rl.watchFile()
	adminDeps.Reload = rl.reload
}
```

Add a `nilIfEmpty` helper:

```go
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
```

- [ ] **Step 5: Verify both modes work**

Run: `go build ./cmd/restitch/`
Expected: compiles cleanly.

Run: `go test ./cmd/restitch/ -v -count=1 -race`
Expected: existing tests pass.

- [ ] **Step 6: Add CLI mutual-exclusivity test**

Add to `cmd/restitch/` tests (or verify manually):

```go
func TestRunCmd_MutualExclusivity(t *testing.T) {
	code := runCmd([]string{"-config", "x.yaml", "-registry-url", "http://example.com"})
	if code != 1 {
		t.Errorf("expected exit 1 for mutual exclusivity, got %d", code)
	}
}
```

- [ ] **Step 7: Run full test suite**

Run: `go test ./... -count=1 -race`
Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add cmd/restitch/run.go
git commit -m "$(cat <<'EOF'
feat(M21): wire registry polling mode into gateway CLI (T21.3)
EOF
)"
```

**Accept:** `go build ./cmd/restitch/ && go test ./cmd/restitch/ -count=1 -race` — builds and tests pass. `grep -q 'registry-url' cmd/restitch/run.go` exits 0.

---

### Task 5: T21.5 — SIGHUP Immediate Poll

**Files:**
- Modify: `internal/hotreload/poller_test.go` (add SIGHUP-specific test if not covered by existing Trigger test)

**Interfaces:**
- Consumes: `Poller.Trigger()` from Task 2, SIGHUP wiring from Task 4
- Produces: verified SIGHUP → immediate poll behavior

The SIGHUP → `poller.Trigger()` wiring was done in Task 4. This task verifies the end-to-end behavior and ensures the Trigger method properly bypasses backoff.

- [ ] **Step 1: Add test for Trigger bypassing backoff**

Add to `internal/hotreload/poller_test.go`:

```go
func TestPoller_Trigger_BypassesBackoff(t *testing.T) {
	var reqCount atomic.Int32
	failFirst := atomic.Bool{}
	failFirst.Store(true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		if n <= 2 && failFirst.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("ETag", "e1")
		json.NewEncoder(w).Encode(map[string]any{
			"yaml_content": "x: y\n", "etag": "e1",
			"composition_count": 0, "composition_names": []string{},
		})
	}))
	defer srv.Close()

	client := NewRegistryClient(srv.URL)
	p := NewPoller(client, 10*time.Second, func([]byte) (string, error) { return "", nil }, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		// Wait for the poller to enter backoff after failures
		time.Sleep(200 * time.Millisecond)
		failFirst.Store(false)
		// Trigger should cause an immediate poll, bypassing the 10s backoff
		p.Trigger()
	}()

	p.Run(ctx)

	// With 10s interval and 10s base backoff, without Trigger we'd get
	// at most 2 requests in 2s. With Trigger bypassing, we should get >= 3.
	if reqCount.Load() < 3 {
		t.Errorf("expected >=3 requests (trigger should bypass backoff), got %d", reqCount.Load())
	}

	status := p.Status()
	if status.ConsecutiveErrors != 0 {
		t.Errorf("consecutive_errors = %d, want 0 after recovery", status.ConsecutiveErrors)
	}
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/hotreload/ -run TestPoller_Trigger -v -count=1 -race`
Expected: both Trigger tests PASS.

- [ ] **Step 3: Run full test suite**

Run: `go test ./... -count=1 -race`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/hotreload/poller_test.go
git commit -m "$(cat <<'EOF'
feat(M21): verify SIGHUP trigger bypasses poll backoff (T21.5)
EOF
)"
```

**Accept:** `go test ./internal/hotreload/ -run TestPoller_Trigger -v -count=1 -race` — both Trigger tests PASS.

---

## Final Verification

After all tasks are complete:

```bash
scripts/verify.sh M21
```

Expected: `RESULT M21: PASS`. Commit the evidence file under `docs/plan-progress/evidence/`.
