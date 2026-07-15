# M20 — Config Registry & Centralized Management

Design spec for database-backed config management in Restitch Studio.

## Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | New dep: Goose (SQL migrations). No Huma, no pgx. | Schema will evolve across M20–M25. Goose is lightweight and embeds SQL files. Huma adds framework inconsistency; Postgres deferred until needed. |
| D2 | New package `internal/registry/` (not `internal/studio/` or `internal/admin/`). | Self-contained. Both Studio and gateway can import it. Avoids coupling to either binary. |
| D3 | Studio binary hosts registry API directly. | Studio = control plane. Gateway = data plane. Clean separation. Gateway polls Studio for bundles (M21). |
| D4 | One config = one YAML file (upstreams + compositions). | Matches how users think about config today. Bundle merges them. Teams own separate entries. |
| D5 | Versioned API: `/api/v1/configs/*`, pass-through `/api/*` for gateway admin. | Clear separation. v1 = Studio-native, unversioned = gateway proxy. |
| D6 | Timestamp-based Goose migration files (`YYYYMMDDHHmmss_name.sql`). | Avoids merge conflicts for concurrent development. |
| D7 | SQLite only (via `modernc.org/sqlite`, already a dep). Postgres deferred. | Single-binary simplicity. `database/sql` makes Postgres a future drop-in. |

## 1. Package Layout

```
internal/registry/
  types.go              — domain types
  store.go              — Store wrapping *sql.DB, all CRUD + bundle
  validator.go          — Validate() using gateway's own parser
  migrations/           — Goose-managed, embedded via go:embed
    20260715000000_registry_schema.sql
```

## 2. Domain Types

```go
// types.go

type Config struct {
    ID              string    `json:"id"`               // ULID
    Name            string    `json:"name"`
    Description     string    `json:"description"`
    Tags            []string  `json:"tags"`
    ActiveVersionID *int64    `json:"active_version_id,omitempty"`
    ActiveVersion   *int      `json:"active_version,omitempty"`
    CreatedAt       time.Time `json:"created_at"`
    UpdatedAt       time.Time `json:"updated_at"`
}

type ConfigVersion struct {
    ID            int64     `json:"id"`
    ConfigID      string    `json:"config_id"`
    VersionNumber int       `json:"version_number"`    // sequential: 1, 2, 3...
    YAMLContent   string    `json:"yaml_content"`
    Author        *string   `json:"author,omitempty"`
    ChangeMessage *string   `json:"change_message,omitempty"`
    CreatedAt     time.Time `json:"created_at"`
}

type ConfigWithContent struct {
    Config
    YAMLContent   string  `json:"yaml_content"`
    VersionNumber int     `json:"version_number"`
    Author        *string `json:"author,omitempty"`
    ChangeMessage *string `json:"change_message,omitempty"`
}

type BundledConfig struct {
    YAMLContent      string   `json:"yaml_content"`
    ETag             string   `json:"etag"`
    CompositionCount int      `json:"composition_count"`
    CompositionNames []string `json:"composition_names"`
}

type CreateConfigInput struct {
    Name          string   `json:"name"`
    Description   string   `json:"description"`
    Tags          []string `json:"tags"`
    YAMLContent   string   `json:"yaml_content"`
    Author        *string  `json:"author,omitempty"`
    ChangeMessage *string  `json:"change_message,omitempty"`
}

type UpdateConfigInput struct {
    YAMLContent   string  `json:"yaml_content"`
    Author        *string `json:"author,omitempty"`
    ChangeMessage *string `json:"change_message,omitempty"`
}

type UpdateConfigMetadataInput struct {
    Name        *string  `json:"name,omitempty"`
    Description *string  `json:"description,omitempty"`
    Tags        []string `json:"tags,omitempty"`
}

type ListConfigsParams struct {
    Cursor string
    Limit  int
}

type PageInfo struct {
    NextCursor *string `json:"next_cursor,omitempty"`
    HasMore    bool    `json:"has_more"`
}
```

## 3. Database Schema

Single Goose migration: `20260715000000_registry_schema.sql`

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

Studio opens its own SQLite file (default `./studio.db`) separate from the gateway's admin storage DB. The registry tables are managed by Goose; no schema overlap with the gateway's `timeseries_buckets`/`request_log` tables.

## 4. Store Operations

