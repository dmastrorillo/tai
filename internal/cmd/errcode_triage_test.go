package cmd_test

import (
	"testing"

	"github.com/dmastrorillo/tai/internal/cmd"
	"github.com/dmastrorillo/tai/internal/cmdtest"
)

// Each TC in this file exercises the foundation contract's footer
// regex for one of the five new triage error codes. Together with
// TC-ERR-002/003/004 (already in test-cases.md under ERR), they pin
// the user-observable shape of every triage error path.

// TestErrcode_TCTRG100_no_scope_footer exercises TC-TRG-100: a verb
// that cannot resolve a scope surfaces `[exit 2: TRIAGE_NO_SCOPE]`.
func TestErrcode_TCTRG100_no_scope_footer(t *testing.T) {
	cmdtest.Isolate(t)
	cmdtest.Chdir(t, t.TempDir())
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "list")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_NO_SCOPE", 2)
}

// TestErrcode_TCTRG101_ambiguous_scope_footer exercises TC-TRG-101:
// when the current branch matches both a PR and a branches row, the
// CLI surfaces `[exit 2: TRIAGE_AMBIGUOUS_SCOPE]`.
func TestErrcode_TCTRG101_ambiguous_scope_footer(t *testing.T) {
	cmdtest.Isolate(t)
	// Seed a PR whose head_branch is "feat/z" AND a branch row with
	// the same name, then chdir into a git checkout on "feat/z".
	prPayload := buildPRPayload(99, "collision", "feat/z",
		commentJSON("r1", "pr-side", "critical", "pending"))
	rImp := cmdtest.RunWithStdin(t, cmd.NewRoot(), prPayload, "import", "-")
	cmdtest.AssertNoError(t, rImp)
	seedBranch(t, "feat/z", commentJSON("r2", "branch-side", "major", "pending"))
	initGitRepoOnBranch(t, "feat/z")

	r := triage(t, "list")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_AMBIGUOUS_SCOPE", 2)
}

// TestErrcode_TCTRG102_not_found_footer exercises TC-TRG-102:
// referencing a non-existent comment position surfaces
// `[exit 2: TRIAGE_NOT_FOUND]`.
func TestErrcode_TCTRG102_not_found_footer(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "accept", "99", "--pr", "1")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_NOT_FOUND", 2)
}

// TestErrcode_TCTRG103_invalid_flags_footer exercises TC-TRG-103:
// conflicting flags on a triage verb surface
// `[exit 1: TRIAGE_INVALID_FLAGS]`.
func TestErrcode_TCTRG103_invalid_flags_footer(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "list", "--pr", "1", "--branch", "feat/x")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_INVALID_FLAGS", 1)
}

// TestErrcode_TCTRG104_confirmation_required_footer exercises
// TC-TRG-104: `tai forget` invoked non-interactively without --yes
// surfaces `[exit 1: TRIAGE_CONFIRMATION_REQUIRED]`.
func TestErrcode_TCTRG104_confirmation_required_footer(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "forget", "--pr", "1")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_CONFIRMATION_REQUIRED", 1)
}
