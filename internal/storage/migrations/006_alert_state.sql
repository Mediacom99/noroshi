-- +goose Up
ALTER TABLE endpoints ADD COLUMN last_notified_at DATETIME;
ALTER TABLE endpoints ADD COLUMN paused_until DATETIME;
ALTER TABLE endpoints ADD COLUMN alert_message_id INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE endpoints DROP COLUMN last_notified_at;
ALTER TABLE endpoints DROP COLUMN paused_until;
ALTER TABLE endpoints DROP COLUMN alert_message_id;
