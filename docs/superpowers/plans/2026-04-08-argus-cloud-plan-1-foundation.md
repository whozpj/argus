# Argus Cloud — Plan 1: Foundation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a `cloud` git branch, swap SQLite for PostgreSQL, add `project_id` scoping to all three existing tables, and add the four new tables (`users`, `projects`, `api_keys`, `oauth_sessions`) — leaving every existing Go test green.

**Architecture:** The existing `store.DB` struct keeps its interface but the underlying driver switches from `modernc.org/sqlite` to `jackc/pgx` (via the `database/sql` adapter). All SQL is updated from SQLite dialect to Postgres dialect (`?` → `$1`, `datetime('now')` → `NOW()`, `INTEGER PRIMARY KEY AUTOINCREMENT` → `BIGSERIAL`i). A `project_id TEXT NOT NULL` column is added to `events`, `baselines`, and `drift_state`. The store functions that read/write those tables gain a `projectID string` parameter. The ingest handler gets a hardcoded sentinel project ID (`"self-hosted"`) for now — real API key auth comes in Plan 2.

**Tech Stack:** Go 1.26, `jackc/pgx/v5` (stdlib adapter), PostgreSQL 15, `testcontainers-go` for integration tests, Docker (local Postgres for dev).

---

## File Map

| File | Action | What changes |
|---|---|---|
| `server/go.mod` | Modify | Add `pgx/v5`, `testcontainers-go`; remove `modernc.org/sqlite` |
| `server/internal/store/db.go` | Modify | Switch driver to pgx stdlib; update `Open()` to accept a DSN |
| `server/internal/store/schema.sql` | Modify | Postgres dialect; add `project_id`; add 4 new tables |
| `server/internal/store/queries.go` | Modify | `?` → `$N`; add `project_id` param to `BaselineSample`, `RecentSample`, `ReadyModels` |
| `server/internal/store/baseline.go` | Modify | `?` → `$N`; add `project_id` param to `UpdateBaseline`, `GetBaseline` |
| `server/internal/store/list.go` | Modify | `?` → `$N`; add `project_id` param to `ListBaselines`, `EventCount` |
| `server/internal/store/drift_state.go` | Modify | `?` → `$N`; add `project_id` param to `UpsertDriftState`, `GetDriftStates` |
| `server/internal/store/db.go` | Modify | Add `project_id` to `Event` struct; `InsertEvent` gains `projectID` |
| `server/internal/store/users.go` | Create | CRUD for `users`, `projects`, `api_keys`, `oauth_sessions` |
| `server/internal/store/testhelper_test.go` | Create | Shared test helper: spin up Postgres via testcontainers, return `*DB` |
| `server/internal/store/baseline_test.go` | Modify | Use testhelper; pass `project_id` to all calls |
| `server/internal/store/list_test.go` | Modify | Use testhelper; pass `project_id` to all calls |
| `server/internal/store/queries_test.go` | Modify | Use testhelper; pass `project_id` to all calls |
| `server/internal/ingest/handler.go` | Modify | Pass `"self-hosted"` as `projectID` to store calls |
| `server/internal/api/baselines.go` | Modify | Pass `"self-hosted"` as `projectID` to store calls |
| `server/internal/drift/detector.go` | Modify | Pass `"self-hosted"` as `projectID` to store calls |
| `server/cmd/main.go` | Modify | Read `POSTGRES_URL` env var; call `store.Open(dsn)` instead of `store.Open(path)` |

---

## Task 1: Create the cloud branch

- [ ] **Step 1: Create and switch to the cloud branch**

```bash
cd /Users/prithviraj/Documents/CS/argus
git checkout -b cloud
```

Expected output: `Switched to a new branch 'cloud'`

- [ ] **Step 2: Verify**

```bash
git branch
```

Expected: `* cloud` is shown.

---

## Task 2: Add pgx dependency, remove sqlite

- [ ] **Step 1: Add pgx and testcontainers**

```bash
cd server
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/stdlib
go get github.com/testcontainers/testcontainers-go@latest
go get github.com/testcontainers/testcontainers-go/modules/postgres@latest
```

- [ ] **Step 2: Remove the sqlite driver**

```bash
go mod edit -droprequire modernc.org/sqlite
go mod tidy
```

- [ ] **Step 3: Verify go.mod compiles**

```bash
go build ./...
```

Expected: build errors in `store/db.go` because it still imports sqlite — that's fine, we fix it next.

