# Studio Observability & Visualization Upgrade — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add time-series metrics storage, Recharts-powered charts, true timeline waterfall, live DAG execution overlay, and per-composition metrics to Restitch Studio.

**Architecture:** Backend gains a `Storage` interface with memory/SQLite/Turso backends for time-bucketed metrics. The executor records `start_offset_ms` per step. Three new admin API endpoints serve time-series, step-aggregate, and single-request data. Frontend adds Recharts for dashboard/composition charts, a true timeline waterfall, request filters, richer DAG nodes with inferred dependency edges, and a live execution overlay.

**Tech Stack:** Go 1.25, `github.com/tursodatabase/go-libsql`, React 19, TypeScript, Recharts, `@xyflow/react`, Tailwind CSS v4, shadcn/ui

## Global Constraints

- Go module path: `github.com/restitch/restitch-gateway`
- Go version: 1.25.6 (go.mod)
- All Go code: `go build ./... && go vet ./...` must be clean after every task
- All frontend code: `cd studio && npm run build` must be clean after every task
- Tests: `go test ./... -count=1` and `cd studio && npm run test` green after every task
- Only two new deps: `github.com/tursodatabase/go-libsql` (Go), `recharts` (npm)
- Commit after each task: `feat(studio-obs): <task title>`
- Existing tests must not break — update tests when interfaces change
- Match existing code style (table-driven tests, internal/ packages, shadcn/ui patterns)

---

### Task 1: Enrich StepRecord with execution detail

**Files:**
- Modify: `internal/reqlog/reqlog.go` (StepRecord struct)
- Modify: `internal/composition/executor.go` (StepTiming struct, capture start offset)
- Modify: `internal/composition/handler.go:226-252` (build enriched StepRecord)
- Modify: `internal/admin/admin_test.go` (update test fixtures if needed)

**Interfaces:**
- Consumes: existing `reqlog.StepRecord`, `composition.StepTiming`
- Produces: enriched `reqlog.StepRecord` with `Upstream`, `URL`, `StartOffsetMS`, `BodySize`, `Error`, `Cached`, `Retries` fields; enriched `composition.StepTiming` with same fields for the handler to copy

- [ ] **Step 1: Write test for enriched StepRecord serialization**

Create file `internal/reqlog/reqlog_test.go`:

```go
package reqlog

import (
	"encoding/json"
	"testing"
)

func TestStepRecord_JSON(t *testing.T) {
	sr := StepRecord{
		Name:          "user",
		Status:        "success",
		Wave:          1,
		DurationMS:    42.5,
		HTTPStatus:    200,
		Upstream:      "api",
		URL:           "http://localhost:8081/users/1",
		StartOffsetMS: 0.3,
		BodySize:      256,
		Error:         "",
		Cached:        false,
		Retries:       0,
	}
	data, err := json.Marshal(sr)
	if err != nil {
		t.Fatal(err)
	}
	var decoded StepRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Upstream != "api" {
		t.Errorf("Upstream = %q, want %q", decoded.Upstream, "api")
	}
	if decoded.URL != "http://localhost:8081/users/1" {
		t.Errorf("URL = %q, want %q", decoded.URL, "http://localhost:8081/users/1")
	}
	if decoded.StartOffsetMS != 0.3 {
		t.Errorf("StartOffsetMS = %f, want 0.3", decoded.StartOffsetMS)
	}
	if decoded.BodySize != 256 {
		t.Errorf("BodySize = %d, want 256", decoded.BodySize)
	}
}

func TestStepRecord_ErrorOmitted(t *testing.T) {
	sr := StepRecord{Name: "x", Status: "success"}
	data, _ := json.Marshal(sr)
	if string(data) != `{"name":"x","status":"success","wave":0,"duration_ms":0,"http_status":0,"upstream":"","url":"","start_offset_ms":0,"body_size":0,"cached":false,"retries":0}` {
		// Just check error is not present (omitempty)
		var m map[string]any
		json.Unmarshal(data, &m)
		if _, hasErr := m["error"]; hasErr {
			t.Error("empty error should be omitted from JSON")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/reqlog/ -v -count=1`
Expected: FAIL — new fields don't exist yet on StepRecord

- [ ] **Step 3: Add new fields to reqlog.StepRecord**

In `internal/reqlog/reqlog.go`, replace the `StepRecord` struct:

```go
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
```

- [ ] **Step 4: Add new fields to composition.StepTiming**

In `internal/composition/executor.go`, replace the `StepTiming` struct:

```go
type StepTiming struct {
	Name          string  `json:"name"`
	Wave          int     `json:"wave"`
	DurationMS    float64 `json:"duration_ms"`
	Status        string  `json:"status"`
	Optional      bool    `json:"optional"`
	Upstream      string  `json:"upstream"`
	URL           string  `json:"url"`
	StartOffsetMS float64 `json:"start_offset_ms"`
	BodySize      int64   `json:"body_size"`
	Error         string  `json:"error,omitempty"`
	Cached        bool    `json:"cached"`
	Retries       int     `json:"retries"`
}
```

- [ ] **Step 5: Record start_offset_ms in the executor**

In `internal/composition/executor.go`, the `Execute` method needs to record `requestStart` and pass it down. At the top of `Execute`, capture start time:

```go
func (e *Executor) Execute(ctx context.Context, compositionName string, rd *RequestData) (*CompositionResult, error) {
	// ... existing code through plan assignment ...
	requestStart := time.Now()  // ADD THIS LINE after plan assignment
```

Then in `executeStepWithErrorHandling`, add a `requestStart time.Time` parameter. Compute `startOffsetMS` at the beginning:

```go
func (e *Executor) executeStepWithErrorHandling(
	ctx context.Context,
	compositionName string,
	stepName string,
	comp *CompiledComposition,
	plan *ExecutionPlan,
	rd *RequestData,
	results map[string]*StepResult,
	resultsMutex *sync.Mutex,
	waveNum int,
	requestStart time.Time,  // NEW PARAMETER
) (*stepError, *StepTiming) {
	stepStart := time.Now()
	startOffsetMS := float64(stepStart.Sub(requestStart).Microseconds()) / 1000.0
```

Pass `requestStart` from the goroutine launch site:

```go
stepErr, timing := e.executeStepWithErrorHandling(ctx, compositionName, stepName, comp, plan, rd, results, &resultsMutex, waveNum, requestStart)
```

Populate `StartOffsetMS` on every `StepTiming` return, and populate `Upstream`, `URL`, `BodySize`, `Cached`, `Retries`, `Error` at the relevant points:

- For the upstream lookup success case, set `timing.Upstream = step.Step.Upstream` and compute the actual URL from the evaluated path.
- For cached responses, set `timing.Cached = true`.
- For errors, set `timing.Error = err.Error()`.
- For successful responses, set `timing.BodySize = int64(len(result.RawBody))`.
- For retries, check if the upstream client has retry info (the retry tripper already counts — add a retry count to the step result or context; if not easily accessible, set to 0 for now and note it as a follow-up).

- [ ] **Step 6: Update handler.go to copy enriched fields into reqlog.StepRecord**

In `internal/composition/handler.go` around line 226-250, update the step record building:

```go
steps[i] = reqlog.StepRecord{
	Name:          t.Name,
	Status:        t.Status,
	Wave:          t.Wave,
	DurationMS:    t.DurationMS,
	HTTPStatus:    httpStatus,
	Upstream:      t.Upstream,
	URL:           t.URL,
	StartOffsetMS: t.StartOffsetMS,
	BodySize:      t.BodySize,
	Error:         t.Error,
	Cached:        t.Cached,
	Retries:       t.Retries,
}
```

- [ ] **Step 7: Update any existing tests that construct StepTiming or StepRecord**

Search for tests that create `StepTiming` or `reqlog.StepRecord` literals and add the new zero-value fields where needed so they compile. The admin_test.go and handler_test.go are likely candidates.

Run: `grep -rn "StepTiming{" internal/ cmd/ --include="*_test.go"` and `grep -rn "StepRecord{" internal/ cmd/ --include="*_test.go"` to find all sites.

- [ ] **Step 8: Run all tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: All pass.

- [ ] **Step 9: Commit**

```bash
git add internal/reqlog/ internal/composition/executor.go internal/composition/handler.go internal/admin/admin_test.go
git commit -m "feat(studio-obs): enrich StepRecord with execution detail"
```

---

### Task 2: Expose inferred dependencies in admin API

**Files:**
- Modify: `internal/admin/server.go` (add `InferredDeps` to `StepInfo`)
- Modify: `cmd/restitch/run.go:314-349` (`compositionsFromPipeline` — populate `InferredDeps`)
- Modify: `internal/admin/admin_test.go` (test the new field)

**Interfaces:**
- Consumes: `composition.CompiledStep.Deps` (all deps), `composition.Step.DependsOn` (explicit deps only)
- Produces: `admin.StepInfo.InferredDeps []string` — the difference between all deps and explicit deps

- [ ] **Step 1: Write test for InferredDeps in admin API response**

Add to `internal/admin/admin_test.go`:

```go
func TestCompositionInfo_InferredDeps(t *testing.T) {
	si := StepInfo{
		Name:         "bonus",
		Upstream:     "api",
		Method:       "GET",
		DependsOn:    []string{},
		InferredDeps: []string{"user", "loyalty"},
	}
	data, _ := json.Marshal(si)
	var m map[string]any
	json.Unmarshal(data, &m)

	inferred, ok := m["inferred_deps"].([]any)
	if !ok {
		t.Fatal("inferred_deps not present or not array")
	}
	if len(inferred) != 2 {
		t.Errorf("inferred_deps length = %d, want 2", len(inferred))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/admin/ -run TestCompositionInfo_InferredDeps -v -count=1`
Expected: FAIL — `InferredDeps` field doesn't exist

- [ ] **Step 3: Add InferredDeps field to admin.StepInfo**

In `internal/admin/server.go`:

```go
type StepInfo struct {
	Name         string   `json:"name"`
	Upstream     string   `json:"upstream"`
	Method       string   `json:"method"`
	Optional     bool     `json:"optional"`
	TimeoutMS    int64    `json:"timeout_ms"`
	DependsOn    []string `json:"depends_on"`
	InferredDeps []string `json:"inferred_deps"`
}
```

- [ ] **Step 4: Populate InferredDeps in compositionsFromPipeline**

In `cmd/restitch/run.go`, inside `compositionsFromPipeline`, after building the `StepInfo`, compute inferred deps from the compiled step:

