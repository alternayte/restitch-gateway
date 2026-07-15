# M20 — Config Registry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a database-backed config registry to Restitch Studio so compositions can be created, versioned, validated, and bundled through an HTTP API.

**Architecture:** New `internal/registry/` package provides domain types, a SQLite-backed store (via `database/sql` + `modernc.org/sqlite`), and a validator that reuses the gateway's own parser. The Studio binary (`cmd/restitch-studio/`) transforms from a pure reverse proxy into a control-plane server hosting registry endpoints at `/api/v1/*` while continuing to proxy gateway admin requests at `/api/*`.

**Tech Stack:** Go stdlib HTTP, `modernc.org/sqlite` (existing dep), `pressly/goose/v3` (new dep for SQL migrations), `oklog/ulid/v2` (existing dep for IDs).

## Global Constraints

- Go module: `github.com/restitch/restitch-gateway`, Go 1.25.6
- Only approved dependency addition: `github.com/pressly/goose/v3`
- Commit messages: `feat(M20): <task title>` or `fix:/test:/docs:` as appropriate
- After EVERY task: run Accept command and paste real output into commit or task report
- After EVERY task: append one row to `docs/plan-progress/LEDGER.md`
- Gate script changes require `gate: <description>` commit prefix and user approval
- NEVER edit `scripts/verify.sh`, `scripts/check-ledger.sh`, or `scripts/lib/`
- Design spec: `docs/superpowers/specs/2026-07-15-m20-config-registry-design.md`

## File Map

```
NEW FILES:
  scripts/gates/m20.sh                              — gate script (replaces placeholder)
  internal/registry/types.go                        — domain types (Config, ConfigVersion, etc.)
  internal/registry/store.go                        — Store with CRUD + bundle operations
  internal/registry/store_test.go                   — store unit tests
  internal/registry/validator.go                    — Validate() using gateway parser
  internal/registry/validator_test.go               — validator unit tests
  internal/registry/migrations/20260715000000_registry_schema.sql — Goose migration
  cmd/restitch-studio/db.go                         — openDB + runMigrations
  cmd/restitch-studio/api.go                        — RegistryAPI HTTP handlers
  cmd/restitch-studio/api_test.go                   — handler integration tests

MODIFIED FILES:
  go.mod / go.sum                                   — add goose dep
  cmd/restitch-studio/main.go                       — add flags, DB init, v1 route registration
  cmd/restitch-studio/main_test.go                  — update proxy test for new routing
```

---

### Task 0: Write the M20 gate script

**Files:**
- Modify: `scripts/gates/m20.sh` (replace placeholder)

**Interfaces:**
- Consumes: harness library (`scripts/lib/harness.sh` — `h_init`, `h_task`, `h_run`, `h_start_studio`, `h_assert_status`, `h_finish`, `h_build`)
- Produces: gate script that `scripts/verify.sh M20` can call

This task replaces the placeholder gate script with a real one encoding the PLAN.md M20 verification gate. Per CLAUDE.md rule 2, this must be done first and get user approval before implementing features.

- [ ] **Step 1: Write the gate script**

Replace `scripts/gates/m20.sh` with a script that:
- Checks T20.1–T20.6 files/tests exist
- Runs `go test ./internal/registry/... -count=1 -race`
- Runs `go test ./cmd/restitch-studio/... -count=1`
- Builds the studio binary
- Starts Studio on an ephemeral port with a temp DB (`-db-path $H_TMP/test.db`)
- Creates a config via `curl -X POST localhost:$STUDIO_PORT/api/v1/configs` with valid YAML
- Asserts 201 status
- Lists configs via `curl localhost:$STUDIO_PORT/api/v1/configs`
- Asserts the list contains the created config
- Gets the bundle via `curl localhost:$STUDIO_PORT/api/v1/registry/bundle`
- Asserts bundle contains composition names and ETag header
- Posts invalid YAML to `/api/v1/configs/validate` and asserts the response has `"valid":false`
- Verifies proxy pass-through still works (requires gateway running — mark as MANUAL if gateway not available)

