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
