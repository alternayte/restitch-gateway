# M22 — Dev Mode Orchestrator Design

**Date:** 2026-07-16
**Status:** Draft
**Milestone:** M22

## Summary

`restitch dev` runs the gateway and studio together for local development.
It spawns both as child processes with colored log prefixes, auto-restart on
crash, and health-check sequencing (gateway must be healthy before studio
starts). SIGINT/SIGTERM shuts both down gracefully.

## Design Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | No cobra — stdlib `flag.FlagSet` with manual switch/case dispatch | Matches the existing CLI pattern (M11). Adding cobra for one subcommand isn't worth the dep + restructuring cost. |
| D2 | Raw ANSI codes, not `fatih/color` | Three color codes + a `NO_COLOR` check is ~10 lines. A whole dependency for that is overkill. |
| D3 | `cenkalti/backoff/v5` for ProcessManager restart backoff and WaitForHealth | Already an indirect dep from M21. Also consolidates the hand-rolled `internal/hotreload/backoff.go` (delete it, use cenkalti in the poller too). |
| D4 | Leave `internal/upstream/retry.go` backoff untouched | Its backoff is embedded in HTTP-specific control flow (Retry-After, status-code dispatch) that doesn't map onto cenkalti's API. |
| D5 | Two separate binaries, not a merged single binary | Preserves K11 ("Studio = separate binary"). Gateway stays lean (no SPA assets). Binary discovery via sibling path + PATH + `--studio-bin` override. Industry research confirms no competitor uses a unified dev orchestrator — this is differentiated. |
| D6 | Fixed ports in dev mode (8080/9090/3080) | Dev mode is opinionated. Fewer flags = simpler UX. Matches the defaults users already know. |

## Architecture

Three components:

1. **`internal/devmode/manager.go`** — ProcessManager: spawns a child process,
   monitors it, restarts on crash with cenkalti/backoff exponential backoff.
2. **`internal/devmode/writer.go`** — PrefixWriter (thread-safe line-prefixed
   io.Writer with ANSI colors) + WaitForHealth (polls a URL with backoff).
3. **`cmd/restitch/dev.go`** — CLI glue: discovers studio binary, spawns
   gateway then studio with sequencing, handles shutdown.

Plus a targeted consolidation: delete `internal/hotreload/backoff.go` and
replace its one call site in `poller.go` with cenkalti/backoff.

## Component Details

### ProcessManager (`internal/devmode/manager.go`)

```go
type ProcessConfig struct {
    Name       string   // display name ("gateway", "studio")
    Executable string   // path to binary
    Args       []string // command-line args
    ExtraEnv   []string // appended to os.Environ()
}

type ProcessManager struct { ... }

func NewProcessManager(cfg ProcessConfig, stdout, stderr io.Writer) *ProcessManager
func (pm *ProcessManager) Run(ctx context.Context) error
```

Behavior:
- `Run` loops: spawn process → wait for exit → on crash: log, backoff, restart
- Backoff: cenkalti/backoff/v5 exponential (initial 1s, max 15s)
- Reset backoff after process stays alive for 30s (considered "stable")
- `cmd.Cancel` sends SIGTERM; `cmd.WaitDelay = 5s` (then SIGKILL by os/exec)
- Context cancellation = clean shutdown, no restart attempt, returns `context.Canceled`
- Clean child exit (exit 0) still triggers restart (dev mode keeps both alive)
- `cmdModifier func(*exec.Cmd)` hook for tests (inject env, swap executable)

### PrefixWriter + WaitForHealth (`internal/devmode/writer.go`)

```go
type PrefixWriter struct { ... }

func NewPrefixWriter(dest io.Writer, prefix, colorCode string) *PrefixWriter
func (pw *PrefixWriter) Write(p []byte) (int, error)

func WaitForHealth(ctx context.Context, url string, timeout time.Duration) error
```

Color constants:
- `ColorCyan = "\033[36m"` — gateway
- `ColorMagenta = "\033[35m"` — studio
- `ColorYellow = "\033[33m"` — dev banner/messages
- `ColorReset = "\033[0m"`

