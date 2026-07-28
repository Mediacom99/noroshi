-- +goose Up
ALTER TABLE endpoints ADD COLUMN paused INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE endpoints DROP COLUMN paused;
