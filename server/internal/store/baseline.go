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
