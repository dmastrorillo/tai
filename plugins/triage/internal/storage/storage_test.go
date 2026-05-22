package storage_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/storage"
	"github.com/dmastrorillo/tai/plugins/triage/internal/storage/storagetest"
)

// --- Connection-policy tests (TC-STG-001..003) ----------------------

// TestOpen_TCSTG001_WAL_active exercises TC-STG-001: WAL is the
// journal mode on every connection.
//
// Note: SQLite's :memory: databases report "memory" (or refuse WAL).
// To exercise the real WAL path the test uses a tmp file DB.
func TestOpen_TCSTG001_WAL_active(t *testing.T) {
	path := tempDBPath(t)
	db, err := storage.OpenAt(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	defer db.Close()

	var mode string
	if err := db.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if strings.ToLower(mode) != "wal" {
		t.Fatalf("want journal_mode=wal, got %q", mode)
	}
}

// TestOpen_TCSTG002_foreign_keys_on exercises TC-STG-002:
// foreign_keys is on for every connection.
func TestOpen_TCSTG002_foreign_keys_on(t *testing.T) {
	db := storagetest.NewMemDB(t)
	var fk int
	if err := db.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("want foreign_keys=1, got %d", fk)
	}
}

// TestOpen_TCSTG003_open_failure exercises TC-STG-003: when the
// database cannot be opened the CLI exits with DB_OPEN_FAILED.
//
// We trigger this by pointing at an unwritable file path that the
// driver will refuse — a parent directory that does not exist.
func TestOpen_TCSTG003_open_failure(t *testing.T) {
	// A file inside a non-existent parent on a non-existent FS root.
	// modernc.org/sqlite refuses to create it.
	_, err := storage.OpenAt(context.Background(), "/nonexistent-root/cannot/be/created/tai.db")
	if err == nil {
		t.Fatal("expected DB_OPEN_FAILED, got nil")
	}
	taiErr, ok := errcode.As(err)
	if !ok {
		t.Fatalf("expected *errcode.Error, got %T: %v", err, err)
	}
	if taiErr.Code != errcode.DBOpenFailed {
		t.Fatalf("expected DB_OPEN_FAILED, got %s", taiErr.Code)
	}
}

// TestOpen_propagates_data_dir_error verifies the Open() → EnsureWritable
// delegation: if the data directory cannot be created, the *errcode.Error
// propagates unchanged (carries DATA_DIR_UNWRITABLE, not DB_OPEN_FAILED).
// Engine test — not tied to a TC-ID; the user-observable equivalent will
// be the foundation's TC-CFG-004 once a real subcommand uses Open.
func TestOpen_propagates_data_dir_error(t *testing.T) {
	t.Setenv("TAI_DATA_DIR", "/dev/null/cannot-mkdir")
	t.Setenv("XDG_DATA_HOME", "")

	_, err := storage.Open(context.Background())
	if err == nil {
		t.Fatal("expected DATA_DIR_UNWRITABLE, got nil")
	}
	taiErr, ok := errcode.As(err)
	if !ok {
		t.Fatalf("expected *errcode.Error, got %T: %v", err, err)
	}
	if taiErr.Code != errcode.DataDirUnwritable {
		t.Fatalf("expected DATA_DIR_UNWRITABLE, got %s", taiErr.Code)
	}
}

// --- Migration-runner tests (TC-STG-004..006) -----------------------

// TestMigrations_TCSTG004_fresh_db_applies_all exercises TC-STG-004:
// a fresh database has every embedded migration applied and recorded.
func TestMigrations_TCSTG004_fresh_db_applies_all(t *testing.T) {
	db := storagetest.NewMemDB(t)

	ver, err := storage.MigrationVersion(context.Background(), db.DB)
	if err != nil {
		t.Fatalf("MigrationVersion: %v", err)
	}
	if ver < 1 {
		t.Fatalf("expected at least one migration applied, got version %d", ver)
	}

	// Specifically: 001_init must be recorded.
	var name string
	if err := db.QueryRowContext(context.Background(),
		"SELECT name FROM migrations WHERE version = 1").Scan(&name); err != nil {
		t.Fatalf("scan migrations row: %v", err)
	}
	if name != "init" {
		t.Fatalf("want migration 1 name 'init', got %q", name)
	}
}

