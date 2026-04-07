package store

import "testing"

func TestListBaselines_EmptyDB(t *testing.T) {
	db := openTestDB(t)
	baselines, err := db.ListBaselines()
	if err != nil {
		t.Fatal(err)
	}
	if len(baselines) != 0 {
		t.Errorf("expected 0 baselines, got %d", len(baselines))
	}
}

func TestListBaselines_ReturnsAllModels(t *testing.T) {
	db := openTestDB(t)
	db.UpdateBaseline("model-b", 50, 200) //nolint:errcheck
	db.UpdateBaseline("model-a", 50, 200) //nolint:errcheck

	baselines, err := db.ListBaselines()
	if err != nil {
		t.Fatal(err)
	}
	if len(baselines) != 2 {
		t.Fatalf("expected 2 baselines, got %d", len(baselines))
	}
	// Should be sorted alphabetically.
	if baselines[0].Model != "model-a" || baselines[1].Model != "model-b" {
		t.Errorf("unexpected order: %v", []string{baselines[0].Model, baselines[1].Model})
	}
}

func TestListBaselines_IncludesComputedStdDev(t *testing.T) {
	db := openTestDB(t)
	for _, tokens := range []int{10, 20, 30} {
		db.UpdateBaseline("gpt-4o", tokens, 100) //nolint:errcheck
	}
	baselines, _ := db.ListBaselines()
	if baselines[0].StdDevOutputTokens == 0 {
		t.Error("expected non-zero stddev for 3 different values")
	}
}

func TestEventCount_Empty(t *testing.T) {
	db := openTestDB(t)
	n, err := db.EventCount()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0, got %d", n)
	}
}

func TestEventCount_AfterInserts(t *testing.T) {
	db := openTestDB(t)
	for i := 0; i < 5; i++ {
		db.InsertEvent(Event{ //nolint:errcheck
			Model: "m", Provider: "p", InputTokens: 10,
			OutputTokens: 50, LatencyMs: 200,
			FinishReason: "stop", TimestampUTC: "2026-04-07T00:00:00Z",
		})
	}
	n, _ := db.EventCount()
	if n != 5 {
		t.Errorf("expected 5, got %d", n)
	}
}
