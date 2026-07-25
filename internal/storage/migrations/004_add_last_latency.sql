-- +goose Up
ALTER TABLE endpoints ADD COLUMN last_latency_ms INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE endpoints DROP COLUMN last_latency_ms;
