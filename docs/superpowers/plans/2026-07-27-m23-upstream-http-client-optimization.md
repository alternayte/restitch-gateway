# M23 — Upstream HTTP Client Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the documented `upstreams.*.transport` YAML block actually configure the upstream HTTP transport, complete `DrainAndClose` coverage, and prove connection pooling works with a k6 A/B load test.

**Architecture:** `internal/composition` gains a config-layer `TransportConfig` mirror type (the pattern `RetryConfig`/`BreakerConfig` already use) that the parser translates into `upstream.TransportConfig`. Zero values keep falling through to `BuildTransport`'s existing defaults, so the change is backwards-compatible. The M23 gate starts one instrumented `mockupstream`, fronts it with a five-upstream fan-out composition, and runs k6 twice — once with `max_idle_conns_per_host: 2`, once with `100` — asserting the tuned run accepts strictly fewer TCP connections.

**Tech Stack:** Go 1.25.6, `gopkg.in/yaml.v3 v3.0.1`, k6 v2.1.0, bash gate harness (`scripts/lib/harness.sh`).

**Spec:** `docs/superpowers/specs/2026-07-27-m23-upstream-http-client-optimization-design.md`

## Global Constraints

- Go module: `github.com/restitch/restitch-gateway`, Go 1.25.6.
- **NEVER** edit `scripts/verify.sh`, `scripts/check-ledger.sh`, `scripts/lib/`, or any gate script other than `scripts/gates/m23.sh`. `m23.sh` composes only existing harness helpers.
- **NEVER** edit or delete existing `LEDGER.md` rows. Append only.
- After every task: run its Accept command, paste real output into the commit message, and append one `LEDGER.md` row.
- Commit prefixes: `feat(M23):`, `test:`, `docs:`, `chore:`. The gate script commit **must** start with `gate:`.
- PLAN.md's M23 section has no `Accept:` lines (verified — the milestone is a bare task table). Accept commands below are derived from the approved spec and encoded in `scripts/gates/m23.sh`.
- k6 is hard-required by the gate. k6 v2.1.0 is installed at `/opt/homebrew/bin/k6`.

---

## File Structure

**Create:**
- `scripts/gates/m23.sh` — replaces the placeholder; the real M23 gate.
- `tests/loadtest/m23_fanout.js` — k6 scenario driving the fan-out composition.
- `internal/composition/transport_test.go` — parser tests for the `transport:` block.

**Modify:**
- `internal/composition/config.go:33-41` — add `Transport *TransportConfig` to `Upstream`; add the `TransportConfig` type.
- `internal/composition/parser.go:150` — translate `up.Transport` instead of hardcoding an empty struct.
- `cmd/mockupstream/main.go` — `ConnState` connection counter + `/__stats` routes.
- `internal/upstream/health.go:127` — `DrainAndClose`.
- `internal/hotreload/client.go:61` — `DrainAndClose`.
- `internal/devmode/writer.go:79` — `DrainAndClose`.
- `.gitignore` — ignore `studio.db*`.
- `docs/plan-progress/LEDGER.md`, `docs/plan-progress/evidence/INDEX.md` — records.

---

## Task 0: Repository cleanup and M22 record reconciliation

No production code. Clears the working tree so M23 commits are clean, and records an M22 gate run that was interrupted before `h_finish` could write its rows.

**Files:**
- Modify: `.gitignore`
- Modify: `docs/plan-progress/evidence/INDEX.md`
- Modify: `docs/plan-progress/LEDGER.md`
- Commit (untracked): `docs/superpowers/plans/2026-07-16-m22-dev-mode-orchestrator.md`, `docs/superpowers/specs/2026-07-16-m22-dev-mode-orchestrator-design.md`, `docs/plan-progress/evidence/2026-07-16-M22-6e233404.log`
- Commit (modified): `cmd/restitch-studio/dist/index.html`

**Interfaces:**
- Consumes: nothing.
- Produces: a clean `git status` for Task 1.

- [ ] **Step 1: Confirm the interrupted-run facts before recording anything**

Run:

```bash
grep -c "^RESULT" docs/plan-progress/evidence/2026-07-16-M22-6e233404.log
grep -n "^##\[" docs/plan-progress/evidence/2026-07-16-M22-6e233404.log
grep -c "6e233404" docs/plan-progress/LEDGER.md
```

Expected: `0` (no RESULT line — the run was interrupted), then exactly two task
headers `##[T22.1]` and `##[T22.2]`, then `0` (no existing rows).

If any of these differ, **STOP and ask the user** — the retroactive rows below
are only correct for an interrupted run containing exactly T22.1 and T22.2.

- [ ] **Step 2: Ignore the runtime database**

Append to `.gitignore`:

```gitignore

# Studio runtime database (created by `restitch-studio` when run from the repo root)
studio.db
studio.db-shm
studio.db-wal
```

- [ ] **Step 3: Verify the database files are now ignored**

Run: `git status --short | grep studio.db || echo "IGNORED"`
Expected: `IGNORED`

- [ ] **Step 4: Collapse the duplicated INDEX row and add the interrupted run**

`docs/plan-progress/evidence/INDEX.md` currently gains two identical rows in the
working tree. INDEX is a chronological index, not an append-only ledger, so the
duplicate is an error, not a record.

Replace the two working-tree lines:

```markdown
| 2026-07-16 | M22 | a0c9bd35 | PASS | 2026-07-16-M22-a0c9bd35.log |
| 2026-07-16 | M22 | a0c9bd35 | PASS | 2026-07-16-M22-a0c9bd35.log |
```

with:

```markdown
| 2026-07-16 | M22 | 6e233404 | FAIL | 2026-07-16-M22-6e233404.log |
| 2026-07-16 | M22 | a0c9bd35 | PASS | 2026-07-16-M22-a0c9bd35.log |
```

