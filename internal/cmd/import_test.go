package cmd_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielmastrorillo/tai/internal/cmd"
	"github.com/danielmastrorillo/tai/internal/cmdtest"
	"github.com/danielmastrorillo/tai/internal/storage"
)

// validImportPR is the canonical happy-path payload used by E2E tests
// of `tai import -`. Mutations are constructed inline per test.
const validImportPR = `{
  "repo": "acme/app",
  "target": {
    "kind": "pr",
    "pr": { "number": 142, "title": "feat: oauth", "url": "https://x", "head_branch": "feat/oauth" }
  },
  "batches": [{ "batch_key": "B1", "title": "Replace execSync" }],
  "comments": [
    {
      "external_refs": [{ "kind": "github-pr-comment", "id": "12345" }],
      "severity": "critical",
      "category": "security",
      "file": "src/x.ts",
      "lines": "1-5",
      "source": "coderabbit",
      "title": "shell injection",
      "description": "execSync interpolates",
      "why_fix": "shell metachars run",
      "suggested_fix": "use execFileSync",
      "consequences": "RCE",
      "batch_key": "B1"
    }
  ]
}`

// TestImport_TCIMP020_stdin_payload_persists exercises the happy path:
// `tai import -` with a valid payload on stdin succeeds, prints the
// success summary, and persists the row.
func TestImport_TCIMP020_stdin_payload_persists(t *testing.T) {
	iso := cmdtest.Isolate(t)

	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), validImportPR, "import", "-")

	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)
	cmdtest.AssertStdoutContains(t, r, "Imported acme/app PR #142")
	cmdtest.AssertStdoutContains(t, r, "[exit 0]")

	// Verify the row landed in the DB.
	assertCommentCount(t, iso.DataDir, 1)
}

// TestImport_TCIMP021_missing_positional_fails exercises the "no
// positional" error: `tai import` (no `-`) reports a usage error.
func TestImport_TCIMP021_missing_positional_fails(t *testing.T) {
	cmdtest.Isolate(t)

	r := cmdtest.Run(t, cmd.NewRoot(), "import")

	cmdtest.AssertError(t, r)
	cmdtest.AssertExitCode(t, r, 1)
	cmdtest.AssertErrorFooter(t, r, "UNKNOWN_SUBCOMMAND", 1)
}

// TestImport_TCIMP022_wrong_positional_fails: `tai import 142` is
// rejected (no PR-number convenience form).
func TestImport_TCIMP022_wrong_positional_fails(t *testing.T) {
	cmdtest.Isolate(t)

	r := cmdtest.Run(t, cmd.NewRoot(), "import", "142")

	cmdtest.AssertError(t, r)
	cmdtest.AssertExitCode(t, r, 1)
	cmdtest.AssertErrorFooter(t, r, "UNKNOWN_SUBCOMMAND", 1)
}

// TestImport_TCIMP023_rejects_repo_flag: combining --repo with `tai
// import` is a usage error.
func TestImport_TCIMP023_rejects_repo_flag(t *testing.T) {
	cmdtest.Isolate(t)

	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), validImportPR,
		"--repo", "acme/app", "import", "-")

	cmdtest.AssertError(t, r)
	cmdtest.AssertExitCode(t, r, 1)
	cmdtest.AssertErrorFooter(t, r, "UNKNOWN_SUBCOMMAND", 1)
}

// TestImport_TCIMP024_outside_git_repo exercises TC-IMP-024: `tai
// import -` succeeds from a non-git working directory because the JSON
// payload is the authoritative source for repo identity (no git
// resolution happens).
func TestImport_TCIMP024_outside_git_repo(t *testing.T) {
	cmdtest.Isolate(t)
	cmdtest.Chdir(t, t.TempDir())

	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), validImportPR, "import", "-")

	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)
	if strings.Contains(r.Stderr, "REPO_NOT_FOUND") {
		t.Fatalf("stderr should not mention REPO_NOT_FOUND, got %q", r.Stderr)
	}
}

// TestImport_TCIMP030_invalid_json_footer exercises TC-IMP-030: malformed
// JSON surfaces IMPORT_INVALID_JSON with the standard footer.
func TestImport_TCIMP030_invalid_json_footer(t *testing.T) {
	cmdtest.Isolate(t)

	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), "{not json", "import", "-")

	cmdtest.AssertError(t, r)
	cmdtest.AssertExitCode(t, r, 1)
	cmdtest.AssertErrorFooter(t, r, "IMPORT_INVALID_JSON", 1)
}

// TestImport_TCIMP031_schema_invalid_footer exercises TC-IMP-031: a
// payload that decodes but fails validation surfaces
// IMPORT_SCHEMA_INVALID. The error body lists every violation.
func TestImport_TCIMP031_schema_invalid_footer(t *testing.T) {
	cmdtest.Isolate(t)

	bad := strings.Replace(validImportPR,
		`"severity": "critical"`, `"severity": "urgent"`, 1)

	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), bad, "import", "-")

	cmdtest.AssertError(t, r)
	cmdtest.AssertExitCode(t, r, 3)
	cmdtest.AssertErrorFooter(t, r, "IMPORT_SCHEMA_INVALID", 3)
	cmdtest.AssertStderrContains(t, r, "comments[0].severity")
}

