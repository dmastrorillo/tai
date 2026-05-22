// migrations.go — the storage package's migration runner.
//
// Migrations are stored as numbered SQL files under migrations/
// (embedded via //go:embed). On every Open the runner reads the
// `migrations` table, applies any files whose version is absent, and
// records each successful application. Failures roll back to the
// pre-migration state and surface as DBMigrationFailed.
//
// Naming convention for new migrations: NNN_<name>.sql, where NNN is a
// zero-padded 3-digit version that is strictly greater than any
// previously-shipped version. Migrations are forward-only.

package storage

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationFilename matches the canonical NNN_<name>.sql filename
// shape. The numeric prefix is the migration version; ascending order
// drives apply order.
var migrationFilename = regexp.MustCompile(`^(\d{3})_([^/]+)\.sql$`)

// migration is one entry in the embedded migrations directory.
type migration struct {
	Version int
	Name    string
	SQL     string
}

// runMigrations applies any embedded migrations that have not yet been
// recorded in the migrations table. All work happens inside a single
// transaction per migration — a failed migration leaves the database
// in its pre-migration state.
func runMigrations(ctx context.Context, db *sql.DB) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	applied, err := readAppliedVersions(ctx, db)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		if err := applyOne(ctx, db, m); err != nil {
			return err
		}
	}
	return nil
}

// loadMigrations enumerates the embedded migrations directory in
// version order. Files not matching the NNN_<name>.sql shape are
// ignored (allows e.g. README.md alongside the SQL).
func loadMigrations() ([]migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, errcode.Wrap(errcode.DBMigrationFailed, err,
			"reading embedded migrations")
	}

	var out []migration
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		match := migrationFilename.FindStringSubmatch(e.Name())
		if match == nil {
			continue
		}
		ver, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, errcode.Wrapf(errcode.DBMigrationFailed, err,
				"parsing migration version from %q", e.Name())
		}
		raw, err := fs.ReadFile(migrationsFS, "migrations/"+e.Name())
		if err != nil {
			return nil, errcode.Wrapf(errcode.DBMigrationFailed, err,
				"reading migration %q", e.Name())
		}
		out = append(out, migration{
			Version: ver,
			Name:    match[2],
			SQL:     string(raw),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })

	// Detect duplicate versions early — a build error rather than a
	// runtime surprise.
	for i := 1; i < len(out); i++ {
		if out[i].Version == out[i-1].Version {
			return nil, errcode.Newf(errcode.DBMigrationFailed,
				"duplicate migration version %03d (%s and %s)",
				out[i].Version, out[i-1].Name, out[i].Name)
		}
	}
	return out, nil
}

// readAppliedVersions returns the set of versions already recorded in
// the migrations table. When the table does not exist (fresh
// database), returns an empty set without error.
func readAppliedVersions(ctx context.Context, db *sql.DB) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, "SELECT version FROM migrations")
	if err != nil {
		// Missing table means no migrations have been applied yet.
		if strings.Contains(err.Error(), "no such table") {
			return map[int]bool{}, nil
		}
		return nil, errcode.Wrap(errcode.DBMigrationFailed, err,
			"reading applied migrations")
	}
	defer rows.Close()

	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, errcode.Wrap(errcode.DBMigrationFailed, err,
				"scanning applied migration version")
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, errcode.Wrap(errcode.DBMigrationFailed, err,
			"iterating applied migrations")
	}
	return applied, nil
}

// applyOne runs a single migration inside a transaction. On any
// failure the transaction rolls back and the database is unchanged.
func applyOne(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errcode.Wrapf(errcode.DBMigrationFailed, err,
			"begin transaction for migration %03d_%s", m.Version, m.Name)
	}
	defer func() {
		// Rollback is a no-op after Commit; safe in the happy path.
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return errcode.Wrapf(errcode.DBMigrationFailed, err,
			"applying migration %03d_%s", m.Version, m.Name).
			WithHelp("verify the migration SQL is syntactically valid")
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.Version, m.Name, time.Now().Unix(),
	); err != nil {
		return errcode.Wrapf(errcode.DBMigrationFailed, err,
			"recording migration %03d_%s", m.Version, m.Name)
	}

	if err := tx.Commit(); err != nil {
		return errcode.Wrapf(errcode.DBMigrationFailed, err,
			"committing migration %03d_%s", m.Version, m.Name)
	}
	return nil
}

// MigrationVersion returns the highest version applied to the database
// behind db. Returns 0 when no migrations have been recorded.
//
// SELECT MAX(version) always returns one row (a single NULL aggregate
// when the table is empty), so we only need to handle the
// NullInt64.Valid==false case to surface "no migrations yet".
func MigrationVersion(ctx context.Context, db *sql.DB) (int, error) {
	var v sql.NullInt64
	if err := db.QueryRowContext(ctx,
		"SELECT MAX(version) FROM migrations").Scan(&v); err != nil {
		return 0, errcode.Wrap(errcode.DBMigrationFailed, err,
			"reading current migration version")
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}
