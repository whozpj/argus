package store

import "fmt"

// DriftState holds the last detector result for one model.
type DriftState struct {
	Model           string
	Score           float64
	POutputTokens   float64
	PLatencyMs      float64
	Alerted         bool
}

// UpsertDriftState writes the latest detector result for a model.
func (d *DB) UpsertDriftState(s DriftState) error {
	alerted := 0
	if s.Alerted {
		alerted = 1
	}
	_, err := d.sql.Exec(`
		INSERT INTO drift_state (model, score, p_output_tokens, p_latency_ms, alerted, checked_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(model) DO UPDATE SET
			score           = excluded.score,
			p_output_tokens = excluded.p_output_tokens,
			p_latency_ms    = excluded.p_latency_ms,
			alerted         = excluded.alerted,
			checked_at      = excluded.checked_at`,
		s.Model, s.Score, s.POutputTokens, s.PLatencyMs, alerted,
	)
	if err != nil {
		return fmt.Errorf("upsert drift state: %w", err)
	}
	return nil
}

// GetDriftStates returns the latest detector result for every model that has
// been checked. Models not yet checked are absent.
func (d *DB) GetDriftStates() (map[string]DriftState, error) {
	rows, err := d.sql.Query(`
		SELECT model, score, p_output_tokens, p_latency_ms, alerted
		FROM drift_state`)
	if err != nil {
		return nil, fmt.Errorf("query drift state: %w", err)
	}
	defer rows.Close()

	out := make(map[string]DriftState)
	for rows.Next() {
		var s DriftState
		var alerted int
		if err := rows.Scan(&s.Model, &s.Score, &s.POutputTokens, &s.PLatencyMs, &alerted); err != nil {
			return nil, err
		}
		s.Alerted = alerted == 1
		out[s.Model] = s
	}
	return out, rows.Err()
}