// TestImport_TCIMP010_multi_violation_body exercises the CLI-boundary
// half of TC-IMP-010: a payload with multiple distinct violations
// surfaces every violation in stderr, not just the first. The bullets
// are rendered under "What to do:" per the foundation contract.
func TestImport_TCIMP010_multi_violation_body(t *testing.T) {
	cmdtest.Isolate(t)

	bad := strings.Replace(validImportPR,
		`"severity": "critical"`, `"severity": "urgent"`, 1)
	bad = strings.Replace(bad,
		`"why_fix": "shell metachars run"`, `"why_fix": ""`, 1)

	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), bad, "import", "-")

	cmdtest.AssertError(t, r)
	cmdtest.AssertExitCode(t, r, 3)
	cmdtest.AssertErrorFooter(t, r, "IMPORT_SCHEMA_INVALID", 3)
	cmdtest.AssertStderrContains(t, r, "comments[0].severity")
	cmdtest.AssertStderrContains(t, r, "comments[0].why_fix")
}

// TestImport_TCIMP032_ambiguous_refs_footer exercises TC-IMP-032: a
// payload whose external_refs resolve to multiple comments surfaces
// IMPORT_AMBIGUOUS_REFS with the conflicting comment IDs in stderr.
func TestImport_TCIMP032_ambiguous_refs_footer(t *testing.T) {
	cmdtest.Isolate(t)

	// Seed two comments via two imports, each with a different ref.
	first := strings.Replace(validImportPR, `"id": "12345"`, `"id": "ref-a"`, 1)
	r1 := cmdtest.RunWithStdin(t, cmd.NewRoot(), first, "import", "-")
	cmdtest.AssertNoError(t, r1)

	second := strings.Replace(validImportPR,
		`"id": "12345"`, `"id": "ref-b"`, 1)
	second = strings.Replace(second, `"src/x.ts"`, `"src/other.ts"`, 1)
	r2 := cmdtest.RunWithStdin(t, cmd.NewRoot(), second, "import", "-")
	cmdtest.AssertNoError(t, r2)

	// Third import combines both refs.
	bad := strings.Replace(validImportPR,
		`[{ "kind": "github-pr-comment", "id": "12345" }]`,
		`[{ "kind": "github-pr-comment", "id": "ref-a" }, { "kind": "github-pr-comment", "id": "ref-b" }]`, 1)
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), bad, "import", "-")

	cmdtest.AssertError(t, r)
	cmdtest.AssertExitCode(t, r, 3)
	cmdtest.AssertErrorFooter(t, r, "IMPORT_AMBIGUOUS_REFS", 3)
	// Spec scenario: "stderr names both conflicting comment IDs." The
	// rendered slice format is `[1 2]` (Go's default %v for []int64).
	cmdtest.AssertStderrContains(t, r, "[1 2]")
}

// TestImport_TCIMP080_pr_header_format exercises the success summary's
// PR header format.
func TestImport_TCIMP080_pr_header_format(t *testing.T) {
	cmdtest.Isolate(t)
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), validImportPR, "import", "-")
	cmdtest.AssertNoError(t, r)
	if !strings.HasPrefix(r.Stdout, "Imported acme/app PR #142 (") {
		t.Fatalf("PR header missing: %q", r.Stdout)
	}
}

// TestImport_TCIMP081_branch_header_format exercises the success
// summary's branch header. Also acts as the CLI-boundary verification
// for TC-IMP-002 ("well-formed branch payload accepted") — the branch
// row is checked in the database after the run.
func TestImport_TCIMP081_branch_header_format(t *testing.T) {
	iso := cmdtest.Isolate(t)
	payload := `{
  "repo": "acme/app",
  "target": { "kind": "branch", "branch": { "name": "feat/x" } },
  "comments": []
}`
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)
	if !strings.HasPrefix(r.Stdout, "Imported acme/app branch feat/x (") {
		t.Fatalf("branch header missing: %q", r.Stdout)
	}

	// TC-IMP-002 persistence half: the branches row must exist.
	dbPath := filepath.Join(iso.DataDir, "tai.db")
	db, err := storage.OpenAt(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	defer db.Close()
	var name string
	_ = db.QueryRow(`SELECT name FROM branches`).Scan(&name)
	if name != "feat/x" {
		t.Fatalf("expected branch row name=feat/x, got %q", name)
	}
}

// TestImport_TCIMP082_empty_payload exercises the empty-payload no-op:
// header line is the only summary line and the run exits 0.
func TestImport_TCIMP082_empty_payload(t *testing.T) {
	cmdtest.Isolate(t)
	payload := `{
  "repo": "acme/app",
  "target": { "kind": "pr", "pr": { "number": 1, "title": "t", "url": "u", "head_branch": "b" } },
  "comments": [],
  "batches": []
}`
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Imported acme/app PR #1 (0 comments, 0 batches)")
	// No counter lines.
	if strings.Contains(r.Stdout, "Inserted:") || strings.Contains(r.Stdout, "Updated:") {
		t.Fatalf("zero-count lines should be suppressed: %q", r.Stdout)
	}
}

// assertCommentCount opens the DB under dataDir and fails the test if
// the comments table doesn't have exactly want rows.
func assertCommentCount(t *testing.T, dataDir string, want int) {
	t.Helper()
	dbPath := filepath.Join(dataDir, "tai.db")
	db, err := storage.OpenAt(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&n); err != nil {
		t.Fatalf("count comments: %v", err)
	}
	if n != want {
		t.Fatalf("expected %d comments, got %d", want, n)
	}
}
