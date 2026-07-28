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