```go
for _, step := range comp.Steps {
	si := admin.StepInfo{
		Name:     step.Name,
		Upstream: step.Upstream,
		Method:   step.Method,
		Optional: step.Optional,
	}
	if step.Timeout != nil {
		si.TimeoutMS = step.Timeout.Milliseconds()
	}
	if step.DependsOn != nil {
		si.DependsOn = step.DependsOn
	} else {
		si.DependsOn = []string{}
	}

	// Compute inferred deps: all deps minus explicit deps
	if cc, ok := p.Compiled.Compositions[name]; ok {
		if cs, ok := cc.Steps[step.Name]; ok {
			explicit := make(map[string]bool, len(step.DependsOn))
			for _, d := range step.DependsOn {
				explicit[d] = true
			}
			var inferred []string
			for _, d := range cs.Deps {
				if !explicit[d] {
					inferred = append(inferred, d)
				}
			}
			if inferred == nil {
				inferred = []string{}
			}
			si.InferredDeps = inferred
		}
	}
	if si.InferredDeps == nil {
		si.InferredDeps = []string{}
	}

	ci.Steps = append(ci.Steps, si)
}
```

- [ ] **Step 5: Run all tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: All pass.

- [ ] **Step 6: Commit**

```bash
git add internal/admin/server.go cmd/restitch/run.go internal/admin/admin_test.go
git commit -m "feat(studio-obs): expose inferred dependencies in admin API"
```

---

### Task 3: Time-series accumulator and memory storage

**Files:**
- Create: `internal/admin/storage.go` (Storage interface + types)
- Create: `internal/admin/memory_storage.go` (in-memory implementation)
- Create: `internal/admin/accumulator.go` (thread-safe request accumulator)
- Create: `internal/admin/storage_test.go` (tests for accumulator + memory storage)

**Interfaces:**
- Consumes: nothing from earlier tasks directly (standalone package)
- Produces:
  - `Storage` interface: `RecordBucket`, `QueryTimeSeries`, `RecordRequest`, `QueryRequests`, `QueryStepMetrics`, `Compact`, `Close`
  - `Accumulator` struct: `Record(composition string, durationMS float64, isError, isPartial bool, steps []StepSample)`, `Flush() []Bucket`
  - `NewMemoryStorage(retention time.Duration) *MemoryStorage`
  - `NewAccumulator() *Accumulator`
  - Latency bucket boundaries: `var LatencyBucketBounds = []float64{10, 50, 100, 250, 500, 1000, 5000}`

- [ ] **Step 1: Write tests for the accumulator**

Create `internal/admin/storage_test.go`:

```go
package admin

import (
	"testing"
	"time"
)

func TestAccumulator_Flush(t *testing.T) {
	acc := NewAccumulator()

	acc.Record("comp1", 42.0, false, false, []StepSample{
		{Name: "s1", Upstream: "api", DurationMS: 30.0, IsError: false},
		{Name: "s2", Upstream: "api", DurationMS: 12.0, IsError: false},
	})
	acc.Record("comp1", 150.0, true, false, []StepSample{
		{Name: "s1", Upstream: "api", DurationMS: 150.0, IsError: true},
	})
	acc.Record("comp2", 5.0, false, true, nil)

	buckets := acc.Flush()

	if len(buckets) != 3 {
		t.Fatalf("expected 3 buckets (global + 2 compositions), got %d", len(buckets))
	}

	var global *Bucket
	for i := range buckets {
		if buckets[i].Composition == "" {
			global = &buckets[i]
		}
	}
	if global == nil {
		t.Fatal("no global bucket")
	}
	if global.Requests != 3 {
		t.Errorf("global requests = %d, want 3", global.Requests)
	}
	if global.Errors != 1 {
		t.Errorf("global errors = %d, want 1", global.Errors)
	}
	if global.Partials != 1 {
		t.Errorf("global partials = %d, want 1", global.Partials)
	}
}

func TestAccumulator_LatencyBuckets(t *testing.T) {
	acc := NewAccumulator()
	acc.Record("c", 5.0, false, false, nil)   // bucket 0 (0-10ms)
	acc.Record("c", 75.0, false, false, nil)  // bucket 2 (50-100ms)
	acc.Record("c", 300.0, false, false, nil) // bucket 4 (250-500ms)

	buckets := acc.Flush()
	for _, b := range buckets {
		if b.Composition == "c" {
			if len(b.LatencyBuckets) != 8 {
				t.Fatalf("latency buckets length = %d, want 8", len(b.LatencyBuckets))
			}
			if b.LatencyBuckets[0] != 1 {
				t.Errorf("bucket[0] = %d, want 1", b.LatencyBuckets[0])
			}
			if b.LatencyBuckets[2] != 1 {
				t.Errorf("bucket[2] = %d, want 1", b.LatencyBuckets[2])
			}
			if b.LatencyBuckets[4] != 1 {
				t.Errorf("bucket[4] = %d, want 1", b.LatencyBuckets[4])
			}
			return
		}
	}
	t.Fatal("comp bucket not found")
}

func TestMemoryStorage_QueryTimeSeries(t *testing.T) {
	s := NewMemoryStorage(time.Hour)
	defer s.Close()

	now := time.Now().Truncate(time.Minute)
	for i := 0; i < 5; i++ {
		s.RecordBucket(nil, Bucket{
			Timestamp:   now.Add(time.Duration(i) * time.Minute),
			Composition: "",
			Requests:    int64(i + 1),
		})
	}

	results, err := s.QueryTimeSeries(nil, now, now.Add(5*time.Minute), time.Minute, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 5 {
		t.Errorf("results length = %d, want 5", len(results))
	}
}

func TestMemoryStorage_Compact(t *testing.T) {
	s := NewMemoryStorage(time.Minute)
	defer s.Close()

	old := time.Now().Add(-2 * time.Minute).Truncate(time.Minute)
	recent := time.Now().Truncate(time.Minute)

	s.RecordBucket(nil, Bucket{Timestamp: old, Composition: "", Requests: 1})
	s.RecordBucket(nil, Bucket{Timestamp: recent, Composition: "", Requests: 2})

	s.Compact(nil, time.Minute)

	results, _ := s.QueryTimeSeries(nil, old, recent.Add(time.Minute), time.Minute, "")
	if len(results) != 1 {
		t.Errorf("after compact, results = %d, want 1 (only recent)", len(results))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/admin/ -run "TestAccumulator|TestMemoryStorage" -v -count=1`
Expected: FAIL — types don't exist

- [ ] **Step 3: Create storage.go with types and interface**

Create `internal/admin/storage.go`:

```go
package admin

import (
	"context"
	"time"

	"github.com/restitch/restitch-gateway/internal/reqlog"
)

var LatencyBucketBounds = []float64{10, 50, 100, 250, 500, 1000, 5000}

type Bucket struct {
	Timestamp      time.Time              `json:"timestamp"`
	Composition    string                 `json:"composition"`
	Requests       int64                  `json:"requests"`
	Errors         int64                  `json:"errors"`
	Partials       int64                  `json:"partials"`
	LatencyP50     float64                `json:"latency_p50"`
	LatencyP95     float64                `json:"latency_p95"`
	LatencyP99     float64                `json:"latency_p99"`
	LatencyBuckets []int64                `json:"latency_buckets"`
	StepMetrics    map[string]*StepBucket `json:"step_metrics,omitempty"`
}

type StepBucket struct {
	Requests int64   `json:"requests"`
	Errors   int64   `json:"errors"`
	AvgMS    float64 `json:"avg_ms"`
	P95MS    float64 `json:"p95_ms"`
}

type StepAggregate struct {
	Name     string  `json:"name"`
	Upstream string  `json:"upstream"`
	Requests int64   `json:"requests"`
	Errors   int64   `json:"errors"`
	AvgMS    float64 `json:"avg_ms"`
	P95MS    float64 `json:"p95_ms"`
	P99MS    float64 `json:"p99_ms"`
}

type RequestQuery struct {
	Limit       int
	Composition string
	StatusMin   int
	StatusMax   int
	MinDuration float64
	Partial     *bool
}

type Storage interface {
	RecordBucket(ctx context.Context, bucket Bucket) error
	QueryTimeSeries(ctx context.Context, from, to time.Time, resolution time.Duration, composition string) ([]Bucket, error)
	RecordRequest(ctx context.Context, record reqlog.Record) error
	QueryRequests(ctx context.Context, opts RequestQuery) ([]reqlog.Record, error)
	QueryStepMetrics(ctx context.Context, composition string, from, to time.Time) ([]StepAggregate, error)
	Compact(ctx context.Context, retention time.Duration) error
	Close() error
}

type StepSample struct {
	Name      string
	Upstream  string
	DurationMS float64
	IsError   bool
}
```

- [ ] **Step 4: Create accumulator.go**

Create `internal/admin/accumulator.go`:

