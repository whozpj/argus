package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/whozpj/argus/server/internal/alerts"
	"github.com/whozpj/argus/server/internal/api"
	"github.com/whozpj/argus/server/internal/auth"
	"github.com/whozpj/argus/server/internal/drift"
	"github.com/whozpj/argus/server/internal/ingest"
	"github.com/whozpj/argus/server/internal/store"
)

func main() {
	dsn := getenv("POSTGRES_URL", "postgres://argus:argus@localhost:5432/argus?sslmode=disable")
	addr := getenv("ARGUS_ADDR", ":4000")

	db, err := store.Open(dsn)
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

	oauthCfg := auth.OAuthConfig{
		BaseURL:            getenv("ARGUS_BASE_URL", "http://localhost:4000"),
		UIURL:              getenv("ARGUS_UI_URL", "http://localhost:3000"),
		GitHubClientID:     getenv("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: getenv("GITHUB_CLIENT_SECRET", ""),
		GoogleClientID:     getenv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getenv("GOOGLE_CLIENT_SECRET", ""),
	}

	mux := http.NewServeMux()

	// Auth routes — OAuth redirects/callbacks, CLI login, token exchange.
	auth.RegisterRoutes(mux, db, oauthCfg)

	// Project and API key management — JWT required.
	auth.RegisterProjectRoutes(mux, db)

	// Ingest — optional API key; resolves projectID, falls back to "self-hosted".
	mux.Handle("POST /api/v1/events", auth.ResolveProject(db)(ingest.NewHandler(db)))

	// Baselines — optional API key; resolves projectID, falls back to "self-hosted".
	mux.Handle("GET /api/v1/baselines", auth.ResolveProject(db)(http.HandlerFunc(api.NewBaselinesHandler(db))))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	slog.Info("argus server starting", "addr", addr, "dsn", dsn)
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
