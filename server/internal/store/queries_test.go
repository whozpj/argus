package store_test

import (
	"testing"

	"github.com/whozpj/argus/server/internal/store"
)

func insertTestEvent(t *testing.T, db *store.DB, projectID, model string, outputTokens, latencyMs int) {
	t.Helper()
	err := db.InsertEvent(store.Event{
		ProjectID:    projectID,
		Model:        model,
		Provider:     "openai",
		InputTokens:  10,
		OutputTokens: outputTokens,
		LatencyMs:    latencyMs,
		FinishReason: "stop",
		TimestampUTC: "2026-04-08T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
}

func TestOwnsProject_Owner(t *testing.T) {
	db := newTestDB(t)
	userID, err := db.UpsertUser("owns@example.com", "gh-1", "")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	proj, err := db.CreateProject(userID, "my-project")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	owns, err := db.OwnsProject(userID, proj.ID)
	if err != nil {
		t.Fatalf("OwnsProject: %v", err)
	}
	if !owns {
		t.Error("expected owner to own their project")
	}
}

func TestOwnsProject_NonOwner(t *testing.T) {
	db := newTestDB(t)
	ownerID, _ := db.UpsertUser("owner@example.com", "gh-2", "")
	attackerID, _ := db.UpsertUser("attacker@example.com", "gh-3", "")
	proj, err := db.CreateProject(ownerID, "owners-project")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	owns, err := db.OwnsProject(attackerID, proj.ID)
	if err != nil {
		t.Fatalf("OwnsProject: %v", err)
	}
	if owns {
		t.Error("non-owner should not own someone else's project")
	}
}

func TestOwnsProject_NonExistentProject(t *testing.T) {
	db := newTestDB(t)
	userID, _ := db.UpsertUser("user@example.com", "gh-4", "")
	owns, err := db.OwnsProject(userID, "does-not-exist")
	if err != nil {
		t.Fatalf("OwnsProject: %v", err)
	}
	if owns {
		t.Error("should not own a non-existent project")
	}
}

func TestReadyModels(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-ready-models"
	model := "gpt-4o"

	models, err := db.ReadyModels(projectID)
	if err != nil {
		t.Fatalf("ReadyModels: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 ready models initially, got %d", len(models))
	}

	for i := 0; i < 200; i++ {
		if err := db.UpdateBaseline(projectID, model, 50, 200); err != nil {
			t.Fatalf("UpdateBaseline[%d]: %v", i, err)
		}
	}

	models, err = db.ReadyModels(projectID)
	if err != nil {
		t.Fatalf("ReadyModels after 200: %v", err)
	}
	if len(models) != 1 || models[0] != model {
		t.Errorf("expected [%s], got %v", model, models)
	}
}

func TestBaselineSample(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-sample"
	model := "claude-sonnet-4-6"

	for i := 0; i < 5; i++ {
		insertTestEvent(t, db, projectID, model, 100+i, 500)
	}

	sample, err := db.BaselineSample(projectID, model, 3)
	if err != nil {
		t.Fatalf("BaselineSample: %v", err)
	}
	if len(sample) != 3 {
		t.Errorf("expected 3 events, got %d", len(sample))
	}
	// Should be oldest first — lowest output tokens inserted first
	if sample[0].OutputTokens > sample[2].OutputTokens {
		t.Error("BaselineSample should return oldest events first")
	}
}

func TestRecentSample(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-recent"
	model := "claude-sonnet-4-6"

	for i := 0; i < 5; i++ {
		insertTestEvent(t, db, projectID, model, 100+i, 500)
	}

	sample, err := db.RecentSample(projectID, model, 3)
	if err != nil {
		t.Fatalf("RecentSample: %v", err)
	}
	if len(sample) != 3 {
		t.Errorf("expected 3 events, got %d", len(sample))
	}
	// Should be newest 3, returned oldest-first within the window
	if sample[0].OutputTokens > sample[2].OutputTokens {
		t.Error("RecentSample should return events oldest-first within the window")
	}
}