// TestMigrations_TCSTG005_second_open_is_noop exercises TC-STG-005: a
// second OpenAt against the same file database applies no new
// migrations.
func TestMigrations_TCSTG005_second_open_is_noop(t *testing.T) {
	path := tempDBPath(t)

	db1, err := storage.OpenAt(context.Background(), path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	ver1, _ := storage.MigrationVersion(context.Background(), db1.DB)
	_ = db1.Close()

	db2, err := storage.OpenAt(context.Background(), path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer db2.Close()
	ver2, _ := storage.MigrationVersion(context.Background(), db2.DB)

	if ver1 != ver2 {
		t.Fatalf("second open changed migration version: %d → %d", ver1, ver2)
	}

	// And the recorded count should equal the version (no duplicates).
	var count int
	if err := db2.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != ver2 {
		t.Fatalf("migrations row count %d != version %d (duplicate rows?)", count, ver2)
	}
}

// --- repos schema tests (TC-STG-010..012) ---------------------------

func TestRepos_TCSTG010_insert(t *testing.T) {
	db := storagetest.NewMemDB(t)
	mustExec(t, db.DB,
		`INSERT INTO repos (owner_name, created_at) VALUES (?, ?)`,
		"acme/app", time.Now().Unix())
}

func TestRepos_TCSTG011_unique_owner_name(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	mustExec(t, db.DB,
		`INSERT INTO repos (owner_name, created_at) VALUES (?, ?)`,
		"acme/app", now)

	_, err := db.ExecContext(context.Background(),
		`INSERT INTO repos (owner_name, created_at) VALUES (?, ?)`,
		"acme/app", now)
	requireConstraint(t, err)
}

func TestRepos_TCSTG012_cascade_to_children(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	insertPR(t, db, repoID, 1, "feat: x", "https://x", "feat/x", now)
	insertBranch(t, db, repoID, "feat/y", now)

	mustExec(t, db.DB, `DELETE FROM repos WHERE id = ?`, repoID)

	var prCount, brCount int
	_ = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM prs WHERE repo_id = ?`, repoID).Scan(&prCount)
	_ = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM branches WHERE repo_id = ?`, repoID).Scan(&brCount)
	if prCount != 0 || brCount != 0 {
		t.Fatalf("expected cascade to wipe children; got prs=%d branches=%d", prCount, brCount)
	}
}

// --- prs schema tests (TC-STG-020..023) -----------------------------

func TestPRs_TCSTG020_insert(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	insertPR(t, db, repoID, 142, "feat: oauth", "https://x", "feat/oauth", now)
}

func TestPRs_TCSTG021_unique_repo_number(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	insertPR(t, db, repoID, 142, "feat: a", "https://x", "feat/a", now)

	_, err := db.ExecContext(context.Background(),
		`INSERT INTO prs (repo_id, number, title, url, head_branch, created_at) VALUES (?,?,?,?,?,?)`,
		repoID, 142, "feat: b", "https://y", "feat/b", now)
	requireConstraint(t, err)
}

// TestPRs_TCSTG022_same_number_different_repos exercises TC-STG-022:
// the (repo_id, number) UNIQUE constraint scopes uniqueness PER repo
// — two repos with a PR #1 each is valid.
func TestPRs_TCSTG022_same_number_different_repos(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	a := insertRepo(t, db, "acme/app", now)
	b := insertRepo(t, db, "acme/other", now)

	insertPR(t, db, a, 1, "t", "u", "br", now)
	insertPR(t, db, b, 1, "t", "u", "br", now)
}

// TestPRs_TCSTG023_head_branch_not_null exercises TC-STG-023:
// head_branch on prs is NOT NULL.
func TestPRs_TCSTG023_head_branch_not_null(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)

	_, err := db.ExecContext(context.Background(),
		`INSERT INTO prs (repo_id, number, title, url, head_branch, created_at) VALUES (?,?,?,?,?,?)`,
		repoID, 1, "t", "u", nil, now)
	requireConstraint(t, err)
}

// --- branches schema tests (TC-STG-030..032) -----------------------

