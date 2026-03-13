-- +goose Up
-- SQLite does not support adding a UNIQUE column via ALTER TABLE.
-- Rebuild the table to include the name column.

CREATE TABLE endpoints_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    url TEXT NOT NULL UNIQUE,
    interval_seconds INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'unknown',
    last_checked_at DATETIME,
    last_failure_at DATETIME,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    failure_notifications_sent INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO endpoints_new (id, name, url, interval_seconds, status, last_checked_at, last_failure_at, consecutive_failures, failure_notifications_sent, created_at)
    SELECT id, 'endpoint-' || id, url, interval_seconds, status, last_checked_at, last_failure_at, consecutive_failures, failure_notifications_sent, created_at
    FROM endpoints;

DROP TABLE endpoints;
ALTER TABLE endpoints_new RENAME TO endpoints;

-- +goose Down
CREATE TABLE endpoints_old (
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

INSERT INTO endpoints_old (id, url, interval_seconds, status, last_checked_at, last_failure_at, consecutive_failures, failure_notifications_sent, created_at)
    SELECT id, url, interval_seconds, status, last_checked_at, last_failure_at, consecutive_failures, failure_notifications_sent, created_at
    FROM endpoints;

DROP TABLE endpoints;
ALTER TABLE endpoints_old RENAME TO endpoints;
