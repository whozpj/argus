package ingest_test

import (
	"context"
	"os"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/whozpj/argus/server/internal/store"
)

func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	if dsn := os.Getenv("POSTGRES_TEST_URL"); dsn != "" {
		db, err := store.Open(dsn)
		if err != nil {
			t.Fatalf("open test DB: %v", err)
		}
		if err := db.Truncate(); err != nil { t.Fatalf("truncate: %v", err) }
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	db, skip := tryTestcontainers(t)
	if skip {
		t.Skip("no POSTGRES_TEST_URL and Docker unavailable")
	}
	return db
}

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