---

## Task 3: Rewrite `schema.sql` for Postgres

- [ ] **Step 1: Replace the contents of `server/internal/store/schema.sql`**

```sql
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
```

---

## Task 4: Rewrite `store/db.go` for Postgres

- [ ] **Step 1: Replace `server/internal/store/db.go`**

```go
package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed schema.sql
var schema string

type DB struct {
	sql *sql.DB
}

// Open connects to a Postgres database at the given DSN and applies the schema.
// DSN example: "postgres://user:pass@localhost:5432/argus?sslmode=disable"
func Open(dsn string) (*DB, error) {
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if _, err := conn.Exec(schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &DB{sql: conn}, nil
}

func (d *DB) Close() error {
	return d.sql.Close()
}

// Event is one captured LLM request/response signal.
type Event struct {
	ProjectID    string
	Model        string
	Provider     string
	InputTokens  int
	OutputTokens int
	LatencyMs    int
	FinishReason string
	TimestampUTC string
}

func (d *DB) InsertEvent(e Event) error {
	_, err := d.sql.Exec(`
		INSERT INTO events (project_id, model, provider, input_tokens, output_tokens, latency_ms, finish_reason, timestamp_utc)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.ProjectID, e.Model, e.Provider, e.InputTokens, e.OutputTokens, e.LatencyMs, e.FinishReason, e.TimestampUTC,
	)
	return err
}

