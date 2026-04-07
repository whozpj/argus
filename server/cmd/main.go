package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/whozpj/argus/server/internal/alerts"
	"github.com/whozpj/argus/server/internal/api"
	"github.com/whozpj/argus/server/internal/drift"
	"github.com/whozpj/argus/server/internal/ingest"
	"github.com/whozpj/argus/server/internal/store"
)

func main() {
	dbPath := getenv("ARGUS_DB_PATH", "argus.db")
	addr := getenv("ARGUS_ADDR", ":4000")

	db, err := store.Open(dbPath)
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	var notifier alerts.Notifier = alerts.Noop{}
	if webhook := getenv("ARGUS_SLACK_WEBHOOK", ""); webhook != "" {
		notifier = alerts.NewSlack(webhook)
		slog.Info("slack alerts enabled")
	}
	go drift.New(db, drift.Interval, notifier).Run()

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/events", ingest.NewHandler(db))
	mux.HandleFunc("GET /api/v1/baselines", api.NewBaselinesHandler(db))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	slog.Info("argus server starting", "addr", addr, "db", dbPath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
