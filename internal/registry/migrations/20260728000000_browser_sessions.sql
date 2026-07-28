-- +goose Up
CREATE TABLE browser_sessions (
    id          TEXT PRIMARY KEY,
    preferences TEXT,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS browser_sessions;