The valid YAML for the smoke test should be a minimal composition:
```yaml
upstreams:
  mock:
    url: "http://localhost:8081"
compositions:
  test-comp:
    path: "/test"
    method: GET
    steps:
      - name: s1
        upstream: mock
        path: "/users/1"
    response:
      body:
        result: "{{ steps.s1.body }}"
```

- [ ] **Step 2: Verify the gate script is syntactically valid**

Run: `bash -n scripts/gates/m20.sh`
Expected: exit 0, no output

- [ ] **Step 3: Commit with gate prefix**

```bash
git add scripts/gates/m20.sh
git commit -m "$(cat <<'EOF'
gate: write M20 config registry verification gate

Replaces placeholder with real gate script encoding PLAN.md
M20 verification: registry CRUD smoke, bundle endpoint,
validation endpoint, unit tests.
EOF
)"
```

**Accept:** `bash -n scripts/gates/m20.sh` exits 0. Gate script is syntactically valid bash. **User must approve this gate script before proceeding to Task 1.**

---

### Task 1: Add Goose dependency + domain types + migration (T20.1, T20.6)

**Files:**
- Create: `internal/registry/types.go`
- Create: `internal/registry/migrations/20260715000000_registry_schema.sql`
- Modify: `go.mod` / `go.sum`

**Interfaces:**
- Consumes: `oklog/ulid/v2` (existing dep for ULID generation)
- Produces: All types consumed by Tasks 2–5: `Config`, `ConfigVersion`, `ConfigWithContent`, `BundledConfig`, `CreateConfigInput`, `UpdateConfigInput`, `UpdateConfigMetadataInput`, `ListConfigsParams`, `PageInfo`, `ValidationResult`, `ValidationError`

- [ ] **Step 1: Add goose dependency**

Run: `go get github.com/pressly/goose/v3`

- [ ] **Step 2: Create the Goose migration file**

Create `internal/registry/migrations/20260715000000_registry_schema.sql` with the schema from the design spec (§3):

```sql
-- +goose Up
CREATE TABLE configs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    tags TEXT NOT NULL DEFAULT '[]',
    active_version_id INTEGER,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE config_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    config_id TEXT NOT NULL REFERENCES configs(id) ON DELETE CASCADE,
    version_number INTEGER NOT NULL,
    yaml_content TEXT NOT NULL,
    author TEXT,
    change_message TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(config_id, version_number)
);

CREATE INDEX idx_config_versions_config_id ON config_versions(config_id);
CREATE INDEX idx_config_versions_lookup ON config_versions(config_id, version_number DESC);

-- +goose Down
DROP TABLE IF EXISTS config_versions;
DROP TABLE IF EXISTS configs;
```

- [ ] **Step 3: Create `internal/registry/types.go`**

Write the domain types exactly as specified in design spec §2. Include all types: `Config`, `ConfigVersion`, `ConfigWithContent`, `BundledConfig`, `CreateConfigInput`, `UpdateConfigInput`, `UpdateConfigMetadataInput`, `ListConfigsParams`, `PageInfo`, `ValidationResult`, `ValidationError`.

- [ ] **Step 4: Verify it compiles**

Run: `go build ./internal/registry/...`
Expected: exit 0

- [ ] **Step 5: Commit**

```bash
git add internal/registry/ go.mod go.sum
git commit -m "feat(M20): add registry domain types and migration schema (T20.1, T20.6)"
```

**Accept:** `go build ./internal/registry/...` exits 0. `test -f internal/registry/types.go && test -f internal/registry/migrations/20260715000000_registry_schema.sql` exits 0.

---

### Task 2: Registry store with CRUD + bundle (T20.2)

**Files:**
- Create: `internal/registry/store.go`
- Create: `internal/registry/store_test.go`

