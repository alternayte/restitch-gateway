-- +goose Up
-- An artifact version is immutable: rows are inserted, never updated or
-- deleted (rollback appends a new version).
CREATE TABLE studio_artifact_versions (
    environment TEXT NOT NULL REFERENCES studio_environments (name) ON DELETE CASCADE,
    number      INTEGER NOT NULL,
    yaml        TEXT NOT NULL,
    author      TEXT NOT NULL,
    message     TEXT NOT NULL,
    created_at  TIMESTAMP NOT NULL,
    PRIMARY KEY (environment, number)
);

-- +goose Down
DROP TABLE studio_artifact_versions;
