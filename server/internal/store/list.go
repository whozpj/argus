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
