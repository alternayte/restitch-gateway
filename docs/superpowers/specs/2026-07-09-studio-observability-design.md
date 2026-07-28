# Studio Observability & Visualization Upgrade

**Date:** 2026-07-09
**Status:** Approved
**Scope:** Backend time-series store with configurable persistence + frontend charts, enhanced request drill-down, live DAG overlay, per-composition metrics page

## 1. Problem

The Studio UI is data-light. The dashboard shows four static stat cards and a table — no time-series charts, no latency distribution, no trend visualization. The request explorer has a basic CSS waterfall that approximates step timing by wave offset rather than actual start times. The DAG view shows disconnected nodes because inferred dependencies (from `{{ steps.x.body }}` expressions) aren't reflected as edges. There is no per-composition observability view.

Apollo Studio, by comparison, offers request rate charts, latency heatmaps, per-operation drill-down with trace waterfalls, and resolver-level timing breakdowns. Restitch Studio needs comparable depth to be a credible control plane.

## 2. Architecture

### 2.1 Backend: Time-Series Store

A new `internal/admin/timeseries.go` package provides time-bucketed metrics storage behind a `Storage` interface.

#### Storage Interface

```go
type Storage interface {
    RecordBucket(ctx context.Context, bucket Bucket) error
    QueryTimeSeries(ctx context.Context, from, to time.Time, resolution time.Duration, composition string) ([]Bucket, error)
    RecordRequest(ctx context.Context, record reqlog.Record) error
    QueryRequests(ctx context.Context, opts RequestQuery) ([]reqlog.Record, error)
    QueryStepMetrics(ctx context.Context, composition string, from, to time.Time) ([]StepAggregate, error)
    Compact(ctx context.Context, retention time.Duration) error
    Close() error
}
```

#### Data Structures

```go
type Bucket struct {
    Timestamp      time.Time              `json:"timestamp"`
    Composition    string                 `json:"composition"`    // "" for global
    Requests       int64                  `json:"requests"`
    Errors         int64                  `json:"errors"`
    Partials       int64                  `json:"partials"`
    LatencyP50     float64                `json:"latency_p50"`
    LatencyP95     float64                `json:"latency_p95"`
    LatencyP99     float64                `json:"latency_p99"`
    LatencyBuckets []int64                `json:"latency_buckets"` // counts per predefined range
    StepMetrics    map[string]*StepBucket `json:"step_metrics"`
}

type StepBucket struct {
    Requests int64   `json:"requests"`
    Errors   int64   `json:"errors"`
    AvgMS    float64 `json:"avg_ms"`
    P95MS    float64 `json:"p95_ms"`
}

type StepAggregate struct {
    Name      string  `json:"name"`
    Upstream  string  `json:"upstream"`
    Requests  int64   `json:"requests"`
    Errors    int64   `json:"errors"`
    AvgMS     float64 `json:"avg_ms"`
    P95MS     float64 `json:"p95_ms"`
    P99MS     float64 `json:"p99_ms"`
}

type RequestQuery struct {
    Limit       int
    Composition string
    StatusMin   int
    StatusMax   int
    MinDuration float64
    Partial     *bool
}
```

#### Latency Histogram Buckets

Predefined ranges for the heatmap: `[0-10ms, 10-50ms, 50-100ms, 100-250ms, 250-500ms, 500ms-1s, 1s-5s, 5s+]` — 8 buckets per time window.

#### Three Storage Backends

**`memory`** (default) — In-memory sliding window. 1-minute resolution, 24-hour retention (1440 buckets per composition). Data lost on restart. Zero configuration.

**`sqlite`** — Local SQLite file via `github.com/tursodatabase/go-libsql`. Configurable retention (default 7 days). Schema:

```sql
CREATE TABLE timeseries_buckets (
    timestamp   INTEGER NOT NULL,
    composition TEXT NOT NULL DEFAULT '',
    data        TEXT NOT NULL,  -- JSON-encoded Bucket
    PRIMARY KEY (timestamp, composition)
);

CREATE TABLE request_log (
    id          TEXT PRIMARY KEY,
    timestamp   INTEGER NOT NULL,
    composition TEXT NOT NULL,
    data        TEXT NOT NULL,  -- JSON-encoded reqlog.Record
    created_at  INTEGER NOT NULL
);

CREATE INDEX idx_requests_ts ON request_log(timestamp DESC);
CREATE INDEX idx_requests_comp ON request_log(composition, timestamp DESC);
```

