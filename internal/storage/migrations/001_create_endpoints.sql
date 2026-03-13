-- +goose Up
CREATE TABLE endpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    interval_seconds INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'unknown',
    last_checked_at DATETIME,
    last_failure_at DATETIME,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    failure_notifications_sent INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE endpoints;
