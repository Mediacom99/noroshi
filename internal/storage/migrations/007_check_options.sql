-- +goose Up
ALTER TABLE endpoints ADD COLUMN expected_status INTEGER NOT NULL DEFAULT 0;
ALTER TABLE endpoints ADD COLUMN expected_keyword TEXT NOT NULL DEFAULT '';
ALTER TABLE endpoints ADD COLUMN last_check_error TEXT NOT NULL DEFAULT '';
ALTER TABLE endpoints ADD COLUMN cert_expires_at DATETIME;
ALTER TABLE endpoints ADD COLUMN last_cert_warning_at DATETIME;

-- +goose Down
ALTER TABLE endpoints DROP COLUMN expected_status;
ALTER TABLE endpoints DROP COLUMN expected_keyword;
ALTER TABLE endpoints DROP COLUMN last_check_error;
ALTER TABLE endpoints DROP COLUMN cert_expires_at;
ALTER TABLE endpoints DROP COLUMN last_cert_warning_at;
