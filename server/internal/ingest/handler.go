package ingest

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/whozpj/argus/server/internal/store"
)

type eventRequest struct {
	Model        string `json:"model"`
	Provider     string `json:"provider"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	LatencyMs    int    `json:"latency_ms"`
	FinishReason string `json:"finish_reason"`
	TimestampUTC string `json:"timestamp_utc"`
}

type Handler struct {
	db *store.DB
}

func NewHandler(db *store.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req eventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Model == "" || req.Provider == "" {
		http.Error(w, "model and provider are required", http.StatusBadRequest)
		return
	}

	err := h.db.InsertEvent(store.Event{
		ProjectID:    "self-hosted",
		Model:        req.Model,
		Provider:     req.Provider,
		InputTokens:  req.InputTokens,
		OutputTokens: req.OutputTokens,
		LatencyMs:    req.LatencyMs,
		FinishReason: req.FinishReason,
		TimestampUTC: req.TimestampUTC,
	})
	if err != nil {
		slog.Error("insert event", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := h.db.UpdateBaseline("self-hosted", req.Model, req.OutputTokens, req.LatencyMs); err != nil {
		slog.Error("update baseline", "err", err, "model", req.Model)
		// Non-fatal: event is saved; don't reject the request over a baseline failure.
	}

	slog.Info("event received", "model", req.Model, "output_tokens", req.OutputTokens, "latency_ms", req.LatencyMs)
	w.WriteHeader(http.StatusAccepted)
}
