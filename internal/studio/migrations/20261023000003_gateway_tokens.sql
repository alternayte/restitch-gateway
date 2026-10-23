-- +goose Up
-- A gateway token belongs to one environment. Only its SHA-256 digest is
-- stored; the token itself is shown once, when it is issued.
CREATE TABLE studio_gateway_tokens (
    id          TEXT PRIMARY KEY,
    environment TEXT NOT NULL REFERENCES studio_environments (name) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    digest      TEXT NOT NULL UNIQUE,
    prefix      TEXT NOT NULL,
    created_at  TIMESTAMP NOT NULL,
    created_by  TEXT NOT NULL,
    revoked_at  TIMESTAMP,
    last_used_at TIMESTAMP
);

-- +goose Down
DROP TABLE studio_gateway_tokens;