// RecentEvents returns the last n events for a model within a project, newest first.
func (d *DB) RecentEvents(projectID, model string, n int) ([]Event, error) {
	rows, err := d.sql.Query(`
		SELECT project_id, model, provider, input_tokens, output_tokens, latency_ms, finish_reason, timestamp_utc
		FROM events WHERE project_id = $1 AND model = $2 ORDER BY created_at DESC LIMIT $3`,
		projectID, model, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ProjectID, &e.Model, &e.Provider, &e.InputTokens, &e.OutputTokens, &e.LatencyMs, &e.FinishReason, &e.TimestampUTC); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
```

- [ ] **Step 2: Verify it compiles (other files still broken — that's fine)**

```bash
cd server && go build ./internal/store/... 2>&1 | head -20
```

---

## Task 5: Update `queries.go` for Postgres + project scoping

- [ ] **Step 1: Replace `server/internal/store/queries.go`**

```go
package store

import "fmt"

// ReadyModels returns the model names within a project whose baseline is ready.
func (d *DB) ReadyModels(projectID string) ([]string, error) {
	rows, err := d.sql.Query(`SELECT model FROM baselines WHERE project_id = $1 AND is_ready = TRUE`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query ready models: %w", err)
	}
	defer rows.Close()

	var models []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

// BaselineSample returns the oldest n events for a model within a project.
func (d *DB) BaselineSample(projectID, model string, n int) ([]Event, error) {
	rows, err := d.sql.Query(`
		SELECT project_id, model, provider, input_tokens, output_tokens, latency_ms, finish_reason, timestamp_utc
		FROM events
		WHERE project_id = $1 AND model = $2
		ORDER BY created_at ASC
		LIMIT $3`, projectID, model, n)
	if err != nil {
		return nil, fmt.Errorf("query baseline sample: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

// RecentSample returns the most recent n events for a model within a project.
func (d *DB) RecentSample(projectID, model string, n int) ([]Event, error) {
	rows, err := d.sql.Query(`
		SELECT project_id, model, provider, input_tokens, output_tokens, latency_ms, finish_reason, timestamp_utc
		FROM events
		WHERE project_id = $1 AND model = $2
		ORDER BY created_at DESC
		LIMIT $3`, projectID, model, n)
	if err != nil {
		return nil, fmt.Errorf("query recent sample: %w", err)
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}

	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events, nil
}

func scanEvents(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]Event, error) {
	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ProjectID, &e.Model, &e.Provider, &e.InputTokens, &e.OutputTokens,
			&e.LatencyMs, &e.FinishReason, &e.TimestampUTC); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
```

---

## Task 6: Update `baseline.go` for Postgres + project scoping

- [ ] **Step 1: Replace `server/internal/store/baseline.go`**

```go
package store

import (
	"database/sql"
	"fmt"
	"math"
)

// Baseline holds the Welford accumulators and derived stats for one model.
type Baseline struct {
	Model              string
	Count              int
	MeanOutputTokens   float64
	StdDevOutputTokens float64
	MeanLatencyMs      float64
	StdDevLatencyMs    float64
	IsReady            bool
}

// UpdateBaseline applies one new observation to the Welford accumulators for a model within a project.
func (d *DB) UpdateBaseline(projectID, model string, outputTokens, latencyMs int) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var count int
	var meanOut, m2Out, meanLat, m2Lat float64

	err = tx.QueryRow(`
		SELECT count, mean_output_tokens, m2_output_tokens, mean_latency_ms, m2_latency_ms
		FROM baselines WHERE project_id = $1 AND model = $2`, projectID, model).
		Scan(&count, &meanOut, &m2Out, &meanLat, &m2Lat)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read baseline: %w", err)
	}

	count++
	meanOut, m2Out = welfordUpdate(count, meanOut, m2Out, float64(outputTokens))
	meanLat, m2Lat = welfordUpdate(count, meanLat, m2Lat, float64(latencyMs))
	isReady := count >= 200

	_, err = tx.Exec(`
		INSERT INTO baselines (project_id, model, count, mean_output_tokens, m2_output_tokens, mean_latency_ms, m2_latency_ms, is_ready, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (project_id, model) DO UPDATE SET
			count              = EXCLUDED.count,
			mean_output_tokens = EXCLUDED.mean_output_tokens,
			m2_output_tokens   = EXCLUDED.m2_output_tokens,
			mean_latency_ms    = EXCLUDED.mean_latency_ms,
			m2_latency_ms      = EXCLUDED.m2_latency_ms,
			is_ready           = EXCLUDED.is_ready,
			updated_at         = EXCLUDED.updated_at`,
		projectID, model, count, meanOut, m2Out, meanLat, m2Lat, isReady,
	)
	if err != nil {
		return fmt.Errorf("upsert baseline: %w", err)
	}

	return tx.Commit()
}

// GetBaseline returns the current baseline for a model within a project.
func (d *DB) GetBaseline(projectID, model string) (Baseline, bool, error) {
	var count int
	var isReady bool
	var meanOut, m2Out, meanLat, m2Lat float64

	err := d.sql.QueryRow(`
		SELECT count, mean_output_tokens, m2_output_tokens, mean_latency_ms, m2_latency_ms, is_ready
		FROM baselines WHERE project_id = $1 AND model = $2`, projectID, model).
		Scan(&count, &meanOut, &m2Out, &meanLat, &m2Lat, &isReady)
	if err == sql.ErrNoRows {
		return Baseline{}, false, nil
	}
	if err != nil {
		return Baseline{}, false, fmt.Errorf("query baseline: %w", err)
	}

	return Baseline{
		Model:              model,
		Count:              count,
		MeanOutputTokens:   meanOut,
		StdDevOutputTokens: stddev(m2Out, count),
		MeanLatencyMs:      meanLat,
		StdDevLatencyMs:    stddev(m2Lat, count),
		IsReady:            isReady,
	}, true, nil
}

func welfordUpdate(count int, mean, m2, x float64) (float64, float64) {
	delta := x - mean
	mean += delta / float64(count)
	delta2 := x - mean
	m2 += delta * delta2
	return mean, m2
}

func stddev(m2 float64, count int) float64 {
	if count < 2 {
		return 0
	}
	return math.Sqrt(m2 / float64(count))
}
```

---

## Task 7: Update `list.go` for Postgres + project scoping

- [ ] **Step 1: Replace `server/internal/store/list.go`**

```go
package store

import "fmt"

// ListBaselines returns all baselines for a project, ordered by model name.
func (d *DB) ListBaselines(projectID string) ([]Baseline, error) {
	rows, err := d.sql.Query(`
		SELECT model, count, mean_output_tokens, m2_output_tokens,
		       mean_latency_ms, m2_latency_ms, is_ready
		FROM baselines
		WHERE project_id = $1
		ORDER BY model ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list baselines: %w", err)
	}
	defer rows.Close()

	var out []Baseline
	for rows.Next() {
		var b Baseline
		var isReady bool
		var count int
		var meanOut, m2Out, meanLat, m2Lat float64
		if err := rows.Scan(&b.Model, &count, &meanOut, &m2Out, &meanLat, &m2Lat, &isReady); err != nil {
			return nil, err
		}
		b.Count = count
		b.MeanOutputTokens = meanOut
		b.StdDevOutputTokens = stddev(m2Out, count)
		b.MeanLatencyMs = meanLat
		b.StdDevLatencyMs = stddev(m2Lat, count)
		b.IsReady = isReady
		out = append(out, b)
	}
	return out, rows.Err()
}

// EventCount returns the total number of events for a project.
func (d *DB) EventCount(projectID string) (int, error) {
	var n int
	err := d.sql.QueryRow(`SELECT COUNT(*) FROM events WHERE project_id = $1`, projectID).Scan(&n)
	return n, err
}
```

---

## Task 8: Update `drift_state.go` for Postgres + project scoping

- [ ] **Step 1: Replace `server/internal/store/drift_state.go`**

```go
package store

import "fmt"

// DriftState holds the last detector result for one model within a project.
type DriftState struct {
	Model         string
	Score         float64
	POutputTokens float64
	PLatencyMs    float64
	Alerted       bool
}

// UpsertDriftState writes the latest detector result for a model within a project.
func (d *DB) UpsertDriftState(projectID string, s DriftState) error {
	_, err := d.sql.Exec(`
		INSERT INTO drift_state (project_id, model, score, p_output_tokens, p_latency_ms, alerted, checked_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (project_id, model) DO UPDATE SET
			score           = EXCLUDED.score,
			p_output_tokens = EXCLUDED.p_output_tokens,
			p_latency_ms    = EXCLUDED.p_latency_ms,
			alerted         = EXCLUDED.alerted,
			checked_at      = EXCLUDED.checked_at`,
		projectID, s.Model, s.Score, s.POutputTokens, s.PLatencyMs, s.Alerted,
	)
	if err != nil {
		return fmt.Errorf("upsert drift state: %w", err)
	}
	return nil
}

// GetDriftStates returns the latest detector result for every model in a project.
func (d *DB) GetDriftStates(projectID string) (map[string]DriftState, error) {
	rows, err := d.sql.Query(`
		SELECT model, score, p_output_tokens, p_latency_ms, alerted
		FROM drift_state WHERE project_id = $1`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query drift state: %w", err)
	}
	defer rows.Close()

	out := make(map[string]DriftState)
	for rows.Next() {
		var s DriftState
		if err := rows.Scan(&s.Model, &s.Score, &s.POutputTokens, &s.PLatencyMs, &s.Alerted); err != nil {
			return nil, err
		}
		out[s.Model] = s
	}
	return out, rows.Err()
}
```

---

## Task 9: Create `users.go` — new tables CRUD

- [ ] **Step 1: Create `server/internal/store/users.go`**

```go
package store

import (
	"database/sql"
	"fmt"
	"time"
)

// User represents an authenticated Argus user.
type User struct {
	ID        string
	Email     string
	GithubID  string
	GoogleID  string
	CreatedAt time.Time
}

// Project is an isolation boundary — events and baselines are scoped per project.
type Project struct {
	ID        string
	UserID    string
	Name      string
	CreatedAt time.Time
}

// APIKey is a hashed ingest credential scoped to a project.
type APIKey struct {
	ID        string
	ProjectID string
	KeyHash   string
	KeyPrefix string
	Name      string
	CreatedAt time.Time
}

// UpsertUser creates or updates a user by their OAuth provider ID.
// Returns the user's ID.
func (d *DB) UpsertUser(email, githubID, googleID string) (string, error) {
	var id string
	err := d.sql.QueryRow(`
		INSERT INTO users (id, email, github_id, google_id)
		VALUES (gen_random_uuid()::text, $1, NULLIF($2,''), NULLIF($3,''))
		ON CONFLICT (email) DO UPDATE SET
			github_id = COALESCE(NULLIF($2,''), users.github_id),
			google_id = COALESCE(NULLIF($3,''), users.google_id)
		RETURNING id`, email, githubID, googleID).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert user: %w", err)
	}
	return id, nil
}

// GetUserByID returns a user by their ID.
func (d *DB) GetUserByID(id string) (User, error) {
	var u User
	var githubID, googleID sql.NullString
	err := d.sql.QueryRow(`
		SELECT id, email, COALESCE(github_id,''), COALESCE(google_id,''), created_at
		FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Email, &githubID, &googleID, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return User{}, fmt.Errorf("user not found")
	}
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	u.GithubID = githubID.String
	u.GoogleID = googleID.String
	return u, nil
}

// CreateProject creates a new project for a user and returns it.
func (d *DB) CreateProject(userID, name string) (Project, error) {
	var p Project
	err := d.sql.QueryRow(`
		INSERT INTO projects (id, user_id, name)
		VALUES (gen_random_uuid()::text, $1, $2)
		RETURNING id, user_id, name, created_at`, userID, name).
		Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt)
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return p, nil
}

// ListProjects returns all projects for a user, ordered by creation time.
func (d *DB) ListProjects(userID string) ([]Project, error) {
	rows, err := d.sql.Query(`
		SELECT id, user_id, name, created_at
		FROM projects WHERE user_id = $1 ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CreateAPIKey stores a new (pre-hashed) API key for a project.
func (d *DB) CreateAPIKey(projectID, keyHash, keyPrefix, name string) (APIKey, error) {
	var k APIKey
	err := d.sql.QueryRow(`
		INSERT INTO api_keys (id, project_id, key_hash, key_prefix, name)
		VALUES (gen_random_uuid()::text, $1, $2, $3, $4)
		RETURNING id, project_id, key_hash, key_prefix, name, created_at`,
		projectID, keyHash, keyPrefix, name).
		Scan(&k.ID, &k.ProjectID, &k.KeyHash, &k.KeyPrefix, &k.Name, &k.CreatedAt)
	if err != nil {
		return APIKey{}, fmt.Errorf("create api key: %w", err)
	}
	return k, nil
}

// ListAPIKeys returns all API keys for a project (key_hash is not returned to callers).
func (d *DB) ListAPIKeys(projectID string) ([]APIKey, error) {
	rows, err := d.sql.Query(`
		SELECT id, project_id, key_prefix, name, created_at
		FROM api_keys WHERE project_id = $1 ORDER BY created_at ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var out []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.KeyPrefix, &k.Name, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ResolveAPIKey looks up a project ID by the raw API key hash.
// Returns ("", false, nil) if the key doesn't exist.
func (d *DB) ResolveAPIKey(keyHash string) (projectID string, ok bool, err error) {
	err = d.sql.QueryRow(`SELECT project_id FROM api_keys WHERE key_hash = $1`, keyHash).Scan(&projectID)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("resolve api key: %w", err)
	}
	return projectID, true, nil
}

// CreateOAuthSession stores a short-lived CLI login code.
func (d *DB) CreateOAuthSession(code, userID string, expiresAt time.Time) error {
	_, err := d.sql.Exec(`
		INSERT INTO oauth_sessions (code, user_id, expires_at) VALUES ($1, $2, $3)`,
		code, userID, expiresAt)
	if err != nil {
		return fmt.Errorf("create oauth session: %w", err)
	}
	return nil
}

// ConsumeOAuthSession exchanges a code for a userID and deletes the session.
// Returns ("", false, nil) if code not found or expired.
func (d *DB) ConsumeOAuthSession(code string) (userID string, ok bool, err error) {
	err = d.sql.QueryRow(`
		DELETE FROM oauth_sessions
		WHERE code = $1 AND expires_at > NOW()
		RETURNING user_id`, code).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("consume oauth session: %w", err)
	}
	return userID, true, nil
}
```

---

## Task 10: Write the shared test helper

- [ ] **Step 1: Create `server/internal/store/testhelper_test.go`**

```go
package store_test

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/whozpj/argus/server/internal/store"
)

// newTestDB spins up a throwaway Postgres container and returns an open *store.DB.
// The container is terminated when the test finishes.
func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	ctx := context.Background()

	pgc, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("argus_test"),
		postgres.WithUsername("argus"),
		postgres.WithPassword("argus"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = pgc.Terminate(ctx) })

	dsn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return db
}
```

---

## Task 11: Update `baseline_test.go` to use testhelper + project scoping

- [ ] **Step 1: Read the current test file**

Read `server/internal/store/baseline_test.go` to see all existing test cases before rewriting.

- [ ] **Step 2: Rewrite `server/internal/store/baseline_test.go`**

```go
package store_test

