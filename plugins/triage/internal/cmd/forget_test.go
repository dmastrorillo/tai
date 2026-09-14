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
// which `tai triage forget` would interpret as the repo-selector mode.
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
// commits the delete, and `tai triage list --pr 1` then exits TRIAGE_NOT_FOUND
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
// `tai triage forget --batch B1 --status completed --yes` deletes only the
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
// multiple `--status` values on `tai triage forget` combine via OR.
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

// TestForget_TCTRG107_status_prune_clears_emptied_batches exercises
// TC-TRG-107: pruning a scope by status removes batches the prune
// emptied, and the consent summary counts them.
func TestForget_TCTRG107_status_prune_clears_emptied_batches(t *testing.T) {
	cmdtest.Isolate(t)
	batches := `[{"batch_key": "B1", "title": "Emptied"}, {"batch_key": "B2", "title": "Survives"}]`
	payload := buildPRPayloadWithBatches(1, "t", "feat/x", batches,
		commentInBatch("r1", "first", "critical", "B1")+","+
			commentInBatch("r2", "second", "major", "B2"))
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)

	// B1's only member is completed; B2's stays pending.
	triage(t, "complete", "1", "--pr", "1")

	r = triage(t, "forget", "--pr", "1", "--status", "completed", "--yes")
	cmdtest.AssertNoError(t, r)
	// The summary is the user's only view of what consent covers, so
	// a batch this prune deletes has to be named in it.
	cmdtest.AssertStdoutContains(t, r, "1 batches")

	rs := triage(t, "status", "--pr", "1")
	if strings.Contains(rs.Stdout, "B1") {
		t.Errorf("B1 lost its only member, so it must not survive the prune:\n%s", rs.Stdout)
	}
	// A batch that still has members is untouched.
	cmdtest.AssertStdoutContains(t, rs, "B2 (1 comments — pending)")
}

// TestForget_TCTRG107_status_prune_keeps_populated_batches exercises
// TC-TRG-107: a prune that empties no batch must not touch any, and
// must not claim to.
func TestForget_TCTRG107_status_prune_keeps_populated_batches(t *testing.T) {
	cmdtest.Isolate(t)
	batches := `[{"batch_key": "B1", "title": "Mixed"}]`
	payload := buildPRPayloadWithBatches(1, "t", "feat/x", batches,
		commentInBatch("r1", "first", "critical", "B1")+","+
			commentInBatch("r2", "second", "major", "B1"))
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)

	triage(t, "complete", "1", "--pr", "1")

	r = triage(t, "forget", "--pr", "1", "--status", "completed", "--yes")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "0 batches")

	rs := triage(t, "status", "--pr", "1")
	cmdtest.AssertStdoutContains(t, rs, "B1 (1 comments — pending)")
}

// TestForget_TCTRG107_repo_status_prune_clears_emptied_batches
// exercises TC-TRG-107: the same rule for a whole-repo prune, which
// deletes across every PR and branch under the repo.
func TestForget_TCTRG107_repo_status_prune_clears_emptied_batches(t *testing.T) {
	cmdtest.Isolate(t)
	batches := `[{"batch_key": "B1", "title": "Emptied"}]`
	payload := buildPRPayloadWithBatches(1, "t", "feat/x", batches,
		commentInBatch("r1", "only", "critical", "B1"))
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)

	triage(t, "complete", "1", "--pr", "1")

	r = triage(t, "forget", "--status", "completed", "--yes")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "1 batches")

	rs := triage(t, "status", "--pr", "1")
	if strings.Contains(rs.Stdout, "B1") {
		t.Errorf("a repo-wide prune must clear emptied batches too:\n%s", rs.Stdout)
	}
}

