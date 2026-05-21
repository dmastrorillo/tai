package cmd_test

import (
	"testing"

	"github.com/dmastrorillo/tai/internal/cmd"
	"github.com/dmastrorillo/tai/internal/cmdtest"
	"github.com/dmastrorillo/tai/internal/version"
)

// TestVersion_TCCMD001_prints_version_string exercises TC-CMD-001 from
// test-cases.md: invoking `tai --version` writes a line of the form
// "tai version <version>" to stdout and exits with code 0, with no stderr.
//
// The assertion includes the literal "tai version " prefix so the test
// fails on accidentally-matching output (a substring search on the
// version alone would pass for free-form text that happened to contain
// "dev"). AssertExitCode encodes the spec's "exit 0" Then clause; the
// helper derives the value from cli.Command.Run's return without spawning
// a subprocess.
func TestVersion_TCCMD001_prints_version_string(t *testing.T) {
	r := cmdtest.Run(t, cmd.NewRoot(), "--version")

	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)
	cmdtest.AssertStderrEmpty(t, r)
	cmdtest.AssertStdoutContains(t, r, "tai version "+version.String)
}
