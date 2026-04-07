package store

import (
	"testing"
)

func insertTestEvent(t *testing.T, db *DB, model string, outputTokens, latencyMs int) {
	t.Helper()
	err := db.InsertEvent(Event{
		Model:        model,
		Provider:     "anthropic",
		InputTokens:  10,
		OutputTokens: outputTokens,
		LatencyMs:    latencyMs,
		FinishReason: "stop",
		TimestampUTC: "2026-04-07T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
}

func TestReadyModels_EmptyWhenNoBaselines(t *testing.T) {
	db := openTestDB(t)
	models, err := db.ReadyModels()
	if err != nil {
		t.Fatalf("ReadyModels: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 ready models, got %v", models)
	}
}

func TestReadyModels_OnlyReturnsReadyOnes(t *testing.T) {
	db := openTestDB(t)

	// Force-insert a ready and a not-ready baseline directly.
	db.sql.Exec(`INSERT INTO baselines (model, count, mean_output_tokens, m2_output_tokens, mean_latency_ms, m2_latency_ms, is_ready, updated_at) VALUES ('ready-model', 200, 50, 0, 200, 0, 1, datetime('now'))`)
	db.sql.Exec(`INSERT INTO baselines (model, count, mean_output_tokens, m2_output_tokens, mean_latency_ms, m2_latency_ms, is_ready, updated_at) VALUES ('not-ready', 50, 50, 0, 200, 0, 0, datetime('now'))`)

	models, err := db.ReadyModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0] != "ready-model" {
		t.Errorf("ReadyModels = %v, want [ready-model]", models)
	}
}

func TestBaselineSample_ReturnsOldestFirst(t *testing.T) {
	db := openTestDB(t)

	// Insert 5 events with distinct output_tokens we can track.
	for i := 1; i <= 5; i++ {
		insertTestEvent(t, db, "claude-sonnet-4-6", i*10, 100)
	}

	events, err := db.BaselineSample("claude-sonnet-4-6", 3)
	if err != nil {
		t.Fatalf("BaselineSample: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	// Oldest first → output_tokens should be 10, 20, 30
	if events[0].OutputTokens != 10 || events[1].OutputTokens != 20 || events[2].OutputTokens != 30 {
		t.Errorf("unexpected order: %v %v %v", events[0].OutputTokens, events[1].OutputTokens, events[2].OutputTokens)
	}
}

func TestRecentSample_ReturnsNewestChronological(t *testing.T) {
	db := openTestDB(t)

	for i := 1; i <= 5; i++ {
		insertTestEvent(t, db, "gpt-4o", i*10, 100)
	}

	events, err := db.RecentSample("gpt-4o", 3)
	if err != nil {
		t.Fatalf("RecentSample: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	// Newest 3 are output_tokens 30, 40, 50 → returned oldest-first → 30, 40, 50
	if events[0].OutputTokens != 30 || events[1].OutputTokens != 40 || events[2].OutputTokens != 50 {
		t.Errorf("unexpected order: %v %v %v", events[0].OutputTokens, events[1].OutputTokens, events[2].OutputTokens)
	}
}

func TestBaselineSample_LimitRespected(t *testing.T) {
	db := openTestDB(t)
	for i := 0; i < 10; i++ {
		insertTestEvent(t, db, "model-x", 50, 200)
	}
	events, _ := db.BaselineSample("model-x", 4)
	if len(events) != 4 {
		t.Errorf("got %d events, want 4", len(events))
	}
}

func TestRecentSample_EmptyForUnknownModel(t *testing.T) {
	db := openTestDB(t)
	events, err := db.RecentSample("no-such-model", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}