import (
	"testing"
)

func TestUpdateAndGetBaseline(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-test"
	model := "claude-sonnet-4-6"

	// No baseline yet
	_, found, err := db.GetBaseline(projectID, model)
	if err != nil {
		t.Fatalf("GetBaseline: %v", err)
	}
	if found {
		t.Fatal("expected no baseline initially")
	}

	// Add 3 observations
	for i := 0; i < 3; i++ {
		if err := db.UpdateBaseline(projectID, model, 100, 500); err != nil {
			t.Fatalf("UpdateBaseline[%d]: %v", i, err)
		}
	}

	b, found, err := db.GetBaseline(projectID, model)
	if err != nil {
		t.Fatalf("GetBaseline after updates: %v", err)
	}
	if !found {
		t.Fatal("expected baseline to exist after updates")
	}
	if b.Count != 3 {
		t.Errorf("count = %d, want 3", b.Count)
	}
	if b.IsReady {
		t.Error("baseline should not be ready at count 3")
	}
	if b.MeanOutputTokens != 100 {
		t.Errorf("mean_output_tokens = %f, want 100", b.MeanOutputTokens)
	}
}

func TestBaselineIsReadyAt200(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-ready"
	model := "gpt-4o"

	for i := 0; i < 200; i++ {
		if err := db.UpdateBaseline(projectID, model, 50, 300); err != nil {
			t.Fatalf("UpdateBaseline[%d]: %v", i, err)
		}
	}

	b, found, err := db.GetBaseline(projectID, model)
	if err != nil {
		t.Fatalf("GetBaseline: %v", err)
	}
	if !found {
		t.Fatal("expected baseline to exist")
	}
	if !b.IsReady {
		t.Errorf("expected IsReady=true at count 200, got false")
	}
}