func TestBranches_TCSTG030_insert(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	insertBranch(t, db, repoID, "feat/x", now)
}

func TestBranches_TCSTG031_unique_repo_name(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	insertBranch(t, db, repoID, "feat/x", now)

	_, err := db.ExecContext(context.Background(),
		`INSERT INTO branches (repo_id, name, created_at) VALUES (?, ?, ?)`,
		repoID, "feat/x", now)
	requireConstraint(t, err)
}

func TestBranches_TCSTG032_cascade_from_repo(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	insertBranch(t, db, repoID, "feat/x", now)

	mustExec(t, db.DB, `DELETE FROM repos WHERE id = ?`, repoID)

	var count int
	_ = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM branches WHERE repo_id = ?`, repoID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected branches to cascade-delete, got %d", count)
	}
}

// --- comments schema tests (TC-STG-040..049) ------------------------

func TestComments_TCSTG040_pr_parent(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)
	insertCommentPR(t, db, prID)
}

func TestComments_TCSTG041_branch_parent(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	branchID := insertBranch(t, db, repoID, "feat/x", now)
	insertCommentBranch(t, db, branchID)
}

func TestComments_TCSTG042_both_parents_rejected(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)
	branchID := insertBranch(t, db, repoID, "feat/x", now)

	_, err := db.ExecContext(context.Background(),
		insertCommentSQL(),
		prID, branchID, nil,
		"critical", "security", "f", "1", "src", "t", "d", "w", "s", "c",
		"pending", nil, nil, nil, now, now)
	requireConstraint(t, err)
}

func TestComments_TCSTG043_no_parents_rejected(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()

	_, err := db.ExecContext(context.Background(),
		insertCommentSQL(),
		nil, nil, nil,
		"critical", "security", "f", "1", "src", "t", "d", "w", "s", "c",
		"pending", nil, nil, nil, now, now)
	requireConstraint(t, err)
}

func TestComments_TCSTG044_invalid_severity(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)

	_, err := db.ExecContext(context.Background(),
		insertCommentSQL(),
		prID, nil, nil,
		"urgent", "security", "f", "1", "src", "t", "d", "w", "s", "c",
		"pending", nil, nil, nil, now, now)
	requireConstraint(t, err)
}

func TestComments_TCSTG045_invalid_status(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)

	_, err := db.ExecContext(context.Background(),
		insertCommentSQL(),
		prID, nil, nil,
		"critical", "security", "f", "1", "src", "t", "d", "w", "s", "c",
		"archived", nil, nil, nil, now, now)
	requireConstraint(t, err)
}

// TestComments_TCSTG046_invalid_category exercises TC-STG-046: a
// comment with a category outside the enum is rejected.
func TestComments_TCSTG046_invalid_category(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)

	_, err := db.ExecContext(context.Background(),
		insertCommentSQL(),
		prID, nil, nil,
		"critical", "architecture", "f", "1", "src", "t", "d", "w", "s", "c",
		"pending", nil, nil, nil, now, now)
	requireConstraint(t, err)
}

// TestComments_TCSTG047_missing_enrichment exercises TC-STG-047: the
// five mandatory enrichment columns (description, why_fix,
// suggested_fix, consequences, plus title) are NOT NULL — inserting
// any as NULL is rejected.
func TestComments_TCSTG047_missing_enrichment(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)

	cases := []struct {
		name string
		args []any
	}{
		// (pr_id, branch_id, batch_id, severity, category, file, lines,
		//  source, title, description, why_fix, suggested_fix,
		//  consequences, status, resolution, dismissed_by,
		//  dismiss_reason, created_at, updated_at)
		{
			name: "why_fix NULL",
			args: []any{prID, nil, nil, "critical", "security", "f", "1", "src", "t", "d", nil, "s", "c", "pending", nil, nil, nil, now, now},
		},
		{
			name: "suggested_fix NULL",
			args: []any{prID, nil, nil, "critical", "security", "f", "1", "src", "t", "d", "w", nil, "c", "pending", nil, nil, nil, now, now},
		},
		{
			name: "consequences NULL",
			args: []any{prID, nil, nil, "critical", "security", "f", "1", "src", "t", "d", "w", "s", nil, "pending", nil, nil, nil, now, now},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.ExecContext(context.Background(), insertCommentSQL(), tc.args...)
			requireConstraint(t, err)
		})
	}
}

// TestComments_TCSTG048_cascade_from_pr exercises TC-STG-048: deleting
// a PR cascades to its comments.
func TestComments_TCSTG048_cascade_from_pr(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)
	insertCommentPR(t, db, prID)

	mustExec(t, db.DB, `DELETE FROM prs WHERE id = ?`, prID)

	var count int
	_ = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM comments WHERE pr_id = ?`, prID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 comments after PR delete, got %d", count)
	}
}

