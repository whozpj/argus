package store

import "fmt"

// ReadyModels returns the names of all models whose baseline is ready
// (count >= 200). Used by the drift detector to know which models to check.
func (d *DB) ReadyModels() ([]string, error) {
	rows, err := d.sql.Query(`SELECT model FROM baselines WHERE is_ready = 1`)
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

// BaselineSample returns the oldest n events for a model, ordered
// chronologically. These form the reference distribution for drift detection.
func (d *DB) BaselineSample(model string, n int) ([]Event, error) {
	rows, err := d.sql.Query(`
		SELECT model, provider, input_tokens, output_tokens, latency_ms, finish_reason, timestamp_utc
		FROM events
		WHERE model = ?
		ORDER BY created_at ASC
		LIMIT ?`, model, n)
	if err != nil {
		return nil, fmt.Errorf("query baseline sample: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

// RecentSample returns the most recent n events for a model, ordered
// chronologically (oldest-first within the window).
func (d *DB) RecentSample(model string, n int) ([]Event, error) {
	// Select newest n DESC, then reverse to chronological order.
	rows, err := d.sql.Query(`
		SELECT model, provider, input_tokens, output_tokens, latency_ms, finish_reason, timestamp_utc
		FROM events
		WHERE model = ?
		ORDER BY created_at DESC
		LIMIT ?`, model, n)
	if err != nil {
		return nil, fmt.Errorf("query recent sample: %w", err)
	}
	defer rows.Close()

	events, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}

	// Reverse so callers see oldest-first within the window.
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
		if err := rows.Scan(&e.Model, &e.Provider, &e.InputTokens, &e.OutputTokens,
			&e.LatencyMs, &e.FinishReason, &e.TimestampUTC); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
