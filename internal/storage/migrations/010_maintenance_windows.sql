-- +goose Up
CREATE TABLE maintenance_windows (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    endpoint_id INTEGER REFERENCES endpoints(id) ON DELETE CASCADE,  -- NULL = applies to all endpoints
    days TEXT NOT NULL,           -- 'all' or comma-separated lowercase day codes: mon,tue,wed,thu,fri,sat,sun
    start_minutes INTEGER NOT NULL,  -- minutes since midnight UTC
    end_minutes INTEGER NOT NULL     -- may be < start_minutes for overnight windows (e.g. 22:00-02:00)
);

-- +goose Down
DROP TABLE maintenance_windows;
