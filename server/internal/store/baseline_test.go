package store_test

import (
	"testing"
)

func TestUpdateAndGetBaseline(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-test"
	model := "claude-sonnet-4-6"

	// No baseline yet
	_, found, err := db.GetBaseline(projectID, model)
	if err != nil {
		t.Fatalf("GetBaseline: %v", err)
	}
	if found {
		t.Fatal("expected no baseline initially")
	}

	// Add 3 observations
	for i := 0; i < 3; i++ {
		if err := db.UpdateBaseline(projectID, model, 100, 500); err != nil {
			t.Fatalf("UpdateBaseline[%d]: %v", i, err)
		}
	}

	b, found, err := db.GetBaseline(projectID, model)
	if err != nil {
		t.Fatalf("GetBaseline after updates: %v", err)
	}
	if !found {
		t.Fatal("expected baseline to exist after updates")
	}
	if b.Count != 3 {
		t.Errorf("count = %d, want 3", b.Count)
	}
	if b.IsReady {
		t.Error("baseline should not be ready at count 3")
	}
	if b.MeanOutputTokens != 100 {
		t.Errorf("mean_output_tokens = %f, want 100", b.MeanOutputTokens)
	}
}

func TestBaselineIsReadyAt200(t *testing.T) {
	db := newTestDB(t)
	projectID := "proj-ready"
	model := "gpt-4o"

	for i := 0; i < 200; i++ {
		if err := db.UpdateBaseline(projectID, model, 50, 300); err != nil {
			t.Fatalf("UpdateBaseline[%d]: %v", i, err)
		}
	}

	b, found, err := db.GetBaseline(projectID, model)
	if err != nil {
		t.Fatalf("GetBaseline: %v", err)
	}
	if !found {
		t.Fatal("expected baseline to exist")
	}
	if !b.IsReady {
		t.Errorf("expected IsReady=true at count 200, got false")
	}
}

func TestBaselineIsolatedByProject(t *testing.T) {
	db := newTestDB(t)
	model := "claude-sonnet-4-6"

	if err := db.UpdateBaseline("project-A", model, 100, 500); err != nil {
		t.Fatalf("UpdateBaseline project-A: %v", err)
	}

	_, found, err := db.GetBaseline("project-B", model)
	if err != nil {
		t.Fatalf("GetBaseline project-B: %v", err)
	}
	if found {
		t.Error("project-B should not see project-A's baseline")
	}
}
