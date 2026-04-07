package drift

import (
	"log/slog"
	"time"

	"github.com/whozpj/argus/server/internal/store"
)

const (
	Interval       = 60 * time.Second
	alertThreshold = 0.7
	clearThreshold = 0.4
	clearWindows   = 3  // consecutive windows below clearThreshold to clear alert
	baselineN      = 200
	recentN        = 50
	minRecentN     = 10 // skip check if fewer recent events than this
)

type modelState struct {
	alerted    bool
	clearCount int // consecutive windows scoring below clearThreshold
}

// Detector runs drift detection on a periodic ticker.
type Detector struct {
	db       *store.DB
	states   map[string]*modelState
	interval time.Duration
}

// New returns a Detector. Pass a custom interval for tests; use Interval for production.
func New(db *store.DB, interval time.Duration) *Detector {
	return &Detector{
		db:       db,
		states:   make(map[string]*modelState),
		interval: interval,
	}
}

// Run starts the detection loop. Call in a goroutine; returns when ctx is done.
func (d *Detector) Run() {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for range ticker.C {
		d.RunOnce()
	}
}

// RunOnce executes one detection pass across all ready models.
// Exported so tests can trigger it directly without waiting for the ticker.
func (d *Detector) RunOnce() {
	models, err := d.db.ReadyModels()
	if err != nil {
		slog.Error("drift: list ready models", "err", err)
		return
	}
	for _, model := range models {
		d.checkModel(model)
	}
}

// DriftResult holds the outcome of one detection window for a model.
type DriftResult struct {
	Model          string
	Score          float64
	POutputTokens  float64
	PLatencyMs     float64
	AlertFired     bool
	AlertCleared   bool
}

func (d *Detector) checkModel(model string) DriftResult {
	result := DriftResult{Model: model}

	baseline, err := d.db.BaselineSample(model, baselineN)
	if err != nil {
		slog.Error("drift: baseline sample", "model", model, "err", err)
		return result
	}
	recent, err := d.db.RecentSample(model, recentN)
	if err != nil {
		slog.Error("drift: recent sample", "model", model, "err", err)
		return result
	}
	if len(recent) < minRecentN {
		return result // not enough data yet
	}

	baseOut := floats(baseline, func(e store.Event) float64 { return float64(e.OutputTokens) })
	recOut := floats(recent, func(e store.Event) float64 { return float64(e.OutputTokens) })
	baseLat := floats(baseline, func(e store.Event) float64 { return float64(e.LatencyMs) })
	recLat := floats(recent, func(e store.Event) float64 { return float64(e.LatencyMs) })

	pOut := mannWhitneyPValue(baseOut, recOut)
	pLat := mannWhitneyPValue(baseLat, recLat)
	score := driftScore([]float64{pOut, pLat})

	result.Score = score
	result.POutputTokens = pOut
	result.PLatencyMs = pLat

	slog.Info("drift check", "model", model, "score", score,
		"p_output_tokens", pOut, "p_latency_ms", pLat)

	state := d.stateFor(model)

	switch {
	case !state.alerted && score > alertThreshold:
		state.alerted = true
		state.clearCount = 0
		result.AlertFired = true
		slog.Warn("DRIFT DETECTED", "model", model, "score", score)

	case state.alerted && score < clearThreshold:
		state.clearCount++
		if state.clearCount >= clearWindows {
			state.alerted = false
			state.clearCount = 0
			result.AlertCleared = true
			slog.Info("drift cleared", "model", model)
		}

	case state.alerted:
		// Still drifting but not yet below clear threshold — reset clear streak.
		state.clearCount = 0
	}

	return result
}

func (d *Detector) stateFor(model string) *modelState {
	if _, ok := d.states[model]; !ok {
		d.states[model] = &modelState{}
	}
	return d.states[model]
}

func floats(events []store.Event, f func(store.Event) float64) []float64 {
	out := make([]float64, len(events))
	for i, e := range events {
		out[i] = f(e)
	}
	return out
}