```go
// store.go

type Store struct {
    db      *sql.DB
    entropy io.Reader  // ULID monotonic entropy
}

func NewStore(db *sql.DB) *Store
func (s *Store) CreateConfig(ctx, CreateConfigInput) (*ConfigWithContent, error)
func (s *Store) GetConfig(ctx, id string) (*ConfigWithContent, error)
func (s *Store) ListConfigs(ctx, ListConfigsParams) ([]Config, PageInfo, error)
func (s *Store) UpdateConfigContent(ctx, id string, UpdateConfigInput) (*ConfigWithContent, error)
func (s *Store) UpdateConfigMetadata(ctx, id string, UpdateConfigMetadataInput) (*Config, error)
func (s *Store) DeleteConfig(ctx, id string) error
func (s *Store) ListVersions(ctx, configID string, limit int) ([]ConfigVersion, error)
func (s *Store) GetVersion(ctx, configID string, versionNumber int) (*ConfigVersion, error)
func (s *Store) SetActiveVersion(ctx, configID string, versionNumber int) error
func (s *Store) GetBundledConfig(ctx) (*BundledConfig, error)
```

Transaction boundaries:
- `CreateConfig`: BEGIN → INSERT config → INSERT version → UPDATE active_version_id → COMMIT
- `UpdateConfigContent`: BEGIN → get max version → INSERT version → UPDATE active_version_id → COMMIT
- `SetActiveVersion`: BEGIN → get version ID → UPDATE active_version_id → COMMIT
- `DeleteConfig`: single DELETE (CASCADE handles versions)

Bundle assembly:
1. Query all configs joined with their active version
2. Parse each YAML via `yaml.Unmarshal` into composition config structs
3. Merge upstreams and compositions into one map (error on name collisions)
4. Marshal merged config to YAML
5. ETag = first 16 hex chars of SHA-256 of sorted `id:version` pairs

## 5. Validation

```go
// validator.go

type ValidationResult struct {
    Valid  bool              `json:"valid"`
    Errors []ValidationError `json:"errors,omitempty"`
}

type ValidationError struct {
    Message string `json:"message"`
    Field   string `json:"field,omitempty"`
    Line    int    `json:"line,omitempty"`
    Column  int    `json:"column,omitempty"`
}

func Validate(yamlContent []byte) *ValidationResult
```

Three-stage validation:
1. YAML syntax — `yaml.Unmarshal` into `yaml.Node` for position info (line/column extraction)
2. Structure — `composition.ParseConfig(data)` which unmarshals + `validateAndApplyDefaults`. For server/admin config validation, also `gwconfig.LoadBytes(expanded, raw)` (already exists)
3. DAG + expressions — `composition.CompileConfig` with `SkipAuthInit: true`

Errors-only (no warnings). Config is either valid or not. All stages run to collect maximum errors (not fail-fast after stage 1).

Env expansion (`gwconfig.ExpandEnvStrict`) runs on the raw YAML before stage 2, same as `gwconfig.ReadAndExpand` does for file-based loading. Missing env vars produce validation errors rather than runtime failures.

## 6. Studio Binary Changes

### 6.1 New routing

```
/api/v1/configs              POST   → handleCreateConfig
/api/v1/configs              GET    → handleListConfigs
/api/v1/configs/validate     POST   → handleValidateConfig
/api/v1/configs/{id}         GET    → handleGetConfig
/api/v1/configs/{id}         PUT    → handleUpdateConfigContent
/api/v1/configs/{id}         PATCH  → handleUpdateConfigMetadata
/api/v1/configs/{id}         DELETE → handleDeleteConfig
/api/v1/configs/{id}/versions          GET  → handleListVersions
/api/v1/configs/{id}/versions/{v}/activate  POST → handleActivateVersion
/api/v1/registry/bundle      GET    → handleGetBundle

/api/*                        → reverse proxy to gateway admin (existing)
/metrics                      → reverse proxy to gateway admin (existing)
/*                            → SPA file server (existing)
```

### 6.2 New files

```
cmd/restitch-studio/
  main.go       — existing, modified: add flags, DB init, route registration
  api.go        — new: RegistryAPI struct + HTTP handlers
  db.go         — new: openDB(), runMigrations() using Goose + embedded SQL
```

### 6.3 New flags/env

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `-db-path` | `STUDIO_DB_PATH` | `./studio.db` | SQLite database path |
| `-no-migrate` | `STUDIO_NO_MIGRATE` | `false` | Skip auto-migration on startup |

### 6.4 Handler pattern

