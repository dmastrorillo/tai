package cmd_test

import (
	"testing"

	"github.com/dmastrorillo/tai/internal/cmd"
	"github.com/dmastrorillo/tai/internal/cmdtest"
)

// TestRoot_TCCMD002_unknown_flag exercises TC-CMD-002 from
// test-cases.md (flag form): an unrecognised flag like `--bogus-flag`
// flows through OnUsageError, gets wrapped as UnknownSubcommand, and
// surfaces with exit 1 and the foundation footer.
func TestRoot_TCCMD002_unknown_flag(t *testing.T) {
	r := cmdtest.Run(t, cmd.NewRoot(), "--bogus-flag")

	cmdtest.AssertError(t, r)
	cmdtest.AssertExitCode(t, r, 1)
	cmdtest.AssertErrorFooter(t, r, "UNKNOWN_SUBCOMMAND", 1)
	cmdtest.AssertStderrContains(t, r, "Error:")
	cmdtest.AssertStderrContains(t, r, "What to do:")
}

// TestRoot_TCCMD002_unknown_positional exercises TC-CMD-002's
// positional form: `tai bogus` (no subcommand named "bogus") flows
// through the root Action's catch-all and surfaces with exit 1 and
// the foundation footer.
//
// Historical regression: a previous wiring had urfave/cli's default
// helpCommandAction swallow the unmatched arg and exit 0. The fix is
// a root Action that emits UnknownSubcommand on any present positional.
func TestRoot_TCCMD002_unknown_positional(t *testing.T) {
	r := cmdtest.Run(t, cmd.NewRoot(), "bogus")

	cmdtest.AssertError(t, r)
	cmdtest.AssertExitCode(t, r, 1)
	cmdtest.AssertErrorFooter(t, r, "UNKNOWN_SUBCOMMAND", 1)
	cmdtest.AssertStderrContains(t, r, "Error:")
	cmdtest.AssertStderrContains(t, r, "bogus")
}
