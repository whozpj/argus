package main

import (
	"log/slog"
	"net/http"
	"os"

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

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/events", ingest.NewHandler(db))
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