`NO_COLOR` env var (any value) disables all ANSI codes. This is the
[standard convention](https://no-color.org/).

PrefixWriter splits input on newlines and prefixes each line with
`<color>[name]<reset> `. Thread-safe via mutex on the destination writer.

WaitForHealth polls the URL with cenkalti/backoff (initial 200ms, max 2s)
under a context with the given timeout. Returns nil on first HTTP 200,
or the context error on timeout.

### CLI (`cmd/restitch/dev.go`)

```go
func devCmd(args []string) int
```

Flags via `flag.FlagSet`:
- `--config` (default `restitch.yaml`) — passed through to gateway
- `--log-format` (default `text`) — text not json, since dev mode is for humans
- `--studio-bin` (default empty — auto-discover)

Binary discovery (`findStudioBinary`):
1. `--studio-bin` flag if set
2. Sibling of `os.Executable()` (same directory)
3. `exec.LookPath("restitch-studio")` (PATH)
4. Error: `"restitch-studio not found — run 'make build-all' or pass --studio-bin"`

Startup sequence:
1. Print yellow banner with gateway/admin/studio URLs
2. Discover studio binary
3. Spawn gateway: `restitch run --config=X --log-format=Y --port=8080`
4. `WaitForHealth("http://localhost:8080/health", 30s)`
5. Print "Gateway healthy, starting studio..."
6. Spawn studio: `restitch-studio --port=3080 --gateway-admin-url=http://localhost:9090`
7. Block on SIGINT/SIGTERM
8. Cancel context → both get SIGTERM → 5s grace → SIGKILL
9. `sync.WaitGroup` wait with 3s forced-exit timeout

`main.go` change — one case added:
```go
case "dev":
    os.Exit(devCmd(os.Args[2:]))
```

### Backoff Consolidation

**Delete** `internal/hotreload/backoff.go` (16 lines, one function).

**Replace** the call site in `internal/hotreload/poller.go`:
- Add `bo *backoff.ExponentialBackOff` field to `Poller`
- Initialize in `NewPoller`: initial = poll interval, max = 5 min
- Error branch: `wait := p.bo.NextBackOff()` (replaces `backoffDuration(...)`)
- Success branch: `p.bo.Reset()` (replaces implicit stateless reset)

**`go.mod`**: `cenkalti/backoff/v5` moves from `// indirect` to direct.

**Not touched**: `internal/upstream/retry.go` — its `retryBackoff` function
stays as-is (HTTP-specific control flow with Retry-After headers).

## Testing

### `internal/devmode/manager_test.go`

Uses the `TestHelperProcess` pattern (test binary re-execs itself as a fake
child process via `GO_WANT_HELPER_PROCESS=1`):

- Context cancellation stops process, Run returns `context.Canceled`
- Crash (exit 1) triggers restart — verify ≥2 starts via "started (PID" count
- ExtraEnv propagated to child process
- Stable process (runs > stableAfter) resets backoff

### `internal/devmode/writer_test.go`

- Single line → `[prefix] line\n`
- Multi-line input → each line prefixed separately
- Concurrent writes (10 goroutines × 10 lines) → correct count, no interleave
- `NO_COLOR=1` → plain `[prefix]` with no ANSI escapes
- io.Writer contract: returned byte count == input length
- `WaitForHealth`: healthy → immediate success; unhealthy-then-healthy → retries;
  always-unhealthy → timeout error; cancelled context → immediate return

### `internal/hotreload/poller_test.go`

Existing tests must continue to pass after the cenkalti swap. No new tests
needed — backoff behavior is tested through the poller, not the deleted helper.

## Verification Gate

`scripts/gates/m22.sh` checks:

1. `go test ./internal/devmode/ -count=1 -race` — all pass
2. `go test ./internal/hotreload/ -count=1 -race` — backoff consolidation clean
3. `grep -rn "backoffDuration" internal/` → empty (old function deleted)
4. `make build-all && bin/restitch dev --help` — exits cleanly
5. `go vet ./...` — clean

MANUAL gate item: live smoke test (spawn gateway + studio, verify colored
output, Ctrl+C shutdown). Requires port availability and process management
that's unreliable in CI.