**`turso`** — Same schema, same `go-libsql` driver, but connects to a Turso/libSQL remote database URL. Configuration:

```yaml
admin:
  storage:
    type: "turso"           # "memory" | "sqlite" | "turso"
    url: "libsql://your-db.turso.io"
    auth_token: "${TURSO_AUTH_TOKEN}"
    retention: "30d"
```

For `sqlite` mode, `url` is a local file path (e.g. `file:./restitch.db`). The `go-libsql` driver handles both transparently.

#### Bucket Aggregation

A background goroutine runs every 60 seconds:
1. Collects accumulated request data from a thread-safe accumulator
2. Computes percentiles from the collected latency samples
3. Writes a `Bucket` to storage (one global + one per active composition)
4. Runs compaction to delete data older than retention

The accumulator uses a mutex-protected struct with append-only slices per composition — the same pattern as the current `Stats` but with more granular data.

#### Enriched Request Records

Extend `reqlog.StepRecord` with execution detail:

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

`start_offset_ms` is computed in the executor as `time.Since(requestStart).Milliseconds()` at the point each step begins execution. This enables true timeline waterfall rendering.

### 2.2 New Admin API Endpoints

All new endpoints sit under `/admin/api/` and go through the existing `requireKey` middleware.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/stats/timeseries` | Time-series buckets. Query params: `range` (1h\|6h\|24h\|7d), `resolution` (1m\|5m\|1h), `composition` (optional filter) |
| GET | `/admin/api/stats/steps` | Per-step aggregated metrics. Query params: `composition` (required), `range` (1h\|6h\|24h) |
| GET | `/admin/api/requests/{id}` | Single request record with full step detail |

The existing `/admin/api/requests` endpoint gains query param filters: `composition`, `status_min`, `status_max`, `min_duration_ms`, `partial` (bool).

### 2.3 Inferred Dependencies in API Response

The `CompositionInfo` response for each step's `depends_on` currently only includes explicit dependencies. Change the admin server's `Compositions()` builder to also include inferred dependencies. Add a separate field to distinguish them:

```go
type StepInfo struct {
    Name        string   `json:"name"`
    Upstream    string   `json:"upstream"`
    Method      string   `json:"method"`
    Optional    bool     `json:"optional"`
    TimeoutMS   int64    `json:"timeout_ms"`
    DependsOn   []string `json:"depends_on"`
    InferredDeps []string `json:"inferred_deps"` // NEW: from expression analysis
}
```

The executor's `BuildDAG` already computes inferred deps — expose them through the admin API.

## 3. Frontend

### 3.1 New Dependency: Recharts

Add `recharts` to `studio/package.json`. Recharts is React-native, composable, supports area/line/bar/scatter charts with responsive containers, tooltips, and legends. ~200KB gzipped.

### 3.2 New API Client Methods

```ts
// In src/lib/api.ts
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

