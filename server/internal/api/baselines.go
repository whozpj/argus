package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/whozpj/argus/server/internal/store"
)

type baselinesResponse struct {
	TotalEvents int            `json:"total_events"`
	Baselines   []baselineJSON `json:"baselines"`
}

type baselineJSON struct {
	Model              string  `json:"model"`
	Count              int     `json:"count"`
	MeanOutputTokens   float64 `json:"mean_output_tokens"`
	StdDevOutputTokens float64 `json:"stddev_output_tokens"`
	MeanLatencyMs      float64 `json:"mean_latency_ms"`
	StdDevLatencyMs    float64 `json:"stddev_latency_ms"`
	IsReady            bool    `json:"is_ready"`
	// Drift fields — zero/false until the detector has run at least once.
	DriftScore     float64 `json:"drift_score"`
	DriftAlerted   bool    `json:"drift_alerted"`
	POutputTokens  float64 `json:"p_output_tokens"`
	PLatencyMs     float64 `json:"p_latency_ms"`
}

// NewBaselinesHandler returns a handler for GET /api/v1/baselines.
func NewBaselinesHandler(db *store.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		baselines, err := db.ListBaselines()
		if err != nil {
			slog.Error("api: list baselines", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		total, err := db.EventCount()
		if err != nil {
			slog.Error("api: event count", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		driftStates, err := db.GetDriftStates()
		if err != nil {
			slog.Error("api: drift states", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		resp := baselinesResponse{
			TotalEvents: total,
			Baselines:   make([]baselineJSON, 0, len(baselines)),
		}
		for _, b := range baselines {
			row := baselineJSON{
				Model:              b.Model,
				Count:              b.Count,
				MeanOutputTokens:   round2(b.MeanOutputTokens),
				StdDevOutputTokens: round2(b.StdDevOutputTokens),
				MeanLatencyMs:      round2(b.MeanLatencyMs),
				StdDevLatencyMs:    round2(b.StdDevLatencyMs),
				IsReady:            b.IsReady,
			}
			if ds, ok := driftStates[b.Model]; ok {
				row.DriftScore = round2(ds.Score)
				row.DriftAlerted = ds.Alerted
				row.POutputTokens = round2(ds.POutputTokens)
				row.PLatencyMs = round2(ds.PLatencyMs)
			}
			resp.Baselines = append(resp.Baselines, row)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
