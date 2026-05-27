CREATE TABLE IF NOT EXISTS operation_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    type TEXT NOT NULL,
    username TEXT NOT NULL,
    action TEXT NOT NULL,
    path TEXT NOT NULL,
    size INTEGER DEFAULT 0,
    detail TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_logs_ts ON operation_logs(timestamp DESC);
