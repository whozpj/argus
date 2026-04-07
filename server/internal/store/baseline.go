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
	IsReady            bool // true when Count >= 200
}

// UpdateBaseline applies one new observation to the running Welford accumulators
// for the given model. Creates the row if it doesn't exist yet.
// Called after every successful InsertEvent.
func (d *DB) UpdateBaseline(model string, outputTokens, latencyMs int) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var count int
	var meanOut, m2Out, meanLat, m2Lat float64

	err = tx.QueryRow(`
		SELECT count, mean_output_tokens, m2_output_tokens, mean_latency_ms, m2_latency_ms
		FROM baselines WHERE model = ?`, model).
		Scan(&count, &meanOut, &m2Out, &meanLat, &m2Lat)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read baseline: %w", err)
	}

	// Welford online update
	count++
	meanOut, m2Out = welfordUpdate(count, meanOut, m2Out, float64(outputTokens))
	meanLat, m2Lat = welfordUpdate(count, meanLat, m2Lat, float64(latencyMs))

	isReady := 0
	if count >= 200 {
		isReady = 1
	}

	_, err = tx.Exec(`
		INSERT INTO baselines (model, count, mean_output_tokens, m2_output_tokens, mean_latency_ms, m2_latency_ms, is_ready, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(model) DO UPDATE SET
			count              = excluded.count,
			mean_output_tokens = excluded.mean_output_tokens,
			m2_output_tokens   = excluded.m2_output_tokens,
			mean_latency_ms    = excluded.mean_latency_ms,
			m2_latency_ms      = excluded.m2_latency_ms,
			is_ready           = excluded.is_ready,
			updated_at         = excluded.updated_at`,
		model, count, meanOut, m2Out, meanLat, m2Lat, isReady,
	)
	if err != nil {
		return fmt.Errorf("upsert baseline: %w", err)
	}

	return tx.Commit()
}

// GetBaseline returns the current baseline for a model.
// The second return value is false if no baseline exists yet.
func (d *DB) GetBaseline(model string) (Baseline, bool, error) {
	var count, isReady int
	var meanOut, m2Out, meanLat, m2Lat float64

	err := d.sql.QueryRow(`
		SELECT count, mean_output_tokens, m2_output_tokens, mean_latency_ms, m2_latency_ms, is_ready
		FROM baselines WHERE model = ?`, model).
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
		IsReady:            isReady == 1,
	}, true, nil
}

// welfordUpdate applies Welford's online algorithm for one new value x.
// Returns the updated (mean, M2) pair.
func welfordUpdate(count int, mean, m2, x float64) (float64, float64) {
	delta := x - mean
	mean += delta / float64(count)
	delta2 := x - mean
	m2 += delta * delta2
	return mean, m2
}

// stddev returns population standard deviation from Welford M2 and count.
func stddev(m2 float64, count int) float64 {
	if count < 2 {
		return 0
	}
	return math.Sqrt(m2 / float64(count))
}
