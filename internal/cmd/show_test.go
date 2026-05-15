package cmd_test

import (
	"strings"
	"testing"

	"github.com/danielmastrorillo/tai/internal/cmd"
	"github.com/danielmastrorillo/tai/internal/cmdtest"
)

// TestShow_TCTRG030_pending_comment exercises TC-TRG-030: a pending
// comment has no Resolution / Dismissed-because sections and no
// Batch meta line.
func TestShow_TCTRG030_pending_comment(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "shell injection", "critical", "pending"))

	r := triage(t, "show", "1", "--pr", "1")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "# acme/app PR #1 — feat: x — comment 1 of 1")
	cmdtest.AssertStdoutContains(t, r, "**Severity:** critical")
	cmdtest.AssertStdoutContains(t, r, "## Title\nshell injection")
	cmdtest.AssertStdoutContains(t, r, "## Why fix it")
	if strings.Contains(r.Stdout, "**Batch:**") {
		t.Fatal("pending unbatched comment should not show Batch meta")
	}
	if strings.Contains(r.Stdout, "## Resolution") || strings.Contains(r.Stdout, "## Dismissed because") {
		t.Fatal("pending comment should not show Resolution/Dismissed sections")
	}
}

// TestShow_TCTRG031_accepted_with_resolution exercises TC-TRG-031:
// `tai show` on an accepted comment with resolution surfaces the
// "## Resolution" section.
func TestShow_TCTRG031_accepted_with_resolution(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))
	triage(t, "accept", "1", "--pr", "1", "--resolution", "use execFileSync")

	r := triage(t, "show", "1", "--pr", "1")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "## Resolution\nuse execFileSync")
}

// TestShow_TCTRG032_dismissed exercises TC-TRG-032: a dismissed
// comment shows the Dismissed-because section with reason and by.
func TestShow_TCTRG032_dismissed(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))
	triage(t, "dismiss", "1", "--pr", "1",
		"--reason", "false positive", "--by", "danm")

	r := triage(t, "show", "1", "--pr", "1")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "## Dismissed because\nfalse positive (by danm)")
	if strings.Contains(r.Stdout, "## Resolution") {
		t.Fatal("dismissed comment should not show Resolution section")
	}
}

// TestShow_TCTRG034_all_two_comments exercises TC-TRG-034: --all
// emits multiple blocks joined by the spec's separator.
func TestShow_TCTRG034_all_two_comments(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "first", "critical", "pending")+","+commentJSON("r2", "second", "major", "pending"))

	r := triage(t, "show", "--all", "--pr", "1")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "first")
	cmdtest.AssertStdoutContains(t, r, "second")
	if !strings.Contains(r.Stdout, "\n---\n") {
		t.Fatalf("expected --- separator, got:\n%s", r.Stdout)
	}
}

// TestShow_TCTRG035_all_empty exercises TC-TRG-035: --all in an
// empty scope produces zero-byte stdout.
func TestShow_TCTRG035_all_empty(t *testing.T) {
	cmdtest.Isolate(t)
	seedBranch(t, "feat/empty", "")

	r := triage(t, "show", "--all", "--branch", "feat/empty")
	cmdtest.AssertNoError(t, r)
	if r.Stdout != "" {
		t.Fatalf("expected zero-byte stdout for empty --all, got %q", r.Stdout)
	}
}

// TestShow_TCTRG033_batch_meta_present exercises the spec's "Comment
// with batch shows batch meta" scenario: a comment whose batch_id is
// set renders a `**Batch:** <key> — <title>` meta line in `tai show`.
func TestShow_TCTRG033_batch_meta_present(t *testing.T) {
	cmdtest.Isolate(t)
	batches := `[{"batch_key": "B1", "title": "Replace execSync"}]`
	payload := buildPRPayloadWithBatches(1, "t", "feat/x", batches,
		commentInBatch("r1", "shell injection", "critical", "B1"))
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)

	r2 := triage(t, "show", "1", "--pr", "1")
	cmdtest.AssertNoError(t, r2)
	cmdtest.AssertStdoutContains(t, r2, "**Batch:** B1 — Replace execSync")
}

// TestShow_TCTRG036_all_status_filter exercises the spec's "tai show
// --all --status" filter: only comments matching the supplied
// statuses appear.
func TestShow_TCTRG036_all_status_filter(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1,
		commentJSON("r1", "alpha", "critical", "pending")+","+
			commentJSON("r2", "beta", "major", "pending"))
	triage(t, "accept", "1", "--pr", "1")

	r := triage(t, "show", "--all", "--pr", "1", "--status", "accepted")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "alpha")
	if strings.Contains(r.Stdout, "beta") {
		t.Fatalf("expected pending comment filtered out; got:\n%s", r.Stdout)
	}
}

// TestShow_TCTRG038_status_rejected_on_single exercises TC-TRG-038:
// --status is rejected on `tai show <id>`.
func TestShow_TCTRG038_status_rejected_on_single(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "show", "1", "--pr", "1", "--status", "pending")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_INVALID_FLAGS", 1)
}
