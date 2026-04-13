package drift

import (
	"testing"
	"time"

	"github.com/whozpj/argus/server/internal/alerts"
	"github.com/whozpj/argus/server/internal/store"
)

// noopNotifier satisfies alerts.Notifier without doing anything.
type noopNotifier struct{}

func (noopNotifier) Fire(string, float64, float64, float64) error { return nil }
func (noopNotifier) Clear(string) error                           { return nil }

// spyNotifier records calls for assertion in tests.
type spyNotifier struct {
	fired  []string // model names passed to Fire
	cleared []string // model names passed to Clear
}

func (s *spyNotifier) Fire(model string, _, _, _ float64) error {
	s.fired = append(s.fired, model)
	return nil
}
func (s *spyNotifier) Clear(model string) error {
	s.cleared = append(s.cleared, model)
	return nil
}

var _ alerts.Notifier = noopNotifier{}
var _ alerts.Notifier = &spyNotifier{}

// seedEvents inserts n events and updates the baseline for each, so that
// after 200 calls the baseline is_ready flag flips automatically.
func seedEvents(t *testing.T, db *store.DB, model string, n, outputTokens, latencyMs int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := db.InsertEvent(store.Event{
			ProjectID:    "self-hosted",
			Model:        model,
			Provider:     "anthropic",
			InputTokens:  50,
			OutputTokens: outputTokens,
			LatencyMs:    latencyMs,
			FinishReason: "stop",
			TimestampUTC: "2026-04-07T00:00:00Z",
		}); err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
		if err := db.UpdateBaseline("self-hosted", model, outputTokens, latencyMs); err != nil {
			t.Fatalf("UpdateBaseline: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// RunOnce / checkModel
// ---------------------------------------------------------------------------

func TestRunOnce_SkipsModelsWithTooFewRecentEvents(t *testing.T) {
	db := newTestDB(t)
	seedEvents(t, db, "model-a", 200, 50, 200) // builds baseline to is_ready=1
	seedEvents(t, db, "model-a", 5, 50, 200)   // only 5 recent — below minRecentN

	d := New(db, time.Hour, noopNotifier{})
	result := d.checkModel("model-a")

	if result.Score != 0 {
		t.Errorf("score = %v, expected 0 (skipped due to insufficient recent data)", result.Score)
	}
}

func TestRunOnce_NoDriftForIdenticalDistributions(t *testing.T) {
	db := newTestDB(t)
	// 200 events build the baseline (is_ready=1); 50 more are the "recent" window.
	seedEvents(t, db, "gpt-4o", 250, 50, 200)

	d := New(db, time.Hour, noopNotifier{})
	result := d.checkModel("gpt-4o")

	if result.Score > 0.3 {
		t.Errorf("score = %v for identical distributions, expected < 0.3", result.Score)
	}
	if result.AlertFired {
		t.Error("alert should not fire for identical distributions")
	}
}

func TestRunOnce_DriftDetectedForShiftedDistribution(t *testing.T) {
	db := newTestDB(t)
	// 200 events build the baseline; 50 more with 10× output_tokens are the "recent" window.
	seedEvents(t, db, "claude-sonnet-4-6", 200, 50, 200)
	seedEvents(t, db, "claude-sonnet-4-6", 50, 500, 200)

	d := New(db, time.Hour, noopNotifier{})
	result := d.checkModel("claude-sonnet-4-6")

	if result.Score < 0.7 {
		t.Errorf("score = %v, expected > 0.7 for a large distribution shift", result.Score)
	}
	if !result.AlertFired {
		t.Error("AlertFired should be true for score > 0.7")
	}
}

func TestRunOnce_ScoreInRange(t *testing.T) {
	db := newTestDB(t)
	seedEvents(t, db, "model-x", 250, 50, 200)

	d := New(db, time.Hour, noopNotifier{})
	result := d.checkModel("model-x")

	if result.Score < 0 || result.Score > 1 {
		t.Errorf("score %v is outside [0, 1]", result.Score)
	}
}

// ---------------------------------------------------------------------------
// Hysteresis state machine
// ---------------------------------------------------------------------------

func TestHysteresis_AlertFiresOnce(t *testing.T) {
	db := newTestDB(t)
	seedEvents(t, db, "model-h", 200, 50, 200)
	seedEvents(t, db, "model-h", 50, 500, 200) // drifted

	d := New(db, time.Hour, noopNotifier{})

	r1 := d.checkModel("model-h")
	r2 := d.checkModel("model-h") // second window — still drifted

	if !r1.AlertFired {
		t.Error("alert should fire on first detection")
	}
	if r2.AlertFired {
		t.Error("alert should not re-fire on second detection while already alerted")
	}
}

func TestHysteresis_AlertClearsAfterThreeGoodWindows(t *testing.T) {
	db := newTestDB(t)
	model := "model-clear"
	seedEvents(t, db, model, 200, 50, 200)
	seedEvents(t, db, model, 50, 500, 200) // trigger drift

	d := New(db, time.Hour, noopNotifier{})
	d.checkModel(model) // fires alert

	// Now insert 50 more events matching the baseline distribution.
	// The recent window will slide to contain only the "normal" events.
	seedEvents(t, db, model, 50, 50, 200)
	// After 3 consecutive clear windows the alert should clear.
	var cleared bool
	for i := 0; i < 3; i++ {
		r := d.checkModel(model)
		if r.AlertCleared {
			cleared = true
		}
	}
	if !cleared {
		t.Error("alert should have cleared after 3 consecutive windows below clearThreshold")
	}
}

func TestHysteresis_ClearCountResetsIfDriftReturns(t *testing.T) {
	db := newTestDB(t)
	model := "model-flap"
	seedEvents(t, db, model, 200, 50, 200)
	seedEvents(t, db, model, 50, 500, 200) // trigger

	d := New(db, time.Hour, noopNotifier{})
	d.checkModel(model) // alerted

	// One "good" window — partially into clearing.
	seedEvents(t, db, model, 50, 50, 200)
	d.checkModel(model)

	state := d.states[model]
	clearCountAfterOneGoodWindow := state.clearCount

	// Drift comes back — re-insert drifted events.
	seedEvents(t, db, model, 50, 500, 200)
	d.checkModel(model)

	if state.clearCount >= clearCountAfterOneGoodWindow {
		t.Error("clear count should reset when drift returns")
	}
}

// ---------------------------------------------------------------------------
// floats helper
// ---------------------------------------------------------------------------

func TestFloats_ExtractsCorrectField(t *testing.T) {
	events := []store.Event{
		{OutputTokens: 10, LatencyMs: 100},
		{OutputTokens: 20, LatencyMs: 200},
	}
	got := floats(events, func(e store.Event) float64 { return float64(e.OutputTokens) })
	if len(got) != 2 || got[0] != 10 || got[1] != 20 {
		t.Errorf("floats = %v, want [10 20]", got)
	}
}

func TestFloats_Empty(t *testing.T) {
	got := floats(nil, func(e store.Event) float64 { return float64(e.OutputTokens) })
	if len(got) != 0 {
		t.Errorf("floats(nil) = %v, want []", got)
	}
}

// ---------------------------------------------------------------------------
// Notifier integration
// ---------------------------------------------------------------------------

func TestNotifier_FireCalledOnAlert(t *testing.T) {
	db := newTestDB(t)
	seedEvents(t, db, "model-n", 200, 50, 200)
	seedEvents(t, db, "model-n", 50, 500, 200) // drifted

	spy := &spyNotifier{}
	d := New(db, time.Hour, spy)
	d.checkModel("model-n")

	if len(spy.fired) != 1 || spy.fired[0] != "model-n" {
		t.Errorf("Fire not called correctly: fired=%v", spy.fired)
	}
}

func TestNotifier_FireNotCalledWhenNoDrift(t *testing.T) {
	db := newTestDB(t)
	seedEvents(t, db, "model-ok", 250, 50, 200) // identical baseline + recent

	spy := &spyNotifier{}
	d := New(db, time.Hour, spy)
	d.checkModel("model-ok")

	if len(spy.fired) != 0 {
		t.Errorf("Fire should not be called when there is no drift: fired=%v", spy.fired)
	}
}

func TestNotifier_ClearCalledAfterResolution(t *testing.T) {
	db := newTestDB(t)
	model := "model-resolve"
	seedEvents(t, db, model, 200, 50, 200)
	seedEvents(t, db, model, 50, 500, 200) // trigger

	spy := &spyNotifier{}
	d := New(db, time.Hour, spy)
	d.checkModel(model) // fires alert

	seedEvents(t, db, model, 50, 50, 200) // slide window back to normal
	for i := 0; i < clearWindows; i++ {
		d.checkModel(model)
	}

	if len(spy.cleared) != 1 || spy.cleared[0] != model {
		t.Errorf("Clear not called correctly: cleared=%v", spy.cleared)
	}
}

func TestNotifier_FireCalledOnceNotRepeatedly(t *testing.T) {
	db := newTestDB(t)
	seedEvents(t, db, "model-once", 200, 50, 200)
	seedEvents(t, db, "model-once", 50, 500, 200)

	spy := &spyNotifier{}
	d := New(db, time.Hour, spy)
	d.checkModel("model-once")
	d.checkModel("model-once") // second window — still drifted
	d.checkModel("model-once") // third

	if len(spy.fired) != 1 {
		t.Errorf("Fire should be called exactly once, got %d calls", len(spy.fired))
	}
}

