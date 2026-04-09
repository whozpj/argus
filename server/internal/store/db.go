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
