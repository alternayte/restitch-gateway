# M23 — Upstream HTTP Client Optimization

**Date:** 2026-07-27
**Milestone:** M23 (PLAN.md §"M23 — Upstream HTTP Client Optimization (extends M5)")
**Status:** design approved, awaiting spec review

---

## Context

M23's three tasks are largely already implemented — M16's production-hardening
work landed the transport tuning ahead of schedule. An audit of the codebase
against PLAN.md found:

| PLAN task | Codebase reality |
|-----------|------------------|
| T23.1 — `MaxIdleConnsPerHost: 100`, TLS 1.2 minimum | **Done.** `internal/upstream/client.go:45-64` |
| T23.2 — `DrainAndClose` helper used on upstream response paths | **Partial.** Helper exists (`internal/upstream/drain.go`), used on the hot path (`internal/composition/step.go:125`); three other paths still use bare `Body.Close()` |
| T23.3 — per-upstream pool config in YAML | **Missing, and actively misleading** |

### The T23.3 defect

`docs/configuration.md:180-189` documents a five-field `upstreams.*.transport`
block with defaults. `PLAN.md:249-254` specifies the same block, annotated
"all optional (M5)". Neither is reachable:

- `internal/composition/config.go:33` — the `Upstream` struct has no
  `Transport` field, so the YAML key does not bind.
- `internal/composition/parser.go:150` — hardcodes
  `Transport: upstream.TransportConfig{}` for every upstream.
- `internal/composition/parser.go:22` — uses non-strict `yaml.Unmarshal`, so
  an unknown `transport:` key raises no error.

Net effect: a user who follows the published documentation gets their
transport tuning **silently ignored** with no warning. This is a latent M5 gap
that M23 inherits. The M5 ledger rows are not being revised — append-only is
absolute — but M23 is where the gap gets closed.

### Schema decision

PLAN.md T23.3 proposes `upstreams.*.pool: { max_idle: 100 }`. This phrasing is
drift from the `experimental/v1.1-v1.2` reference repo and contradicts PLAN.md's
own configuration schema at line 249. **We wire `transport:` with all five
documented fields** — a superset of what T23.3 asked for, consistent with both
PLAN.md and the docs. No `pool:` alias is added.

---

## Scope

### Pre-work: repository cleanup

Committed first, separately from M23 code.

1. Commit the M22 planning artifacts left untracked:
   `docs/superpowers/plans/2026-07-16-m22-dev-mode-orchestrator.md` and
   `docs/superpowers/specs/2026-07-16-m22-dev-mode-orchestrator-design.md`.
2. Commit `cmd/restitch-studio/dist/index.html`.
3. Reconcile `docs/plan-progress/evidence/INDEX.md`: the working tree adds two
   identical rows for `M22 / a0c9bd35`. INDEX is a chronological index, not an
   append-only ledger — collapse to one row.
4. Add `studio.db`, `studio.db-shm`, `studio.db-wal` to `.gitignore`. These are
   runtime artifacts sitting untracked in the repository root.
5. Commit `docs/plan-progress/evidence/2026-07-16-M22-6e233404.log` and record
   it — see below.

#### The unrecorded M22 FAIL run

`2026-07-16-M22-6e233404.log` is a real M22 gate run that left no rows in
`LEDGER.md` and no row in `INDEX.md`. Inspection shows the file is **truncated
at line 45, mid-T22.2, with no `RESULT M22:` line** — the run was interrupted
before `h_finish` executed, which is precisely why nothing was recorded. This is
an interrupted run, not a harness recording bug.

The log contains completed assertions for exactly two tasks:

- `T22.1` — 5 FAIL assertions (`writer.go`, `writer_test.go`, `PrefixWriter`,
  `WaitForHealth`, `NO_COLOR support`)
- `T22.2` — 4 FAIL assertions (`manager.go`, `manager_test.go`,
  `ProcessConfig`, `ProcessManager`)

Both failed because the milestone was not yet implemented at `6e233404`.

**Action:** commit the log, add one `INDEX.md` row, and append retroactive rows
to `LEDGER.md` for `T22.1` and `T22.2` only. No rows for `T22.3`, `T22.4`,
`M22.unit`, or `M22.gate`: those assertions never executed, and inventing rows
for them would be fabricating evidence.

`scripts/check-ledger.sh:139` resolves "last row per task wins" by **file
order**, not by date — it overwrites `LEDGER_STATUS[task]` as it reads
sequentially. Appending the FAIL rows alone would therefore flip T22.1 and T22.2
to FAIL and trip the `STALE-FAIL` check. The ledger header already prescribes
the remedy: *"A failure is recorded as FAIL and later superseded by a PASS
row."* Four rows are appended, in order:

