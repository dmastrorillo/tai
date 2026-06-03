package cmd_test

import (
	"testing"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmd"
	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdtest"
	"github.com/dmastrorillo/tai/plugins/triage/internal/version"
)

// TestVersion_TCMIG001_binary_self_identifies_as_triage exercises
// TC-MIG-001 from plugins/triage/test-cases.md: invoking `--version`
// on the triage plugin binary writes a line of the form
// "triage version <version>" to stdout and exits with code 0, with
// no stderr.
//
// Phase 6 of pivot-to-ai-as-code repackaged the in-process Triage
// codebase as a standalone `triage` plugin binary. Pre-Phase-6 the
// binary shipped inside `tai` and identified as such (the TC-CMD-001
// in core/test-cases.md owns that contract for the host binary).
// This test owns the triage-side equivalent: the renamed binary's
// version line prefix tracks its own NewRoot Name field. Keeping
// the TC-IDs distinct preserves CLAUDE.md's global-uniqueness rule
// for TC identifiers.
func TestVersion_TCMIG001_binary_self_identifies_as_triage(t *testing.T) {
	r := cmdtest.Run(t, cmd.NewRoot(), "--version")

	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)
	cmdtest.AssertStderrEmpty(t, r)
	cmdtest.AssertStdoutContains(t, r, "triage version "+version.String)
}
