package cmd_test

import (
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmd"
	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdtest"
)

// TestForget_TCTRG090_zero_selectors exercises TC-TRG-090: no
// selector → TRIAGE_INVALID_FLAGS. This test deliberately bypasses
// the `triage()` helper because the helper auto-prepends --repo,
// which `tai forget` would interpret as the repo-selector mode.
func TestForget_TCTRG090_zero_selectors(t *testing.T) {
	cmdtest.Isolate(t)
	cmdtest.Chdir(t, t.TempDir())
	r := cmdtest.Run(t, cmd.NewRoot(), "forget")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_INVALID_FLAGS", 1)
}

// TestForget_TCTRG091_two_local_selectors exercises TC-TRG-091:
// `--pr` + `--branch` is TRIAGE_INVALID_FLAGS.
func TestForget_TCTRG091_two_local_selectors(t *testing.T) {
	cmdtest.Isolate(t)
	r := triage(t, "forget", "--pr", "1", "--branch", "feat/x", "--yes")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_INVALID_FLAGS", 1)
}

// TestForget_TCTRG092_repo_with_yes_outside_git exercises TC-TRG-092:
// `tai --repo acme/app forget --yes` succeeds from any working
// directory (no git resolution), prints the destructive summary,
// commits the delete, and `tai list --pr 1` then exits TRIAGE_NOT_FOUND
// because the cascade removed the PR row alongside the repo row.
func TestForget_TCTRG092_repo_with_yes_outside_git(t *testing.T) {
	cmdtest.Isolate(t)
	cmdtest.Chdir(t, t.TempDir())
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "forget", "--yes")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "You're about to delete:")
	cmdtest.AssertStdoutContains(t, r, "Done.")
	if strings.Contains(r.Stderr, "REPO_NOT_FOUND") {
		t.Fatalf("--repo mode should not require git resolution; got stderr:\n%s", r.Stderr)
	}

	// The cascade should have deleted the repo row and everything
	// underneath it. A follow-up triage verb now reports the repo as
	// having no triage data.
	r2 := triage(t, "list", "--pr", "1")
	cmdtest.AssertError(t, r2)
	cmdtest.AssertErrorFooter(t, r2, "TRIAGE_NOT_FOUND", 2)
}

// TestForget_TCTRG093_non_interactive_no_consent exercises TC-TRG-093:
// non-TTY stdin without --yes / env var → exit 1
// TRIAGE_CONFIRMATION_REQUIRED. No rows deleted.
func TestForget_TCTRG093_non_interactive_no_consent(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "forget", "--pr", "1")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_CONFIRMATION_REQUIRED", 1)

	// Row should still be present.
	rl := triage(t, "list", "--pr", "1")
	cmdtest.AssertStdoutContains(t, rl, "  ID  SEV")
}

// TestForget_TCTRG094_env_skips_prompt exercises TC-TRG-094:
// TAI_ACCEPT_DESTRUCTIVE=1 grants consent.
func TestForget_TCTRG094_env_skips_prompt(t *testing.T) {
	cmdtest.Isolate(t)
	t.Setenv("TAI_ACCEPT_DESTRUCTIVE", "1")
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "forget", "--pr", "1")
	cmdtest.AssertNoError(t, r)
}

// TestForget_TCTRG095_status_prune_pr exercises TC-TRG-095: --status
// on --pr prunes only matching comments, preserves the PR row.
func TestForget_TCTRG095_status_prune_pr(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "first", "critical", "pending")+","+commentJSON("r2", "second", "major", "pending"))
	triage(t, "complete", "1", "--pr", "1")

	r := triage(t, "forget", "--pr", "1", "--status", "completed", "--yes")
	cmdtest.AssertNoError(t, r)

	// PR row survives; only the non-completed row remains.
	rl := triage(t, "list", "--pr", "1")
	cmdtest.AssertStdoutContains(t, rl, "second")
	if strings.Contains(rl.Stdout, "first") {
		t.Fatalf("completed comment should have been pruned, got:\n%s", rl.Stdout)
	}
}

// TestForget_TCTRG096_status_on_comment_rejected exercises TC-TRG-096:
// `--status` + `--comment` is rejected.
func TestForget_TCTRG096_status_on_comment_rejected(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))
	r := triage(t, "forget", "--comment", "1", "--status", "completed", "--pr", "1", "--yes")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_INVALID_FLAGS", 1)
}

// TestForget_TCTRG097_repo_status_prune exercises TC-TRG-097:
// `tai --repo X forget --status completed --yes` prunes matching
// comments across the entire repo while preserving the repos / prs /
// branches rows.
func TestForget_TCTRG097_repo_status_prune(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "alpha", "critical", "pending"))
	seedPR(t, 2, commentJSON("r2", "beta", "major", "pending"))
	triage(t, "complete", "1", "--pr", "1")
	triage(t, "complete", "1", "--pr", "2")

	r := triage(t, "forget", "--status", "completed", "--yes")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Done.")

	// PR rows still exist (they should be queryable via list).
	r1 := triage(t, "list", "--pr", "1")
	cmdtest.AssertNoError(t, r1)
	cmdtest.AssertStdoutContains(t, r1, "(no comments)")
	r2 := triage(t, "list", "--pr", "2")
	cmdtest.AssertNoError(t, r2)
	cmdtest.AssertStdoutContains(t, r2, "(no comments)")
}

// TestForget_TCTRG098_batch_status_recompute exercises TC-TRG-098:
// `tai forget --batch B1 --status completed --yes` deletes only the
// matching members, preserves the batch row, and recomputes the
// batch status against the surviving members.
func TestForget_TCTRG098_batch_status_recompute(t *testing.T) {
	cmdtest.Isolate(t)
	batches := `[{"batch_key": "B1", "title": "T"}]`
	payload := buildPRPayloadWithBatches(1, "t", "feat/x", batches,
		commentInBatch("r1", "first", "critical", "B1")+","+
			commentInBatch("r2", "second", "major", "B1"))
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)

	triage(t, "complete", "1", "--pr", "1")
	triage(t, "accept", "2", "--pr", "1")

	r = triage(t, "forget", "--pr", "1", "--batch", "B1", "--status", "completed", "--yes")
	cmdtest.AssertNoError(t, r)

	// Surviving member is the accepted one; batch row remains and
	// recomputes to `accepted`.
	rs := triage(t, "status", "--pr", "1")
	cmdtest.AssertStdoutContains(t, rs, "B1 (1 comments — accepted)")
}

// TestForget_TCTRG099_multi_value_status exercises TC-TRG-099:
// multiple `--status` values on `tai forget` combine via OR.
func TestForget_TCTRG099_multi_value_status(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1,
		commentJSON("r1", "a", "critical", "pending")+","+
			commentJSON("r2", "b", "major", "pending")+","+
			commentJSON("r3", "c", "minor", "pending"))
	triage(t, "complete", "1", "--pr", "1")
	triage(t, "dismiss", "2", "--pr", "1", "--reason", "x")
	// r3 stays pending.

	r := triage(t, "forget", "--pr", "1", "--status", "completed", "--status", "dismissed", "--yes")
	cmdtest.AssertNoError(t, r)

	rl := triage(t, "list", "--pr", "1")
	cmdtest.AssertNoError(t, rl)
	// Only the pending row survives.
	cmdtest.AssertStdoutContains(t, rl, "c")
	for _, gone := range []string{"  1   crit", "  2   maj"} {
		if strings.Contains(rl.Stdout, gone) {
			t.Fatalf("expected pruned rows absent; found %q in:\n%s", gone, rl.Stdout)
		}
	}
}