export const api = {
  // ... existing methods ...
  timeseries: (range: string, resolution: string, composition?: string) =>
    get<TimeSeriesBucket[]>(`/api/stats/timeseries?range=${range}&resolution=${resolution}${composition ? `&composition=${composition}` : ''}`),
  stepMetrics: (composition: string, range: string) =>
    get<StepAggregate[]>(`/api/stats/steps?composition=${composition}&range=${range}`),
  request: (id: string) =>
    get<RequestRecord>(`/api/requests/${id}`),
}
```

### 3.3 Dashboard Page Overhaul

**File:** `src/pages/Dashboard.tsx`

The dashboard becomes a chart-rich observability view:

#### Time Range Selector
A segmented button group at the top right: `1h | 6h | 24h`. Controls all charts. Default: `1h`.

#### Stat Cards Row (enhanced)
Four cards, same data as current but each contains a **sparkline** — a tiny `<AreaChart>` from Recharts (no axes, no labels, just the shape) showing the trend over the selected time range. Colors:
- Total requests: primary/accent
- Error rate: warning when > 0, muted otherwise
- Partial responses: amber
- Compositions: static count (no sparkline needed)

#### Request Rate Chart
Full-width `<AreaChart>`, stacked areas:
- Green area: successful requests/min
- Amber area: partial responses/min
- Red area: errors/min

Tooltip shows exact values. Legend below chart. `<ResponsiveContainer>` for fluid width.

#### Latency Percentiles Chart
Full-width `<LineChart>` with three lines:
- p50: muted/subtle line
- p95: accent color (primary)
- p99: warning color

Y-axis in ms, X-axis timestamps. Tooltip shows all three values.

#### Latency Heatmap
Custom component using SVG rectangles within a Recharts-style container. X-axis: time. Y-axis: latency ranges (0-10ms through 5s+). Cell color intensity proportional to request count in that bucket. Sequential color scale from surface color (zero) to accent (high traffic). Hover shows exact count.

Implementation: since Recharts lacks native heatmap support, build a `<LatencyHeatmap>` component that takes `TimeSeriesBucket[]` data and renders an SVG grid. Style it to match the Recharts aesthetic (same fonts, same tooltip pattern).

#### Per-Composition Table (enhanced)
Keep the existing table but add:
- A mini sparkline column showing request rate trend
- Click row → navigates to `/compositions/:name` (metrics tab)

### 3.4 Composition Metrics Page

**File:** `src/pages/CompositionDetail.tsx` (extend with new Metrics tab)

Tab order changes to: `Metrics | DAG | Steps | Route` (Metrics becomes default).

#### Metrics Tab Content

**Overview Cards** — Four cards scoped to this composition:
- Request count + sparkline
- Error rate + sparkline
- Avg latency + sparkline
- P95 latency + sparkline

**Charts** — Same types as dashboard but filtered:
- Request rate area chart
- Latency percentile lines
- Latency heatmap

**Step Latency Breakdown** — The signature view:
- `<BarChart>` with horizontal stacked bars. Each bar represents a recent time window. Segments within each bar represent steps, colored by upstream. Width/length proportional to latency. This immediately shows which step dominates total time.
- Alternative view: a single horizontal stacked bar showing average step distribution (simpler, always available even without time-series data).

**Step Table** — Sortable table:
| Name | Upstream | Avg Latency | P95 Latency | Error Rate | Requests |
Each row expandable to show a mini trend line for that step's latency.

**Step Comparison Chart** — `<LineChart>` with one line per step, showing individual step latencies over time. Toggle steps on/off via legend clicks.

**Recent Traces** — Last 20 requests for this composition, same expandable waterfall format as the Requests page. Link to "View all" opens Requests page pre-filtered.

### 3.5 Enhanced Request Explorer

**File:** `src/pages/Requests.tsx`

#### Filters Bar
Above the table, a filter row:
- Composition dropdown (all / specific)
- Status filter (All / 2xx / 4xx / 5xx)
- Duration filter (All / >100ms / >500ms / >1s)
- Partial toggle

#### True Timeline Waterfall
Replace the current CSS-approximated waterfall with a proper timeline:
- X-axis represents time from request start (0ms) to request end
- Each step is a horizontal bar starting at `start_offset_ms` and spanning `duration_ms`
- Bars colored by status (green/red/grey)
- Parallel steps visually overlap in the Y axis, showing true concurrency
- A thin vertical line at `x=0` marks request start
- Gateway overhead (time not spent in steps) shown as a subtle background region

#### Step Detail Panel
Clicking a step bar in the waterfall opens a slide-out or inline detail panel:
- Step name + upstream name
- Actual URL called
- HTTP method + status code
- Duration
- Response body size
- Cached / coalesced badges
- Retry count
- Error message (if failed)

#### Request Summary Header
When a request row is expanded, show above the waterfall:
- Total duration
- Gateway overhead (total - sum of parallel step time)
- Step count
- Mini donut/pie showing time distribution across steps

### 3.6 DAG with Live Execution Overlay

**File:** `src/pages/CompositionDetail.tsx` (DAGView component)

#### Fix: Inferred Dependency Edges
The DAG now draws edges for both explicit and inferred dependencies:
- **Solid lines** for explicit `depends_on`
- **Dashed lines** for inferred deps (from `inferred_deps` API field)
- Edge labels optional (could show "inferred" on hover)

This fixes the "two disconnected squares" issue with the quickstart YAML.

#### Richer Step Nodes
Replace the minimal dark boxes with richer cards:
- Step name (bold, top)
- Upstream name + URL (subtitle)
- Method badge (colored pill: GET=blue, POST=green, PUT=amber, DELETE=red)
- Optional badge (if applicable)
- Timeout value
- Wave number indicator (subtle)

#### Execution Overlay Mode
A toggle on the DAG view: "Show latest trace" or select from recent requests dropdown.

When active:
- **Node borders change color:**
  - Green border + subtle green glow: success
  - Red border + subtle red glow: failed
  - Grey border, muted content: skipped
- **Timing labels appear:** Each node shows `duration_ms` prominently and `start_offset_ms` as a subtle label
- **Edge animation:** Edges animate sequentially by wave to show execution flow. CSS `stroke-dasharray` animation on the SVG paths.
- **Click to expand:** Clicking a node in overlay mode shows the step detail panel (same as waterfall click)

Implementation: the overlay state is managed by passing the selected `RequestRecord` to the DAG component. Node styling is computed from the step records matched by name.

## 4. Chart Theming

All Recharts components follow the Studio's existing design language:
- Background: `surface-1` (chart container backgrounds)
- Grid lines: `hairline-soft`
- Text: `ink-muted` for axis labels, `ink` for values
- Colors: accent palette for primary data, success/warning/error for status
- Tooltips: `surface-2` background, `ink` text, `hairline` border, 12px rounded
- Font: inherit from Tailwind (system font for labels, mono for values)

Dark mode is the primary theme (matching the existing Studio). Light mode support via `prefers-color-scheme` media query on the Recharts theme values.

## 5. Component Structure

New files under `studio/src/`:

```
src/
  components/
    charts/
      RequestRateChart.tsx     # Stacked area chart
      LatencyChart.tsx         # p50/p95/p99 line chart
      LatencyHeatmap.tsx       # Custom SVG heatmap
      SparklineCard.tsx        # Stat card with inline sparkline
      StepBreakdownChart.tsx   # Horizontal stacked bar
      StepComparisonChart.tsx  # Multi-line step latency
      TimeRangeSelector.tsx    # 1h/6h/24h segmented control
      StepDonut.tsx            # Mini time distribution donut
    waterfall/
      TimelineWaterfall.tsx    # True timeline waterfall
      StepDetailPanel.tsx      # Step drill-down panel
      RequestSummary.tsx       # Request header with stats
    dag/
      EnhancedStepNode.tsx     # Rich DAG node
      ExecutionOverlay.tsx     # Overlay state management
    filters/
      RequestFilters.tsx       # Filter bar component
  pages/
    Dashboard.tsx              # Overhauled with charts
    CompositionDetail.tsx      # Extended with Metrics tab
    Requests.tsx               # Enhanced with filters + timeline waterfall