```go
package admin

import (
	"math"
	"sort"
	"sync"
	"time"
)

type Accumulator struct {
	mu       sync.Mutex
	global   *accBucket
	perComp  map[string]*accBucket
}

type accBucket struct {
	requests  int64
	errors    int64
	partials  int64
	latencies []float64
	steps     map[string]*accStep
}

type accStep struct {
	latencies []float64
	errors    int64
	upstream  string
}

func NewAccumulator() *Accumulator {
	return &Accumulator{
		global:  newAccBucket(),
		perComp: make(map[string]*accBucket),
	}
}

func newAccBucket() *accBucket {
	return &accBucket{steps: make(map[string]*accStep)}
}

func (a *Accumulator) Record(composition string, durationMS float64, isError, isPartial bool, steps []StepSample) {
	a.mu.Lock()
	defer a.mu.Unlock()

	recordBucket(a.global, durationMS, isError, isPartial, steps)

	cb, ok := a.perComp[composition]
	if !ok {
		cb = newAccBucket()
		a.perComp[composition] = cb
	}
	recordBucket(cb, durationMS, isError, isPartial, steps)
}

func recordBucket(b *accBucket, durationMS float64, isError, isPartial bool, steps []StepSample) {
	b.requests++
	if isError {
		b.errors++
	}
	if isPartial {
		b.partials++
	}
	b.latencies = append(b.latencies, durationMS)
	for _, s := range steps {
		as, ok := b.steps[s.Name]
		if !ok {
			as = &accStep{upstream: s.Upstream}
			b.steps[s.Name] = as
		}
		as.latencies = append(as.latencies, s.DurationMS)
		if s.IsError {
			as.errors++
		}
	}
}

func (a *Accumulator) Flush() []Bucket {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now().Truncate(time.Minute)
	var buckets []Bucket

	buckets = append(buckets, toBucket(now, "", a.global))
	for comp, ab := range a.perComp {
		buckets = append(buckets, toBucket(now, comp, ab))
	}

	a.global = newAccBucket()
	a.perComp = make(map[string]*accBucket)

	return buckets
}

func toBucket(ts time.Time, composition string, ab *accBucket) Bucket {
	b := Bucket{
		Timestamp:      ts,
		Composition:    composition,
		Requests:       ab.requests,
		Errors:         ab.errors,
		Partials:       ab.partials,
		LatencyBuckets: computeLatencyBuckets(ab.latencies),
	}

	if len(ab.latencies) > 0 {
		b.LatencyP50 = percentile(ab.latencies, 0.50)
		b.LatencyP95 = percentile(ab.latencies, 0.95)
		b.LatencyP99 = percentile(ab.latencies, 0.99)
	}

	if len(ab.steps) > 0 {
		b.StepMetrics = make(map[string]*StepBucket, len(ab.steps))
		for name, as := range ab.steps {
			sb := &StepBucket{
				Requests: int64(len(as.latencies)),
				Errors:   as.errors,
			}
			if len(as.latencies) > 0 {
				var sum float64
				for _, v := range as.latencies {
					sum += v
				}
				sb.AvgMS = math.Round(sum/float64(len(as.latencies))*100) / 100
				sb.P95MS = percentile(as.latencies, 0.95)
			}
			b.StepMetrics[name] = sb
		}
	}

	return b
}

func computeLatencyBuckets(latencies []float64) []int64 {
	buckets := make([]int64, len(LatencyBucketBounds)+1)
	for _, v := range latencies {
		placed := false
		for i, bound := range LatencyBucketBounds {
			if v <= bound {
				buckets[i]++
				placed = true
				break
			}
		}
		if !placed {
			buckets[len(buckets)-1]++
		}
	}
	return buckets
}

func percentile(data []float64, p float64) float64 {
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return math.Round(sorted[idx]*100) / 100
}
```

- [ ] **Step 5: Create memory_storage.go**

Create `internal/admin/memory_storage.go`:

```go
package admin

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/restitch/restitch-gateway/internal/reqlog"
)

type MemoryStorage struct {
	mu        sync.RWMutex
	buckets   []Bucket
	requests  []reqlog.Record
	retention time.Duration
	maxReqs   int
}

func NewMemoryStorage(retention time.Duration) *MemoryStorage {
	if retention == 0 {
		retention = 24 * time.Hour
	}
	return &MemoryStorage{
		retention: retention,
		maxReqs:   10000,
	}
}

func (m *MemoryStorage) RecordBucket(_ context.Context, bucket Bucket) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buckets = append(m.buckets, bucket)
	return nil
}

func (m *MemoryStorage) QueryTimeSeries(_ context.Context, from, to time.Time, resolution time.Duration, composition string) ([]Bucket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []Bucket
	for _, b := range m.buckets {
		if b.Timestamp.Before(from) || !b.Timestamp.Before(to) {
			continue
		}
		if composition != "" && b.Composition != composition {
			continue
		}
		if composition == "" && b.Composition != "" {
			continue
		}
		results = append(results, b)
	}

	if resolution > time.Minute {
		results = aggregateBuckets(results, resolution)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.Before(results[j].Timestamp)
	})

	return results, nil
}

func (m *MemoryStorage) RecordRequest(_ context.Context, record reqlog.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, record)
	if len(m.requests) > m.maxReqs {
		m.requests = m.requests[len(m.requests)-m.maxReqs:]
	}
	return nil
}

func (m *MemoryStorage) QueryRequests(_ context.Context, opts RequestQuery) ([]reqlog.Record, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}

	var filtered []reqlog.Record
	for i := len(m.requests) - 1; i >= 0 && len(filtered) < limit; i-- {
		r := m.requests[i]
		if opts.Composition != "" && r.Composition != opts.Composition {
			continue
		}
		if opts.StatusMin > 0 && r.Status < opts.StatusMin {
			continue
		}
		if opts.StatusMax > 0 && r.Status > opts.StatusMax {
			continue
		}
		if opts.MinDuration > 0 && r.DurationMS < opts.MinDuration {
			continue
		}
		if opts.Partial != nil && r.Partial != *opts.Partial {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered, nil
}

func (m *MemoryStorage) QueryStepMetrics(_ context.Context, composition string, from, to time.Time) ([]StepAggregate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stepAcc := make(map[string]*stepAggAcc)
	for _, b := range m.buckets {
		if b.Composition != composition {
			continue
		}
		if b.Timestamp.Before(from) || !b.Timestamp.Before(to) {
			continue
		}
		for name, sm := range b.StepMetrics {
			acc, ok := stepAcc[name]
			if !ok {
				acc = &stepAggAcc{}
				stepAcc[name] = acc
			}
			acc.requests += sm.Requests
			acc.errors += sm.Errors
			acc.avgSum += sm.AvgMS * float64(sm.Requests)
			acc.p95Samples = append(acc.p95Samples, sm.P95MS)
		}
	}

	var results []StepAggregate
	for name, acc := range stepAcc {
		sa := StepAggregate{
			Name:     name,
			Requests: acc.requests,
			Errors:   acc.errors,
		}
		if acc.requests > 0 {
			sa.AvgMS = acc.avgSum / float64(acc.requests)
		}
		if len(acc.p95Samples) > 0 {
			sa.P95MS = percentile(acc.p95Samples, 0.95)
			sa.P99MS = percentile(acc.p95Samples, 0.99)
		}
		results = append(results, sa)
	}
	return results, nil
}

func (m *MemoryStorage) Compact(_ context.Context, retention time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-retention)
	var kept []Bucket
	for _, b := range m.buckets {
		if !b.Timestamp.Before(cutoff) {
			kept = append(kept, b)
		}
	}
	m.buckets = kept

	var keptReqs []reqlog.Record
	for _, r := range m.requests {
		if !r.Time.Before(cutoff) {
			keptReqs = append(keptReqs, r)
		}
	}
	m.requests = keptReqs
	return nil
}

func (m *MemoryStorage) Close() error {
	return nil
}

type stepAggAcc struct {
	requests   int64
	errors     int64
	avgSum     float64
	p95Samples []float64
}

func aggregateBuckets(buckets []Bucket, resolution time.Duration) []Bucket {
	groups := make(map[int64]*Bucket)
	for _, b := range buckets {
		key := b.Timestamp.Truncate(resolution).Unix()
		if existing, ok := groups[key]; ok {
			existing.Requests += b.Requests
			existing.Errors += b.Errors
			existing.Partials += b.Partials
		} else {
			copy := b
			copy.Timestamp = b.Timestamp.Truncate(resolution)
			groups[key] = &copy
		}
	}
	var result []Bucket
	for _, b := range groups {
		result = append(result, *b)
	}
	return result
}
```

- [ ] **Step 6: Run tests**

Run: `go build ./... && go vet ./... && go test ./internal/admin/ -v -count=1`
Expected: All pass.

- [ ] **Step 7: Commit**

```bash
git add internal/admin/storage.go internal/admin/accumulator.go internal/admin/memory_storage.go internal/admin/storage_test.go
git commit -m "feat(studio-obs): add time-series accumulator and memory storage"
```

---

### Task 4: SQLite/Turso storage backend

**Files:**
- Create: `internal/admin/sql_storage.go` (SQLite/Turso implementation)
- Create: `internal/admin/sql_storage_test.go`
- Modify: `go.mod` (add `github.com/tursodatabase/go-libsql`)

**Interfaces:**
- Consumes: `Storage` interface from Task 3
- Produces: `NewSQLStorage(driverURL string, authToken string, retention time.Duration) (*SQLStorage, error)` — implements `Storage`

- [ ] **Step 1: Add go-libsql dependency**

Run: `go get github.com/tursodatabase/go-libsql`

Note: if `go-libsql` requires CGO, use `modernc.org/sqlite` as a pure-Go fallback with `database/sql` driver name `"sqlite"`. Check which is available: run `go get github.com/tursodatabase/go-libsql@latest` first; if it fails on darwin/arm64 without CGO, fall back to `go get modernc.org/sqlite@latest` and use `_ "modernc.org/sqlite"` import with driver `"sqlite"`. Both support `database/sql` interface; the code is the same either way.

- [ ] **Step 2: Write tests for SQL storage**

Create `internal/admin/sql_storage_test.go`:

```go
package admin

import (
	"context"
	"testing"
	"time"

	"github.com/restitch/restitch-gateway/internal/reqlog"
)

func TestSQLStorage_RecordAndQuery(t *testing.T) {
	s, err := NewSQLStorage("file::memory:?cache=shared", "", 24*time.Hour)
	if err != nil {
		t.Fatalf("NewSQLStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().Truncate(time.Minute)

	err = s.RecordBucket(ctx, Bucket{
		Timestamp:   now,
		Composition: "",
		Requests:    10,
		Errors:      2,
	})
	if err != nil {
		t.Fatal(err)
	}

	results, err := s.QueryTimeSeries(ctx, now.Add(-time.Minute), now.Add(time.Minute), time.Minute, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].Requests != 10 {
		t.Errorf("requests = %d, want 10", results[0].Requests)
	}
}

func TestSQLStorage_RecordRequest(t *testing.T) {
	s, err := NewSQLStorage("file::memory:?cache=shared", "", 24*time.Hour)
	if err != nil {
		t.Fatalf("NewSQLStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	rec := reqlog.Record{
		ID:          "req-1",
		Time:        time.Now(),
		Composition: "test",
		Method:      "GET",
		Path:        "/api/test",
		Status:      200,
		DurationMS:  42.5,
	}
	if err := s.RecordRequest(ctx, rec); err != nil {
		t.Fatal(err)
	}

	results, err := s.QueryRequests(ctx, RequestQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].ID != "req-1" {
		t.Errorf("id = %q, want %q", results[0].ID, "req-1")
	}
}

func TestSQLStorage_Compact(t *testing.T) {
	s, err := NewSQLStorage("file::memory:?cache=shared", "", time.Minute)
	if err != nil {
		t.Fatalf("NewSQLStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	old := time.Now().Add(-2 * time.Minute).Truncate(time.Minute)
	recent := time.Now().Truncate(time.Minute)

	s.RecordBucket(ctx, Bucket{Timestamp: old, Requests: 1})
	s.RecordBucket(ctx, Bucket{Timestamp: recent, Requests: 2})

	s.Compact(ctx, time.Minute)

	results, _ := s.QueryTimeSeries(ctx, old, recent.Add(time.Minute), time.Minute, "")
	if len(results) != 1 {
		t.Errorf("after compact = %d, want 1", len(results))
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/admin/ -run "TestSQLStorage" -v -count=1`
Expected: FAIL — `NewSQLStorage` doesn't exist