```
| 2026-07-16 | T22.1 | M22 | FAIL | 6e233404 | evidence/2026-07-16-M22-6e233404.log#T22.1 | retroactive: run interrupted before h_finish wrote rows |
| 2026-07-16 | T22.2 | M22 | FAIL | 6e233404 | evidence/2026-07-16-M22-6e233404.log#T22.2 | retroactive: run interrupted before h_finish wrote rows |
| 2026-07-16 | T22.1 | M22 | PASS | a0c9bd35 | evidence/2026-07-16-M22-a0c9bd35.log#T22.1 | supersedes the retroactive 6e233404 FAIL above |
| 2026-07-16 | T22.2 | M22 | PASS | a0c9bd35 | evidence/2026-07-16-M22-a0c9bd35.log#T22.2 | supersedes the retroactive 6e233404 FAIL above |
```

The two PASS rows restate evidence already recorded earlier in the ledger and
already present in the `a0c9bd35` log. They are not new claims. Append-only is
preserved throughout; nothing is reordered or deleted.

### T23.0 — Gate script (first task, requires approval)

Per CLAUDE.md rule 2, `scripts/gates/m23.sh` replaces its placeholder before any
feature work, in a commit prefixed `gate:`.

PLAN.md's M23 verification gate is prose, not commands:

```
# Load test with k6: fan-out composition hitting 5 upstreams
# Before: observe connection churn in netstat, P95 latency > 200ms
# After: stable idle connections, P95 latency < 50ms
```

It is encoded as follows.

**k6 is hard-required.** `h_require_tool k6` failing is an `h_fail`, not an
`h_skip`. The harness's skip-if-absent convention (used for `docker`, `gh`,
`golangci-lint`) would let M23 go green with zero performance evidence, which
defeats the purpose. k6 v2.1.0 is installed on the development machine; M24's
T24.3 needs it regardless.

**Topology.** One `mockupstream` process started via the existing
`h_start_mockupstream` helper. The gateway config declares **five logical
upstreams** all pointing at that process. `upstream.Build` constructs a separate
`*http.Transport` per upstream, so five independent connection pools exist —
faithful to PLAN's "5 upstreams" fan-out without modifying `scripts/lib/`.

> `scripts/lib/harness.sh` is not edited. Per CLAUDE.md's hard rules, gate
> scripts and harness library code are off-limits except by explicit approval;
> `m23.sh` composes only existing helpers.

**Assertions.**

```
m23.pool          transport: YAML reaches the live *http.Transport, all 5 fields
m23.drain         no bare Body.Close() on upstream response paths
m23.unit          go test ./internal/upstream/... ./internal/composition/...
m23.k6.baseline   run with max_idle_conns_per_host: 2    → record P95, conns
m23.k6.tuned      run with max_idle_conns_per_host: 100  → record P95, conns
```

Hard thresholds:

| Assertion | Rationale |
|-----------|-----------|
| `tuned.conns_accepted < baseline.conns_accepted` | The non-flaky signal. Directly measures connection reuse. |
| `tuned.p95 < 50ms` | PLAN's stated post-optimization headline. |
| `tuned.error_rate < 1%` | Guards against "fast because it 500s". |

`baseline.p95` is **recorded without a threshold**. PLAN's "before: P95 > 200ms"
was measured against real network upstreams; on loopback, connection setup is
cheap enough that a baseline run may not exceed 200ms. Asserting it would make
the gate flaky or unpassable. The connection-count delta carries the proof.

The A/B structure means the gate demonstrates T23.3's new configuration surface
by using it — `max_idle_conns_per_host: 2` is only expressible once the
`transport:` block is wired.

**Load profile.** 50 VUs, 20s per run, matching the shape M24's T24.3 will
reuse. Two runs plus build and startup keeps the gate under roughly 90 seconds.

### T23.0b — mockupstream connection instrumentation

The gate needs ground truth on TCP connections accepted. `cmd/mockupstream` is
currently a bare `http.ListenAndServe` with no instrumentation.

`cmd/mockupstream/main.go` gains:

```go
srv := &http.Server{
    Addr:    addr,
    Handler: handler,
    ConnState: func(_ net.Conn, s http.ConnState) {
        if s == http.StateNew {
            conns.Add(1)
        }
    },
}
```

plus two routes wrapping `mockupstream.Handler()`:

- `GET /__stats` → `{"conns_accepted":N,"requests":M}`
- `POST /__stats/reset` → zeroes both counters (called between the A/B runs)

