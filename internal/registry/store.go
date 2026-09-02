package registry

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/oklog/ulid/v2"
	"gopkg.in/yaml.v3"

	"github.com/alternayte/restitch-gateway/internal/composition"
)

// defaultListLimit is used when ListConfigsParams.Limit is not set.
const defaultListLimit = 20

// maxListLimit caps the number of configs returned by a single ListConfigs call.
const maxListLimit = 100

// Store provides data access for config registry operations backed by SQLite.
type Store struct {
	db      *sql.DB
	entropy io.Reader // ULID monotonic entropy source
}

// NewStore creates a new Store instance wrapping db.
func NewStore(db *sql.DB) *Store {
	return &Store{
		db:      db,
		entropy: ulid.Monotonic(rand.Reader, 0),
	}
}

// newID generates a new monotonic ULID string for use as a config ID.
func (s *Store) newID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), s.entropy).String()
}

// CreateConfig creates a new config with its first version (version_number=1)
// and sets it as the active version. All writes happen in a single transaction.
func (s *Store) CreateConfig(ctx context.Context, input CreateConfigInput) (*ConfigWithContent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	id := s.newID()

	tags := input.Tags
	if tags == nil {
		tags = []string{}
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return nil, fmt.Errorf("marshal tags: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO configs (id, name, description, tags)
		VALUES (?, ?, ?, ?)
	`, id, input.Name, input.Description, string(tagsJSON)); err != nil {
		return nil, fmt.Errorf("insert config: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO config_versions (config_id, version_number, yaml_content, author, change_message)
		VALUES (?, 1, ?, ?, ?)
	`, id, input.YAMLContent, input.Author, input.ChangeMessage)
	if err != nil {
		return nil, fmt.Errorf("insert version: %w", err)
	}
	versionID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get last insert id: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE configs SET active_version_id = ? WHERE id = ?
	`, versionID, id); err != nil {
		return nil, fmt.Errorf("update active version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return s.GetConfig(ctx, id)
}

// GetConfig retrieves a config with its active version's content.
// Returns nil, nil if the config does not exist.
func (s *Store) GetConfig(ctx context.Context, id string) (*ConfigWithContent, error) {
	var result ConfigWithContent
	var tagsJSON string
	var versionID int64

	err := s.db.QueryRowContext(ctx, `
		SELECT
			c.id, c.name, c.description, c.tags, c.created_at, c.updated_at,
			v.id, v.version_number, v.yaml_content, v.author, v.change_message
		FROM configs c
		JOIN config_versions v ON c.active_version_id = v.id
		WHERE c.id = ?
	`, id).Scan(
		&result.ID, &result.Name, &result.Description, &tagsJSON, &result.CreatedAt, &result.UpdatedAt,
		&versionID, &result.VersionNumber, &result.YAMLContent, &result.Author, &result.ChangeMessage,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query config: %w", err)
	}

	if err := json.Unmarshal([]byte(tagsJSON), &result.Tags); err != nil {
		return nil, fmt.Errorf("unmarshal tags: %w", err)
	}

	result.ActiveVersionID = &versionID
	versionNumber := result.VersionNumber
	result.ActiveVersion = &versionNumber

	return &result, nil
}

// ListConfigs lists configs (metadata only, no YAML content) ordered by ID
// with cursor-based pagination.
func (s *Store) ListConfigs(ctx context.Context, params ListConfigsParams) ([]Config, PageInfo, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	var rows *sql.Rows
	var err error
	if params.Cursor != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, name, description, tags, active_version_id, created_at, updated_at
			FROM configs
			WHERE id > ?
			ORDER BY id
			LIMIT ?
		`, params.Cursor, limit+1)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, name, description, tags, active_version_id, created_at, updated_at
			FROM configs
			ORDER BY id
			LIMIT ?
		`, limit+1)
	}
	if err != nil {
		return nil, PageInfo{}, fmt.Errorf("query configs: %w", err)
	}
	defer rows.Close()

	configs := make([]Config, 0, limit)
	for rows.Next() {
		var c Config
		var tagsJSON string
		var activeVersionID sql.NullInt64

		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &tagsJSON, &activeVersionID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, PageInfo{}, fmt.Errorf("scan config: %w", err)
		}
		if err := json.Unmarshal([]byte(tagsJSON), &c.Tags); err != nil {
			return nil, PageInfo{}, fmt.Errorf("unmarshal tags: %w", err)
		}
		if activeVersionID.Valid {
			c.ActiveVersionID = &activeVersionID.Int64
		}
		configs = append(configs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, PageInfo{}, fmt.Errorf("rows error: %w", err)
	}

	var pageInfo PageInfo
	if len(configs) > limit {
		nextCursor := configs[limit-1].ID
		pageInfo.NextCursor = &nextCursor
		pageInfo.HasMore = true
		configs = configs[:limit]
	}

	return configs, pageInfo, nil
}

// UpdateConfigContent creates a new immutable version with the given content
// and sets it as the active version.
func (s *Store) UpdateConfigContent(ctx context.Context, id string, input UpdateConfigInput) (*ConfigWithContent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var maxVersion int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version_number), 0) FROM config_versions WHERE config_id = ?
	`, id).Scan(&maxVersion)
	if err != nil {
		return nil, fmt.Errorf("get max version: %w", err)
	}
	if maxVersion == 0 {
		return nil, fmt.Errorf("config not found: %s", id)
	}

	newVersionNumber := maxVersion + 1

	result, err := tx.ExecContext(ctx, `
		INSERT INTO config_versions (config_id, version_number, yaml_content, author, change_message)
		VALUES (?, ?, ?, ?, ?)
	`, id, newVersionNumber, input.YAMLContent, input.Author, input.ChangeMessage)
	if err != nil {
		return nil, fmt.Errorf("insert version: %w", err)
	}
	versionID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get last insert id: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE configs SET active_version_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, versionID, id); err != nil {
		return nil, fmt.Errorf("update active version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return s.GetConfig(ctx, id)
}

// UpdateConfigMetadata updates name/description/tags without creating a new version.
// Nil fields in input are left unchanged.
func (s *Store) UpdateConfigMetadata(ctx context.Context, id string, input UpdateConfigMetadataInput) (*Config, error) {
	setClauses := make([]string, 0, 4)
	args := make([]any, 0, 4)

	if input.Name != nil {
		setClauses = append(setClauses, "name = ?")
		args = append(args, *input.Name)
	}
	if input.Description != nil {
		setClauses = append(setClauses, "description = ?")
		args = append(args, *input.Description)
	}
	if input.Tags != nil {
		tagsJSON, err := json.Marshal(input.Tags)
		if err != nil {
			return nil, fmt.Errorf("marshal tags: %w", err)
		}
		setClauses = append(setClauses, "tags = ?")
		args = append(args, string(tagsJSON))
	}

	if len(setClauses) > 0 {
		setClauses = append(setClauses, "updated_at = CURRENT_TIMESTAMP")
		query := "UPDATE configs SET "
		for i, clause := range setClauses {
			if i > 0 {
				query += ", "
			}
			query += clause
		}
		query += " WHERE id = ?"
		args = append(args, id)

		result, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("update config metadata: %w", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("get rows affected: %w", err)
		}
		if rowsAffected == 0 {
			return nil, fmt.Errorf("config not found: %s", id)
		}
	}

	return s.getConfigMetadata(ctx, id)
}

// getConfigMetadata retrieves config metadata (no YAML content).
func (s *Store) getConfigMetadata(ctx context.Context, id string) (*Config, error) {
	var c Config
	var tagsJSON string
	var activeVersionID sql.NullInt64

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, tags, active_version_id, created_at, updated_at
		FROM configs
		WHERE id = ?
	`, id).Scan(&c.ID, &c.Name, &c.Description, &tagsJSON, &activeVersionID, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("config not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("query config: %w", err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &c.Tags); err != nil {
		return nil, fmt.Errorf("unmarshal tags: %w", err)
	}
	if activeVersionID.Valid {
		c.ActiveVersionID = &activeVersionID.Int64
	}
	return &c, nil
}

// DeleteConfig deletes a config and all its versions (CASCADE).
func (s *Store) DeleteConfig(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM configs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete config: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("config not found: %s", id)
	}
	return nil
}

// ListVersions lists all versions for a config, newest first.
func (s *Store) ListVersions(ctx context.Context, configID string, limit int) ([]ConfigVersion, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, config_id, version_number, yaml_content, author, change_message, created_at
		FROM config_versions
		WHERE config_id = ?
		ORDER BY version_number DESC
		LIMIT ?
	`, configID, limit)
	if err != nil {
		return nil, fmt.Errorf("query versions: %w", err)
	}
	defer rows.Close()

	versions := make([]ConfigVersion, 0)
	for rows.Next() {
		var v ConfigVersion
		if err := rows.Scan(&v.ID, &v.ConfigID, &v.VersionNumber, &v.YAMLContent, &v.Author, &v.ChangeMessage, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return versions, nil
}

// GetVersion retrieves a specific version by version_number.
// Returns nil, nil if not found.
func (s *Store) GetVersion(ctx context.Context, configID string, versionNumber int) (*ConfigVersion, error) {
	var v ConfigVersion
	err := s.db.QueryRowContext(ctx, `
		SELECT id, config_id, version_number, yaml_content, author, change_message, created_at
		FROM config_versions
		WHERE config_id = ? AND version_number = ?
	`, configID, versionNumber).Scan(&v.ID, &v.ConfigID, &v.VersionNumber, &v.YAMLContent, &v.Author, &v.ChangeMessage, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query version: %w", err)
	}
	return &v, nil
}

// SetActiveVersion sets the active version pointer for a config to a specific version_number.
func (s *Store) SetActiveVersion(ctx context.Context, configID string, versionNumber int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var versionID int64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM config_versions WHERE config_id = ? AND version_number = ?
	`, configID, versionNumber).Scan(&versionID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("version not found: config=%s version=%d", configID, versionNumber)
	}
	if err != nil {
		return fmt.Errorf("query version: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE configs SET active_version_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, versionID, configID)
	if err != nil {
		return fmt.Errorf("update active version: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("config not found: %s", configID)
	}

	return tx.Commit()
}

// bundleRow is a single config's active version, used to assemble the bundle.
type bundleRow struct {
	id            string
	versionNumber int
	yamlContent   string
}

// GetBundledConfig merges all configs' active versions into a single YAML
// document, erroring on upstream/composition name collisions between configs.
func (s *Store) GetBundledConfig(ctx context.Context) (*BundledConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, v.version_number, v.yaml_content
		FROM configs c
		JOIN config_versions v ON c.active_version_id = v.id
		ORDER BY c.id
	`)
	if err != nil {
		return nil, fmt.Errorf("query bundle rows: %w", err)
	}
	defer rows.Close()

	var bundleRows []bundleRow
	for rows.Next() {
		var br bundleRow
		if err := rows.Scan(&br.id, &br.versionNumber, &br.yamlContent); err != nil {
			return nil, fmt.Errorf("scan bundle row: %w", err)
		}
		bundleRows = append(bundleRows, br)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	merged := composition.Config{
		Upstreams:    map[string]composition.Upstream{},
		Compositions: map[string]composition.Composition{},
	}

	for _, br := range bundleRows {
		var cfg composition.Config
		if err := yaml.Unmarshal([]byte(br.yamlContent), &cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", br.id, err)
		}

		for name, upstream := range cfg.Upstreams {
			if _, exists := merged.Upstreams[name]; exists {
				return nil, fmt.Errorf("upstream name collision: %q defined in multiple configs", name)
			}
			merged.Upstreams[name] = upstream
		}

		for name, comp := range cfg.Compositions {
			if _, exists := merged.Compositions[name]; exists {
				return nil, fmt.Errorf("composition name collision: %q defined in multiple configs", name)
			}
			merged.Compositions[name] = comp
		}
	}

	mergedYAML, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("marshal merged config: %w", err)
	}

	compositionNames := make([]string, 0, len(merged.Compositions))
	for name := range merged.Compositions {
		compositionNames = append(compositionNames, name)
	}
	sort.Strings(compositionNames)

	etag := computeBundleETag(bundleRows)

	return &BundledConfig{
		YAMLContent:      string(mergedYAML),
		ETag:             etag,
		CompositionCount: len(merged.Compositions),
		CompositionNames: compositionNames,
	}, nil
}

// computeBundleETag computes a stable ETag from the sorted "id:version" pairs
// of the configs included in the bundle: the first 16 hex characters of the
// SHA-256 digest of the newline-joined, sorted pairs.
func computeBundleETag(rows []bundleRow) string {
	pairs := make([]string, 0, len(rows))
	for _, br := range rows {
		pairs = append(pairs, fmt.Sprintf("%s:%d", br.id, br.versionNumber))
	}
	sort.Strings(pairs)

	h := sha256.New()
	for _, p := range pairs {
		io.WriteString(h, p)    //nolint:errcheck
		io.WriteString(h, "\n") //nolint:errcheck
	}
	digest := h.Sum(nil)
	return hex.EncodeToString(digest)[:16]
}
