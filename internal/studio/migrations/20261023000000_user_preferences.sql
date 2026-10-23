-- +goose Up
CREATE TABLE studio_user_preferences (
    user_id    TEXT PRIMARY KEY,
    data       TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE studio_user_preferences;