Counters are `atomic.Int64`. `internal/mockupstream/handler.go` is unchanged —
the counter and the two routes live in the server wrapper in `main.go`, since
`ConnState` is a server-level hook and the request counter belongs beside it.

This is a development and test binary. Nothing shipped changes. The alternatives
were parsing `netstat`/`lsof` from bash (brittle on macOS, sampled rather than
cumulative) or adding a gateway-side pool metric (Go's `http.Transport` exposes
no pool introspection; it would need a wrapping dialer — more invasive than
instrumenting a test binary).

### T23.3 — Wire the `transport:` block

`internal/composition/config.go` — add to the `Upstream` struct:

```go
Transport *TransportConfig `yaml:"transport"`
```

and define the mirror type in the same file:

```go
type TransportConfig struct {
    DialTimeout           time.Duration `yaml:"dial_timeout"`
    TLSHandshakeTimeout   time.Duration `yaml:"tls_handshake_timeout"`
    ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"`
    MaxIdleConnsPerHost   int           `yaml:"max_idle_conns_per_host"`
    InsecureSkipVerify    bool          `yaml:"insecure_skip_verify"`
}
```

A config-layer type mirroring `upstream.TransportConfig` follows the file's
existing pattern — `RetryConfig` and `BreakerConfig` are already declared this
way and translated in the parser, keeping `internal/composition` free of a
structural dependency on `internal/upstream`'s wire shape.

`internal/composition/parser.go:150` — replace the hardcoded empty struct with a
translation of `up.Transport` when non-nil, empty otherwise.

The `time.Duration` fields decode from strings such as `5s` without a custom
unmarshaller — verified against `gopkg.in/yaml.v3 v3.0.1`, and consistent with
the existing `Upstream.Timeout` field at `config.go:36`.

**Backwards compatibility.** Zero values continue to fall through to
`BuildTransport`'s defaults (`client.go:35-48`), so omitting the block, or
omitting individual fields within it, behaves exactly as today.

`insecure_skip_verify: true` becomes reachable for the first time. The startup
warning at `client.go:86` already exists but has never been triggerable from
YAML; a test now covers it.

### T23.2 — Complete `DrainAndClose` coverage

Replace bare `Body.Close()` with `upstreampkg.DrainAndClose(resp.Body)` at:

- `internal/upstream/health.go:127`
- `internal/hotreload/client.go:61`
- `internal/devmode/writer.go:79`

`internal/upstream/retry.go:120` already drains via the local `drainBody` helper
and is left alone.

A `h_grep_repo` assertion in the gate prevents regression.

### T23.1 — Verify and record

No code change. The gate's `m23.pool` assertion and the existing tests at
`internal/upstream/client_test.go:14-42` provide the evidence for the ledger row.

---

## Testing

**Unit.** `internal/composition` gains parser tests: a `transport:` block with
all five fields reaches `upstream.TransportConfig`; a partial block leaves
unspecified fields at zero; an omitted block yields the empty struct; a config
with `insecure_skip_verify: true` emits the startup warning.

**Integration.** The gate's `m23.pool` assertion drives a real gateway from
generated YAML and confirms the values reach the live transport, closing the
loop that unit tests alone cannot.

**Performance.** The k6 A/B run, per T23.0 above.

---

## Task order

| ID | Task | Commit prefix |
|----|------|---------------|
| — | Pre-work cleanup + retroactive M22 ledger rows | `docs:` / `chore:` |
| T23.0 | `scripts/gates/m23.sh` — real gate, user-approved | `gate:` |
| T23.0b | mockupstream `ConnState` + `/__stats` | `test:` |
| T23.3 | Wire `upstreams.*.transport` YAML block | `feat(M23):` |
| T23.2 | `DrainAndClose` on remaining response paths | `feat(M23):` |
| T23.1 | Verify pool + TLS settings, record evidence | `docs:` |
| M23.gate | `scripts/verify.sh M23` → `RESULT M23: PASS` | `docs:` |

Each task runs its Accept command and appends a `LEDGER.md` row with real output,
per CLAUDE.md rules 3 and 4.

---

## Out of scope

- Revising M5's ledger rows. Append-only is absolute; the gap is recorded here
  and closed by M23, not retconned.
- A `pool: { max_idle }` alias. One way to express a setting.
- Strict YAML decoding (`KnownFields`). Rejecting unknown keys repo-wide would
  catch this whole class of defect, but it is a breaking change to config
  loading across four call sites and belongs in its own milestone.
- Gateway-side connection pool metrics. Noted as a candidate for M24.