**Interfaces:**
- Consumes: types from Task 1 (`Config`, `ConfigVersion`, `ConfigWithContent`, `BundledConfig`, all input types, `PageInfo`); `database/sql`; `oklog/ulid/v2`; `crypto/sha256`; `composition.Config` / `composition.Upstream` / `composition.Composition` (for bundle YAML merge)
- Produces: `Store` struct and methods consumed by Tasks 4 and 5:
  - `func NewStore(db *sql.DB) *Store`
  - `func (s *Store) CreateConfig(ctx context.Context, input CreateConfigInput) (*ConfigWithContent, error)`
  - `func (s *Store) GetConfig(ctx context.Context, id string) (*ConfigWithContent, error)`
  - `func (s *Store) ListConfigs(ctx context.Context, params ListConfigsParams) ([]Config, PageInfo, error)`
  - `func (s *Store) UpdateConfigContent(ctx context.Context, id string, input UpdateConfigInput) (*ConfigWithContent, error)`
  - `func (s *Store) UpdateConfigMetadata(ctx context.Context, id string, input UpdateConfigMetadataInput) (*Config, error)`
  - `func (s *Store) DeleteConfig(ctx context.Context, id string) error`
  - `func (s *Store) ListVersions(ctx context.Context, configID string, limit int) ([]ConfigVersion, error)`
  - `func (s *Store) GetVersion(ctx context.Context, configID string, versionNumber int) (*ConfigVersion, error)`
  - `func (s *Store) SetActiveVersion(ctx context.Context, configID string, versionNumber int) error`
  - `func (s *Store) GetBundledConfig(ctx context.Context) (*BundledConfig, error)`

- [ ] **Step 1: Write store tests**

Create `internal/registry/store_test.go` with a test helper that opens an in-memory SQLite DB (`:memory:`), runs the Goose migration using the embedded SQL, and returns a `*Store`. Then write table-driven tests covering:

1. `TestStore_CreateAndGet` — create a config, get it back, assert all fields match
2. `TestStore_ListConfigs` — create 3 configs, list with limit 2 → has_more=true + next_cursor; list with cursor → gets remaining
3. `TestStore_UpdateContent` — create config, update content → version_number increments to 2, YAML is new content
4. `TestStore_UpdateMetadata` — create config, patch name → name changes, no new version
5. `TestStore_Delete` — create config, delete → get returns nil
6. `TestStore_Versions` — create config, update twice → ListVersions returns 3 versions newest-first
7. `TestStore_SetActiveVersion` — create config, update (v2 active), set active to v1 → GetConfig returns v1 YAML
8. `TestStore_BundleConfig` — create 2 configs with different compositions, bundle → merged YAML contains both, ETag is non-empty, composition_count=2
9. `TestStore_BundleNameCollision` — create 2 configs with same composition name → GetBundledConfig returns error

The test helper function:
```go
func testStore(t *testing.T) *Store {
    t.Helper()
    db, err := sql.Open("sqlite", ":memory:")
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { db.Close() })

    goose.SetBaseFS(embedMigrations)
    if err := goose.SetDialect("sqlite3"); err != nil {
        t.Fatal(err)
    }
    if err := goose.Up(db, "migrations"); err != nil {
        t.Fatal(err)
    }
    return NewStore(db)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/registry/ -run TestStore -v`
Expected: FAIL (Store not implemented)

- [ ] **Step 3: Implement the Store**