- [ ] **Step 4: Implement SQLStorage**

Create `internal/admin/sql_storage.go`:

```go
package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"

	_ "modernc.org/sqlite" // or go-libsql driver
	"github.com/restitch/restitch-gateway/internal/reqlog"
)

type SQLStorage struct {
	db        *sql.DB
	retention time.Duration
}

func NewSQLStorage(dsn string, authToken string, retention time.Duration) (*SQLStorage, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS timeseries_buckets (
			timestamp   INTEGER NOT NULL,
			composition TEXT NOT NULL DEFAULT '',
			data        TEXT NOT NULL,
			PRIMARY KEY (timestamp, composition)
		);
		CREATE TABLE IF NOT EXISTS request_log (
			id          TEXT PRIMARY KEY,
			timestamp   INTEGER NOT NULL,
			composition TEXT NOT NULL,
			data        TEXT NOT NULL,
			created_at  INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_requests_ts ON request_log(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_requests_comp ON request_log(composition, timestamp DESC);
	`); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLStorage{db: db, retention: retention}, nil
}

func (s *SQLStorage) RecordBucket(ctx context.Context, bucket Bucket) error {
	data, err := json.Marshal(bucket)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO timeseries_buckets (timestamp, composition, data) VALUES (?, ?, ?)`,
		bucket.Timestamp.Unix(), bucket.Composition, string(data))
	return err
}

func (s *SQLStorage) QueryTimeSeries(ctx context.Context, from, to time.Time, resolution time.Duration, composition string) ([]Bucket, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data FROM timeseries_buckets WHERE timestamp >= ? AND timestamp < ? AND composition = ? ORDER BY timestamp`,
		from.Unix(), to.Unix(), composition)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Bucket
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var b Bucket
		if err := json.Unmarshal([]byte(data), &b); err != nil {
			continue
		}
		results = append(results, b)
	}

	if resolution > time.Minute {
		results = aggregateBuckets(results, resolution)
		sort.Slice(results, func(i, j int) bool {
			return results[i].Timestamp.Before(results[j].Timestamp)
		})
	}

	return results, nil
}

func (s *SQLStorage) RecordRequest(ctx context.Context, record reqlog.Record) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO request_log (id, timestamp, composition, data, created_at) VALUES (?, ?, ?, ?, ?)`,
		record.ID, record.Time.Unix(), record.Composition, string(data), time.Now().Unix())
	return err
}

func (s *SQLStorage) QueryRequests(ctx context.Context, opts RequestQuery) ([]reqlog.Record, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT data FROM request_log ORDER BY timestamp DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []reqlog.Record
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var r reqlog.Record
		if err := json.Unmarshal([]byte(data), &r); err != nil {
			continue
		}
		if opts.Composition != "" && r.Composition != opts.Composition {
			continue
		}
		if opts.StatusMin > 0 && r.Status < opts.StatusMin {
			continue
		}
		if opts.StatusMax > 0 && r.Status > opts.StatusMax {
			continue
		}
		if opts.MinDuration > 0 && r.DurationMS < opts.MinDuration {
			continue
		}
		if opts.Partial != nil && r.Partial != *opts.Partial {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

func (s *SQLStorage) QueryStepMetrics(ctx context.Context, composition string, from, to time.Time) ([]StepAggregate, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT data FROM timeseries_buckets WHERE composition = ? AND timestamp >= ? AND timestamp < ?`,
		composition, from.Unix(), to.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stepAcc := make(map[string]*stepAggAcc)
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var b Bucket
		if err := json.Unmarshal([]byte(data), &b); err != nil {
			continue
		}
		for name, sm := range b.StepMetrics {
			acc, ok := stepAcc[name]
			if !ok {
				acc = &stepAggAcc{}
				stepAcc[name] = acc
			}
			acc.requests += sm.Requests
			acc.errors += sm.Errors
			acc.avgSum += sm.AvgMS * float64(sm.Requests)
			acc.p95Samples = append(acc.p95Samples, sm.P95MS)
		}
	}

	var results []StepAggregate
	for name, acc := range stepAcc {
		sa := StepAggregate{
			Name:     name,
			Requests: acc.requests,
			Errors:   acc.errors,
		}
		if acc.requests > 0 {
			sa.AvgMS = acc.avgSum / float64(acc.requests)
		}
		if len(acc.p95Samples) > 0 {
			sa.P95MS = percentile(acc.p95Samples, 0.95)
			sa.P99MS = percentile(acc.p95Samples, 0.99)
		}
		results = append(results, sa)
	}
	return results, nil
}

func (s *SQLStorage) Compact(ctx context.Context, retention time.Duration) error {
	cutoff := time.Now().Add(-retention).Unix()
	_, err := s.db.ExecContext(ctx, `DELETE FROM timeseries_buckets WHERE timestamp < ?`, cutoff)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM request_log WHERE timestamp < ?`, cutoff)
	return err
}

func (s *SQLStorage) Close() error {
	return s.db.Close()
}
```

- [ ] **Step 5: Run tests**

Run: `go build ./... && go vet ./... && go test ./internal/admin/ -v -count=1`
Expected: All pass.

- [ ] **Step 6: Commit**

```bash
git add internal/admin/sql_storage.go internal/admin/sql_storage_test.go go.mod go.sum
git commit -m "feat(studio-obs): add SQLite/Turso storage backend"
```

---

### Task 5: Wire time-series store + new admin API endpoints

**Files:**
- Modify: `internal/admin/server.go` (add 3 new endpoints, add `StorageConfig`, wire `Accumulator` + `Storage`)
- Modify: `internal/admin/recorder.go` (extend `MultiRecorder` to feed accumulator)
- Modify: `cmd/restitch/run.go` (create storage from config, pass to admin, start flush goroutine)
- Modify: `internal/composition/parser.go` or config types (add `StorageConfig` to admin config)
- Create: `internal/admin/timeseries_api_test.go` (test the new API endpoints)

**Interfaces:**
- Consumes: `Storage` (Task 3), `Accumulator` (Task 3), `StepSample` (Task 3), enriched `StepTiming` (Task 1)
- Produces:
  - `GET /admin/api/stats/timeseries?range=1h&resolution=1m&composition=X` → `[]Bucket`
  - `GET /admin/api/stats/steps?composition=X&range=1h` → `[]StepAggregate`
  - `GET /admin/api/requests/{id}` → `reqlog.Record`
  - Existing `GET /admin/api/requests` gains filter query params

- [ ] **Step 1: Write test for timeseries endpoint**

Create `internal/admin/timeseries_api_test.go`:

```go
package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleTimeSeries(t *testing.T) {
	store := NewMemoryStorage(time.Hour)
	now := time.Now().Truncate(time.Minute)
	store.RecordBucket(nil, Bucket{
		Timestamp:   now,
		Composition: "",
		Requests:    10,
		Errors:      1,
	})

	s := &Server{
		deps: Deps{Storage: store},
	}

	req := httptest.NewRequest("GET", "/admin/api/stats/timeseries?range=1h&resolution=1m", nil)
	w := httptest.NewRecorder()
	s.handleTimeSeries(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var buckets []Bucket
	json.NewDecoder(w.Body).Decode(&buckets)
	if len(buckets) == 0 {
		t.Error("expected at least one bucket")
	}
}

func TestHandleRequestByID(t *testing.T) {
	store := NewMemoryStorage(time.Hour)
	store.RecordRequest(nil, reqlog.Record{
		ID:          "test-123",
		Composition: "comp1",
		Status:      200,
		DurationMS:  42.5,
		Time:        time.Now(),
	})

	s := &Server{
		deps: Deps{Storage: store},
	}

	req := httptest.NewRequest("GET", "/admin/api/requests/test-123", nil)
	req.SetPathValue("id", "test-123")
	w := httptest.NewRecorder()
	s.handleRequestByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}
```

(Add import for `reqlog` package at top.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/admin/ -run "TestHandleTimeSeries|TestHandleRequestByID" -v -count=1`
Expected: FAIL

- [ ] **Step 3: Add Storage field to Deps and StorageConfig**

In `internal/admin/server.go`, add to `Deps`:

```go
type Deps struct {
	// ... existing fields ...
	Storage Storage
}
```

Add `StorageConfig`:

```go
type StorageConfig struct {
	Type      string `yaml:"type"`
	URL       string `yaml:"url"`
	AuthToken string `yaml:"auth_token"`
	Retention string `yaml:"retention"`
}
```

Add to `Config`:

```go
type Config struct {
	Enabled        bool          `yaml:"enabled"`
	Port           int           `yaml:"port"`
	APIKey         string        `yaml:"api_key"`
	RequestLogSize int           `yaml:"request_log_size"`
	Storage        StorageConfig `yaml:"storage"`
}
```

- [ ] **Step 4: Add three new route handlers**

In `internal/admin/server.go`, register new routes in `New()`:

```go
mux.HandleFunc("GET /admin/api/stats/timeseries", s.requireKey(s.handleTimeSeries))
mux.HandleFunc("GET /admin/api/stats/steps", s.requireKey(s.handleStepMetrics))
mux.HandleFunc("GET /admin/api/requests/{id}", s.requireKey(s.handleRequestByID))
```

Implement the handlers:

```go
func (s *Server) handleTimeSeries(w http.ResponseWriter, r *http.Request) {
	if s.deps.Storage == nil {
		writeJSON(w, http.StatusOK, []Bucket{})
		return
	}

	rangeDur := parseDuration(r.URL.Query().Get("range"), time.Hour)
	resolution := parseDuration(r.URL.Query().Get("resolution"), time.Minute)
	composition := r.URL.Query().Get("composition")

	to := time.Now()
	from := to.Add(-rangeDur)

	buckets, err := s.deps.Storage.QueryTimeSeries(r.Context(), from, to, resolution, composition)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if buckets == nil {
		buckets = []Bucket{}
	}
	writeJSON(w, http.StatusOK, buckets)
}

func (s *Server) handleStepMetrics(w http.ResponseWriter, r *http.Request) {
	if s.deps.Storage == nil {
		writeJSON(w, http.StatusOK, []StepAggregate{})
		return
	}

	composition := r.URL.Query().Get("composition")
	if composition == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "composition parameter required"})
		return
	}

	rangeDur := parseDuration(r.URL.Query().Get("range"), time.Hour)
	to := time.Now()
	from := to.Add(-rangeDur)

	metrics, err := s.deps.Storage.QueryStepMetrics(r.Context(), composition, from, to)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if metrics == nil {
		metrics = []StepAggregate{}
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleRequestByID(w http.ResponseWriter, r *http.Request) {
	if s.deps.Storage == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "storage not configured"})
		return
	}

	id := r.PathValue("id")
	records, err := s.deps.Storage.QueryRequests(r.Context(), RequestQuery{Limit: 1000})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for _, rec := range records {
		if rec.ID == id {
			writeJSON(w, http.StatusOK, rec)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "request not found"})
}

