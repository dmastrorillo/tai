package cmd_test

import (
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/internal/cmd"
	"github.com/dmastrorillo/tai/internal/cmdtest"
)

// TestAccept_TCTRG040_accept_pending exercises TC-TRG-040: accept
// flips a pending row to accepted and records the resolution.
func TestAccept_TCTRG040_accept_pending(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "accept", "1", "--pr", "1",
		"--resolution", "use execFileSync")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Accepted comment 1")
	cmdtest.AssertStdoutContains(t, r, "[exit 0]")

	// Confirm via `tai show`.
	r2 := triage(t, "show", "1", "--pr", "1")
	cmdtest.AssertStdoutContains(t, r2, "**Status:** accepted")
	cmdtest.AssertStdoutContains(t, r2, "## Resolution\nuse execFileSync")
}

// TestAccept_TCTRG041_reversal exercises TC-TRG-041: accepting a
// previously-dismissed comment clears dismissed_by/dismiss_reason.
func TestAccept_TCTRG041_reversal_from_dismissed(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))
	triage(t, "dismiss", "1", "--pr", "1", "--reason", "x")
	triage(t, "accept", "1", "--pr", "1")

	r := triage(t, "show", "1", "--pr", "1")
	cmdtest.AssertStdoutContains(t, r, "**Status:** accepted")
	if strings.Contains(r.Stdout, "Dismissed because") {
		t.Fatal("accept after dismiss should clear dismissed_by/reason")
	}
}

// TestAccept_TCTRG042_idempotent exercises TC-TRG-042: re-accepting
// an already-accepted comment is a no-op success.
func TestAccept_TCTRG042_idempotent(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))
	triage(t, "accept", "1", "--pr", "1")
	r := triage(t, "accept", "1", "--pr", "1")
	cmdtest.AssertNoError(t, r)
}

// TestAccept_TCTRG043_by_batch exercises TC-TRG-043: --batch flips
// every member.
func TestAccept_TCTRG043_by_batch(t *testing.T) {
	cmdtest.Isolate(t)
	batches := `[{"batch_key": "B1", "title": "T"}]`
	payload := buildPRPayloadWithBatches(1, "t", "feat/x", batches,
		commentInBatch("r1", "first", "critical", "B1")+","+
			commentInBatch("r2", "second", "major", "B1"))
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)

	r = triage(t, "accept", "--pr", "1", "--batch", "B1")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Accepted batch B1")

	// Both members must now be accepted.
	r2 := triage(t, "list", "--pr", "1", "--status", "accepted")
	cmdtest.AssertStdoutContains(t, r2, "first")
	cmdtest.AssertStdoutContains(t, r2, "second")
}

// TestAccept_TCTRG044_mutex exercises TC-TRG-044: <id> + --batch is a
// usage error.
func TestAccept_TCTRG044_mutex(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))
	r := triage(t, "accept", "1", "--pr", "1", "--batch", "B1")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_INVALID_FLAGS", 1)
}

// TestAccept_TCTRG045_not_found exercises TC-TRG-045: accept on a
// non-existent position fails TRIAGE_NOT_FOUND.
func TestAccept_TCTRG045_not_found(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))
	r := triage(t, "accept", "99", "--pr", "1")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_NOT_FOUND", 2)
}

// TestDismiss_TCTRG050_missing_reason exercises TC-TRG-050: missing
// --reason is TRIAGE_INVALID_FLAGS.
func TestDismiss_TCTRG050_missing_reason(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))
	r := triage(t, "dismiss", "1", "--pr", "1")
	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_INVALID_FLAGS", 1)
}

// TestDismiss_TCTRG051_records_by exercises TC-TRG-051: --by override
// stamps the dismissed_by field.
func TestDismiss_TCTRG051_records_by(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))
	triage(t, "dismiss", "1", "--pr", "1",
		"--reason", "not in scope", "--by", "alice")
	r := triage(t, "show", "1", "--pr", "1")
	cmdtest.AssertStdoutContains(t, r, "not in scope (by alice)")
}