Write `internal/registry/store.go` with all CRUD methods per the design spec §4. Key implementation details:
- `NewStore` creates a monotonic ULID entropy source with `ulid.Monotonic(rand.Reader, 0)`
- `CreateConfig` uses a transaction: INSERT config → INSERT version (number=1) → UPDATE active_version_id
- `UpdateConfigContent` uses a transaction: get max version_number → INSERT new version → UPDATE active_version_id
- `GetConfig` JOINs configs with config_versions on active_version_id
- `ListConfigs` uses cursor-based pagination (fetch limit+1, detect has_more)
- `DeleteConfig` relies on CASCADE for version cleanup
- `GetBundledConfig` queries all configs joined with active versions, parses each YAML into `composition.Config`, merges upstreams/compositions maps (error on name collision), marshals back to YAML, computes ETag from SHA-256 of sorted `id:version` pairs
- Tags stored as JSON text in SQLite, marshaled/unmarshaled with `encoding/json`
- Embed migrations: `//go:embed migrations/*.sql` on a package-level `var embedMigrations embed.FS`

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/registry/ -run TestStore -v -count=1`
Expected: all 9 tests PASS

- [ ] **Step 5: Run with race detector**

Run: `go test ./internal/registry/ -run TestStore -race -count=1`
Expected: PASS, no race conditions

- [ ] **Step 6: Commit**

```bash
git add internal/registry/store.go internal/registry/store_test.go
git commit -m "feat(M20): implement registry store with CRUD and bundle (T20.2)"
```

**Accept:** `go test ./internal/registry/ -run TestStore -v -count=1 -race` — all tests PASS.

---

### Task 3: Validation layer (T20.3)

**Files:**
- Create: `internal/registry/validator.go`
- Create: `internal/registry/validator_test.go`

**Interfaces:**
- Consumes: `ValidationResult`, `ValidationError` from Task 1; `composition.ParseConfig([]byte) (*Config, error)`; `composition.CompileConfig(ctx, *Config, CompileOptions) (*CompiledConfig, error)` with `SkipAuthInit: true`; `gwconfig.ExpandEnvStrict(string) (string, error)`
- Produces: `func Validate(yamlContent []byte) *ValidationResult` — consumed by Task 4 (API handlers)

- [ ] **Step 1: Write validator tests**

Create `internal/registry/validator_test.go` with table-driven tests:

1. `TestValidate_ValidConfig` — valid minimal YAML → `Valid: true, Errors: []`
2. `TestValidate_InvalidYAMLSyntax` — `"compositions: [broken"` → `Valid: false`, error has line number
3. `TestValidate_MissingUpstreamRef` — step references non-existent upstream → `Valid: false`, error message mentions the upstream name
4. `TestValidate_EmptyInput` — empty bytes → `Valid: true` (valid empty config)
5. `TestValidate_InvalidExpression` — `"{{ bad ++ expr }}"` in a step path → `Valid: false`
6. `TestValidate_EnvVarExpansion` — YAML with `${SOME_VAR}` where var is unset → `Valid: false`, error mentions SOME_VAR

Test YAML fixtures:
```go
validYAML := `
upstreams:
  mock:
    url: "http://localhost:8081"
compositions:
  test:
    path: "/test"
    method: GET
    steps:
      - name: s1
        upstream: mock
        path: "/users/1"
    response:
      body:
        result: "{{ steps.s1.body }}"
`

invalidSyntaxYAML := `compositions: [broken`

