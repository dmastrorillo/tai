package cmd_test

import (
	"testing"

	"github.com/danielmastrorillo/tai/internal/cmd"
	"github.com/danielmastrorillo/tai/internal/cmdtest"
)

// TestScope_TCTRG001_pr_flag exercises TC-TRG-001: an explicit --pr
// flag wins precedence.
func TestScope_TCTRG001_pr_flag(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "list", "--pr", "1")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Scope: PR #1")
}

// TestScope_TCTRG002_branch_flag exercises TC-TRG-002: --branch resolves
// to a branch scope.
func TestScope_TCTRG002_branch_flag(t *testing.T) {
	cmdtest.Isolate(t)
	seedBranch(t, "feat/x", commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "list", "--branch", "feat/x")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Scope: branch feat/x")
}

// TestScope_TCTRG003_auto_detect_pr exercises TC-TRG-003: when the
// current branch matches a PR's head_branch and no branch row, the
// scope auto-detects to that PR.
func TestScope_TCTRG003_auto_detect_pr(t *testing.T) {
	cmdtest.Isolate(t)
	// Seed BEFORE chdir-ing into the git fixture — seed runs `tai
	// import` which doesn't need git context.
	seedPR(t, 142, commentJSON("r1", "t", "critical", "pending"))
	initGitRepoOnBranch(t, "feat/x")

	r := triage(t, "list")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Scope: PR #142")
}

// TestScope_TCTRG004_auto_detect_branch exercises TC-TRG-004: when
// the current branch matches a `branches.name` row (and no PR
// head_branch), the scope auto-detects to that branch.
func TestScope_TCTRG004_auto_detect_branch(t *testing.T) {
	cmdtest.Isolate(t)
	seedBranch(t, "feat/y", commentJSON("r1", "t", "critical", "pending"))
	initGitRepoOnBranch(t, "feat/y")

	r := triage(t, "list")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Scope: branch feat/y")
}

// TestScope_TCTRG005_auto_detect_ambiguous exercises TC-TRG-005: when
// the current branch matches BOTH a PR's head_branch and a branch
// row, the CLI exits TRIAGE_AMBIGUOUS_SCOPE.
func TestScope_TCTRG005_auto_detect_ambiguous(t *testing.T) {
	cmdtest.Isolate(t)
	// PR whose head_branch is "feat/z" AND a standalone branch named
	// "feat/z" share the same name.
	seedPR(t, 5, commentJSON("r1", "pr-side", "critical", "pending"))
	// seedPR uses head=feat/x by default; we need feat/z. Use the
	// lower-level builder.
	prPayload := buildPRPayload(99, "feat: collision", "feat/z",
		commentJSON("r99", "pr-side", "critical", "pending"))
	rImport := cmdtest.RunWithStdin(t, cmd.NewRoot(), prPayload, "import", "-")
	cmdtest.AssertNoError(t, rImport)
	seedBranch(t, "feat/z", commentJSON("r2", "branch-side", "major", "pending"))

	initGitRepoOnBranch(t, "feat/z")

	r := triage(t, "list")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_AMBIGUOUS_SCOPE", 2)
}

// TestScope_TCTRG006_no_scope exercises TC-TRG-006: when no flag and
// auto-detect fails (e.g. outside a git repo), the verb exits
// TRIAGE_NO_SCOPE.
func TestScope_TCTRG006_no_scope(t *testing.T) {
	cmdtest.Isolate(t)
	cmdtest.Chdir(t, t.TempDir())
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))

	// No --pr / --branch and not in a git repo → no scope.
	r := triage(t, "list")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_NO_SCOPE", 2)
}

// TestScope_TCTRG007_mutex exercises TC-TRG-007: --pr + --branch is a
// usage error.
func TestScope_TCTRG007_mutex(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "list", "--pr", "1", "--branch", "feat/x")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_INVALID_FLAGS", 1)
}

// TestPosition_TCTRG010_starts_at_1 exercises TC-TRG-010: positions
// start at 1 within a target.
func TestPosition_TCTRG010_starts_at_1(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "first", "critical", "pending"))

	r := triage(t, "list", "--pr", "1")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "  1   crit")
}

// TestPosition_TCTRG012_shift_after_delete exercises TC-TRG-012:
// after deleting a comment, the surviving comments' positions
// renumber from 1.
func TestPosition_TCTRG012_shift_after_delete(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1,
		commentJSON("r1", "first", "critical", "pending")+","+
			commentJSON("r2", "second", "major", "pending")+","+
			commentJSON("r3", "third", "minor", "pending"))

	triage(t, "forget", "--comment", "2", "--pr", "1", "--yes")

	r := triage(t, "list", "--pr", "1")
	cmdtest.AssertStdoutContains(t, r, "first")
	cmdtest.AssertStdoutContains(t, r, "third")
	// Second-line position should now be 2 (was 3).
	cmdtest.AssertStdoutContains(t, r, "  2   ")
}