// TestComplete_TCTRG060_complete_pending exercises TC-TRG-060:
// complete sets status='completed' and records resolution.
func TestComplete_TCTRG060_complete_pending(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))
	r := triage(t, "complete", "1", "--pr", "1",
		"--resolution", "already fixed in e7eeec0")
	cmdtest.AssertNoError(t, r)
	r2 := triage(t, "show", "1", "--pr", "1")
	cmdtest.AssertStdoutContains(t, r2, "**Status:** completed")
	cmdtest.AssertStdoutContains(t, r2, "## Resolution\nalready fixed in e7eeec0")
}

// TestBatch_TCTRG070_uniform_terminal exercises TC-TRG-070: when
// every member of a batch transitions to the same terminal state,
// the batch's status matches that state.
func TestBatch_TCTRG070_uniform_terminal(t *testing.T) {
	cmdtest.Isolate(t)
	batches := `[{"batch_key": "B1", "title": "T"}]`
	payload := buildPRPayloadWithBatches(1, "t", "feat/x", batches,
		commentInBatch("r1", "first", "critical", "B1")+","+
			commentInBatch("r2", "second", "major", "B1"))
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)

	// Accept all members of B1 via the batch verb.
	triage(t, "accept", "--pr", "1", "--batch", "B1")

	rs := triage(t, "status", "--pr", "1")
	cmdtest.AssertStdoutContains(t, rs, "B1 (2 comments — accepted)")
}

// TestBatch_TCTRG071_split_is_mixed exercises TC-TRG-071: a batch
// whose members split across distinct terminal states becomes "mixed".
func TestBatch_TCTRG071_split_is_mixed(t *testing.T) {
	cmdtest.Isolate(t)
	batches := `[{"batch_key": "B1", "title": "T"}]`
	payload := buildPRPayloadWithBatches(1, "t", "feat/x", batches,
		commentInBatch("r1", "first", "critical", "B1")+","+
			commentInBatch("r2", "second", "major", "B1"))
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)

	triage(t, "accept", "1", "--pr", "1")
	triage(t, "dismiss", "2", "--pr", "1", "--reason", "x")

	rs := triage(t, "status", "--pr", "1")
	cmdtest.AssertStdoutContains(t, rs, "B1 (2 comments — mixed)")
}

// TestBatch_TCTRG072_pending_plus_terminal_is_mixed exercises
// TC-TRG-072: when some members are still pending and others have
// transitioned to a terminal state, the batch is "mixed".
func TestBatch_TCTRG072_pending_plus_terminal_is_mixed(t *testing.T) {
	cmdtest.Isolate(t)
	batches := `[{"batch_key": "B1", "title": "T"}]`
	payload := buildPRPayloadWithBatches(1, "t", "feat/x", batches,
		commentInBatch("r1", "first", "critical", "B1")+","+
			commentInBatch("r2", "second", "major", "B1"))
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)

	// Accept just the first; leave the second pending.
	triage(t, "accept", "1", "--pr", "1")

	rs := triage(t, "status", "--pr", "1")
	cmdtest.AssertStdoutContains(t, rs, "B1 (2 comments — mixed)")
}

// TestDismiss_TCTRG052_reversal_clears_resolution exercises the
// spec's "Dismiss state reversal" scenario: dismissing a comment
// that was previously accepted clears its `resolution` field.
func TestDismiss_TCTRG052_reversal_clears_resolution(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1, commentJSON("r1", "t", "critical", "pending"))
	triage(t, "accept", "1", "--pr", "1", "--resolution", "use execFileSync")
	triage(t, "dismiss", "1", "--pr", "1", "--reason", "not in scope")

	r := triage(t, "show", "1", "--pr", "1")
	cmdtest.AssertStdoutContains(t, r, "**Status:** dismissed")
	if strings.Contains(r.Stdout, "## Resolution") {
		t.Fatalf("dismiss after accept should clear resolution; got:\n%s", r.Stdout)
	}
}
