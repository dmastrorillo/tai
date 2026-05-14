package storage

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/danielmastrorillo/tai/internal/errcode"

	_ "modernc.org/sqlite"
)

// TestMigrations_TCSTG006_failed_migration_rolls_back exercises
// TC-STG-006: a migration whose SQL fails to apply causes the runner
// to roll back the transaction. No row is recorded in the migrations
// table and the returned error is *errcode.Error{Code:
// DBMigrationFailed}.
//
// This test lives in package storage (not storage_test) so it can call
// applyOne directly with a synthetic bad migration — the public Open
// path only consumes the embedded migrations directory, which doesn't
// admit invalid SQL.
func TestMigrations_TCSTG006_failed_migration_rolls_back(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	// Bootstrap a `migrations` table by hand so applyOne can record /
	// fail-to-record into it. The real Open path does this via the
	// first migration; we synthesise it here.
	if _, err := db.ExecContext(context.Background(), `
		CREATE TABLE migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at INTEGER NOT NULL
		)
	`); err != nil {
		t.Fatalf("bootstrap migrations table: %v", err)
	}

	bad := migration{
		Version: 999,
		Name:    "intentionally_broken",
		SQL:     "THIS IS NOT VALID SQL",
	}

	err = applyOne(context.Background(), db, bad)
	if err == nil {
		t.Fatal("expected DBMigrationFailed, got nil")
	}
	taiErr, ok := errcode.As(err)
	if !ok {
		t.Fatalf("expected *errcode.Error, got %T: %v", err, err)
	}
	if taiErr.Code != errcode.DBMigrationFailed {
		t.Fatalf("expected DBMigrationFailed, got %s", taiErr.Code)
	}
	if !strings.Contains(taiErr.Msg, "999") {
		t.Fatalf("error message should reference the migration version, got %q", taiErr.Msg)
	}

	// The transaction MUST have rolled back. Specifically: no row was
	// inserted into the migrations table for version 999.
	var count int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM migrations WHERE version = ?`, bad.Version).
		Scan(&count); err != nil {
		t.Fatalf("count migrations row: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no migrations row after rollback, got %d", count)
	}
}