func parseDuration(s string, fallback time.Duration) time.Duration {
	switch s {
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	default:
		return fallback
	}
}
```

- [ ] **Step 5: Add filter params to existing handleRequests**

Update `handleRequests` to accept filter query params:

```go
func (s *Server) handleRequests(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	if s.deps.Storage != nil {
		opts := RequestQuery{Limit: limit}
		opts.Composition = r.URL.Query().Get("composition")
		if v := r.URL.Query().Get("status_min"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				opts.StatusMin = n
			}
		}
		if v := r.URL.Query().Get("status_max"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				opts.StatusMax = n
			}
		}
		if v := r.URL.Query().Get("min_duration_ms"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				opts.MinDuration = f
			}
		}
		if v := r.URL.Query().Get("partial"); v != "" {
			p := v == "true"
			opts.Partial = &p
		}
		records, err := s.deps.Storage.QueryRequests(r.Context(), opts)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, records)
		return
	}

	writeJSON(w, http.StatusOK, s.deps.Requests.List(limit))
}
```

- [ ] **Step 6: Extend MultiRecorder to feed accumulator and storage**

In `internal/admin/recorder.go`:

```go
type MultiRecorder struct {
	Ring        *RingBuffer
	Stats       *Stats
	Accumulator *Accumulator
	Storage     Storage
}

func (mr *MultiRecorder) Record(rec reqlog.Record) {
	mr.Ring.Record(rec)
	mr.Stats.Record(rec.Composition, rec.DurationMS, rec.Status >= 500, rec.Partial)

	if mr.Accumulator != nil {
		var stepSamples []StepSample
		for _, s := range rec.Steps {
			stepSamples = append(stepSamples, StepSample{
				Name:       s.Name,
				Upstream:   s.Upstream,
				DurationMS: s.DurationMS,
				IsError:    s.Status == "failed",
			})
		}
		mr.Accumulator.Record(rec.Composition, rec.DurationMS, rec.Status >= 500, rec.Partial, stepSamples)
	}

	if mr.Storage != nil {
		mr.Storage.RecordRequest(nil, rec)
	}
}
```

- [ ] **Step 7: Wire storage creation and flush goroutine in run.go**

In `cmd/restitch/run.go`, in the admin setup section, create storage based on config and start the flush goroutine. This is specific to where the admin `Deps` struct is constructed (around line 214). Add:

```go
var store admin.Storage
var acc *admin.Accumulator

switch adminCfg.Storage.Type {
case "sqlite":
	var err error
	retention := parseDuration(adminCfg.Storage.Retention, 7*24*time.Hour)
	store, err = admin.NewSQLStorage(adminCfg.Storage.URL, adminCfg.Storage.AuthToken, retention)
	if err != nil {
		slog.Error("failed to open SQLite storage", "error", err)
		// fall back to memory
		store = admin.NewMemoryStorage(retention)
	}
case "turso":
	var err error
	retention := parseDuration(adminCfg.Storage.Retention, 30*24*time.Hour)
	store, err = admin.NewSQLStorage(adminCfg.Storage.URL, adminCfg.Storage.AuthToken, retention)
	if err != nil {
		slog.Error("failed to connect to Turso", "error", err)
		store = admin.NewMemoryStorage(retention)
	}
default:
	retention := parseDuration(adminCfg.Storage.Retention, 24*time.Hour)
	store = admin.NewMemoryStorage(retention)
}

acc = admin.NewAccumulator()

// Start flush goroutine
go func() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			buckets := acc.Flush()
			for _, b := range buckets {
				if err := store.RecordBucket(context.Background(), b); err != nil {
					slog.Error("failed to record time-series bucket", "error", err)
				}
			}
			retentionDur := parseDuration(adminCfg.Storage.Retention, 24*time.Hour)
			store.Compact(context.Background(), retentionDur)
		case <-ctx.Done():
			return
		}
	}
}()
```

Pass `store` to `admin.Deps{Storage: store}` and `acc` + `store` to the `MultiRecorder`.

- [ ] **Step 8: Run all tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: All pass.

- [ ] **Step 9: Commit**

```bash
git add internal/admin/server.go internal/admin/recorder.go internal/admin/timeseries_api_test.go cmd/restitch/run.go
git commit -m "feat(studio-obs): wire time-series store and new admin API endpoints"
```

---

### Task 6: Install Recharts + add frontend API client methods + chart theme

**Files:**
- Modify: `studio/package.json` (add `recharts`)
- Modify: `studio/src/lib/api.ts` (add new types + API methods)
- Create: `studio/src/lib/chart-theme.ts` (Recharts theme constants)
- Create: `studio/src/components/charts/TimeRangeSelector.tsx`

**Interfaces:**
- Consumes: new backend endpoints from Task 5
- Produces:
  - `api.timeseries(range, resolution, composition?)` → `TimeSeriesBucket[]`
  - `api.stepMetrics(composition, range)` → `StepAggregate[]`
  - `api.request(id)` → `RequestRecord`
  - `chartTheme` object with colors, fonts, grid styles
  - `<TimeRangeSelector value={range} onChange={setRange} />` component

- [ ] **Step 1: Install recharts**

Run: `cd studio && npm install recharts`

- [ ] **Step 2: Add new types and API methods to api.ts**

In `studio/src/lib/api.ts`, add the new interfaces after the existing ones:

```ts
export interface TimeSeriesBucket {
  timestamp: string
  composition: string
  requests: number
  errors: number
  partials: number
  latency_p50: number
  latency_p95: number
  latency_p99: number
  latency_buckets: number[]
  step_metrics: Record<string, { requests: number; errors: number; avg_ms: number; p95_ms: number }>
}

export interface StepAggregate {
  name: string
  upstream: string
  requests: number
  errors: number
  avg_ms: number
  p95_ms: number
  p99_ms: number
}
```

Add to the `api` object:

```ts
timeseries: (range: string, resolution: string, composition?: string) =>
  get<TimeSeriesBucket[]>(`/api/stats/timeseries?range=${range}&resolution=${resolution}${composition ? `&composition=${composition}` : ''}`),
stepMetrics: (composition: string, range: string) =>
  get<StepAggregate[]>(`/api/stats/steps?composition=${composition}&range=${range}`),
request: (id: string) =>
  get<RequestRecord>(`/api/requests/${id}`),
```

Also update `StepRecord` interface to match the enriched backend:

```ts
export interface StepRecord {
  name: string
  status: string
  wave: number
  duration_ms: number
  http_status: number
  upstream: string
  url: string
  start_offset_ms: number
  body_size: number
  error?: string
  cached: boolean
  retries: number
}
```

- [ ] **Step 3: Create chart theme**

Create `studio/src/lib/chart-theme.ts`:

```ts
export const chartTheme = {
  colors: {
    success: "oklch(0.72 0.17 142)",
    warning: "oklch(0.75 0.18 65)",
    error: "oklch(0.65 0.2 25)",
    accent: "oklch(0.75 0.18 65)",
    p50: "oklch(0.6 0.05 260)",
    p95: "oklch(0.75 0.18 65)",
    p99: "oklch(0.65 0.2 25)",
    gridLine: "rgba(178,182,189,0.08)",
    axisText: "rgba(178,182,189,0.5)",
    tooltipBg: "#1e1e21",
    tooltipBorder: "rgba(178,182,189,0.12)",
  },
  font: {
    family: "inherit",
    mono: "ui-monospace, 'Cascadia Code', 'Source Code Pro', Menlo, Consolas, monospace",
    size: 11,
  },
}

export const stepColors = [
  "oklch(0.72 0.17 142)",
  "oklch(0.68 0.16 210)",
  "oklch(0.75 0.18 65)",
  "oklch(0.7 0.15 300)",
  "oklch(0.65 0.14 170)",
  "oklch(0.72 0.16 30)",
  "oklch(0.68 0.12 240)",
  "oklch(0.74 0.14 100)",
]
```

- [ ] **Step 4: Create TimeRangeSelector component**

Create `studio/src/components/charts/TimeRangeSelector.tsx`:

```tsx
export type TimeRange = "1h" | "6h" | "24h"

export function TimeRangeSelector({ value, onChange }: { value: TimeRange; onChange: (v: TimeRange) => void }) {
  const options: TimeRange[] = ["1h", "6h", "24h"]
  return (
    <div className="inline-flex rounded-lg border border-hairline bg-surface-1 p-0.5">
      {options.map((opt) => (
        <button
          key={opt}
          onClick={() => onChange(opt)}
          className={`px-3 py-1 rounded-md text-[12px] font-semibold transition-colors ${
            value === opt
              ? "bg-surface-2 text-ink"
              : "text-ink-muted hover:text-ink"
          }`}
        >
          {opt}
        </button>
      ))}
    </div>
  )
}
```

- [ ] **Step 5: Run build**

Run: `cd studio && npm run build`
Expected: Clean build.

- [ ] **Step 6: Commit**

```bash
git add studio/package.json studio/package-lock.json studio/src/lib/api.ts studio/src/lib/chart-theme.ts studio/src/components/charts/TimeRangeSelector.tsx
git commit -m "feat(studio-obs): install Recharts, add API client methods, chart theme"
```

---

### Task 7: Dashboard charts — SparklineCard, RequestRateChart, LatencyChart

**Files:**
- Create: `studio/src/components/charts/SparklineCard.tsx`
- Create: `studio/src/components/charts/RequestRateChart.tsx`
- Create: `studio/src/components/charts/LatencyChart.tsx`
- Create: `studio/src/components/charts/LatencyHeatmap.tsx`
- Modify: `studio/src/pages/Dashboard.tsx` (overhaul with charts)

**Interfaces:**
- Consumes: `api.timeseries()`, `api.stats()`, `TimeSeriesBucket`, `chartTheme`, `TimeRangeSelector`
- Produces: chart components used in Dashboard; also reused by Task 9 (Composition Metrics)

- [ ] **Step 1: Create SparklineCard**

Create `studio/src/components/charts/SparklineCard.tsx`:

```tsx
import { AreaChart, Area, ResponsiveContainer } from "recharts"
import { chartTheme } from "../../lib/chart-theme"

