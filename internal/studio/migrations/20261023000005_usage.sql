-- +goose Up
-- One row per gateway instance, minute and composition (an empty
-- composition is the gateway-wide total). Studio sums the instances.
CREATE TABLE studio_usage (
    environment     TEXT NOT NULL REFERENCES studio_environments (name) ON DELETE CASCADE,
    instance_id     TEXT NOT NULL,
    minute          TIMESTAMP NOT NULL,
    composition     TEXT NOT NULL,
    requests        INTEGER NOT NULL,
    errors          INTEGER NOT NULL,
    partials        INTEGER NOT NULL,
    latency_buckets TEXT NOT NULL,
    steps           TEXT NOT NULL,
    PRIMARY KEY (environment, instance_id, minute, composition)
);
CREATE INDEX studio_usage_by_minute ON studio_usage (environment, composition, minute);

ALTER TABLE studio_environments ADD COLUMN usage_retention_days INTEGER NOT NULL DEFAULT 7;
ALTER TABLE studio_gateway_instances ADD COLUMN breakers TEXT NOT NULL DEFAULT '{}';

-- +goose Down
DROP TABLE studio_usage;