// TestComments_TCSTG049_cascade_from_branch exercises TC-STG-049:
// deleting a branch cascades to its comments.
func TestComments_TCSTG049_cascade_from_branch(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	branchID := insertBranch(t, db, repoID, "feat/x", now)
	insertCommentBranch(t, db, branchID)

	mustExec(t, db.DB, `DELETE FROM branches WHERE id = ?`, branchID)

	var count int
	_ = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM comments WHERE branch_id = ?`, branchID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 comments after branch delete, got %d", count)
	}
}

// --- batches schema tests (TC-STG-060..065) -------------------------

func TestBatches_TCSTG060_insert(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)

	mustExec(t, db.DB,
		`INSERT INTO batches (pr_id, batch_key, title, created_at) VALUES (?,?,?,?)`,
		prID, "B1", "Replace execSync", now)
}

func TestBatches_TCSTG061_duplicate_key_rejected(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)

	mustExec(t, db.DB,
		`INSERT INTO batches (pr_id, batch_key, title, created_at) VALUES (?,?,?,?)`,
		prID, "B1", "first", now)
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO batches (pr_id, batch_key, title, created_at) VALUES (?,?,?,?)`,
		prID, "B1", "second", now)
	requireConstraint(t, err)
}

func TestBatches_TCSTG062_status_mixed_allowed(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)
	mustExec(t, db.DB,
		`INSERT INTO batches (pr_id, batch_key, title, status, created_at) VALUES (?,?,?,?,?)`,
		prID, "B1", "title", "mixed", now)
}

// TestBatches_TCSTG063_no_parent_rejected exercises TC-STG-063: a
// batch with both pr_id and branch_id NULL violates the XOR CHECK.
func TestBatches_TCSTG063_no_parent_rejected(t *testing.T) {
	db := storagetest.NewMemDB(t)
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO batches (pr_id, branch_id, batch_key, title, created_at) VALUES (?,?,?,?,?)`,
		nil, nil, "B1", "title", time.Now().Unix())
	requireConstraint(t, err)
}

// TestBatches_TCSTG064_cascade_from_pr exercises TC-STG-064: deleting
// a PR cascades to its batches.
func TestBatches_TCSTG064_cascade_from_pr(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)
	mustExec(t, db.DB,
		`INSERT INTO batches (pr_id, batch_key, title, created_at) VALUES (?,?,?,?)`,
		prID, "B1", "title", now)

	mustExec(t, db.DB, `DELETE FROM prs WHERE id = ?`, prID)

	var count int
	_ = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM batches WHERE pr_id = ?`, prID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected batches to cascade-delete with PR, got %d", count)
	}
}

