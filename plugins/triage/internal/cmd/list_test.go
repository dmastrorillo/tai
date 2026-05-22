package cmd_test

import (
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdtest"
)

// TestList_TCTRG020_with_comments exercises TC-TRG-020: `tai list`
// with comments in scope prints a header and one row per comment.
func TestList_TCTRG020_with_comments(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "first", "critical", "pending")+","+commentJSON("r2", "second", "major", "pending"))

	r := triage(t, "list", "--pr", "1")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Repo: acme/app")
	cmdtest.AssertStdoutContains(t, r, "Scope: PR #1")
	cmdtest.AssertStdoutContains(t, r, "  ID  SEV")
	// Both titles appear.
	cmdtest.AssertStdoutContains(t, r, "first")
	cmdtest.AssertStdoutContains(t, r, "second")
}

// TestList_TCTRG021_empty_scope exercises TC-TRG-021: empty scope
// emits the header and the literal "(no comments)".
func TestList_TCTRG021_empty_scope(t *testing.T) {
	cmdtest.Isolate(t)
	seedBranch(t, "feat/empty", "")

	r := triage(t, "list", "--branch", "feat/empty")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "(no comments)")
}

// TestList_TCTRG022_severity_abbreviated exercises TC-TRG-022:
// severity is abbreviated to crit/maj/min/nit.
func TestList_TCTRG022_severity_abbreviated(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending")+","+commentJSON("r2", "t", "nitpick", "pending"))

	r := triage(t, "list", "--pr", "1")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "crit")
	cmdtest.AssertStdoutContains(t, r, "nit")
	// Full severity words should not appear in the data rows.
	for _, full := range []string{"critical ", "nitpick "} {
		if strings.Contains(r.Stdout, full) {
			t.Fatalf("expected severity to be abbreviated, found %q in:\n%s", full, r.Stdout)
		}
	}
}

// TestList_TCTRG025_single_status_filter exercises TC-TRG-025:
// `tai list --status accepted` returns only accepted rows.
func TestList_TCTRG025_single_status_filter(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "first", "critical", "pending")+","+commentJSON("r2", "second", "major", "pending"))

	// Accept the second one so we have a heterogeneous status mix.
	triage(t, "accept", "2", "--pr", "1")

	r := triage(t, "list", "--pr", "1", "--status", "accepted")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "second")
	if strings.Contains(r.Stdout, "first") {
		t.Fatalf("expected pending row to be filtered out, got:\n%s", r.Stdout)
	}
}

// TestList_TCTRG023_multi_status_or exercises TC-TRG-023: multiple
// --status values combine via OR.
func TestList_TCTRG023_multi_status_or(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1,
		commentJSON("r1", "alpha", "critical", "pending")+","+
			commentJSON("r2", "beta", "major", "pending")+","+
			commentJSON("r3", "gamma", "nitpick", "pending"))
	triage(t, "accept", "1", "--pr", "1")
	triage(t, "complete", "2", "--pr", "1")
	// gamma stays pending.

	r := triage(t, "list", "--pr", "1", "--status", "accepted", "--status", "completed")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "alpha")
	cmdtest.AssertStdoutContains(t, r, "beta")
	if strings.Contains(r.Stdout, "gamma") {
		t.Fatalf("expected pending row to be filtered out; got:\n%s", r.Stdout)
	}
}

// TestList_TCTRG026_unknown_status exercises TC-TRG-026: unknown
// status value rejects with TRIAGE_INVALID_FLAGS.
func TestList_TCTRG026_unknown_status(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "list", "--pr", "1", "--status", "urgent")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_INVALID_FLAGS", 1)
}
