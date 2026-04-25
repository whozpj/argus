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

// DeleteModel removes all events, the baseline, and the drift state for one
// model within a project.
func (d *DB) DeleteModel(projectID, model string) error {
	for _, q := range []string{
		`DELETE FROM events      WHERE project_id = $1 AND model = $2`,
		`DELETE FROM baselines   WHERE project_id = $1 AND model = $2`,
		`DELETE FROM drift_state WHERE project_id = $1 AND model = $2`,
	} {
		if _, err := d.sql.Exec(q, projectID, model); err != nil {
			return fmt.Errorf("delete model %q: %w", model, err)
		}
	}
	return nil
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
