-- +goose Up
-- One row per gateway instance that has sent a heartbeat, keyed by the
-- environment of its token.
CREATE TABLE studio_gateway_instances (
    environment     TEXT NOT NULL REFERENCES studio_environments (name) ON DELETE CASCADE,
    instance_id     TEXT NOT NULL,
    binary_version  TEXT NOT NULL,
    applied_version INTEGER NOT NULL,
    last_pull_error TEXT NOT NULL,
    first_seen_at   TIMESTAMP NOT NULL,
    last_seen_at    TIMESTAMP NOT NULL,
    PRIMARY KEY (environment, instance_id)
);

-- +goose Down
DROP TABLE studio_gateway_instances;
