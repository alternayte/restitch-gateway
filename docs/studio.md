# Restitch Studio

Restitch Studio is a web UI for monitoring and managing the Restitch gateway.
It runs as a separate binary that proxies to the gateway's admin API.

## Architecture

```
Browser → restitch-studio (:3080) → restitch admin API (:9090)
                │
         embedded React SPA
```

- **`restitch-studio`** is a Go binary that embeds the built React SPA via
  `go:embed`.
- All `/api/*` requests are reverse-proxied to the gateway's admin API
  (rewriting `/api/*` → `/admin/api/*`).
- The `X-Admin-Key` header is attached automatically when configured.
- Everything else serves the SPA with client-side routing fallback.

## Setup

### Build from source

```bash
make build-all    # builds both restitch and restitch-studio
```

Or build Studio separately:

```bash
make studio       # builds the React app (npm ci + npm run build)
go build -o bin/restitch-studio ./cmd/restitch-studio
```

### Prerequisites

- Go 1.25.7+
- Node.js 24+ (the CI floor; see `.github/workflows/ci.yml`)
- npm

## Running

Start the gateway first, then Studio:

```bash
# Terminal 1: start the gateway
restitch run -config restitch.yaml

# Terminal 2: start Studio
restitch-studio
```

Studio is available at `http://localhost:3080`.

## Configuration

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `3080` | Studio HTTP port |
| `-bind` | `127.0.0.1` | Bind address. Use `0.0.0.0` only for deliberate remote access |
| `-gateway-admin-url` | `http://localhost:9090` | Gateway admin API URL to proxy to |
| `-admin-key` | (none) | Admin API key (attached to proxied requests) |
| `-registry-key` | (none) | Registry API key. The gateway's `-registry-key` must match it, or the gateway cannot poll the bundle |

### Environment Variables

| Variable | Overrides |
|----------|-----------|
| `STUDIO_PORT` | `-port` |
| `STUDIO_BIND` | `-bind` |
| `STUDIO_GATEWAY_ADMIN_URL` | `-gateway-admin-url` |
| `STUDIO_ADMIN_KEY` | `-admin-key` |
| `STUDIO_REGISTRY_KEY` | `-registry-key` |

### Registry API Authentication

Every `/api/v1/configs*` and `/api/v1/registry/bundle` handler requires an
`X-Admin-Key` header that matches the configured `-registry-key`. With no key
configured, the registry API rejects every request. Run the gateway with the
same key:

```bash
restitch-studio -registry-key "$REGISTRY_KEY"
restitch run -registry-url http://localhost:3080 -registry-key "$REGISTRY_KEY"
```

The browser preferences API (`/api/v1/preferences`) stays cookie-bound and
needs no key.

## Pages

### Dashboard

Overview of gateway status:

- **Stat tiles** — total requests, error rate, partial responses, composition
  count.
- **Per-composition table** — request counts, errors, average and p95
  latency.
- **Upstream health strip** — health badge per upstream (green/red) with
  latency and error details.

### Compositions

- **List view** — all compositions with method, path, step count, and public
  badge.
- **Detail view** (`/compositions/:name`) with three tabs:
  - **Graph** — DAG visualization of the step execution plan (React Flow).
    Nodes show step name, upstream, method, and optional badge. Edges show
    dependencies. Layout is by wave (execution level).
  - **Steps** — table with all step details (name, upstream, method, timeout,
    optional, depends_on).
  - **Route** — method, path, public flag, and a copyable `curl` example.

### Requests

Request explorer backed by the admin API's ring buffer:

- **Table** — time, composition, method + path, status badge, duration,
  partial badge.
- **Expandable rows** — step waterfall visualization showing each step's
  timing, wave, status, and HTTP response code as color-coded bars.
- **Limit selector** — view 50, 100, or 500 recent requests.

### Config

YAML configuration editor with validation:

- **CodeMirror** editor with YAML syntax highlighting.
- **Validate** — sends the YAML to the gateway's validate endpoint and
  displays errors.
- **Download** — download the config as `restitch.yaml`.
- **Load current** — regenerates YAML from the gateway's runtime state
  (secrets and comments are not included).

Config editing is validate-and-download only. Deployment stays git-driven;
reload is triggered explicitly via the Dashboard or admin API.

### Builder

Visual composition builder — a form-based interface for creating
compositions:

- **Composition meta** — name, path, method, public toggle.
- **Upstreams** — add/remove upstream definitions.
- **Steps** — add/remove steps with upstream, method, path, optional toggle,
  timeout, and dependencies.
- **Response** — body template as a YAML fragment.
- **Live preview** — generated YAML updates in real time.
- **DAG preview** — inferred dependency graph with wave layout.
- **Actions** — validate against the gateway, copy, or download.

## Development Workflow

For frontend development with hot reload:

```bash
# Terminal 1: mock upstream
go run ./cmd/mockupstream

# Terminal 2: gateway with config
restitch run -config examples/quickstart/restitch.yaml

# Terminal 3: Studio binary (proxies admin API)
go run ./cmd/restitch-studio

# Terminal 4: Vite dev server (hot reload, proxies to Studio)
cd studio && npm run dev
```

The Vite dev server (port 5173) proxies `/api` to Studio (port 3080), which
proxies to the gateway admin API (port 9090).

## Tech Stack

- [React 19](https://react.dev/) + TypeScript (strict mode)
- [Vite](https://vitejs.dev/) (build tool)
- [Tailwind CSS v4](https://tailwindcss.com/) (styling)
- [shadcn/ui](https://ui.shadcn.com/) (component library)
- [React Flow](https://reactflow.dev/) (DAG visualization)
- [CodeMirror 6](https://codemirror.net/) (YAML editor)
- [js-yaml](https://github.com/nodeca/js-yaml) (YAML generation)
- [Lucide React](https://lucide.dev/) (icons)