interface SparklineCardProps {
  label: string
  value: string | number
  data: { value: number }[]
  color?: string
  accent?: boolean
}

export function SparklineCard({ label, value, data, color, accent }: SparklineCardProps) {
  const fillColor = color || (accent ? chartTheme.colors.warning : chartTheme.colors.accent)

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl p-5 relative overflow-hidden">
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
        {label}
      </div>
      <div className={`text-[28px] font-semibold leading-[1.17] tracking-[-0.6px] tabular-nums ${
        accent ? "text-warning" : "text-ink"
      }`}>
        {value}
      </div>
      {data.length > 1 && (
        <div className="absolute bottom-0 left-0 right-0 h-12 opacity-30">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={data} margin={{ top: 0, right: 0, bottom: 0, left: 0 }}>
              <Area
                type="monotone"
                dataKey="value"
                stroke={fillColor}
                fill={fillColor}
                strokeWidth={1.5}
                fillOpacity={0.3}
                isAnimationActive={false}
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Create RequestRateChart**

Create `studio/src/components/charts/RequestRateChart.tsx`:

```tsx
import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend } from "recharts"
import { chartTheme } from "../../lib/chart-theme"
import type { TimeSeriesBucket } from "../../lib/api"

export function RequestRateChart({ data }: { data: TimeSeriesBucket[] }) {
  const chartData = data.map((b) => ({
    time: new Date(b.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    success: b.requests - b.errors - b.partials,
    partials: b.partials,
    errors: b.errors,
  }))

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl p-5">
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-4">
        Request rate
      </div>
      <ResponsiveContainer width="100%" height={240}>
        <AreaChart data={chartData} margin={{ top: 0, right: 0, bottom: 0, left: 0 }}>
          <XAxis dataKey="time" tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} />
          <YAxis tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} width={40} />
          <Tooltip
            contentStyle={{ background: chartTheme.colors.tooltipBg, border: `1px solid ${chartTheme.colors.tooltipBorder}`, borderRadius: 8, fontSize: 12 }}
            labelStyle={{ color: chartTheme.colors.axisText }}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          <Area type="monotone" dataKey="success" stackId="1" stroke={chartTheme.colors.success} fill={chartTheme.colors.success} fillOpacity={0.3} strokeWidth={1.5} />
          <Area type="monotone" dataKey="partials" stackId="1" stroke={chartTheme.colors.warning} fill={chartTheme.colors.warning} fillOpacity={0.3} strokeWidth={1.5} />
          <Area type="monotone" dataKey="errors" stackId="1" stroke={chartTheme.colors.error} fill={chartTheme.colors.error} fillOpacity={0.3} strokeWidth={1.5} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}
```

- [ ] **Step 3: Create LatencyChart**

Create `studio/src/components/charts/LatencyChart.tsx`:

```tsx
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend } from "recharts"
import { chartTheme } from "../../lib/chart-theme"
import type { TimeSeriesBucket } from "../../lib/api"

export function LatencyChart({ data }: { data: TimeSeriesBucket[] }) {
  const chartData = data.map((b) => ({
    time: new Date(b.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    p50: b.latency_p50,
    p95: b.latency_p95,
    p99: b.latency_p99,
  }))

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl p-5">
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-4">
        Latency (ms)
      </div>
      <ResponsiveContainer width="100%" height={240}>
        <LineChart data={chartData} margin={{ top: 0, right: 0, bottom: 0, left: 0 }}>
          <XAxis dataKey="time" tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} />
          <YAxis tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} width={50} unit="ms" />
          <Tooltip
            contentStyle={{ background: chartTheme.colors.tooltipBg, border: `1px solid ${chartTheme.colors.tooltipBorder}`, borderRadius: 8, fontSize: 12 }}
            labelStyle={{ color: chartTheme.colors.axisText }}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          <Line type="monotone" dataKey="p50" stroke={chartTheme.colors.p50} strokeWidth={1.5} dot={false} name="p50" />
          <Line type="monotone" dataKey="p95" stroke={chartTheme.colors.p95} strokeWidth={2} dot={false} name="p95" />
          <Line type="monotone" dataKey="p99" stroke={chartTheme.colors.p99} strokeWidth={1.5} dot={false} strokeDasharray="4 2" name="p99" />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
```

- [ ] **Step 4: Create LatencyHeatmap**

Create `studio/src/components/charts/LatencyHeatmap.tsx`:

```tsx
import { useMemo } from "react"
import type { TimeSeriesBucket } from "../../lib/api"

const LATENCY_LABELS = ["0-10ms", "10-50ms", "50-100ms", "100-250ms", "250-500ms", "500ms-1s", "1-5s", "5s+"]

export function LatencyHeatmap({ data }: { data: TimeSeriesBucket[] }) {
  const { grid, maxCount, times } = useMemo(() => {
    let max = 0
    const g: number[][] = []
    const t: string[] = []
    for (const bucket of data) {
      const buckets = bucket.latency_buckets || []
      const row = LATENCY_LABELS.map((_, i) => buckets[i] || 0)
      for (const v of row) {
        if (v > max) max = v
      }
      g.push(row)
      t.push(new Date(bucket.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }))
    }
    return { grid: g, maxCount: max, times: t }
  }, [data])

  if (data.length === 0) return null

  const cellW = Math.max(Math.min(Math.floor(800 / data.length), 40), 8)
  const cellH = 24
  const labelW = 70
  const svgW = labelW + data.length * cellW
  const svgH = LATENCY_LABELS.length * cellH + 30

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl p-5">
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-4">
        Latency distribution
      </div>
      <div className="overflow-x-auto">
        <svg width={svgW} height={svgH} className="text-ink-muted">
          {LATENCY_LABELS.map((label, row) => (
            <text key={label} x={labelW - 6} y={row * cellH + cellH / 2 + 4} textAnchor="end" fontSize={10} fill="currentColor">
              {label}
            </text>
          ))}
          {grid.map((cols, col) =>
            cols.map((count, row) => {
              const intensity = maxCount > 0 ? count / maxCount : 0
              return (
                <rect
                  key={`${col}-${row}`}
                  x={labelW + col * cellW}
                  y={row * cellH}
                  width={cellW - 1}
                  height={cellH - 1}
                  rx={3}
                  fill={`oklch(0.75 ${0.18 * intensity} 65 / ${0.1 + 0.8 * intensity})`}
                >
                  <title>{`${times[col]}: ${count} requests (${LATENCY_LABELS[row]})`}</title>
                </rect>
              )
            })
          )}
          {times.filter((_, i) => i % Math.max(1, Math.floor(times.length / 8)) === 0).map((t, i, arr) => (
            <text
              key={t}
              x={labelW + (i * (data.length / arr.length)) * cellW + cellW / 2}
              y={LATENCY_LABELS.length * cellH + 16}
              textAnchor="middle"
              fontSize={10}
              fill="currentColor"
            >
              {t}
            </text>
          ))}
        </svg>
      </div>
    </div>
  )
}
```

- [ ] **Step 5: Overhaul Dashboard.tsx**

Rewrite `studio/src/pages/Dashboard.tsx` to use the new chart components. Keep the existing `usePoll(api.stats, 5000)` and `usePoll(api.upstreams, 10000)`, and add `usePoll(api.timeseries, 30000)`. Add time range state with `TimeRangeSelector`. Replace static `StatCard` with `SparklineCard`. Add `RequestRateChart`, `LatencyChart`, and `LatencyHeatmap` below the sparkline cards. Keep the upstream health strip and per-composition table, but add a mini sparkline column to the table and make rows clickable to navigate to `/compositions/:name`.

The full implementation follows the component APIs shown above. Use `useNavigate()` from `react-router-dom` for table row clicks.

- [ ] **Step 6: Run build**

Run: `cd studio && npm run build`
Expected: Clean build.

- [ ] **Step 7: Commit**

```bash
git add studio/src/components/charts/ studio/src/pages/Dashboard.tsx
git commit -m "feat(studio-obs): add dashboard charts with sparklines, request rate, latency, heatmap"
```

---

### Task 8: Enhanced request explorer — filters, timeline waterfall, step detail

**Files:**
- Create: `studio/src/components/filters/RequestFilters.tsx`
- Create: `studio/src/components/waterfall/TimelineWaterfall.tsx`
- Create: `studio/src/components/waterfall/StepDetailPanel.tsx`
- Create: `studio/src/components/waterfall/RequestSummary.tsx`
- Modify: `studio/src/pages/Requests.tsx` (overhaul with filters + timeline waterfall)

**Interfaces:**
- Consumes: `api.requests()`, `api.compositions()`, enriched `StepRecord` with `start_offset_ms`, `url`, etc.
- Produces: `<RequestFilters>`, `<TimelineWaterfall>`, `<StepDetailPanel>`, `<RequestSummary>` — all used in Requests page

- [ ] **Step 1: Create RequestFilters**

Create `studio/src/components/filters/RequestFilters.tsx`:

```tsx
interface RequestFiltersProps {
  compositions: string[]
  composition: string
  onCompositionChange: (v: string) => void
  statusFilter: string
  onStatusChange: (v: string) => void
  durationFilter: string
  onDurationChange: (v: string) => void
}

export function RequestFilters({
  compositions, composition, onCompositionChange,
  statusFilter, onStatusChange,
  durationFilter, onDurationChange,
}: RequestFiltersProps) {
  return (
    <div className="flex flex-wrap items-center gap-3 mb-6">
      <FilterSelect label="Composition" value={composition} onChange={onCompositionChange}
        options={[{ value: "", label: "All" }, ...compositions.map((c) => ({ value: c, label: c }))]} />
      <FilterSelect label="Status" value={statusFilter} onChange={onStatusChange}
        options={[
          { value: "", label: "All" },
          { value: "2xx", label: "2xx" },
          { value: "4xx", label: "4xx" },
          { value: "5xx", label: "5xx" },
        ]} />
      <FilterSelect label="Duration" value={durationFilter} onChange={onDurationChange}
        options={[
          { value: "", label: "All" },
          { value: "100", label: ">100ms" },
          { value: "500", label: ">500ms" },
          { value: "1000", label: ">1s" },
        ]} />
    </div>
  )
}

function FilterSelect({ label, value, onChange, options }: {
  label: string; value: string; onChange: (v: string) => void
  options: { value: string; label: string }[]
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-[11px] font-semibold tracking-[0.5px] uppercase text-ink-subtle">{label}</span>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="bg-surface-1 border border-hairline rounded-lg px-2.5 py-1 text-[12px] text-ink outline-none"
      >
        {options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
      </select>
    </div>
  )
}
```

- [ ] **Step 2: Create TimelineWaterfall**

Create `studio/src/components/waterfall/TimelineWaterfall.tsx`:

```tsx
import type { StepRecord } from "../../lib/api"

function stepStatusColor(status: string) {
  if (status === "success") return "bg-success"
  if (status === "failed") return "bg-error"
  return "bg-ink-subtle/40"
}

export function TimelineWaterfall({
  steps,
  totalDuration,
  onStepClick,
}: {
  steps: StepRecord[]
  totalDuration: number
  onStepClick?: (step: StepRecord) => void
}) {
  if (steps.length === 0) return null

  const sorted = [...steps].sort((a, b) => a.start_offset_ms - b.start_offset_ms || a.wave - b.wave)
  const maxTime = totalDuration || Math.max(...steps.map((s) => s.start_offset_ms + s.duration_ms), 1)

  return (
    <div className="space-y-1">
      <div className="text-[11px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-2">
        Timeline
      </div>
      <div className="relative">
        <div className="absolute left-28 top-0 bottom-0 w-px bg-hairline-soft" />
        {sorted.map((step) => {
          const leftPct = (step.start_offset_ms / maxTime) * 100
          const widthPct = Math.max((step.duration_ms / maxTime) * 100, 1)

          return (
            <div
              key={step.name}
              className={`flex items-center gap-3 h-7 ${onStepClick ? "cursor-pointer hover:bg-surface-2 rounded" : ""}`}
              onClick={() => onStepClick?.(step)}
            >
              <div className="w-28 text-[12px] font-medium text-ink truncate pl-1">{step.name}</div>
              <div className="flex-1 h-5 relative">
                <div
                  className={`absolute top-0.5 h-4 rounded ${stepStatusColor(step.status)} opacity-80`}
                  style={{ left: `${leftPct}%`, width: `${widthPct}%`, minWidth: 4 }}
                />
              </div>
              <div className="w-52 text-right text-[11px] text-ink-muted font-mono tabular-nums whitespace-nowrap pr-1">
                +{step.start_offset_ms.toFixed(0)}ms · {step.duration_ms.toFixed(1)}ms
                {step.http_status > 0 ? ` · ${step.http_status}` : ""}
                {step.cached ? " · cached" : ""}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
```

- [ ] **Step 3: Create StepDetailPanel**

Create `studio/src/components/waterfall/StepDetailPanel.tsx`:

```tsx
import type { StepRecord } from "../../lib/api"
import { X } from "lucide-react"

export function StepDetailPanel({ step, onClose }: { step: StepRecord; onClose: () => void }) {
  return (
    <div className="bg-surface-2 border border-hairline rounded-xl p-4 mt-2">
      <div className="flex items-center justify-between mb-3">
        <div className="text-[14px] font-semibold text-ink">{step.name}</div>
        <button onClick={onClose} className="p-1 text-ink-subtle hover:text-ink">
          <X size={14} />
        </button>
      </div>
      <div className="grid grid-cols-2 gap-3 text-[12px]">
        <Detail label="Upstream" value={step.upstream || "—"} />
        <Detail label="URL" value={step.url || "—"} mono />
        <Detail label="Status" value={step.http_status > 0 ? String(step.http_status) : "—"} />
        <Detail label="Duration" value={`${step.duration_ms.toFixed(1)}ms`} />
        <Detail label="Start offset" value={`+${step.start_offset_ms.toFixed(1)}ms`} />
        <Detail label="Body size" value={step.body_size > 0 ? `${step.body_size} bytes` : "—"} />
        <Detail label="Cached" value={step.cached ? "Yes" : "No"} />
        <Detail label="Retries" value={String(step.retries)} />
        {step.error && <Detail label="Error" value={step.error} span2 error />}
      </div>
    </div>
  )
}

function Detail({ label, value, mono, span2, error }: {
  label: string; value: string; mono?: boolean; span2?: boolean; error?: boolean
}) {
  return (
    <div className={span2 ? "col-span-2" : ""}>
      <div className="text-ink-subtle mb-0.5">{label}</div>
      <div className={`${mono ? "font-mono text-[11px]" : ""} ${error ? "text-error" : "text-ink"} break-all`}>
        {value}
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Create RequestSummary**

Create `studio/src/components/waterfall/RequestSummary.tsx`:

```tsx
import type { RequestRecord } from "../../lib/api"

export function RequestSummary({ req }: { req: RequestRecord }) {
  const stepTime = req.steps?.reduce((sum, s) => sum + s.duration_ms, 0) ?? 0
  const overhead = Math.max(req.duration_ms - stepTime, 0)

  return (
    <div className="flex items-center gap-6 mb-3 text-[12px]">
      <div>
        <span className="text-ink-subtle">Total: </span>
        <span className="font-mono tabular-nums text-ink font-medium">{req.duration_ms.toFixed(1)}ms</span>
      </div>
      <div>
        <span className="text-ink-subtle">Gateway overhead: </span>
        <span className="font-mono tabular-nums text-ink-muted">{overhead.toFixed(1)}ms</span>
      </div>
      <div>
        <span className="text-ink-subtle">Steps: </span>
        <span className="text-ink">{req.steps?.length ?? 0}</span>
      </div>
    </div>
  )
}
```

- [ ] **Step 5: Overhaul Requests.tsx**

Rewrite `studio/src/pages/Requests.tsx`:
- Add filter state (`composition`, `statusFilter`, `durationFilter`) and `<RequestFilters>` component
- Also poll `api.compositions()` to populate the composition dropdown
- Pass filter params to `api.requests()` calls (the backend now accepts them, but for backward compat also filter client-side)
- Replace the old `StepWaterfall` component with `<TimelineWaterfall>` + `<RequestSummary>`
- Add `<StepDetailPanel>` state — when a step is clicked in the waterfall, show the panel
- Keep the expandable row pattern but use the new components inside

- [ ] **Step 6: Run build**

Run: `cd studio && npm run build`
Expected: Clean build.

- [ ] **Step 7: Commit**

```bash
git add studio/src/components/filters/ studio/src/components/waterfall/ studio/src/pages/Requests.tsx
git commit -m "feat(studio-obs): enhanced request explorer with filters, timeline waterfall, step detail"
```

---

### Task 9: Composition metrics page

**Files:**
- Create: `studio/src/components/charts/StepBreakdownChart.tsx`
- Create: `studio/src/components/charts/StepComparisonChart.tsx`
- Modify: `studio/src/pages/CompositionDetail.tsx` (add Metrics tab as default)

**Interfaces:**
- Consumes: `api.timeseries(range, resolution, composition)`, `api.stepMetrics(composition, range)`, `api.requests()` (filtered), `SparklineCard`, `RequestRateChart`, `LatencyChart`, `LatencyHeatmap`, `TimeRangeSelector`, `TimelineWaterfall`
- Produces: complete Metrics tab on composition detail page

- [ ] **Step 1: Create StepBreakdownChart**

Create `studio/src/components/charts/StepBreakdownChart.tsx`:

```tsx
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend, Cell } from "recharts"
import { chartTheme, stepColors } from "../../lib/chart-theme"
import type { StepAggregate } from "../../lib/api"

export function StepBreakdownChart({ steps }: { steps: StepAggregate[] }) {
  const sorted = [...steps].sort((a, b) => b.avg_ms - a.avg_ms)
  const data = sorted.map((s, i) => ({
    name: s.name,
    avg: s.avg_ms,
    p95: s.p95_ms,
    color: stepColors[i % stepColors.length],
  }))

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl p-5">
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-4">
        Step latency breakdown
      </div>
      <ResponsiveContainer width="100%" height={Math.max(steps.length * 40 + 20, 120)}>
        <BarChart data={data} layout="vertical" margin={{ top: 0, right: 20, bottom: 0, left: 80 }}>
          <XAxis type="number" tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} unit="ms" />
          <YAxis type="category" dataKey="name" tick={{ fill: chartTheme.colors.axisText, fontSize: 12 }} axisLine={false} tickLine={false} width={80} />
          <Tooltip
            contentStyle={{ background: chartTheme.colors.tooltipBg, border: `1px solid ${chartTheme.colors.tooltipBorder}`, borderRadius: 8, fontSize: 12 }}
            formatter={(value: number) => [`${value.toFixed(1)}ms`]}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          <Bar dataKey="avg" name="Avg" radius={[0, 4, 4, 0]}>
            {data.map((d, i) => <Cell key={i} fill={d.color} fillOpacity={0.7} />)}
          </Bar>
          <Bar dataKey="p95" name="P95" radius={[0, 4, 4, 0]}>
            {data.map((d, i) => <Cell key={i} fill={d.color} fillOpacity={0.3} />)}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  )
}
```

- [ ] **Step 2: Create StepComparisonChart**

Create `studio/src/components/charts/StepComparisonChart.tsx`:

```tsx
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend } from "recharts"
import { chartTheme, stepColors } from "../../lib/chart-theme"
import type { TimeSeriesBucket } from "../../lib/api"