- [ ] **Step 5: Append the retroactive FAIL rows, then supersede them**

`scripts/check-ledger.sh:139` resolves "last row per task wins" by **file
order**, not by date — it overwrites `LEDGER_STATUS[task]` as it reads
sequentially. Appending only the FAIL rows would therefore make T22.1 and T22.2
resolve to FAIL, land them in `STALE-FAIL`, and break the check.

The ledger header already prescribes the correct pattern: *"A failure is
recorded as FAIL and later superseded by a PASS row."* So append **four** rows,
in this exact order:

```markdown
| 2026-07-16 | T22.1 | M22 | FAIL | 6e233404 | evidence/2026-07-16-M22-6e233404.log#T22.1 | retroactive: run interrupted before h_finish wrote rows |
| 2026-07-16 | T22.2 | M22 | FAIL | 6e233404 | evidence/2026-07-16-M22-6e233404.log#T22.2 | retroactive: run interrupted before h_finish wrote rows |
| 2026-07-16 | T22.1 | M22 | PASS | a0c9bd35 | evidence/2026-07-16-M22-a0c9bd35.log#T22.1 | supersedes the retroactive 6e233404 FAIL above |
| 2026-07-16 | T22.2 | M22 | PASS | a0c9bd35 | evidence/2026-07-16-M22-a0c9bd35.log#T22.2 | supersedes the retroactive 6e233404 FAIL above |
```

The two PASS rows restate evidence already recorded earlier in the ledger and
already present in `2026-07-16-M22-a0c9bd35.log` — they are not new claims.

**Do NOT add rows for `T22.3`, `T22.4`, `M22.unit`, or `M22.gate`.** Those
assertions never executed in the interrupted run. Rows for them would be
fabricated evidence, which CLAUDE.md forbids.

- [ ] **Step 6: Verify the ledger still resolves clean**

Run: `./scripts/check-ledger.sh; echo "EXIT=$?"`
Expected: `COVERAGE: 96/96 green` and `EXIT=0`, with no `STALE-FAIL` section.

If `STALE-FAIL` lists T22.1 or T22.2, the superseding PASS rows are missing or
out of order. Fix by appending them — **never** by reordering or deleting rows.

- [ ] **Step 7: Commit**

```bash
git add .gitignore \
  docs/plan-progress/LEDGER.md \
  docs/plan-progress/evidence/INDEX.md \
  docs/plan-progress/evidence/2026-07-16-M22-6e233404.log \
  docs/superpowers/plans/2026-07-16-m22-dev-mode-orchestrator.md \
  docs/superpowers/specs/2026-07-16-m22-dev-mode-orchestrator-design.md \
  cmd/restitch-studio/dist/index.html
git commit -m "chore: commit M22 artifacts, ignore studio.db, record interrupted M22 run

The 6e233404 M22 gate run was interrupted mid-T22.2 (log truncated, no
RESULT line), so h_finish never appended its rows. Recording the two
tasks that actually produced assertions; T22.3/T22.4/M22.unit/M22.gate
never ran and get no rows.

Also collapses a duplicate INDEX row for a0c9bd35 and ignores the
studio.db* runtime artifacts."
```

---

## Task 1: T23.0 — Real M23 gate script (REQUIRES USER APPROVAL BEFORE COMMIT)

Per CLAUDE.md rule 2, the gate replaces its placeholder **before** any feature
work. It is expected to FAIL when first run — T23.3's config wiring and
T23.0b's instrumentation do not exist yet. That is the intended state.

**Files:**
- Modify: `scripts/gates/m23.sh` (replaces placeholder entirely)
- Create: `tests/loadtest/m23_fanout.js`

**Interfaces:**
- Consumes: harness helpers `h_init`, `h_task`, `h_run`, `h_pass`, `h_fail`, `h_log`, `h_evidence`, `h_grep_repo`, `h_require_tool`, `h_build`, `h_start_mockupstream`, `h_wait_for_port`, `h_finish`, and the `GW_PORT`/`MOCK_PORT`/`H_TMP`/`REPO_ROOT`/`H_PIDS` variables they set.
- Produces: gate task IDs `T23.1`, `T23.2`, `T23.3`, `M23.unit`, `M23.gate` — these become the `LEDGER.md` row names `h_finish` writes.

- [ ] **Step 1: Write the k6 fan-out scenario**

Create `tests/loadtest/m23_fanout.js`:

```javascript
// M23 fan-out load scenario.
//
// Drives the /fanout composition, which calls five upstreams per request.
// Used twice by scripts/gates/m23.sh: once against a gateway configured with
// max_idle_conns_per_host: 2 (Go's old default) and once with 100, to prove
// connection pooling reduces TCP connection churn.
//
// GW_URL is injected by the gate via the environment.
import http from 'k6/http';
import { check } from 'k6';

export const options = {
  vus: 50,
  duration: '20s',
  // Thresholds are asserted by the gate from the exported summary, not here,
  // so that the baseline run can complete without a non-zero k6 exit code.
  thresholds: {},
};

const GW_URL = __ENV.GW_URL;

export default function () {
  const res = http.get(`${GW_URL}/fanout`);
  check(res, { 'status is 200': (r) => r.status === 200 });
}
```

- [ ] **Step 2: Write the gate script**

Replace the entire contents of `scripts/gates/m23.sh`:

