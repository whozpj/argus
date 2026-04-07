package store

import "fmt"

// ListBaselines returns all baselines ordered by model name.
// Used by the dashboard API.
func (d *DB) ListBaselines() ([]Baseline, error) {
	rows, err := d.sql.Query(`
		SELECT model, count, mean_output_tokens, m2_output_tokens,
		       mean_latency_ms, m2_latency_ms, is_ready
		FROM baselines
		ORDER BY model ASC`)
	if err != nil {
		return nil, fmt.Errorf("list baselines: %w", err)
	}
	defer rows.Close()

	var out []Baseline
	for rows.Next() {
		var b Baseline
		var isReady, count int
		var meanOut, m2Out, meanLat, m2Lat float64
		if err := rows.Scan(&b.Model, &count, &meanOut, &m2Out, &meanLat, &m2Lat, &isReady); err != nil {
			return nil, err
		}
		b.Count = count
		b.MeanOutputTokens = meanOut
		b.StdDevOutputTokens = stddev(m2Out, count)
		b.MeanLatencyMs = meanLat
		b.StdDevLatencyMs = stddev(m2Lat, count)
		b.IsReady = isReady == 1
		out = append(out, b)
	}
	return out, rows.Err()
}

// EventCount returns the total number of events in the database.
func (d *DB) EventCount() (int, error) {
	var n int
	err := d.sql.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n)
	return n, err
}
