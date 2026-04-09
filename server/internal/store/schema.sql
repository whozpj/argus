CREATE TABLE IF NOT EXISTS users (
    id         TEXT PRIMARY KEY,
    email      TEXT UNIQUE NOT NULL,
    github_id  TEXT UNIQUE,
    google_id  TEXT UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS projects (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    key_hash    TEXT UNIQUE NOT NULL,
    key_prefix  TEXT NOT NULL,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS oauth_sessions (
    code        TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at  TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
    id            BIGSERIAL PRIMARY KEY,
    project_id    TEXT NOT NULL,
    model         TEXT NOT NULL,
    provider      TEXT NOT NULL,
    input_tokens  INTEGER NOT NULL,
    output_tokens INTEGER NOT NULL,
    latency_ms    INTEGER NOT NULL,
    finish_reason TEXT NOT NULL,
    timestamp_utc TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_events_model ON events(project_id, model, created_at);

CREATE TABLE IF NOT EXISTS drift_state (
    project_id         TEXT NOT NULL,
    model              TEXT NOT NULL,
    score              REAL NOT NULL DEFAULT 0,
    p_output_tokens    REAL NOT NULL DEFAULT 1,
    p_latency_ms       REAL NOT NULL DEFAULT 1,
    alerted            BOOLEAN NOT NULL DEFAULT FALSE,
    checked_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, model)
);

CREATE TABLE IF NOT EXISTS baselines (
    project_id         TEXT NOT NULL,
    model              TEXT NOT NULL,
    count              INTEGER NOT NULL DEFAULT 0,
    mean_output_tokens REAL NOT NULL DEFAULT 0,
    m2_output_tokens   REAL NOT NULL DEFAULT 0,
    mean_latency_ms    REAL NOT NULL DEFAULT 0,
    m2_latency_ms      REAL NOT NULL DEFAULT 0,
    is_ready           BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, model)
);