func TestBaselineIsolatedByProject(t *testing.T) {
	db := newTestDB(t)
	model := "claude-sonnet-4-6"

	if err := db.UpdateBaseline("project-A", model, 100, 500); err != nil {
		t.Fatalf("UpdateBaseline project-A: %v", err)
	}

	_, found, err := db.GetBaseline("project-B", model)
	if err != nil {
		t.Fatalf("GetBaseline project-B: %v", err)
	}
	if found {
		t.Error("project-B should not see project-A's baseline")
	}
}
```

- [ ] **Step 3: Run the test**

```bash
cd server && go test ./internal/store/... -run TestUpdateAndGetBaseline -v -timeout 120s
```

Expected: PASS (testcontainers pulls Postgres image on first run — may take ~30s).

---

## Task 12: Update `list_test.go`

- [ ] **Step 1: Rewrite `server/internal/store/list_test.go`**

```go
package store_test

import (
	"testing"
)

func TestEventCount(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-count"

	n, err := db.EventCount(projectID)
	if err != nil {
		t.Fatalf("EventCount: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 events initially, got %d", n)
	}

	err = db.InsertEvent(store.Event{
		ProjectID:    projectID,
		Model:        "gpt-4o",
		Provider:     "openai",
		InputTokens:  10,
		OutputTokens: 20,
		LatencyMs:    100,
		FinishReason: "stop",
		TimestampUTC: "2026-04-08T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	n, err = db.EventCount(projectID)
	if err != nil {
		t.Fatalf("EventCount after insert: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 event, got %d", n)
	}
}

func TestListBaselines(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-list"

	baselines, err := db.ListBaselines(projectID)
	if err != nil {
		t.Fatalf("ListBaselines empty: %v", err)
	}
	if len(baselines) != 0 {
		t.Errorf("expected 0 baselines, got %d", len(baselines))
	}

	if err := db.UpdateBaseline(projectID, "model-a", 100, 500); err != nil {
		t.Fatalf("UpdateBaseline: %v", err)
	}
	if err := db.UpdateBaseline(projectID, "model-b", 200, 800); err != nil {
		t.Fatalf("UpdateBaseline: %v", err)
	}

	baselines, err = db.ListBaselines(projectID)
	if err != nil {
		t.Fatalf("ListBaselines: %v", err)
	}
	if len(baselines) != 2 {
		t.Errorf("expected 2 baselines, got %d", len(baselines))
	}
	if baselines[0].Model != "model-a" || baselines[1].Model != "model-b" {
		t.Errorf("unexpected order: %v", baselines)
	}
}

func TestEventCountIsolatedByProject(t *testing.T) {
	db := newTestDB(t)

	_ = db.InsertEvent(store.Event{
		ProjectID: "proj-x", Model: "gpt-4o", Provider: "openai",
		InputTokens: 1, OutputTokens: 2, LatencyMs: 3,
		FinishReason: "stop", TimestampUTC: "2026-04-08T00:00:00Z",
	})

	n, err := db.EventCount("proj-y")
	if err != nil {
		t.Fatalf("EventCount: %v", err)
	}
	if n != 0 {
		t.Errorf("proj-y should not see proj-x events, got %d", n)
	}
}
```

Note: add `"github.com/whozpj/argus/server/internal/store"` to the import block in this file since it references `store.Event`.

- [ ] **Step 2: Run**

```bash
cd server && go test ./internal/store/... -run TestEventCount -v -timeout 120s
```

Expected: PASS.

---

## Task 13: Update `queries_test.go`

- [ ] **Step 1: Rewrite `server/internal/store/queries_test.go`**

```go
package store_test

import (
	"testing"

	"github.com/whozpj/argus/server/internal/store"
)

func insertTestEvent(t *testing.T, db *store.DB, projectID, model string, outputTokens, latencyMs int) {
	t.Helper()
	err := db.InsertEvent(store.Event{
		ProjectID:    projectID,
		Model:        model,
		Provider:     "openai",
		InputTokens:  10,
		OutputTokens: outputTokens,
		LatencyMs:    latencyMs,
		FinishReason: "stop",
		TimestampUTC: "2026-04-08T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
}

func TestReadyModels(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-ready-models"
	model := "gpt-4o"

	models, err := db.ReadyModels(projectID)
	if err != nil {
		t.Fatalf("ReadyModels: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 ready models initially, got %d", len(models))
	}

	for i := 0; i < 200; i++ {
		if err := db.UpdateBaseline(projectID, model, 50, 200); err != nil {
			t.Fatalf("UpdateBaseline[%d]: %v", i, err)
		}
	}

	models, err = db.ReadyModels(projectID)
	if err != nil {
		t.Fatalf("ReadyModels after 200: %v", err)
	}
	if len(models) != 1 || models[0] != model {
		t.Errorf("expected [%s], got %v", model, models)
	}
}

func TestBaselineSample(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-sample"
	model := "claude-sonnet-4-6"

	for i := 0; i < 5; i++ {
		insertTestEvent(t, db, projectID, model, 100+i, 500)
	}

	sample, err := db.BaselineSample(projectID, model, 3)
	if err != nil {
		t.Fatalf("BaselineSample: %v", err)
	}
	if len(sample) != 3 {
		t.Errorf("expected 3 events, got %d", len(sample))
	}
	// Should be oldest first
	if sample[0].OutputTokens > sample[2].OutputTokens {
		t.Error("BaselineSample should return oldest events first")
	}
}

func TestRecentSample(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-recent"
	model := "claude-sonnet-4-6"

	for i := 0; i < 5; i++ {
		insertTestEvent(t, db, projectID, model, 100+i, 500)
	}

	sample, err := db.RecentSample(projectID, model, 3)
	if err != nil {
		t.Fatalf("RecentSample: %v", err)
	}
	if len(sample) != 3 {
		t.Errorf("expected 3 events, got %d", len(sample))
	}
	// Should be newest 3, returned in chronological (oldest-first) order
	if sample[0].OutputTokens > sample[2].OutputTokens {
		t.Error("RecentSample should return events oldest-first within the window")
	}
}
```

- [ ] **Step 2: Run**

```bash
cd server && go test ./internal/store/... -v -timeout 180s
```

Expected: all store tests PASS.

---

## Task 14: Update callers — `ingest`, `api`, `drift`

- [ ] **Step 1: Update `server/internal/ingest/handler.go`**

Add `"self-hosted"` as the hardcoded `projectID` in all store calls:

```go
// In ServeHTTP, replace:
err := h.db.InsertEvent(store.Event{
    Model:        req.Model,
    ...
})
// With:
err := h.db.InsertEvent(store.Event{
    ProjectID:    "self-hosted",
    Model:        req.Model,
    Provider:     req.Provider,
    InputTokens:  req.InputTokens,
    OutputTokens: req.OutputTokens,
    LatencyMs:    req.LatencyMs,
    FinishReason: req.FinishReason,
    TimestampUTC: req.TimestampUTC,
})

// Replace:
if err := h.db.UpdateBaseline(req.Model, req.OutputTokens, req.LatencyMs); err != nil {
// With:
if err := h.db.UpdateBaseline("self-hosted", req.Model, req.OutputTokens, req.LatencyMs); err != nil {
```

- [ ] **Step 2: Update `server/internal/api/baselines.go`**

Find all calls to `db.ListBaselines()`, `db.EventCount()`, `db.GetDriftStates()` and add `"self-hosted"` as the first argument:

```go
baselines, err := h.db.ListBaselines("self-hosted")
count, err := h.db.EventCount("self-hosted")
states, err := h.db.GetDriftStates("self-hosted")
```

- [ ] **Step 3: Update `server/internal/drift/detector.go`**

Find all calls to `db.ReadyModels()`, `db.BaselineSample()`, `db.RecentSample()`, `db.UpsertDriftState()` and add `"self-hosted"`:

```go
models, err := d.db.ReadyModels("self-hosted")
sample, err := d.db.BaselineSample("self-hosted", model, n)
recent, err := d.db.RecentSample("self-hosted", model, n)
d.db.UpsertDriftState("self-hosted", state)
```

- [ ] **Step 4: Build the whole server**

```bash
cd server && go build ./...
```

Expected: zero errors.

---

## Task 15: Update `main.go` to use `POSTGRES_URL`

- [ ] **Step 1: Update `server/cmd/main.go`**

```go
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/whozpj/argus/server/internal/alerts"
	"github.com/whozpj/argus/server/internal/api"
	"github.com/whozpj/argus/server/internal/drift"
	"github.com/whozpj/argus/server/internal/ingest"
	"github.com/whozpj/argus/server/internal/store"
)

func main() {
	dsn := getenv("POSTGRES_URL", "postgres://argus:argus@localhost:5432/argus?sslmode=disable")
	addr := getenv("ARGUS_ADDR", ":4000")

	db, err := store.Open(dsn)
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	var notifier alerts.Notifier = alerts.Noop{}
	if webhook := getenv("ARGUS_SLACK_WEBHOOK", ""); webhook != "" {
		notifier = alerts.NewSlack(webhook)
		slog.Info("slack alerts enabled")
	}
	go drift.New(db, drift.Interval, notifier).Run()

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/events", ingest.NewHandler(db))
	mux.HandleFunc("GET /api/v1/baselines", api.NewBaselinesHandler(db))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	slog.Info("argus server starting", "addr", addr, "dsn", dsn)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 2: Build**

```bash
cd server && go build ./...
```

Expected: zero errors.

---

## Task 16: Run all server tests + commit

- [ ] **Step 1: Run all tests**

```bash
cd server && go test ./... -v -timeout 180s
```

Expected: all tests PASS (testcontainers manages Postgres automatically).

- [ ] **Step 2: Commit**

```bash
cd /Users/prithviraj/Documents/CS/argus
git add server/
git commit -m "feat(server): migrate from SQLite to Postgres; add project_id scoping and user/project/api_key tables"
```

---

## Task 17: Manual smoke test (local Postgres)

- [ ] **Step 1: Start a local Postgres**

```bash
docker run --rm -p 5432:5432 \
  -e POSTGRES_USER=argus \
  -e POSTGRES_PASSWORD=argus \
  -e POSTGRES_DB=argus \
  postgres:15-alpine
```

- [ ] **Step 2: Run the server**

```bash
cd server && POSTGRES_URL="postgres://argus:argus@localhost:5432/argus?sslmode=disable" go run ./cmd/main.go
```

Expected output: `argus server starting addr=:4000`

- [ ] **Step 3: Send a test event**

```bash
curl -s -X POST http://localhost:4000/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","provider":"openai","input_tokens":10,"output_tokens":50,"latency_ms":300,"finish_reason":"stop","timestamp_utc":"2026-04-08T00:00:00Z"}'
```

Expected: HTTP 202 (empty body).

- [ ] **Step 4: Check baselines**

```bash
curl -s http://localhost:4000/api/v1/baselines | python3 -m json.tool
```

Expected: `{"total_events":1,"baselines":[{"model":"gpt-4o",...}]}`

---

## What comes next

**Plan 2 — Auth & API Keys:** JWT middleware, GitHub + Google OAuth endpoints, API key generation + validation, `/api/v1/me`, `/api/v1/projects`, `/api/v1/projects/:id/keys`.

**Plan 3 — SDK + CLI:** `api_key` parameter in `patch()`, `argus login` / `argus status` / `argus projects` CLI commands.

**Plan 4 — Dashboard:** Login page, project selector, per-project dashboard URL.

**Plan 5 — Infrastructure:** Terraform for ECS, RDS, ALB, S3, CloudFront, Secrets Manager. GitHub Actions CI.