```bash
#!/usr/bin/env bash
# Gate M23 — Upstream HTTP Client Optimization (extends M5)
#
# Encodes PLAN.md "M23 Verification gate":
#   # Load test with k6: fan-out composition hitting 5 upstreams
#   # Before: observe connection churn in netstat, P95 latency > 200ms
#   # After: stable idle connections, P95 latency < 50ms
#
# Encoding decisions (approved design:
# docs/superpowers/specs/2026-07-27-m23-upstream-http-client-optimization-design.md):
#
#   * k6 is HARD-REQUIRED. The harness's skip-if-absent convention (docker, gh,
#     golangci-lint) would let M23 go green with zero performance evidence.
#   * "netstat" is replaced by server-side ground truth: mockupstream counts
#     accepted TCP connections via a ConnState hook and reports them at
#     /__stats. Cumulative and exact, where netstat would be sampled and
#     brittle to parse on macOS.
#   * The gate runs k6 TWICE against the same composition — max_idle_conns_per_host
#     2 vs 100 — and asserts the tuned run accepts strictly fewer connections.
#     This is the non-flaky signal, and it exercises the config surface T23.3 adds.
#   * baseline P95 is RECORDED but NOT asserted. PLAN's ">200ms before" was
#     measured against real network upstreams; on loopback, connection setup is
#     cheap enough that asserting it would make the gate flaky.
#
# Written before T23.0b/T23.2/T23.3 land (CLAUDE.md rule 2): expect the grep
# checks and both k6 runs to FAIL until those tasks are implemented. That is the
# intended state for this gate immediately after Task 1.
set -euo pipefail
source "$(dirname "$0")/../lib/harness.sh"
h_init M23

# ── k6 is hard-required ──────────────────────────────────────────────

h_task M23.gate
if h_require_tool k6; then
    h_evidence "$ k6 version"
    k6 version >> "${H_EVIDENCE_FILE}" 2>&1 || true
    h_pass "k6 available (hard requirement)"
else
    h_fail "k6 not installed — M23 gate requires k6 (brew install k6). Not skippable."
    h_finish
fi

# ── T23.1: pool + TLS defaults ───────────────────────────────────────

h_task T23.1
h_run "BuildTransport defined" -- grep -q 'func BuildTransport' internal/upstream/client.go
h_run "MaxIdleConnsPerHost defaults to 100" -- \
    grep -q 'maxIdleConnsPerHost = 100' internal/upstream/client.go
h_run "TLS 1.2 minimum enforced" -- \
    grep -q 'MinVersion:         tls.VersionTLS12' internal/upstream/client.go
h_run "transport defaults covered by tests" -- \
    go test -race -count=1 -run 'TestBuildTransport' ./internal/upstream/...

# ── T23.2: DrainAndClose coverage ────────────────────────────────────

h_task T23.2
h_run "DrainAndClose defined" -- grep -q 'func DrainAndClose' internal/upstream/drain.go
h_run "composition step drains" -- \
    grep -q 'DrainAndClose(resp.Body)' internal/composition/step.go
h_run "health check drains" -- \
    grep -q 'DrainAndClose(resp.Body)' internal/upstream/health.go
h_run "hotreload client drains" -- \
    grep -q 'DrainAndClose(resp.Body)' internal/hotreload/client.go
h_run "devmode writer drains" -- \
    grep -q 'DrainAndClose(resp.Body)' internal/devmode/writer.go
h_grep_repo 'defer resp.Body.Close()' \
    'internal/upstream internal/hotreload internal/devmode internal/composition' \
    --empty "no bare resp.Body.Close() on upstream response paths"

# ── T23.3: transport block wiring ────────────────────────────────────

h_task T23.3
h_run "TransportConfig type in config.go" -- \
    grep -q 'type TransportConfig struct' internal/composition/config.go
h_run "Upstream.Transport field bound to yaml" -- \
    grep -q 'Transport .*\`yaml:"transport"\`' internal/composition/config.go
h_run "max_idle_conns_per_host yaml tag" -- \
    grep -q 'yaml:"max_idle_conns_per_host"' internal/composition/config.go
h_run "insecure_skip_verify yaml tag" -- \
    grep -q 'yaml:"insecure_skip_verify"' internal/composition/config.go
h_run "parser no longer hardcodes empty transport" -- \
    bash -c '! grep -q "Transport:        upstream.TransportConfig{}," internal/composition/parser.go'
h_run "transport parser tests pass" -- \
    go test -race -count=1 -run 'TestTransport' ./internal/composition/...

# ── Unit tests ───────────────────────────────────────────────────────

h_task M23.unit
h_run "go vet upstream" -- go vet ./internal/upstream/...
h_run "go vet composition" -- go vet ./internal/composition/...
h_run "go test upstream" -- go test -race -count=1 ./internal/upstream/...
h_run "go test composition" -- go test -race -count=1 ./internal/composition/...
h_run "go test full suite" -- go test -race -count=1 ./...

# ── A/B load test ────────────────────────────────────────────────────

h_task M23.gate

h_build
h_start_mockupstream

# Verify the mockupstream stats endpoint exists before relying on it.
if ! curl -sf "http://127.0.0.1:${MOCK_PORT}/__stats" > "${H_TMP}/stats_probe.json" 2>/dev/null; then
    h_fail "mockupstream /__stats endpoint missing (T23.0b not implemented)"
    h_finish
fi
h_evidence "$ curl /__stats (probe)"
h_evidence "$(cat "${H_TMP}/stats_probe.json")"
h_pass "mockupstream /__stats endpoint available"

# Generate a five-upstream fan-out config at a given max_idle_conns_per_host.
# All five point at the same mockupstream process; upstream.Build gives each its
# own *http.Transport, so there are five independent connection pools.
m23_write_config() {
    local max_idle="$1" out="$2"
    {
        echo "upstreams:"
        local i
        for i in 1 2 3 4 5; do
            echo "  up${i}:"
            echo "    url: \"http://127.0.0.1:${MOCK_PORT}\""
            echo "    timeout: 10s"
            echo "    transport:"
            echo "      max_idle_conns_per_host: ${max_idle}"
        done
        echo "compositions:"
        echo "  fanout:"
        echo "    path: \"/fanout\""
        echo "    method: GET"
        echo "    steps:"
        for i in 1 2 3 4 5; do
            echo "      - name: s${i}"
            echo "        upstream: up${i}"
            echo "        path: \"/users/${i}\""
        done
        echo "    response:"
        echo "      body:"
        for i in 1 2 3 4 5; do
            echo "        u${i}: \"{{ steps.s${i}.body.name }}\""
        done
    } > "${out}"
}

# Run one arm of the A/B: start a gateway on the given config, reset the
# mockupstream counters, run k6, and echo "<p95_ms> <error_rate> <conns>".
m23_run_arm() {
    local label="$1" config="$2"
    local port
    port="$(h_free_port)"

    "${REPO_ROOT}/bin/restitch" run \
        -config "${config}" \
        -port "${port}" \
        -log-format json \
        > "${H_TMP}/gw_${label}.log" 2>&1 &
    local gw_pid=$!
    H_PIDS+=("${gw_pid}")
    h_wait_for_port "${port}" "gw_${label}" 15

    curl -sf -X POST "http://127.0.0.1:${MOCK_PORT}/__stats/reset" > /dev/null 2>&1

    GW_URL="http://127.0.0.1:${port}" k6 run \
        --summary-export="${H_TMP}/k6_${label}.json" \
        "${REPO_ROOT}/tests/loadtest/m23_fanout.js" \
        >> "${H_TMP}/k6_${label}.log" 2>&1 || true

    local stats
    stats="$(curl -sf "http://127.0.0.1:${MOCK_PORT}/__stats" 2>/dev/null || echo '{}')"

    kill "${gw_pid}" 2>/dev/null || true
    wait "${gw_pid}" 2>/dev/null || true

    python3 - "${H_TMP}/k6_${label}.json" "${stats}" <<'PY'
import json, sys
with open(sys.argv[1]) as fh:
    m = json.load(fh)["metrics"]
stats = json.loads(sys.argv[2] or "{}")
p95 = m["http_req_duration"]["p(95)"]          # milliseconds
err = m["http_req_failed"]["value"]            # rate, 0.0-1.0
reqs = m["http_reqs"]["count"]
print(f'{p95:.3f} {err:.6f} {stats.get("conns_accepted", -1)} {reqs}')
PY
}

m23_write_config 2 "${H_TMP}/fanout_baseline.yaml"
m23_write_config 100 "${H_TMP}/fanout_tuned.yaml"

h_log "Running k6 baseline arm (max_idle_conns_per_host: 2)..."
read -r BASE_P95 BASE_ERR BASE_CONNS BASE_REQS <<< "$(m23_run_arm baseline "${H_TMP}/fanout_baseline.yaml")"

h_log "Running k6 tuned arm (max_idle_conns_per_host: 100)..."
read -r TUNED_P95 TUNED_ERR TUNED_CONNS TUNED_REQS <<< "$(m23_run_arm tuned "${H_TMP}/fanout_tuned.yaml")"

h_evidence "baseline  max_idle=2    p95=${BASE_P95}ms  error_rate=${BASE_ERR}  conns_accepted=${BASE_CONNS}  http_reqs=${BASE_REQS}"
h_evidence "tuned     max_idle=100  p95=${TUNED_P95}ms error_rate=${TUNED_ERR} conns_accepted=${TUNED_CONNS} http_reqs=${TUNED_REQS}"

h_run "tuned run accepted fewer upstream connections than baseline" -- \
    python3 -c "import sys; b=int(sys.argv[1]); t=int(sys.argv[2]); assert b>0 and t>0, f'no connections recorded: baseline={b} tuned={t}'; assert t<b, f'tuned={t} not < baseline={b}'" \
    "${BASE_CONNS}" "${TUNED_CONNS}"

h_run "tuned P95 < 50ms" -- \
    python3 -c "import sys; p=float(sys.argv[1]); assert p<50.0, f'p95={p}ms'" "${TUNED_P95}"

h_run "tuned error rate < 1%" -- \
    python3 -c "import sys; e=float(sys.argv[1]); assert e<0.01, f'error_rate={e}'" "${TUNED_ERR}"

h_run "tuned run actually generated load" -- \
    python3 -c "import sys; n=int(sys.argv[1]); assert n>1000, f'http_reqs={n}'" "${TUNED_REQS}"

h_finish
```

