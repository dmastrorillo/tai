// Package storagetest provides test fixtures for the storage layer.
//
// NewMemDB returns a fresh in-memory SQLite database with all
// migrations applied — the standard fixture for unit tests in
// internal/storage and downstream packages (import, triage-state,
// etc.) that need to exercise queries without a real file on disk.
package storagetest

import (
	"context"
	"testing"

	"github.com/danielmastrorillo/tai/internal/storage"
)

// NewMemDB returns a fully-migrated in-memory database. The DB is
// closed automatically via t.Cleanup, so callers do not need to defer.
//
// Each call returns a fresh database — in-memory DBs are not shared
// across calls. This isolates tests.
func NewMemDB(t *testing.T) *storage.DB {
	t.Helper()
	// "file::memory:?cache=shared" would let multiple connections see
	// the same in-memory DB; we deliberately use plain ":memory:" so
	// each test gets its own isolated database with no risk of
	// cross-test contamination.
	db, err := storage.OpenAt(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("storagetest.NewMemDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
