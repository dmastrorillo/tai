// Package cmdtest is the test harness for the triage plugin's CLI.
//
// The harness body — in-process Run/RunWithStdin with captured stdio,
// the Result type, and the Assert* vocabulary — lives in pkg/clitest,
// shared with the core tree's cmd tests so error rendering and
// exit-code mapping cannot drift between the two suites (both route
// through cliexec.Exit, the same translation every binary's main
// performs). This package re-exports that surface under the names the
// triage test files use, and adds the triage-specific Isolate
// filesystem/env fixture (see isolate.go).
//
// # Phase 6 (pivot-to-ai-as-code) scope decision
//
// Pre-Phase-6 the harness drove the in-process triage cmd tree that
// the `tai` binary embedded directly. Post-Phase-6 the same cmd tree
// is the entry point for the standalone `triage` plugin binary that
// the host invokes via subprocess.
//
// The harness still drives the cmd tree IN-PROCESS, not via
// subprocess exec. Two reasons:
//
//  1. The host-side subprocess wiring (env-var injection, stdio
//     passthrough, exit-code propagation) is verified by core's
//     plugin-host tests (TC-PLG-002, TC-PLG-005). Those tests run
//     a POSIX shell-stub plugin and assert the host fulfils the
//     contract; the plugin-side consumption (taiplugin.Load →
//     storage path → verb dispatch) is exercised by this harness's
//     in-process tests. Re-testing both ends in the same suite would
//     duplicate coverage AND add a build-binary-then-exec
//     dependency to every TC-IMP/TRG/INST test.
//  2. The verb-dispatch logic exercised by these tests is the same
//     regardless of whether the cmd tree is reached in-process or
//     via subprocess. The harness pins what only the triage cmd
//     tree owns; the subprocess transport is the host's
//     responsibility.
//
// A future revision MAY introduce an `ExecRoot` variant that builds
// the triage binary and exec's it for end-to-end coverage of both
// transports. Today's TC-IDs do not require it.
//
// Conventions:
//
//   - Every test that maps to a BDD case in plugins/triage/test-cases.md
//     (or core/test-cases.md for CMD-001/002/008 and the ERR cases) names the
//     TC-ID in its function name AND its t.Run subtest descriptions, so a
//     failure surfaces the trace back to the spec. Example:
//
//     func TestVersion_TCCMD001_prints_version_string(t *testing.T) { ... }
//
//     The pattern is Test<Cmd>_<TCID>_<short_description>, where <TCID>
//     is the TC-ID with hyphens removed (TC-CMD-001 → TCCMD001).
//
//   - Helpers t.Helper() themselves so failure line numbers point at the
//     caller, not at this file.
//
//   - The assert vocabulary is intentionally narrow. If you reach for a
//     helper that does not exist, prefer adding it in pkg/clitest over
//     inlining the check — the harness is the contract every test obeys.
package cmdtest

import (
	"testing"

	"github.com/dmastrorillo/tai/pkg/clitest"
	"github.com/urfave/cli/v3"
)

// Result is pkg/clitest's captured-run result.
type Result = clitest.Result

// Run invokes cmd with the given argv (NOT including the executable
// name — the harness prepends cmd.Name, "triage" for this tree's
// NewRoot). Stdin is empty; stdout and stderr are captured.
func Run(t *testing.T, cmd *cli.Command, argv ...string) Result {
	t.Helper()
	return clitest.Run(t, cmd, argv...)
}

// RunWithStdin is like Run but pipes stdin as standard input.
func RunWithStdin(t *testing.T, cmd *cli.Command, stdin string, argv ...string) Result {
	t.Helper()
	return clitest.RunWithStdin(t, cmd, stdin, argv...)
}

// The Assert* vocabulary delegates to pkg/clitest unchanged.

func AssertNoError(t *testing.T, r Result) { t.Helper(); clitest.AssertNoError(t, r) }
func AssertError(t *testing.T, r Result)   { t.Helper(); clitest.AssertError(t, r) }
func AssertExitCode(t *testing.T, r Result, want int) {
	t.Helper()
	clitest.AssertExitCode(t, r, want)
}
func AssertStdoutContains(t *testing.T, r Result, substr string) {
	t.Helper()
	clitest.AssertStdoutContains(t, r, substr)
}
func AssertStdoutMatches(t *testing.T, r Result, pattern string) {
	t.Helper()
	clitest.AssertStdoutMatches(t, r, pattern)
}
func AssertStdoutEquals(t *testing.T, r Result, want string) {
	t.Helper()
	clitest.AssertStdoutEquals(t, r, want)
}
func AssertStderrEmpty(t *testing.T, r Result) { t.Helper(); clitest.AssertStderrEmpty(t, r) }
func AssertStderrContains(t *testing.T, r Result, substr string) {
	t.Helper()
	clitest.AssertStderrContains(t, r, substr)
}
func AssertErrorFooter(t *testing.T, r Result, wantCode string, wantExitCode int) {
	t.Helper()
	clitest.AssertErrorFooter(t, r, wantCode, wantExitCode)
}