- [ ] **Step 3: Make the gate executable and syntax-check it**

Run:

```bash
chmod +x scripts/gates/m23.sh
bash -n scripts/gates/m23.sh && echo "SYNTAX OK"
```

Expected: `SYNTAX OK`

- [ ] **Step 4: Dry-run the gate to confirm it fails for the right reasons**

Run: `H_NO_EVIDENCE=true ./scripts/gates/m23.sh 2>&1 | tail -30`

Expected: `k6 available (hard requirement)` PASSes, T23.1 checks PASS (already
implemented), and the T23.2/T23.3 grep checks plus the `/__stats` probe FAIL.
`RESULT M23: FAIL`. This is correct — the features do not exist yet.

`H_NO_EVIDENCE=true` prevents this exploratory run from writing an evidence file
or ledger rows.

- [ ] **Step 5: STOP — get user approval**

Per CLAUDE.md rule 2, gate scripts require explicit user approval before the
milestone proceeds. Show the user the gate script and the dry-run output, and
ask them to approve. **Do not commit until they do.**

- [ ] **Step 6: Commit with the `gate:` prefix**

```bash
git add scripts/gates/m23.sh tests/loadtest/m23_fanout.js
git commit -m "gate: implement M23 verification gate with k6 A/B load test

Replaces the m23.sh placeholder. Encodes PLAN.md's M23 gate: k6 fan-out
across 5 upstreams, asserting the tuned arm (max_idle_conns_per_host: 100)
accepts strictly fewer TCP connections than the baseline arm (2), with
tuned P95 < 50ms and error rate < 1%.

k6 is hard-required, not skip-if-absent. Baseline P95 is recorded without
a threshold — PLAN's >200ms figure was measured against network upstreams
and does not hold on loopback.

Approved by user before commit per CLAUDE.md rule 2."
```