export function StepComparisonChart({ data, stepNames }: { data: TimeSeriesBucket[]; stepNames: string[] }) {
  const chartData = data.map((b) => {
    const point: Record<string, any> = {
      time: new Date(b.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
    }
    for (const name of stepNames) {
      point[name] = b.step_metrics?.[name]?.avg_ms ?? 0
    }
    return point
  })

  return (
    <div className="bg-surface-1 border border-hairline rounded-xl p-5">
      <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle mb-4">
        Step latency over time
      </div>
      <ResponsiveContainer width="100%" height={240}>
        <LineChart data={chartData} margin={{ top: 0, right: 0, bottom: 0, left: 0 }}>
          <XAxis dataKey="time" tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} />
          <YAxis tick={{ fill: chartTheme.colors.axisText, fontSize: chartTheme.font.size }} axisLine={false} tickLine={false} width={50} unit="ms" />
          <Tooltip
            contentStyle={{ background: chartTheme.colors.tooltipBg, border: `1px solid ${chartTheme.colors.tooltipBorder}`, borderRadius: 8, fontSize: 12 }}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          {stepNames.map((name, i) => (
            <Line key={name} type="monotone" dataKey={name} stroke={stepColors[i % stepColors.length]} strokeWidth={1.5} dot={false} />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
```

- [ ] **Step 3: Add Metrics tab to CompositionDetail.tsx**

Modify `studio/src/pages/CompositionDetail.tsx`:

- Change tab type to include `"metrics"`: `useState<"metrics" | "graph" | "steps" | "route">("metrics")`
- Update tab bar order: `["metrics", "graph", "steps", "route"]` with display labels `["Metrics", "DAG", "Steps", "Route"]`
- Add the `MetricsTab` component that renders:
  - `<TimeRangeSelector>` at the top right
  - Four `<SparklineCard>` components for request count, error rate, avg latency, p95
  - `<RequestRateChart>` with composition-filtered data
  - `<LatencyChart>` with composition-filtered data
  - `<LatencyHeatmap>` with composition-filtered data
  - `<StepBreakdownChart>` with step metrics
  - `<StepComparisonChart>` with composition time-series and step names
  - Recent traces section: last 20 requests filtered to this composition, with `<TimelineWaterfall>`

Use `usePoll(api.timeseries(range, "1m", comp.name), 30000)` and `usePoll(api.stepMetrics(comp.name, range), 30000)` for data.

- [ ] **Step 4: Run build**

Run: `cd studio && npm run build`
Expected: Clean build.

- [ ] **Step 5: Commit**

```bash
git add studio/src/components/charts/StepBreakdownChart.tsx studio/src/components/charts/StepComparisonChart.tsx studio/src/pages/CompositionDetail.tsx
git commit -m "feat(studio-obs): add per-composition metrics page with step breakdown"
```

---

### Task 10: DAG view — fix inferred edges, richer nodes, execution overlay

**Files:**
- Modify: `studio/src/pages/CompositionDetail.tsx` (DAGView component + EnhancedStepNode + overlay)
- Modify: `studio/src/lib/api.ts` (add `inferred_deps` to `StepInfo` type)

**Interfaces:**
- Consumes: `CompositionInfo.steps[].inferred_deps` (from Task 2), `api.requests()` for overlay, `StepDetailPanel` (from Task 8)
- Produces: fixed DAG with inferred edges, richer nodes, execution overlay toggle

- [ ] **Step 1: Update StepInfo type in api.ts**

In `studio/src/lib/api.ts`, add `inferred_deps` to `StepInfo`:

```ts
export interface StepInfo {
  name: string
  upstream: string
  method: string
  optional: boolean
  timeout_ms: number
  depends_on: string[]
  inferred_deps: string[]
}
```

- [ ] **Step 2: Rewrite DAGView with inferred edges**

In the `DAGView` component inside `CompositionDetail.tsx`, update the edge generation to include inferred deps:

```tsx
const edges: Edge[] = [
  ...comp.steps.flatMap((step) =>
    (step.depends_on || []).map((dep) => ({
      id: `explicit-${dep}-${step.name}`,
      source: dep,
      target: step.name,
      style: { stroke: "rgba(178,182,189,0.4)", strokeWidth: 2 },
      animated: false,
    }))
  ),
  ...comp.steps.flatMap((step) =>
    (step.inferred_deps || []).map((dep) => ({
      id: `inferred-${dep}-${step.name}`,
      source: dep,
      target: step.name,
      style: { stroke: "rgba(178,182,189,0.25)", strokeWidth: 1.5, strokeDasharray: "6 3" },
      animated: false,
      label: "inferred",
      labelStyle: { fontSize: 9, fill: "rgba(178,182,189,0.4)" },
    }))
  ),
]
```

- [ ] **Step 3: Create EnhancedStepNode**

Replace the `StepNode` component with a richer version:

```tsx
const methodColors: Record<string, string> = {
  GET: "bg-blue-500/15 text-blue-400",
  POST: "bg-green-500/15 text-green-400",
  PUT: "bg-amber-500/15 text-amber-400",
  DELETE: "bg-red-500/15 text-red-400",
}

function EnhancedStepNode({ data }: { data: StepNodeData }) {
  return (
    <div className={`px-3 py-2.5 min-w-[180px] transition-all ${data.overlayBorder || ""}`}>
      <div className="flex items-center gap-2">
        <span className="font-medium text-[13px]">{data.label}</span>
        {data.optional && (
          <span className="text-[9px] font-semibold tracking-[0.5px] uppercase px-1.5 py-0.5 rounded bg-warning/15 text-warning">opt</span>
        )}
      </div>
      <div className="flex items-center gap-2 mt-1.5">
        <span className={`text-[10px] font-bold px-1.5 py-0.5 rounded ${methodColors[data.method] || "bg-ink-subtle/15 text-ink-muted"}`}>
          {data.method}
        </span>
        <span className="text-[11px] text-ink-muted truncate">{data.upstream}</span>
      </div>
      {data.durationLabel && (
        <div className="mt-1.5 text-[11px] font-mono tabular-nums text-ink-subtle">
          {data.durationLabel}
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 4: Add execution overlay state**

Add overlay toggle and request selection to `DAGView`:

```tsx
function DAGView({ comp }: { comp: CompositionInfo }) {
  const [showOverlay, setShowOverlay] = useState(false)
  const { data: requests } = usePoll(() => api.requests(20), 3000)

  const latestRequest = requests?.find((r) => r.composition === comp.name)

  const traceSteps = showOverlay && latestRequest ? latestRequest.steps : []

  // Build nodes with overlay data
  const nodes: Node[] = comp.steps.map((step) => {
    const wave = comp.waves.findIndex((w) => w.includes(step.name))
    const inWave = comp.waves[wave]?.indexOf(step.name) ?? 0
    const traceStep = traceSteps.find((s) => s.name === step.name)

    let overlayBorder = ""
    let durationLabel = ""
    if (traceStep) {
      if (traceStep.status === "success") overlayBorder = "ring-2 ring-success/50"
      else if (traceStep.status === "failed") overlayBorder = "ring-2 ring-error/50"
      else overlayBorder = "ring-1 ring-ink-subtle/20 opacity-50"
      durationLabel = `${traceStep.duration_ms.toFixed(1)}ms`
    }

    return {
      id: step.name,
      position: { x: wave * 280, y: inWave * 130 },
      data: { label: step.name, upstream: step.upstream, method: step.method, optional: step.optional, overlayBorder, durationLabel },
      type: "enhanced",
      style: {
        background: "#161618",
        border: "1px solid rgba(178,182,189,0.12)",
        borderRadius: "12px",
        padding: "0",
        color: "#fff",
        fontSize: "13px",
      },
    }
  })

  // ... edges as in Step 2 ...

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <div className="text-[12px] font-semibold tracking-[0.6px] uppercase text-ink-subtle">
          Execution DAG
        </div>
        {latestRequest && (
          <button
            onClick={() => setShowOverlay(!showOverlay)}
            className={`text-[12px] px-3 py-1 rounded-lg border transition-colors ${
              showOverlay ? "bg-surface-2 border-hairline text-ink" : "border-hairline-soft text-ink-muted hover:text-ink"
            }`}
          >
            {showOverlay ? "Hide trace" : "Show latest trace"}
          </button>
        )}
      </div>
      <div className="bg-surface-1 border border-hairline rounded-xl overflow-hidden" style={{ height: 450 }}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          fitView
          proOptions={{ hideAttribution: true }}
          nodeTypes={{ enhanced: EnhancedStepNode }}
        >
          <Background color="rgba(178,182,189,0.05)" />
          <Controls style={{ background: "#222225", border: "1px solid rgba(178,182,189,0.12)", borderRadius: "8px" }} />
        </ReactFlow>
      </div>
    </div>
  )
}
```

Register the `enhanced` node type properly with `nodeTypes={{ enhanced: EnhancedStepNode }}`.

- [ ] **Step 5: Run build and tests**

Run: `cd studio && npm run test && npm run build`
Expected: All pass, clean build.

- [ ] **Step 6: Commit**

```bash
git add studio/src/lib/api.ts studio/src/pages/CompositionDetail.tsx
git commit -m "feat(studio-obs): fix DAG inferred edges, richer nodes, execution overlay"
```

---

### Task 11: Integration verification

**Files:**
- No new files — this is a verification task

**Interfaces:**
- Consumes: everything from Tasks 1-10

- [ ] **Step 1: Run full Go test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: All pass.

- [ ] **Step 2: Run full frontend build + tests**

Run: `cd studio && npm run test && npm run build`
Expected: All pass, clean build.

- [ ] **Step 3: Manual smoke test**

Start the full stack:
```bash
go run ./cmd/mockupstream &
./bin/restitch -config examples/quickstart/restitch.yaml &
./bin/restitch-studio &
```

Generate some traffic:
```bash
for i in $(seq 1 20); do curl -s "http://localhost:8080/api/user-posts?id=$i" > /dev/null; done
```

Open `http://localhost:3080` and verify:
1. Dashboard: sparkline cards show trends, request rate chart has data, latency chart shows p50/p95/p99 lines, heatmap shows colored cells
2. Click a composition row → Composition detail → Metrics tab shows charts, Step breakdown shows per-step latency bars
3. DAG tab shows nodes with edges connecting them (inferred deps from `{{ steps.user.body.id }}` show as dashed lines)
4. Click "Show latest trace" — nodes get green borders with duration labels
5. Requests page: filters work, expanded row shows timeline waterfall with bars starting at actual offsets, click a step bar to see detail panel
6. Config page still works (no regression)
7. Builder still works (no regression)

- [ ] **Step 4: Commit final state**

If any manual fixes were needed, commit them:

```bash
git add -A
git commit -m "feat(studio-obs): integration fixes after smoke test"
```
