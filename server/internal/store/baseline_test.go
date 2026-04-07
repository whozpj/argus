package store

import (
	"math"
	"testing"
)

// openTestDB creates a temporary in-memory SQLite DB for testing.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// ---------------------------------------------------------------------------
// welfordUpdate unit tests
// ---------------------------------------------------------------------------

func TestWelfordUpdate_KnownSeries(t *testing.T) {
	// Dataset: [2, 4, 4, 4, 5, 5, 7, 9]
	// Mean = 5, population variance = 4, stddev = 2
	data := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	var mean, m2 float64
	for i, x := range data {
		mean, m2 = welfordUpdate(i+1, mean, m2, x)
	}

	if math.Abs(mean-5.0) > 1e-9 {
		t.Errorf("mean = %v, want 5", mean)
	}
	got := stddev(m2, len(data))
	if math.Abs(got-2.0) > 1e-9 {
		t.Errorf("stddev = %v, want 2", got)
	}
}

func TestWelfordUpdate_SingleValue(t *testing.T) {
	mean, m2 := welfordUpdate(1, 0, 0, 42.0)
	if mean != 42.0 {
		t.Errorf("mean = %v, want 42", mean)
	}
	// With one sample, M2 = 0 and stddev should be 0
	if stddev(m2, 1) != 0 {
		t.Errorf("stddev(1 sample) = %v, want 0", stddev(m2, 1))
	}
}

func TestWelfordUpdate_IdenticalValues(t *testing.T) {
	// All values the same → stddev = 0
	var mean, m2 float64
	for i := 0; i < 10; i++ {
		mean, m2 = welfordUpdate(i+1, mean, m2, 7.0)
	}
	if math.Abs(mean-7.0) > 1e-9 {
		t.Errorf("mean = %v, want 7", mean)
	}
	if stddev(m2, 10) > 1e-9 {
		t.Errorf("stddev = %v, want 0 for identical values", stddev(m2, 10))
	}
}

// ---------------------------------------------------------------------------
// UpdateBaseline integration tests
// ---------------------------------------------------------------------------

func TestUpdateBaseline_FirstEvent(t *testing.T) {
	db := openTestDB(t)

	if err := db.UpdateBaseline("claude-sonnet-4-6", 50, 300); err != nil {
		t.Fatalf("UpdateBaseline: %v", err)
	}

	b, ok, err := db.GetBaseline("claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("GetBaseline: %v", err)
	}
	if !ok {
		t.Fatal("expected baseline to exist after first event")
	}
	if b.Count != 1 {
		t.Errorf("Count = %d, want 1", b.Count)
	}
	if b.MeanOutputTokens != 50 {
		t.Errorf("MeanOutputTokens = %v, want 50", b.MeanOutputTokens)
	}
	if b.MeanLatencyMs != 300 {
		t.Errorf("MeanLatencyMs = %v, want 300", b.MeanLatencyMs)
	}
	if b.IsReady {
		t.Error("IsReady should be false with only 1 event")
	}
}

func TestUpdateBaseline_AccumulatesCorrectly(t *testing.T) {
	db := openTestDB(t)

	// Insert three events with output_tokens = [10, 20, 30] → mean = 20
	for _, tokens := range []int{10, 20, 30} {
		if err := db.UpdateBaseline("gpt-4o", tokens, 100); err != nil {
			t.Fatalf("UpdateBaseline: %v", err)
		}
	}

	b, _, err := db.GetBaseline("gpt-4o")
	if err != nil {
		t.Fatalf("GetBaseline: %v", err)
	}
	if b.Count != 3 {
		t.Errorf("Count = %d, want 3", b.Count)
	}
	if math.Abs(b.MeanOutputTokens-20.0) > 1e-9 {
		t.Errorf("MeanOutputTokens = %v, want 20", b.MeanOutputTokens)
	}
	// population stddev of [10,20,30] = sqrt(200/3) ≈ 8.165
	wantStdDev := math.Sqrt(200.0 / 3.0)
	if math.Abs(b.StdDevOutputTokens-wantStdDev) > 1e-9 {
		t.Errorf("StdDevOutputTokens = %v, want %v", b.StdDevOutputTokens, wantStdDev)
	}
}

func TestUpdateBaseline_IsReadyFlipsAt200(t *testing.T) {
	db := openTestDB(t)

	for i := 0; i < 199; i++ {
		if err := db.UpdateBaseline("claude-haiku-4-5", 50, 200); err != nil {
			t.Fatalf("UpdateBaseline at %d: %v", i, err)
		}
	}

	b, _, _ := db.GetBaseline("claude-haiku-4-5")
	if b.IsReady {
		t.Error("IsReady should be false at count=199")
	}

	if err := db.UpdateBaseline("claude-haiku-4-5", 50, 200); err != nil {
		t.Fatalf("UpdateBaseline at 200: %v", err)
	}

	b, _, _ = db.GetBaseline("claude-haiku-4-5")
	if !b.IsReady {
		t.Error("IsReady should be true at count=200")
	}
	if b.Count != 200 {
		t.Errorf("Count = %d, want 200", b.Count)
	}
}

func TestUpdateBaseline_IndependentPerModel(t *testing.T) {
	db := openTestDB(t)

	if err := db.UpdateBaseline("model-a", 100, 500); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateBaseline("model-b", 200, 1000); err != nil {
		t.Fatal(err)
	}

	a, _, _ := db.GetBaseline("model-a")
	b, _, _ := db.GetBaseline("model-b")

	if a.MeanOutputTokens != 100 {
		t.Errorf("model-a mean = %v, want 100", a.MeanOutputTokens)
	}
	if b.MeanOutputTokens != 200 {
		t.Errorf("model-b mean = %v, want 200", b.MeanOutputTokens)
	}
}

// ---------------------------------------------------------------------------
// GetBaseline tests
// ---------------------------------------------------------------------------

func TestGetBaseline_NotFound(t *testing.T) {
	db := openTestDB(t)

	_, ok, err := db.GetBaseline("no-such-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok=false for unknown model")
	}
}

func TestGetBaseline_StdDevZeroForSingleSample(t *testing.T) {
	db := openTestDB(t)
	db.UpdateBaseline("gpt-4o", 50, 200) //nolint:errcheck

	b, _, _ := db.GetBaseline("gpt-4o")
	if b.StdDevOutputTokens != 0 {
		t.Errorf("StdDevOutputTokens = %v, want 0 for single sample", b.StdDevOutputTokens)
	}
}