---

## Task 2: T23.0b — mockupstream connection instrumentation

The gate needs ground truth on accepted TCP connections. `cmd/mockupstream` is
currently a bare `http.ListenAndServe` with no instrumentation.

**Files:**
- Modify: `cmd/mockupstream/main.go`

**Interfaces:**
- Consumes: `mockupstream.Handler()` from `internal/mockupstream/handler.go` (unchanged).
- Produces: `GET /__stats` → `{"conns_accepted":N,"requests":M}`; `POST /__stats/reset` → `{"reset":true}`. Consumed by `scripts/gates/m23.sh`.

- [ ] **Step 1: Write the implementation**

Replace the entire contents of `cmd/mockupstream/main.go`:

```go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/restitch/restitch-gateway/internal/mockupstream"
)

// connCounter tracks accepted TCP connections and served requests. The M23 gate
// reads these to prove connection pooling reduces churn: with a large
// MaxIdleConnsPerHost the gateway reuses connections, so connsAccepted stays
// near peak parallelism instead of tracking request count.
type connCounter struct {
	connsAccepted atomic.Int64
	requests      atomic.Int64
}

func (c *connCounter) onConnState(_ net.Conn, state http.ConnState) {
	if state == http.StateNew {
		c.connsAccepted.Add(1)
	}
}

// countRequests wraps a handler, counting every request that is not itself a
// stats query.
func (c *connCounter) countRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.requests.Add(1)
		next.ServeHTTP(w, r)
	})
}

func main() {
	port := flag.Int("port", 8081, "server port")
	flag.Parse()

	counter := &connCounter{}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /__stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int64{
			"conns_accepted": counter.connsAccepted.Load(),
			"requests":       counter.requests.Load(),
		})
	})

	mux.HandleFunc("POST /__stats/reset", func(w http.ResponseWriter, _ *http.Request) {
		counter.connsAccepted.Store(0)
		counter.requests.Store(0)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"reset": true})
	})

	mux.Handle("/", counter.countRequests(mockupstream.Handler()))

	addr := fmt.Sprintf(":%d", *port)
	srv := &http.Server{
		Addr:      addr,
		Handler:   mux,
		ConnState: counter.onConnState,
	}

	log.Printf("mockupstream listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}
```

Note: `/__stats` and `/__stats/reset` are registered directly on the mux, so
they bypass `countRequests` and do not inflate the request counter. The
`ConnState` hook still counts the connection the gate's `curl` opens — which is
why the gate resets counters *after* the gateway starts and *before* k6 runs.

- [ ] **Step 2: Build and verify it compiles**

Run: `go build -o bin/mockupstream ./cmd/mockupstream && echo "BUILD OK"`
Expected: `BUILD OK`

- [ ] **Step 3: Verify the endpoints behave**

Run:

```bash
./bin/mockupstream -port 18123 & MOCK_PID=$!
sleep 1
curl -s localhost:18123/users/7
curl -s localhost:18123/users/8
echo "--- stats after 2 requests:"
curl -s localhost:18123/__stats
echo "--- reset:"
curl -s -X POST localhost:18123/__stats/reset
echo "--- stats after reset:"
curl -s localhost:18123/__stats
kill $MOCK_PID
```

Expected: the user payloads, then `{"conns_accepted":N,"requests":2}` with
`N >= 1`, then `{"reset":true}`, then `requests` back to `0`.

- [ ] **Step 4: Vet**

Run: `go vet ./cmd/mockupstream/... && echo "VET OK"`
Expected: `VET OK`

- [ ] **Step 5: Commit**

```bash
git add cmd/mockupstream/main.go
git commit -m "test: instrument mockupstream with connection and request counters

Adds a ConnState hook counting accepted TCP connections, plus
GET /__stats and POST /__stats/reset. The M23 gate uses these as
server-side ground truth for connection reuse, replacing PLAN's
'observe netstat' with an exact cumulative count.

Dev/test binary only — internal/mockupstream/handler.go is unchanged
and nothing shipped is affected."
```

---

## Task 3: T23.3 — Wire the `upstreams.*.transport` YAML block

The defect this milestone exists to fix. `docs/configuration.md:180-189` and
`PLAN.md:249-254` both document this block; neither binds, and non-strict
`yaml.Unmarshal` swallows the key silently.

**Files:**
- Create: `internal/composition/transport_test.go`
- Modify: `internal/composition/config.go:33-41`
- Modify: `internal/composition/parser.go:150`

**Interfaces:**
- Consumes: `upstream.TransportConfig{DialTimeout, TLSHandshakeTimeout, ResponseHeaderTimeout time.Duration; MaxIdleConnsPerHost int; InsecureSkipVerify bool}` from `internal/upstream/client.go:15-21`.
- Produces: `composition.TransportConfig` (config-layer mirror) and `Upstream.Transport *TransportConfig`.

- [ ] **Step 1: Write the failing tests**

Create `internal/composition/transport_test.go`:

```go
package composition

import (
	"testing"
	"time"
)

// The transport: block is documented in docs/configuration.md and PLAN.md but
// was never bound to a struct field, so yaml.Unmarshal silently discarded it.
// These tests pin the binding.

func TestTransportBlockParsesAllFields(t *testing.T) {
	yamlSrc := `
upstreams:
  api:
    url: "http://example.test"
    transport:
      dial_timeout: 3s
      tls_handshake_timeout: 4s
      response_header_timeout: 7s
      max_idle_conns_per_host: 250
      insecure_skip_verify: true
