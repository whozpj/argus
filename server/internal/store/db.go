package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type DB struct {
	sql *sql.DB
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
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
		INSERT INTO events (model, provider, input_tokens, output_tokens, latency_ms, finish_reason, timestamp_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.Model, e.Provider, e.InputTokens, e.OutputTokens, e.LatencyMs, e.FinishReason, e.TimestampUTC,
	)
	return err
}

// RecentEvents returns the last n events for a model, newest first.
func (d *DB) RecentEvents(model string, n int) ([]Event, error) {
	rows, err := d.sql.Query(`
		SELECT model, provider, input_tokens, output_tokens, latency_ms, finish_reason, timestamp_utc
		FROM events WHERE model = ? ORDER BY created_at DESC LIMIT ?`, model, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Model, &e.Provider, &e.InputTokens, &e.OutputTokens, &e.LatencyMs, &e.FinishReason, &e.TimestampUTC); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
