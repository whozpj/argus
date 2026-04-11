package ingest_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whozpj/argus/server/internal/auth"
	"github.com/whozpj/argus/server/internal/ingest"
)

const validEventBody = `{
	"model":"gpt-4o","provider":"openai","input_tokens":10,
	"output_tokens":50,"latency_ms":200,"finish_reason":"stop",
	"timestamp_utc":"2026-04-09T00:00:00Z"
}`

func TestHandler_NoAPIKey_StoresAsSelfHosted(t *testing.T) {
	db := newTestDB(t)
	// Wrap handler with ResolveProject middleware (as main.go does)
	h := auth.ResolveProject(db)(ingest.NewHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(validEventBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202; body: %s", rr.Code, rr.Body.String())
	}
	count, err := db.EventCount("self-hosted")
	if err != nil {
		t.Fatalf("event count: %v", err)
	}
	if count != 1 {
		t.Errorf("event count = %d, want 1", count)
	}
}

func TestHandler_WithAPIKey_StoresToProject(t *testing.T) {
	db := newTestDB(t)
	// Create user → project → api key
	userID, _ := db.UpsertUser("ingest@example.com", "gh-ingest", "")
	proj, _ := db.CreateProject(userID, "my-proj")
	rawKey, hash, prefix, _ := auth.GenerateAPIKey()
	db.CreateAPIKey(proj.ID, hash, prefix, "ingest-key") //nolint:errcheck

	h := auth.ResolveProject(db)(ingest.NewHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(validEventBody))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", rr.Code)
	}
	count, err := db.EventCount(proj.ID)
	if err != nil {
		t.Fatalf("event count: %v", err)
	}
	if count != 1 {
		t.Errorf("event count for project = %d, want 1", count)
	}
	// Nothing stored under self-hosted
	shCount, _ := db.EventCount("self-hosted")
	if shCount != 0 {
		t.Errorf("self-hosted count = %d, want 0", shCount)
	}
}

func TestHandler_InvalidAPIKey_Returns401(t *testing.T) {
	db := newTestDB(t)
	h := auth.ResolveProject(db)(ingest.NewHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(validEventBody))
	req.Header.Set("Authorization", "Bearer argus_sk_doesnotexist123456")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestHandler_MissingFields_Returns400(t *testing.T) {
	db := newTestDB(t)
	h := auth.ResolveProject(db)(ingest.NewHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events",
		strings.NewReader(`{"model":"","provider":""}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}