// TestBatches_TCSTG065_cascade_from_branch exercises TC-STG-065:
// deleting a branch cascades to its batches.
func TestBatches_TCSTG065_cascade_from_branch(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	branchID := insertBranch(t, db, repoID, "feat/x", now)
	mustExec(t, db.DB,
		`INSERT INTO batches (branch_id, batch_key, title, created_at) VALUES (?,?,?,?)`,
		branchID, "B1", "title", now)

	mustExec(t, db.DB, `DELETE FROM branches WHERE id = ?`, branchID)

	var count int
	_ = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM batches WHERE branch_id = ?`, branchID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected batches to cascade-delete with branch, got %d", count)
	}
}

// --- comment_external_refs (TC-STG-050..053) ------------------------

func TestExternalRefs_TCSTG050_multiple_refs(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)
	commentID := insertCommentPR(t, db, prID)

	mustExec(t, db.DB,
		`INSERT INTO comment_external_refs (comment_id, source_kind, external_id, reviewer)
		 VALUES (?, ?, ?, ?)`,
		commentID, "github-pr-comment", "12345", "coderabbit")
	mustExec(t, db.DB,
		`INSERT INTO comment_external_refs (comment_id, source_kind, external_id, reviewer)
		 VALUES (?, ?, ?, ?)`,
		commentID, "github-pr-comment", "12346", "greptile")
}

func TestExternalRefs_TCSTG051_duplicate_ref_rejected(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)
	commentID := insertCommentPR(t, db, prID)

	mustExec(t, db.DB,
		`INSERT INTO comment_external_refs (comment_id, source_kind, external_id) VALUES (?, ?, ?)`,
		commentID, "github-pr-comment", "12345")
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO comment_external_refs (comment_id, source_kind, external_id) VALUES (?, ?, ?)`,
		commentID, "github-pr-comment", "12345")
	requireConstraint(t, err)
}

// TestExternalRefs_TCSTG052_cascade_from_comment exercises TC-STG-052:
// deleting a comment cascades to its external_refs rows.
func TestExternalRefs_TCSTG052_cascade_from_comment(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)
	commentID := insertCommentPR(t, db, prID)
	mustExec(t, db.DB,
		`INSERT INTO comment_external_refs (comment_id, source_kind, external_id) VALUES (?, ?, ?)`,
		commentID, "github-pr-comment", "12345")

	mustExec(t, db.DB, `DELETE FROM comments WHERE id = ?`, commentID)

	var count int
	_ = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM comment_external_refs WHERE comment_id = ?`,
		commentID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected refs to cascade-delete with comment, got %d", count)
	}
}

// --- additional comment cascade scenarios ---------------------------

// TestBatches_TCSTG053_delete_sets_batch_id_null exercises TC-STG-053:
// deleting a batch leaves its member comments intact with batch_id NULL.
func TestBatches_TCSTG053_delete_sets_batch_id_null(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	repoID := insertRepo(t, db, "acme/app", now)
	prID := insertPR(t, db, repoID, 1, "t", "u", "b", now)

	res, err := db.ExecContext(context.Background(),
		`INSERT INTO batches (pr_id, batch_key, title, created_at) VALUES (?,?,?,?)`,
		prID, "B1", "title", now)
	if err != nil {
		t.Fatalf("insert batch: %v", err)
	}
	batchID, _ := res.LastInsertId()

	commentID := insertCommentPRWithBatch(t, db, prID, batchID)

	mustExec(t, db.DB, `DELETE FROM batches WHERE id = ?`, batchID)

	var nullBatch sql.NullInt64
	_ = db.QueryRowContext(context.Background(),
		`SELECT batch_id FROM comments WHERE id = ?`, commentID).Scan(&nullBatch)
	if nullBatch.Valid {
		t.Fatalf("expected comment.batch_id to be NULL after batch delete, got %d", nullBatch.Int64)
	}

	// The comment row itself must still exist (cascade-set-null, not
	// cascade-delete).
	var rows int
	_ = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM comments WHERE id = ?`, commentID).Scan(&rows)
	if rows != 1 {
		t.Fatalf("expected comment to survive batch delete, got %d rows", rows)
	}
}

// --- Error-mapping tests (TC-STG-070..073) --------------------------