missingUpstreamYAML := `
compositions:
  test:
    path: "/test"
    steps:
      - name: s1
        upstream: nonexistent
        path: "/x"
    response:
      body: "{{ steps.s1.body }}"
`
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/registry/ -run TestValidate -v`
Expected: FAIL (Validate not implemented)

- [ ] **Step 3: Implement the validator**

Write `internal/registry/validator.go` implementing `Validate(yamlContent []byte) *ValidationResult`:

Three-stage validation:
1. YAML syntax: `yaml.Unmarshal(yamlContent, &yaml.Node{})` — if error, extract line/column from error message using regex `line (\d+)` and `column (\d+)`, return immediately
2. Env expansion + structure: `gwconfig.ExpandEnvStrict(string(yamlContent))` — if error, collect it. Then `composition.ParseConfig(expanded)` — if error, collect it with field path extraction
3. DAG + expressions: `composition.CompileConfig(context.Background(), cfg, composition.CompileOptions{SkipAuthInit: true})` — if error, collect it

Field path extraction from error messages: parse patterns like `"composition X: step Y: ..."` into `"compositions.X.steps.Y"` using regex.

For env var errors: catch them in stage 2 and report as validation errors (don't crash).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/registry/ -run TestValidate -v -count=1`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/registry/validator.go internal/registry/validator_test.go
git commit -m "feat(M20): implement config validation using gateway parser (T20.3)"
```

**Accept:** `go test ./internal/registry/ -run TestValidate -v -count=1` — all tests PASS.

---

### Task 4: Studio binary: DB init + API handlers + routing (T20.4, T20.5)

**Files:**
- Create: `cmd/restitch-studio/db.go`
- Create: `cmd/restitch-studio/api.go`
- Create: `cmd/restitch-studio/api_test.go`
- Modify: `cmd/restitch-studio/main.go`
- Modify: `cmd/restitch-studio/main_test.go`

**Interfaces:**
- Consumes: `registry.Store` (Task 2), `registry.Validate` (Task 3), all domain types (Task 1)
- Produces: Working Studio binary with `/api/v1/configs/*` and `/api/v1/registry/bundle` endpoints

- [ ] **Step 1: Write `cmd/restitch-studio/db.go`**

Create database initialization:
```go
package main

import (
    "database/sql"
    "embed"
    "fmt"

    "github.com/pressly/goose/v3"
    _ "modernc.org/sqlite"
)

//go:embed ../../internal/registry/migrations/*.sql
var registryMigrations embed.FS

func openDB(path string) (*sql.DB, error) {
    db, err := sql.Open("sqlite", path)
    if err != nil {
        return nil, fmt.Errorf("open database: %w", err)
    }
    // SQLite: single writer, WAL mode for concurrent reads
    if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
        db.Close()
        return nil, fmt.Errorf("set WAL mode: %w", err)
    }
    return db, nil
}

func runMigrations(db *sql.DB) error {
    goose.SetBaseFS(registryMigrations)
    if err := goose.SetDialect("sqlite3"); err != nil {
        return fmt.Errorf("set dialect: %w", err)
    }
    if err := goose.Up(db, "internal/registry/migrations"); err != nil {
        return fmt.Errorf("run migrations: %w", err)
    }
    return nil
}
```

Note: The embed path `../../internal/registry/migrations/*.sql` is relative to the source file. The goose directory path `"internal/registry/migrations"` must match the FS structure inside the embedded filesystem. Verify the embed path resolves correctly by checking the directory structure from `cmd/restitch-studio/` — two levels up reaches the repo root, then into `internal/registry/migrations/`.

- [ ] **Step 2: Write `cmd/restitch-studio/api.go`**

Create the HTTP handlers struct and all endpoint handlers:

```go
package main

import (
    "encoding/json"
    "net/http"
    "strconv"
    "strings"

    "github.com/restitch/restitch-gateway/internal/registry"
)

type RegistryAPI struct {
    store *registry.Store
}

func NewRegistryAPI(store *registry.Store) *RegistryAPI {
    return &RegistryAPI{store: store}
}
```

Implement each handler following the pattern:
- `handleCreateConfig` — decode JSON → validate YAML → store.CreateConfig → 201
- `handleListConfigs` — parse `cursor`/`limit` query params → store.ListConfigs → 200 with `{items, page_info}`
- `handleGetConfig` — extract `{id}` from path → store.GetConfig → 200 or 404
- `handleUpdateConfigContent` — extract `{id}`, decode JSON → validate YAML → store.UpdateConfigContent → 200
- `handleUpdateConfigMetadata` — extract `{id}`, decode JSON → store.UpdateConfigMetadata → 200
- `handleDeleteConfig` — extract `{id}` → store.DeleteConfig → 204
- `handleListVersions` — extract `{id}`, parse `limit` → store.ListVersions → 200
- `handleActivateVersion` — extract `{id}` and `{version}` from path → store.SetActiveVersion → 200 with updated config
- `handleValidateConfig` — decode JSON → registry.Validate → 200 (always)
- `handleGetBundle` — store.GetBundledConfig → 200 with ETag header

For path parameter extraction with Go 1.22 `http.ServeMux`, use `r.PathValue("id")` and `r.PathValue("version")`.

Add helper functions:
```go
func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
    writeJSON(w, status, map[string]string{"error": message})
}
```

For validation failures (422), return the `ValidationResult` directly as the response body.

- [ ] **Step 3: Modify `cmd/restitch-studio/main.go`**

Add new flags and DB startup:
```go
dbPath := flag.String("db-path", "./studio.db", "SQLite database path")
noMigrate := flag.Bool("no-migrate", false, "skip auto-migration on startup")
```

Add env overrides:
```go
if v := os.Getenv("STUDIO_DB_PATH"); v != "" {
    *dbPath = v
}
if os.Getenv("STUDIO_NO_MIGRATE") == "true" {
    *noMigrate = true
}
```

After flag parsing, before building the mux:
```go
db, err := openDB(*dbPath)
if err != nil {
    log.Fatal(err)
}
defer db.Close()

if !*noMigrate {
    if err := runMigrations(db); err != nil {
        log.Fatal(err)
    }
}

store := registry.NewStore(db)
registryAPI := NewRegistryAPI(store)
```

Modify `buildMux` to accept `*RegistryAPI` parameter and register v1 routes BEFORE the proxy catch-all:
```go
func buildMux(gatewayAdminURL, adminKey string, registryAPI *RegistryAPI) *http.ServeMux {
    // ... existing proxy setup ...

    mux := http.NewServeMux()

    // V1 routes (Studio-native) — registered before proxy catch-all
    if registryAPI != nil {
        mux.HandleFunc("POST /api/v1/configs/validate", registryAPI.handleValidateConfig)
        mux.HandleFunc("POST /api/v1/configs", registryAPI.handleCreateConfig)
        mux.HandleFunc("GET /api/v1/configs", registryAPI.handleListConfigs)
        mux.HandleFunc("GET /api/v1/configs/{id}", registryAPI.handleGetConfig)
        mux.HandleFunc("PUT /api/v1/configs/{id}", registryAPI.handleUpdateConfigContent)
        mux.HandleFunc("PATCH /api/v1/configs/{id}", registryAPI.handleUpdateConfigMetadata)
        mux.HandleFunc("DELETE /api/v1/configs/{id}", registryAPI.handleDeleteConfig)
        mux.HandleFunc("GET /api/v1/configs/{id}/versions", registryAPI.handleListVersions)
        mux.HandleFunc("POST /api/v1/configs/{id}/versions/{version}/activate", registryAPI.handleActivateVersion)
        mux.HandleFunc("GET /api/v1/registry/bundle", registryAPI.handleGetBundle)
    }

    // Proxy routes (gateway admin pass-through)
    mux.Handle("/api/", proxy)
    mux.Handle("/metrics", proxy)

    // SPA file server
    // ... existing code ...
}
```

Route registration order matters: `/api/v1/configs/validate` must be registered before `/api/v1/configs/{id}` to avoid the `POST /api/v1/configs/validate` being matched by `{id}=validate`. With Go 1.22 ServeMux method-based patterns, `POST /api/v1/configs/validate` (exact method+path) takes priority over `GET /api/v1/configs/{id}` (different method), so this only matters if `validate` could match the `{id}` wildcard on GET — register the more specific pattern first to be safe.

- [ ] **Step 4: Update `cmd/restitch-studio/main_test.go`**

Update existing tests to pass `nil` for `registryAPI` in `buildMux` (preserving existing proxy/SPA tests), then add new tests:

```go
// Update existing calls:
mux := buildMux(admin.URL, adminKey, nil)
// and:
mux := buildMux("http://localhost:9999", "", nil)
```

- [ ] **Step 5: Write `cmd/restitch-studio/api_test.go`**

Integration tests using httptest that open a real in-memory SQLite DB, run migrations, create a Store, create a RegistryAPI, build a mux, and send HTTP requests:

1. `TestAPI_CreateConfig_201` — POST valid YAML → 201, response has `id`, `version_number: 1`
2. `TestAPI_CreateConfig_422` — POST invalid YAML → 422, response has `valid: false`
3. `TestAPI_GetConfig_200` — create then GET → 200 with YAML content
4. `TestAPI_GetConfig_404` — GET nonexistent ID → 404
5. `TestAPI_ListConfigs` — create 2, list → 200 with 2 items
6. `TestAPI_UpdateContent` — create then PUT new YAML → 200, version_number: 2
7. `TestAPI_DeleteConfig` — create then DELETE → 204, GET → 404
8. `TestAPI_ValidateConfig_Valid` — POST valid YAML to `/validate` → 200 with `valid: true`
9. `TestAPI_ValidateConfig_Invalid` — POST broken YAML → 200 with `valid: false`
10. `TestAPI_Bundle` — create 2 configs, GET bundle → 200 with merged YAML and ETag header
11. `TestAPI_ListVersions` — create, update twice, list versions → 3 versions
12. `TestAPI_ActivateVersion` — create, update (v2), activate v1 → GET returns v1 YAML

Test helper:
```go
func testMux(t *testing.T) *http.ServeMux {
    t.Helper()
    db, err := sql.Open("sqlite", ":memory:")
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { db.Close() })

    if err := runMigrations(db); err != nil {
        t.Fatal(err)
    }
    store := registry.NewStore(db)
    api := NewRegistryAPI(store)
    return buildMux("http://localhost:9999", "", api)
}
```

- [ ] **Step 6: Run all tests**

Run: `go test ./cmd/restitch-studio/... -v -count=1`
Expected: all tests PASS (existing + new)

Run: `go test ./internal/registry/... -v -count=1`
Expected: all tests PASS

- [ ] **Step 7: Build the binary**

Run: `go build -o bin/restitch-studio ./cmd/restitch-studio`
Expected: exit 0, binary produced

- [ ] **Step 8: Quick manual smoke test**

Run:
```bash
./bin/restitch-studio -db-path /tmp/test-m20.db &
STUDIO_PID=$!
sleep 1

# Create a config
curl -s -X POST http://localhost:3080/api/v1/configs \
  -H 'Content-Type: application/json' \
  -d '{"name":"test","yaml_content":"upstreams:\n  mock:\n    url: \"http://localhost:8081\"\ncompositions:\n  test:\n    path: \"/test\"\n    method: GET\n    steps:\n      - name: s1\n        upstream: mock\n        path: \"/users/1\"\n    response:\n      body:\n        result: \"{{ steps.s1.body }}\""}' | python3 -m json.tool

# List configs
curl -s http://localhost:3080/api/v1/configs | python3 -m json.tool

# Get bundle
curl -si http://localhost:3080/api/v1/registry/bundle | head -20

kill $STUDIO_PID
rm /tmp/test-m20.db
```

Expected: 201 with config object, list shows 1 item, bundle returns merged YAML with ETag header.

- [ ] **Step 9: Commit**

```bash
git add cmd/restitch-studio/ internal/registry/
git commit -m "feat(M20): Studio registry API with CRUD, validation, and bundle endpoints (T20.4, T20.5)"
```

**Accept:** `go test ./cmd/restitch-studio/... ./internal/registry/... -v -count=1` — all tests PASS. `go build -o bin/restitch-studio ./cmd/restitch-studio` — exit 0.

---

### Task 5: Full test suite + gate verification

**Files:**
- No new files — runs existing tests and the gate script

**Interfaces:**
- Consumes: everything from Tasks 0–4

- [ ] **Step 1: Run full test suite with race detector**

Run: `go test ./... -count=1 -race`
Expected: all PASS

- [ ] **Step 2: Run go vet**

Run: `go vet ./...`
Expected: exit 0

- [ ] **Step 3: Run the M20 gate script**

Run: `scripts/verify.sh M20`
Expected: `RESULT M20: PASS`

If there are MANUAL items, list them for the user. If the gate fails, fix the issues before proceeding.

- [ ] **Step 4: Commit evidence**

The gate script auto-appends ledger rows and writes evidence files. Verify:
```bash
tail -20 docs/plan-progress/LEDGER.md
ls -la docs/plan-progress/evidence/*M20*
```

Commit the evidence:
```bash
git add docs/plan-progress/
git commit -m "docs: record M20.gate and evidence"
```

- [ ] **Step 5: Run check-ledger**

Run: `scripts/check-ledger.sh`
Expected: exit 0 (or list only future tasks not in scope)

**Accept:** `scripts/verify.sh M20` prints `RESULT M20: PASS`. `scripts/check-ledger.sh` exits 0.
