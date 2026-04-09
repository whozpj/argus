package store_test

import (
	"testing"

	"github.com/whozpj/argus/server/internal/store"
)

func TestEventCount(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-count"

	n, err := db.EventCount(projectID)
	if err != nil {
		t.Fatalf("EventCount: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 events initially, got %d", n)
	}

	err = db.InsertEvent(store.Event{
		ProjectID:    projectID,
		Model:        "gpt-4o",
		Provider:     "openai",
		InputTokens:  10,
		OutputTokens: 20,
		LatencyMs:    100,
		FinishReason: "stop",
		TimestampUTC: "2026-04-08T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	n, err = db.EventCount(projectID)
	if err != nil {
		t.Fatalf("EventCount after insert: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 event, got %d", n)
	}
}

func TestListBaselines(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-list"

	baselines, err := db.ListBaselines(projectID)
	if err != nil {
		t.Fatalf("ListBaselines empty: %v", err)
	}
	if len(baselines) != 0 {
		t.Errorf("expected 0 baselines, got %d", len(baselines))
	}

	if err := db.UpdateBaseline(projectID, "model-a", 100, 500); err != nil {
		t.Fatalf("UpdateBaseline: %v", err)
	}
	if err := db.UpdateBaseline(projectID, "model-b", 200, 800); err != nil {
		t.Fatalf("UpdateBaseline: %v", err)
	}

	baselines, err = db.ListBaselines(projectID)
	if err != nil {
		t.Fatalf("ListBaselines: %v", err)
	}
	if len(baselines) != 2 {
		t.Errorf("expected 2 baselines, got %d", len(baselines))
	}
	if baselines[0].Model != "model-a" || baselines[1].Model != "model-b" {
		t.Errorf("unexpected order: %v", baselines)
	}
}

func TestEventCountIsolatedByProject(t *testing.T) {
	db := newTestDB(t)

	_ = db.InsertEvent(store.Event{
		ProjectID: "proj-x", Model: "gpt-4o", Provider: "openai",
		InputTokens: 1, OutputTokens: 2, LatencyMs: 3,
		FinishReason: "stop", TimestampUTC: "2026-04-08T00:00:00Z",
	})

	n, err := db.EventCount("proj-y")
	if err != nil {
		t.Fatalf("EventCount: %v", err)
	}
	if n != 0 {
		t.Errorf("proj-y should not see proj-x events, got %d", n)
	}
}
