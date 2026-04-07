CREATE TABLE IF NOT EXISTS events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    model         TEXT    NOT NULL,
    provider      TEXT    NOT NULL,
    input_tokens  INTEGER NOT NULL,
    output_tokens INTEGER NOT NULL,
    latency_ms    INTEGER NOT NULL,
    finish_reason TEXT    NOT NULL,
    timestamp_utc TEXT    NOT NULL,
    created_at    TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_events_model ON events(model, created_at);

CREATE TABLE IF NOT EXISTS drift_state (
    model              TEXT    PRIMARY KEY,
    score              REAL    NOT NULL DEFAULT 0,
    p_output_tokens    REAL    NOT NULL DEFAULT 1,
    p_latency_ms       REAL    NOT NULL DEFAULT 1,
    alerted            INTEGER NOT NULL DEFAULT 0, -- 1 when alert is active
    checked_at         TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS baselines (
    model              TEXT    PRIMARY KEY,
    count              INTEGER NOT NULL DEFAULT 0,
    -- Welford accumulators for output_tokens
    mean_output_tokens REAL    NOT NULL DEFAULT 0,
    m2_output_tokens   REAL    NOT NULL DEFAULT 0,
    -- Welford accumulators for latency_ms
    mean_latency_ms    REAL    NOT NULL DEFAULT 0,
    m2_latency_ms      REAL    NOT NULL DEFAULT 0,
    is_ready           INTEGER NOT NULL DEFAULT 0, -- 1 when count >= 200
    updated_at         TEXT    NOT NULL DEFAULT (datetime('now'))
);
