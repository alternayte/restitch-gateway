-- +goose Up
-- Sampled requests: every failed one, and successes up to the
-- environment's per-minute cap. The record is JSON with redacted headers
-- and step URLs and no bodies.
CREATE TABLE studio_requests (
    environment TEXT NOT NULL REFERENCES studio_environments (name) ON DELETE CASCADE,
    instance_id TEXT NOT NULL,
    request_id  TEXT NOT NULL,
    at          TIMESTAMP NOT NULL,
    minute      TIMESTAMP NOT NULL,
    composition TEXT NOT NULL,
    status      INTEGER NOT NULL,
    failed      INTEGER NOT NULL,
    duration_ms REAL NOT NULL,
    record      TEXT NOT NULL,
    PRIMARY KEY (environment, instance_id, request_id)
);
CREATE INDEX studio_requests_by_time ON studio_requests (environment, at);

ALTER TABLE studio_environments ADD COLUMN request_samples_per_minute INTEGER NOT NULL DEFAULT 10;

-- +goose Down
DROP TABLE studio_requests;