// TestForget_TCTRG107_status_prune_leaves_pre_existing_empty_batches
// exercises TC-TRG-107: a batch that was already empty before the
// prune is not the prune's business, and must survive it.
//
// Import creates this state: every entry in a payload's batches[] is
// inserted whether or not a comment references it, so a batch can
// exist with no members from the moment it lands.
func TestForget_TCTRG107_status_prune_leaves_pre_existing_empty_batches(t *testing.T) {
	cmdtest.Isolate(t)
	batches := `[{"batch_key": "B1", "title": "Emptied by the prune"}, {"batch_key": "B2", "title": "Already empty"}]`
	payload := buildPRPayloadWithBatches(1, "t", "feat/x", batches,
		commentInBatch("r1", "only", "critical", "B1"))
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)

	triage(t, "complete", "1", "--pr", "1")

	r = triage(t, "forget", "--pr", "1", "--status", "completed", "--yes")
	cmdtest.AssertNoError(t, r)
	// One batch is emptied by this prune. B2 was empty before it ran,
	// so deleting it would be a deletion the user never agreed to.
	cmdtest.AssertStdoutContains(t, r, "1 batches")

	rs := triage(t, "status", "--pr", "1")
	if strings.Contains(rs.Stdout, "B1") {
		t.Errorf("B1 lost its only member, so it must not survive:\n%s", rs.Stdout)
	}
	if !strings.Contains(rs.Stdout, "B2") {
		t.Errorf("B2 was already empty and is not this prune's business:\n%s", rs.Stdout)
	}
}

// TestForget_TCTRG107_branch_status_prune_maintains_batches exercises
// TC-TRG-107 on the branch selector: batches reached by branch_id get
// the same maintenance as those reached by pr_id.
//
// The PR cases above cannot speak for this — scopeComments builds a
// different column, and a transposed one would leave every
// branch-scoped batch unmaintained with the suite still green.
func TestForget_TCTRG107_branch_status_prune_maintains_batches(t *testing.T) {
	cmdtest.Isolate(t)
	batches := `[{"batch_key": "B1", "title": "Emptied"}, {"batch_key": "B2", "title": "Survives"}]`
	payload := buildBranchPayloadWithBatches("feat/x", batches,
		commentInBatch("r1", "first", "critical", "B1")+","+
			commentInBatch("r2", "second", "major", "B2"))
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)

	triage(t, "complete", "1", "--branch", "feat/x")

	r = triage(t, "forget", "--branch", "feat/x", "--status", "completed", "--yes")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "1 batches")

	rs := triage(t, "status", "--branch", "feat/x")
	if strings.Contains(rs.Stdout, "B1") {
		t.Errorf("B1 lost its only member, so it must not survive:\n%s", rs.Stdout)
	}
	cmdtest.AssertStdoutContains(t, rs, "B2 (1 comments — pending)")
}

// TestForget_TCTRG107_repo_prune_spares_another_repo exercises
// TC-TRG-107's isolation guarantee: a repo-wide prune reaches batches
// through prs and branches, and must not cross into a repo it was not
// asked about.
func TestForget_TCTRG107_repo_prune_spares_another_repo(t *testing.T) {
	cmdtest.Isolate(t)
	mine := buildPRPayloadInRepo("acme/app", 1, "t", "feat/x",
		`[{"batch_key": "B1", "title": "Mine"}]`,
		commentInBatch("r1", "mine", "critical", "B1"))
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), mine, "import", "-")
	cmdtest.AssertNoError(t, r)

	theirs := buildPRPayloadInRepo("other/app", 1, "t", "feat/y",
		`[{"batch_key": "B1", "title": "Theirs"}]`,
		commentInBatch("r2", "theirs", "critical", "B1"))
	r = cmdtest.RunWithStdin(t, cmd.NewRoot(), theirs, "import", "-")
	cmdtest.AssertNoError(t, r)

	// Both repos' batches hold exactly one completed member, so an
	// ownership filter that leaked across repos would see two batches
	// about to be emptied and say so. A pending member in the other
	// repo would mask the leak: it can never be counted as emptied,
	// whatever the filter matches.
	triage(t, "complete", "1", "--pr", "1")
	cmdtest.Run(t, cmd.NewRoot(), "--repo", "other/app", "complete", "1", "--pr", "1")

	r = triage(t, "forget", "--status", "completed", "--yes")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "1 batches")

	// The other repo's comment was never deleted, so its batch keeps
	// the member and the status the prune had no business changing.
	rs := cmdtest.Run(t, cmd.NewRoot(), "--repo", "other/app", "status", "--pr", "1")
	cmdtest.AssertNoError(t, rs)
	cmdtest.AssertStdoutContains(t, rs, "B1 (1 comments — completed)")
}
