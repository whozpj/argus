package store_test

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/whozpj/argus/server/internal/store"
)

// tryTestcontainers attempts to spin up a throwaway Postgres container.
// Returns (db, false) on success, (nil, true) if Docker is unavailable.
func tryTestcontainers(t *testing.T) (*store.DB, bool) {
	t.Helper()
	ctx := context.Background()

	pgc, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("argus_test"),
		postgres.WithUsername("argus"),
		postgres.WithPassword("argus"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		// Docker unavailable — signal skip.
		return nil, true
	}
	t.Cleanup(func() { _ = pgc.Terminate(ctx) })

	dsn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get container DSN: %v", err)
	}
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, false
}