compositions: {}
`
	cfg, err := ParseConfig([]byte(yamlSrc))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	tr := cfg.Upstreams["api"].Transport
	if tr == nil {
		t.Fatal("transport block was not parsed (nil)")
	}
	if tr.DialTimeout != 3*time.Second {
		t.Errorf("DialTimeout = %v, want 3s", tr.DialTimeout)
	}
	if tr.TLSHandshakeTimeout != 4*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 4s", tr.TLSHandshakeTimeout)
	}
	if tr.ResponseHeaderTimeout != 7*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 7s", tr.ResponseHeaderTimeout)
	}
	if tr.MaxIdleConnsPerHost != 250 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 250", tr.MaxIdleConnsPerHost)
	}
	if !tr.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = false, want true")
	}
}

func TestTransportBlockOmittedIsNil(t *testing.T) {
	yamlSrc := `
upstreams:
  api:
    url: "http://example.test"
compositions: {}
`
	cfg, err := ParseConfig([]byte(yamlSrc))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Upstreams["api"].Transport != nil {
		t.Error("Transport should be nil when the block is omitted")
	}
}

func TestTransportPartialBlockLeavesOthersZero(t *testing.T) {
	yamlSrc := `
upstreams:
  api:
    url: "http://example.test"
    transport:
      max_idle_conns_per_host: 42
compositions: {}
`
	cfg, err := ParseConfig([]byte(yamlSrc))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	tr := cfg.Upstreams["api"].Transport
	if tr == nil {
		t.Fatal("transport block was not parsed (nil)")
	}
	if tr.MaxIdleConnsPerHost != 42 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 42", tr.MaxIdleConnsPerHost)
	}
	// Unspecified fields stay zero so BuildTransport applies its defaults.
	if tr.DialTimeout != 0 {
		t.Errorf("DialTimeout = %v, want 0 (so BuildTransport defaults apply)", tr.DialTimeout)
	}
}

func TestTransportTranslatesToUpstreamConfig(t *testing.T) {
	yamlSrc := `
upstreams:
  api:
    url: "http://example.test"
    transport:
      max_idle_conns_per_host: 77
      response_header_timeout: 9s
compositions: {}
`
	cfg, err := ParseConfig([]byte(yamlSrc))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	// toUpstreamTransport is the exact translation the parser uses, so building
	// a transport from its output asserts the real code path. upstream.Build
	// wraps the base transport in a metrics/retry/breaker/auth RoundTripper
	// chain that cannot be unwrapped from outside the package, which is why
	// this asserts on BuildTransport rather than on the compiled client.
	tr := upstream.BuildTransport(toUpstreamTransport(cfg.Upstreams["api"].Transport))
	if tr.MaxIdleConnsPerHost != 77 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 77", tr.MaxIdleConnsPerHost)
	}
	if tr.ResponseHeaderTimeout != 9*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 9s", tr.ResponseHeaderTimeout)
	}

	// And the full compile path accepts a config carrying a transport block.
	if _, err := CompileConfig(t.Context(), cfg, CompileOptions{SkipAuthInit: true}); err != nil {
		t.Fatalf("CompileConfig: %v", err)
	}
}
```

The test file's imports are:

```go
import (
	"testing"
	"time"

	"github.com/restitch/restitch-gateway/internal/upstream"
)
```

`toUpstreamTransport` is the translation function written in Step 4.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -count=1 -run 'TestTransport' ./internal/composition/... 2>&1 | head -20`
Expected: compile failure — `cfg.Upstreams["api"].Transport` undefined,
`TransportConfig` undefined, `toUpstreamTransport` undefined.

- [ ] **Step 3: Add the config type and field**

Note: the compiled-path assertion uses `CompileConfig(ctx, cfg, opts...)` —
verified signature at `internal/composition/parser.go:99`. There is no function
named `Compile`.

In `internal/composition/config.go`, add `Transport` to the `Upstream` struct
(currently lines 33-41). The field goes after `MaxResponseBytes` to match the
ordering in `docs/configuration.md`:

```go
// Upstream represents a named backend service that steps can call.
// Auth configuration is optional - if omitted, no authentication is applied.
type Upstream struct {
	URL              string           `yaml:"url"`
	Auth             *auth.Config     `yaml:"auth"`
	Timeout          time.Duration    `yaml:"timeout"`
	HealthPath       string           `yaml:"health_path"`
	MaxResponseBytes int64            `yaml:"max_response_bytes"`
	Transport        *TransportConfig `yaml:"transport"`
	Retry            *RetryConfig     `yaml:"retry"`
	CircuitBreaker   *BreakerConfig   `yaml:"circuit_breaker"`
}
```

Then add the type immediately after the `Upstream` struct, before
`RetryConfig`. It mirrors `upstream.TransportConfig`, following the same
config-layer-mirror pattern `RetryConfig` and `BreakerConfig` already use — this
keeps `internal/composition` free of a structural dependency on
`internal/upstream`'s wire shape:

```go
// TransportConfig configures the HTTP transport for an upstream. All fields are
// optional; zero values fall through to the defaults in upstream.BuildTransport.
type TransportConfig struct {
	DialTimeout           time.Duration `yaml:"dial_timeout"`
	TLSHandshakeTimeout   time.Duration `yaml:"tls_handshake_timeout"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"`
	MaxIdleConnsPerHost   int           `yaml:"max_idle_conns_per_host"`
	InsecureSkipVerify    bool          `yaml:"insecure_skip_verify"`
}
```

`gopkg.in/yaml.v3 v3.0.1` decodes `3s` into `time.Duration` natively — no custom
unmarshaller is needed. This is verified behaviour and matches the existing
`Upstream.Timeout` field.

- [ ] **Step 4: Wire it in the parser**

In `internal/composition/parser.go`, add the translation function just above
`Compile` (or beside the existing retry/breaker translation block):

```go
// toUpstreamTransport converts the config-layer transport block into the
// upstream package's TransportConfig. A nil block yields the zero value, which
// upstream.BuildTransport fills in with its defaults.
func toUpstreamTransport(tc *TransportConfig) upstream.TransportConfig {
	if tc == nil {
		return upstream.TransportConfig{}
	}
	return upstream.TransportConfig{
		DialTimeout:           tc.DialTimeout,
		TLSHandshakeTimeout:   tc.TLSHandshakeTimeout,
		ResponseHeaderTimeout: tc.ResponseHeaderTimeout,
		MaxIdleConnsPerHost:   tc.MaxIdleConnsPerHost,
		InsecureSkipVerify:    tc.InsecureSkipVerify,
	}
}
```

Then replace line 150 — currently:

```go
			Transport:        upstream.TransportConfig{},