```

## 6. Data Flow

```
Request hits gateway
    → executor records start_offset_ms per step
    → handler records enriched StepRecord to reqlog.Record
    → admin.Stats.Record() (existing, for backward compat)
    → admin.TimeSeriesStore.Accumulate() (new)
        → background goroutine flushes to Storage every 60s
    → admin.RingBuffer.Add() or Storage.RecordRequest() (if persistent)

Studio frontend
    → usePoll(api.timeseries, 30000) for charts
    → usePoll(api.stats, 5000) for stat cards (existing)
    → usePoll(api.requests, 3000) for request table (existing)
    → api.stepMetrics(name, range) on composition detail page
```

## 7. Dependencies

### Go (backend)
- `github.com/tursodatabase/go-libsql` — SQLite + Turso/libSQL driver

### npm (frontend)
- `recharts` — charting library

No other new dependencies.

## 8. Configuration

```yaml
# Memory mode (default — no persistence):
admin:
  storage:
    type: "memory"
    retention: "24h"        # sliding window, default 24h

# SQLite mode (local file persistence):
admin:
  storage:
    type: "sqlite"
    url: "file:./restitch.db"
    retention: "7d"

# Turso mode (remote libSQL database):
admin:
  storage:
    type: "turso"
    url: "libsql://your-db.turso.io"
    auth_token: "${TURSO_AUTH_TOKEN}"
    retention: "30d"
```

All three modes use the same flat `storage` config block — `type` selects the backend, `url` and `auth_token` are only read when relevant. When `type` is `memory` (or omitted), behavior is identical to current — no persistence, no new dependencies loaded. The `go-libsql` driver is initialized lazily (only when `type` is `sqlite` or `turso`) so the binary size impact is minimal for memory-only users.

## 9. Non-Goals

- Real-time streaming (WebSocket push) — polling at 3-30s intervals is sufficient
- Distributed tracing (OpenTelemetry spans) — out of scope, Prometheus + this covers it
- Alerting — that's Grafana/PagerDuty territory
- Historical data export — query the SQLite/Turso DB directly
- Custom chart building — fixed dashboard layout, not a BI tool
