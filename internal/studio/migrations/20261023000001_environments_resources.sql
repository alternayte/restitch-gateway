-- +goose Up
CREATE TABLE studio_environments (
    name       TEXT PRIMARY KEY,
    created_at TIMESTAMP NOT NULL,
    created_by TEXT NOT NULL
);

-- A resource is one typed config item in an environment's draft. The
-- primary key makes a name unique per environment and kind.
CREATE TABLE studio_resources (
    environment TEXT NOT NULL REFERENCES studio_environments (name) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    name        TEXT NOT NULL,
    revision    INTEGER NOT NULL,
    body        TEXT NOT NULL,
    updated_at  TIMESTAMP NOT NULL,
    updated_by  TEXT NOT NULL,
    PRIMARY KEY (environment, kind, name)
);

-- +goose Down
DROP TABLE studio_resources;
DROP TABLE studio_environments;