```

with:

```go
			Transport:        toUpstreamTransport(up.Transport),
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -race -count=1 -run 'TestTransport' ./internal/composition/... -v 2>&1 | tail -20`
Expected: `PASS` for all four tests.

- [ ] **Step 6: Verify nothing else regressed**

Run: `go test -race -count=1 ./... 2>&1 | grep -v "^ok" | head -20`
Expected: no `FAIL` lines.

- [ ] **Step 7: Update the docs cross-reference**

`docs/configuration.md:180-189` already documents these fields correctly — it
was the code that was wrong, not the docs. Verify no change is needed:

Run: `sed -n '178,190p' docs/configuration.md`
Expected: the five-row table matching the struct you just wrote. If any field
name differs from your yaml tags, fix the **code** to match the published docs.

- [ ] **Step 8: Commit**

```bash
git add internal/composition/config.go internal/composition/parser.go internal/composition/transport_test.go
git commit -m "feat(M23): wire upstreams.*.transport YAML block (T23.3)

docs/configuration.md:180-189 and PLAN.md:249-254 both document a
five-field transport block, but Upstream had no Transport field and
parser.go:150 hardcoded an empty upstream.TransportConfig{}. Because
yaml.Unmarshal is non-strict, users following the docs had their
transport tuning silently ignored.

Adds a config-layer TransportConfig mirror (same pattern as RetryConfig
and BreakerConfig) and translates it in the parser. Zero values still
fall through to BuildTransport's defaults, so this is backwards-compatible.

PLAN's T23.3 wording proposes 'pool: { max_idle }'; that is drift from
the experimental reference repo and contradicts PLAN.md's own schema at
line 249. Wiring transport: is a superset."
```

---

## Task 4: T23.2 — Complete `DrainAndClose` coverage

Three upstream response paths still use bare `Body.Close()`, destroying the
connection instead of returning it to the pool.

**Files:**
- Modify: `internal/upstream/health.go:127`
- Modify: `internal/hotreload/client.go:61`
- Modify: `internal/devmode/writer.go:79`

**Interfaces:**
- Consumes: `upstream.DrainAndClose(body io.ReadCloser)` from `internal/upstream/drain.go:9`.
- Produces: nothing new.

- [ ] **Step 1: Fix the health check path**

`internal/upstream/health.go` is already in package `upstream`, so the call is
unqualified. Replace line 127:

```go
	defer resp.Body.Close()
```

with:

```go
	defer DrainAndClose(resp.Body)
```

- [ ] **Step 2: Fix the hotreload client path**

In `internal/hotreload/client.go`, add the import:

```go
	"github.com/restitch/restitch-gateway/internal/upstream"
```

and replace line 61:

```go
	defer resp.Body.Close()
```

with:

```go
	defer upstream.DrainAndClose(resp.Body)
```

`internal/upstream` imports only `internal/auth` and `internal/observability`,
so this creates no import cycle.

- [ ] **Step 3: Fix the devmode writer path**

In `internal/devmode/writer.go`, add the import:

```go
	"github.com/restitch/restitch-gateway/internal/upstream"
```

and replace line 79:

```go
		defer resp.Body.Close()
```

with:

```go
		defer upstream.DrainAndClose(resp.Body)
```

- [ ] **Step 4: Verify no bare closes remain on these paths**

Run:

```bash
grep -rn 'defer resp.Body.Close()' internal/upstream internal/hotreload internal/devmode internal/composition || echo "NONE REMAINING"
```

Expected: `NONE REMAINING`

- [ ] **Step 5: Verify the drain calls are in place**

Run:

```bash
grep -rn 'DrainAndClose(resp.Body)' internal/upstream/health.go internal/hotreload/client.go internal/devmode/writer.go internal/composition/step.go
```

Expected: four lines, one per file.

- [ ] **Step 6: Run the affected test suites**

Run: `go test -race -count=1 ./internal/upstream/... ./internal/hotreload/... ./internal/devmode/... 2>&1 | tail -10`
Expected: `ok` for all three packages.

- [ ] **Step 7: Vet and full suite**

Run: `go vet ./... && go test -race -count=1 ./... 2>&1 | grep -v "^ok" | head -20`
Expected: no output from vet, no `FAIL` lines from the tests.

- [ ] **Step 8: Commit**

```bash
git add internal/upstream/health.go internal/hotreload/client.go internal/devmode/writer.go
git commit -m "feat(M23): drain response bodies on remaining upstream paths (T23.2)

Health checks, registry bundle fetches, and devmode health polling closed
response bodies without draining, destroying the connection instead of
returning it to the idle pool. The composition hot path already drained
via step.go:125.

