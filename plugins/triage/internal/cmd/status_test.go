package cmd_test

import (
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmd"
	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdtest"
)

// TestStatus_TCTRG080_pr_with_batches exercises TC-TRG-080: PR
// scope with batches shows Repo / Scope / Counts / Batches blocks.
func TestStatus_TCTRG080_pr_with_batches(t *testing.T) {
	cmdtest.Isolate(t)
	batches := `[{"batch_key": "B1", "title": "Replace execSync"}]`
	payload := buildPRPayloadWithBatches(142, "feat: oauth", "feat/oauth", batches,
		commentInBatch("r1", "first", "critical", "B1"))
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)

	rs := triage(t, "status", "--pr", "142")
	cmdtest.AssertNoError(t, rs)
	cmdtest.AssertStdoutContains(t, rs, "Repo: acme/app")
	cmdtest.AssertStdoutContains(t, rs, "Scope: PR #142 — feat: oauth (branch: feat/oauth)")
	cmdtest.AssertStdoutContains(t, rs, "Total:      1")
	cmdtest.AssertStdoutContains(t, rs, "Pending:    1")
	cmdtest.AssertStdoutContains(t, rs, "Batches: 1")
	cmdtest.AssertStdoutContains(t, rs, "B1 (1 comments — pending) — Replace execSync")
	cmdtest.AssertStdoutContains(t, rs, "[exit 0]")
}

// TestStatus_TCTRG081_branch_without_batches exercises TC-TRG-081:
// branch scope without batches omits the Batches block.
func TestStatus_TCTRG081_branch_without_batches(t *testing.T) {
	cmdtest.Isolate(t)
	seedBranch(t, "feat/x", commentJSON("r1", "t", "critical", "pending"))

	r := triage(t, "status", "--branch", "feat/x")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Scope: branch feat/x")
	cmdtest.AssertStdoutContains(t, r, "Total:      1")
	if strings.Contains(r.Stdout, "Batches:") {
		t.Fatalf("branch with no batches should omit Batches block, got:\n%s", r.Stdout)
	}
}

// TestStatus_TCTRG082_empty_scope exercises TC-TRG-082: a target with
// no comments still emits the header and Total: 0.
func TestStatus_TCTRG082_empty_scope(t *testing.T) {
	cmdtest.Isolate(t)
	seedBranch(t, "feat/empty", "")

	r := triage(t, "status", "--branch", "feat/empty")
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Total:      0")
}
