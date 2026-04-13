package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/whozpj/argus/server/internal/store"
)

func TestBaselinesHandler_EmptyDB(t *testing.T) {
	db := newTestDB(t)
	h := NewBaselinesHandler(db)

	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/api/v1/baselines", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp baselinesResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalEvents != 0 {
		t.Errorf("total_events = %d, want 0", resp.TotalEvents)
	}
	if len(resp.Baselines) != 0 {
		t.Errorf("baselines = %v, want []", resp.Baselines)
	}
}

func TestBaselinesHandler_ReturnsJSON(t *testing.T) {
	db := newTestDB(t)
	db.InsertEvent(store.Event{ //nolint:errcheck
		ProjectID: "self-hosted",
		Model: "gpt-4o", Provider: "openai", InputTokens: 10,
		OutputTokens: 50, LatencyMs: 200,
		FinishReason: "stop", TimestampUTC: "2026-04-07T00:00:00Z",
	})
	db.UpdateBaseline("self-hosted", "gpt-4o", 50, 200) //nolint:errcheck

	h := NewBaselinesHandler(db)
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/api/v1/baselines", nil))

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var resp baselinesResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalEvents != 1 {
		t.Errorf("total_events = %d, want 1", resp.TotalEvents)
	}
	if len(resp.Baselines) != 1 {
		t.Fatalf("len(baselines) = %d, want 1", len(resp.Baselines))
	}
	if resp.Baselines[0].Model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", resp.Baselines[0].Model)
	}
}

func TestBaselinesHandler_CORSHeader(t *testing.T) {
	db := newTestDB(t)
	h := NewBaselinesHandler(db)
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/api/v1/baselines", nil))

	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
}

func TestBaselinesHandler_IsReadyField(t *testing.T) {
	db := newTestDB(t)
	for i := 0; i < 200; i++ {
		db.UpdateBaseline("self-hosted", "claude-sonnet-4-6", 50, 200) //nolint:errcheck
	}

	h := NewBaselinesHandler(db)
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/api/v1/baselines", nil))

	var resp baselinesResponse
	json.NewDecoder(rr.Body).Decode(&resp) //nolint:errcheck
	if !resp.Baselines[0].IsReady {
		t.Error("is_ready should be true after 200 events")
	}
}