internal/upstream/retry.go:120 already drains via its local drainBody
helper and is unchanged."
```

---

## Task 5: T23.1 — Verify and record pool + TLS defaults

No code change. M16's hardening already satisfies T23.1; this task produces the
evidence for its ledger row.

**Files:** none modified.

**Interfaces:**
- Consumes: `upstream.BuildTransport` at `internal/upstream/client.go:34`.
- Produces: evidence only.

- [ ] **Step 1: Confirm the defaults in source**

Run:

```bash
grep -n 'maxIdleConnsPerHost = 100' internal/upstream/client.go
grep -n 'MinVersion:         tls.VersionTLS12' internal/upstream/client.go
grep -n 'MaxIdleConnsPerHost:   maxIdleConnsPerHost' internal/upstream/client.go
```

Expected: one match each, at roughly lines 46, 62, and 58.

If the `MinVersion` grep fails because of whitespace, adjust the **gate script's**
matching pattern in `scripts/gates/m23.sh` to match the real source — but only
with user approval and a `gate:` commit, per CLAUDE.md.

- [ ] **Step 2: Run the transport unit tests**

Run: `go test -race -count=1 -run 'TestBuildTransport' ./internal/upstream/... -v 2>&1 | tail -20`
Expected: PASS. Existing coverage lives at `internal/upstream/client_test.go:14-42`.

- [ ] **Step 3: Commit the ledger row**

There is no code change, so this commit carries only the ledger row. Append the
row (Task 6 covers the format), then:

```bash
git add docs/plan-progress/LEDGER.md
git commit -m "docs: record T23.1 — pool and TLS defaults verified

MaxIdleConnsPerHost defaults to 100 and TLS 1.2 minimum is enforced in
upstream.BuildTransport, landed by M16 hardening ahead of M23. No code
change required; recording the evidence."
```

---

## Task 6: M23.gate — Run the gate and close the milestone

**Files:**
- Modify: `docs/plan-progress/LEDGER.md`, `docs/plan-progress/evidence/INDEX.md` (both written automatically by `h_finish`)

**Interfaces:**
- Consumes: everything above.
- Produces: `RESULT M23: PASS` and a committed evidence file.

- [ ] **Step 1: Build everything fresh**

Run: `make build && go build -o bin/mockupstream ./cmd/mockupstream && echo "BUILD OK"`
Expected: `BUILD OK`

- [ ] **Step 2: Run the gate**

Run: `./scripts/verify.sh M23 2>&1 | tail -40`
Expected: `RESULT M23: PASS`

The two k6 arms take 20s each, so the gate runs for roughly 60-90 seconds.

- [ ] **Step 3: If the connection-count assertion fails, diagnose before changing anything**

If `tuned run accepted fewer upstream connections than baseline` FAILs, read the
recorded numbers in the evidence log first. Common causes, in order of
likelihood:

1. `/__stats/reset` did not run between arms — both counts include the other
   arm's connections. Check the `conns_accepted` values are plausible.
2. The gateway kept HTTP/2 to the mock (H2 multiplexes over one connection, so
   both arms would show near-identical low counts). `mockupstream` serves plain
   HTTP/1.1 over cleartext, so this should not occur — but if it does, both
   counts will be tiny and roughly equal.
3. The config did not apply — verify by grepping the generated config in the
   evidence log for `max_idle_conns_per_host: 2`.

**Do NOT weaken the assertion or the gate to make it pass.** If the gate is
genuinely wrong, STOP and ask the user, then change it in a `gate:` commit.

- [ ] **Step 4: Confirm the evidence file and ledger rows were written**

Run:

```bash
ls -la docs/plan-progress/evidence/ | tail -3
tail -8 docs/plan-progress/LEDGER.md
```

Expected: a new `<date>-M23-<sha>.log`, and rows for `T23.1`, `T23.2`, `T23.3`,
`M23.unit`, `M23.gate` — all PASS.

- [ ] **Step 5: Run the ledger check**

Run: `./scripts/check-ledger.sh; echo "EXIT=$?"`
Expected: `EXIT=0`, coverage green.

- [ ] **Step 6: Commit the evidence**

```bash
git add docs/plan-progress/
git commit -m "docs: record M23.gate PASS

$(tail -1 docs/plan-progress/LEDGER.md)"
```

- [ ] **Step 7: Report MANUAL lines to the user — do not check them off**

If `verify.sh` printed any `MANUAL` lines, list them for the user and **stop**.
Per CLAUDE.md rule 6, only the user may confirm them, after which a
`MANUAL-VERIFIED` row citing their confirmation may be appended.

The gate as designed emits no MANUAL lines — every assertion is mechanical. If
one appears, something changed and it is worth investigating.

---

## Out of scope

Deliberately excluded, per the approved spec:

- **Revising M5's ledger rows.** Append-only is absolute. The gap is recorded in the spec and closed by M23.
- **A `pool: { max_idle }` alias.** One way to express a setting.
- **Strict YAML decoding (`KnownFields`).** Would catch this whole class of defect, but it is a breaking change across four `yaml.Unmarshal` call sites (`internal/composition/parser.go:22`, `internal/registry/store.go:466`, `internal/registry/validator.go:38`, `internal/gwconfig/config.go:131`) and belongs in its own milestone.
- **Gateway-side connection pool metrics.** Go's `http.Transport` exposes no pool introspection; it needs a wrapping dialer. Candidate for M24.

### Flagged for the user, not implemented

`upstream.DrainAndClose` (`internal/upstream/drain.go:11`) drains with an
unbounded `io.Copy`. On the composition hot path the body has already been read
through a `LimitReader`, but any remaining bytes are still copied without a cap,
so a hostile or misbehaving upstream could make a drain read arbitrarily large.
Bounding it (`io.CopyN(io.Discard, body, 64<<10)`) would be a small, sensible
hardening — but it is not in the approved spec and it changes shipped behaviour
on the hot path, so it is **not** included here. Raise it with the user as a
possible follow-up.
