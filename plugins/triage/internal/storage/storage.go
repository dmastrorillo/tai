// Package storage owns tai's SQLite database — the schema, connection
// policy, migration runner, and the boundary every other internal
// package goes through for reads and writes.
//
// What lives here today (foundation of the storage capability):
//
//   - Open / OpenAt — open the database, apply the connection pragmas,
//     run any unapplied migrations.
//   - DB — a thin wrapper around *sql.DB; subcommand packages query
//     through methods on this type so the package boundary is real.
//   - Driver-error translation — sqlite3 errors map to
//     errcode.DBOpenFailed / DBMigrationFailed / DBConstraintViolation
//     per the foundation contract.
//
// Schema definitions live as embedded SQL files under migrations/. The
// canonical schema reference is openspec/specs/storage/spec.md; this
// package implements that contract.
//
// Connection policy (every Open):
//
//   - PRAGMA journal_mode = WAL
//   - PRAGMA foreign_keys = ON
//   - PRAGMA busy_timeout = 1000
//
// Each tai invocation opens a single DB, runs migrations, performs its
// work, and closes; there is no long-lived process. WAL + the 1-second
// busy timeout handles the edge case of two concurrent invocations.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/datadir"

	// Pure-Go SQLite driver registers itself as "sqlite". No CGo, so
	// tai stays a single static binary that cross-compiles cleanly.
	_ "modernc.org/sqlite"
)

// DB is the storage façade. It wraps a *sql.DB plus the resolved path
// the database was opened against. Methods on DB are added as
// subsequent OpenSpec changes need them; for now Close and the embedded
// *sql.DB are sufficient.
//
// Path is the file path the database was opened against; for in-memory
// databases (e.g. those produced by storagetest.NewMemDB) it is the
// literal ":memory:" string.
type DB struct {
	*sql.DB
	Path string
}

// Open resolves the data directory, ensures it is writable, opens the
// SQLite database at <data_dir>/tai.db (creating it if necessary),
// applies the connection pragmas, and runs any unapplied migrations.
//
// On any failure, returns a *errcode.Error with the appropriate code:
//
//   - DataDirUnwritable: the data directory cannot be created/written
//   - DBOpenFailed: open / pragma failure
//   - DBMigrationFailed: a migration script failed to apply
func Open(ctx context.Context) (*DB, error) {
	dir, err := datadir.EnsureWritable()
	if err != nil {
		return nil, err
	}
	return OpenAt(ctx, filepath.Join(dir, "tai.db"))
}

// OpenAt is like Open but takes an explicit database file path. It is
// the seam for tests and for any future caller that needs to point at
// a non-default location. The path's parent directory MUST already
// exist and be writable.
//
// The literal `:memory:` and any URI starting with `file::memory:` are
// recognised as in-memory databases — used by storagetest.NewMemDB.
//
// The returned DB has SetMaxOpenConns(1). SQLite pragmas are
// per-connection state; pooling multiple connections would silently
// disable foreign-key enforcement on any connection whose pragmas
// were never set. Constraining the pool to a single connection makes
// the pragma policy enforceable and matches SQLite's single-writer
// reality. tai is a per-user CLI with no concurrent-writer workload,
// so the single-connection cap costs nothing.
func OpenAt(ctx context.Context, path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, errcode.Wrapf(errcode.DBOpenFailed, err,
			"open database %q", path).
			WithHelp("check file permissions and that the parent directory is writable")
	}
	sqlDB.SetMaxOpenConns(1)

	if err := applyPragmas(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	if err := runMigrations(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	return &DB{DB: sqlDB, Path: path}, nil
}

// applyPragmas runs the connection-open pragmas required by the spec.
// Any failure surfaces as DBOpenFailed with a message naming the
// failed pragma.
func applyPragmas(ctx context.Context, db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 1000",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			return errcode.Wrapf(errcode.DBOpenFailed, err,
				"applying %s", p).
				WithHelp(
					"check the database file is not corrupt",
					"try removing the tai.db file and re-running (data will be lost)",
				)
		}
	}

	// WAL is best-effort on :memory: databases (where it is ignored);
	// the spec only requires us to set it, not verify the result.
	// Verify foreign_keys is on, however — it is the constraint
	// enforcement we rely on for the schema's XOR checks.
	var fk int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		return errcode.Wrap(errcode.DBOpenFailed, err,
			"verifying foreign_keys pragma")
	}
	if fk != 1 {
		return errcode.New(errcode.DBOpenFailed,
			fmt.Sprintf("foreign_keys pragma did not take effect (got %d, want 1)", fk))
	}
	return nil
}

// ErrConstraint maps a sqlite3 constraint-violation error to a
// *errcode.Error with the constraint's name (or kind) included in the
// message. Callers wrap their queries' errors with this helper before
// returning.
//
// Already-wrapped errors (any value already convertible to
// *errcode.Error) pass through unchanged, so callers can apply
// ErrConstraint at multiple layers without double-wrap.
//
// The pure-Go driver formats constraint failures with the substring
// "constraint" in the message (e.g. "constraint failed: UNIQUE
// constraint failed: repos.owner_name"). We sniff for that substring
// rather than depending on driver-internal types so the mapping stays
// stable across driver releases.
func ErrConstraint(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errcode.As(err); ok {
		return err
	}

	msg := err.Error()
	if isConstraintErr(msg) {
		return errcode.Wrapf(errcode.DBConstraintViolation, err,
			"constraint violation: %s", msg)
	}
	return err
}

// isConstraintErr returns true when msg looks like a SQLite constraint
// failure. modernc.org/sqlite's messages always contain the literal
// "constraint" substring for these conditions (UNIQUE / NOT NULL /
// CHECK / FOREIGN KEY); the single-substring check is sufficient and
// stable across the driver's message variants.
func isConstraintErr(msg string) bool {
	return strings.Contains(msg, "constraint")
}
