package store_test

import (
	"os"
	"testing"

	"github.com/whozpj/argus/server/internal/store"
)

// newTestDB returns a *store.DB for tests.
//
// Strategy (in order):
//  1. If POSTGRES_TEST_URL is set, connect directly — no Docker needed.
//  2. Otherwise, try to spin up a testcontainer. If Docker is unavailable,
//     skip the test with a clear message rather than failing it.
func newTestDB(t *testing.T) *store.DB {
	t.Helper()

	if dsn := os.Getenv("POSTGRES_TEST_URL"); dsn != "" {
		db, err := store.Open(dsn)
		if err != nil {
			t.Fatalf("open test DB (%s): %v", dsn, err)
		}
		if err := db.Truncate(); err != nil { t.Fatalf("truncate: %v", err) }
		t.Cleanup(func() { _ = db.Close() })
		return db
	}

	// Attempt testcontainers; skip if Docker unavailable.
	db, skip := tryTestcontainers(t)
	if skip {
		t.Skip("no POSTGRES_TEST_URL set and Docker unavailable — skipping integration test")
	}
	return db
}