```go
type RegistryAPI struct {
    store *registry.Store
}

func (a *RegistryAPI) handleCreateConfig(w http.ResponseWriter, r *http.Request) {
    // 1. Decode JSON body → CreateConfigInput
    // 2. Validate YAML content → if invalid, 422 with ValidationResult
    // 3. store.CreateConfig()
    // 4. JSON response, 201 Created
}
```

Error responses: `{"error":"<message>"}` with appropriate HTTP status. Validation failures: 422 with `{"valid":false,"errors":[...]}`.

### 6.5 Startup flow

1. Parse flags + env overrides
2. Open SQLite at `-db-path`
3. Run Goose migrations (unless `-no-migrate`)
4. Create `registry.Store`
5. Create `RegistryAPI`
6. Build mux: v1 routes → proxy → SPA
7. Start HTTP server
8. Graceful shutdown: close DB

## 7. API Contract

### POST /api/v1/configs

Create a new config. YAML is validated before storing.

Request:
```json
{
  "name": "user-service",
  "description": "User service compositions",
  "tags": ["team-platform"],
  "yaml_content": "upstreams:\n  users-api:\n    url: ...",
  "author": "nate",
  "change_message": "Initial config"
}
```

Success: 201 → `ConfigWithContent`
Validation failure: 422 → `ValidationResult`

### GET /api/v1/configs?cursor=&limit=20

List configs (metadata only, no YAML content).

Response:
```json
{
  "items": [{"id": "...", "name": "...", ...}],
  "page_info": {"next_cursor": "...", "has_more": true}
}
```

### GET /api/v1/configs/{id}

Get config with active version's YAML content.

Response: `ConfigWithContent`
Not found: 404

### PUT /api/v1/configs/{id}

Update YAML content (creates new immutable version).

Request:
```json
{
  "yaml_content": "...",
  "author": "nate",
  "change_message": "Added retry config"
}
```

Success: 200 → `ConfigWithContent` (with new version number)
Validation failure: 422

### PATCH /api/v1/configs/{id}

Update metadata only (no new version created).

Request:
```json
{"name": "user-service-v2", "tags": ["team-platform", "v2"]}
```

### DELETE /api/v1/configs/{id}

Delete config and all versions. 204 No Content.

### GET /api/v1/configs/{id}/versions?limit=20

List versions for a config (newest first).

### POST /api/v1/configs/{id}/versions/{version}/activate

Set the active version pointer (rollback).

### POST /api/v1/configs/validate

Standalone validation (no persistence).

Request:
```json
{"yaml_content": "..."}
```

Response: always 200
```json
{"valid": true, "errors": []}
```
or
```json
{"valid": false, "errors": [{"message": "...", "line": 5, "field": "..."}]}
```

### GET /api/v1/registry/bundle

Merge all active configs into one YAML document.

Response headers: `ETag: "a1b2c3d4e5f6g7h8"`

Response body:
```json
{
  "yaml_content": "upstreams:\n  ...\ncompositions:\n  ...",
  "etag": "\"a1b2c3d4e5f6g7h8\"",
  "composition_count": 3,
  "composition_names": ["user-dashboard", "orders", "health"]
}
```

## 8. Testing Strategy

- `internal/registry/store_test.go` — CRUD operations, version incrementing, bundle merge, name collision detection, cursor pagination
- `internal/registry/validator_test.go` — valid YAML, invalid syntax, missing upstream ref, DAG cycle, position info extraction
- `cmd/restitch-studio/api_test.go` — HTTP handler tests via httptest: 201 on create, 422 on invalid YAML, 404 on missing, bundle endpoint returns merged YAML
- `cmd/restitch-studio/main_test.go` — existing proxy test updated for new routing (v1 routes handled locally, `/api/*` proxied)

## 9. New Dependency

```
go get github.com/pressly/goose/v3
```

Used only in `internal/registry/` and `cmd/restitch-studio/db.go` for embedding and running migrations. No other new dependencies.

## 10. Scope Boundaries

**In scope (M20):**
- Registry package with full CRUD + bundle
- Studio binary transformation (proxy → control plane)
- Goose migrations
- Validation using gateway's parser
- Tests

**Out of scope (future milestones):**
- Gateway polling the bundle endpoint (M21)
- Postgres support (future, when needed)
- Studio frontend changes for config management UI (future — frontend already has a Config page with validation)
- `restitch dev` orchestrator (M22)