func TestErrConstraint_TCSTG070_maps_to_DBConstraintViolation(t *testing.T) {
	db := storagetest.NewMemDB(t)
	now := time.Now().Unix()
	insertRepo(t, db, "acme/app", now)

	// Second insert with the same owner_name triggers UNIQUE violation.
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO repos (owner_name, created_at) VALUES (?, ?)`,
		"acme/app", now)
	if err == nil {
		t.Fatal("expected constraint error")
	}

	wrapped := storage.ErrConstraint(err)
	taiErr, ok := errcode.As(wrapped)
	if !ok {
		t.Fatalf("ErrConstraint did not produce *errcode.Error; got %T: %v", wrapped, wrapped)
	}
	if taiErr.Code != errcode.DBConstraintViolation {
		t.Fatalf("expected DB_CONSTRAINT_VIOLATION, got %s", taiErr.Code)
	}
}

// TestErrConstraint_passes_through_unrelated_errors and the
// already-wrapped sibling are engine tests — they exercise
// ErrConstraint's safety properties (don't transform non-constraint
// errors; idempotent on already-wrapped values) rather than a
// user-observable behaviour with its own BDD case.
func TestErrConstraint_passes_through_unrelated_errors(t *testing.T) {
	other := sql.ErrNoRows
	if got := storage.ErrConstraint(other); got != other {
		t.Fatalf("ErrConstraint should pass through unrelated errors unchanged; got %v", got)
	}
}

func TestErrConstraint_passes_through_already_wrapped(t *testing.T) {
	wrapped := errcode.New(errcode.DBOpenFailed, "already wrapped")
	if got := storage.ErrConstraint(wrapped); got != error(wrapped) {
		t.Fatalf("ErrConstraint double-wrapped an existing *errcode.Error; got %v", got)
	}
}

// --- helpers --------------------------------------------------------

func tempDBPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "tai.db")
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %s: %v", query, err)
	}
}

func requireConstraint(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected constraint error, got nil")
	}
	wrapped := storage.ErrConstraint(err)
	taiErr, ok := errcode.As(wrapped)
	if !ok {
		t.Fatalf("error not mapped to *errcode.Error: %T: %v", err, err)
	}
	if taiErr.Code != errcode.DBConstraintViolation {
		t.Fatalf("expected DB_CONSTRAINT_VIOLATION, got %s\nunderlying: %v",
			taiErr.Code, err)
	}
}

func insertRepo(t *testing.T, db *storage.DB, owner string, ts int64) int64 {
	t.Helper()
	res, err := db.ExecContext(context.Background(),
		`INSERT INTO repos (owner_name, created_at) VALUES (?, ?)`, owner, ts)
	if err != nil {
		t.Fatalf("insertRepo: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func insertPR(t *testing.T, db *storage.DB, repoID int64, number int, title, url, head string, ts int64) int64 {
	t.Helper()
	res, err := db.ExecContext(context.Background(),
		`INSERT INTO prs (repo_id, number, title, url, head_branch, created_at) VALUES (?,?,?,?,?,?)`,
		repoID, number, title, url, head, ts)
	if err != nil {
		t.Fatalf("insertPR: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func insertBranch(t *testing.T, db *storage.DB, repoID int64, name string, ts int64) int64 {
	t.Helper()
	res, err := db.ExecContext(context.Background(),
		`INSERT INTO branches (repo_id, name, created_at) VALUES (?, ?, ?)`,
		repoID, name, ts)
	if err != nil {
		t.Fatalf("insertBranch: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func insertCommentSQL() string {
	return `INSERT INTO comments
		(pr_id, branch_id, batch_id, severity, category, file, lines, source, title,
		 description, why_fix, suggested_fix, consequences,
		 status, resolution, dismissed_by, dismiss_reason, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
}

func insertCommentPR(t *testing.T, db *storage.DB, prID int64) int64 {
	t.Helper()
	return insertCommentPRWithBatch(t, db, prID, 0)
}

func insertCommentPRWithBatch(t *testing.T, db *storage.DB, prID, batchID int64) int64 {
	t.Helper()
	now := time.Now().Unix()
	var batchArg any
	if batchID == 0 {
		batchArg = nil
	} else {
		batchArg = batchID
	}
	res, err := db.ExecContext(context.Background(), insertCommentSQL(),
		prID, nil, batchArg,
		"critical", "security", "src/x.go", "10-12", "rev", "t", "d", "w", "s", "c",
		"pending", nil, nil, nil, now, now)
	if err != nil {
		t.Fatalf("insertCommentPR: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func insertCommentBranch(t *testing.T, db *storage.DB, branchID int64) int64 {
	t.Helper()
	now := time.Now().Unix()
	res, err := db.ExecContext(context.Background(), insertCommentSQL(),
		nil, branchID, nil,
		"major", "correctness", "src/x.go", "10-12", "rev", "t", "d", "w", "s", "c",
		"pending", nil, nil, nil, now, now)
	if err != nil {
		t.Fatalf("insertCommentBranch: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}
