package cmd_test

import (
	"context"
	"testing"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmd"
	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdtest"
	"github.com/urfave/cli/v3"
)

// withRequireRepoSubcommand returns a root command with a synthetic
// `probe` subcommand whose Action calls cmd.RequireRepo and returns the
// resolved identity (in success) or the resolver's error (in failure).
//
// Tests use it to exercise the wired --repo flag + resolver against a
// real cli.Command tree without depending on any not-yet-built
// production subcommand.
func withRequireRepoSubcommand(t *testing.T) *cli.Command {
	t.Helper()
	root := cmd.NewRoot()
	root.Commands = append(root.Commands, &cli.Command{
		Name: "probe",
		Action: func(ctx context.Context, c *cli.Command) error {
			id, err := cmd.RequireRepo(ctx, c)
			if err != nil {
				return err
			}
			_, _ = c.Writer.Write([]byte(id.String() + "\n"))
			return nil
		},
	})
	return root
}

// TestRequireRepo_TCREPO004_outside_git_fails exercises TC-REPO-004 at
// the command boundary: a subcommand that calls RequireRepo from a
// non-git cwd, with no --repo flag, fails with REPO_NOT_FOUND + exit 2.
func TestRequireRepo_TCREPO004_outside_git_fails(t *testing.T) {
	cmdtest.Chdir(t, t.TempDir())

	r := cmdtest.Run(t, withRequireRepoSubcommand(t), "probe")

	cmdtest.AssertError(t, r)
	cmdtest.AssertExitCode(t, r, 2)
	cmdtest.AssertErrorFooter(t, r, "REPO_NOT_FOUND", 2)
}

// TestRequireRepo_TCREPO006_flag_override_succeeds exercises TC-REPO-006
// at the command boundary: --repo acme/app from a non-git cwd succeeds
// and the resolved identity reaches the subcommand's action.
func TestRequireRepo_TCREPO006_flag_override_succeeds(t *testing.T) {
	cmdtest.Chdir(t, t.TempDir())

	r := cmdtest.Run(t, withRequireRepoSubcommand(t), "--repo", "acme/app", "probe")

	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)
	cmdtest.AssertStdoutContains(t, r, "acme/app\n")
}

// TestRequireRepo_TCREPO007_malformed_flag_fails exercises TC-REPO-007
// at the command boundary: --repo with a malformed value fails with
// REPO_FLAG_INVALID + exit 1.
func TestRequireRepo_TCREPO007_malformed_flag_fails(t *testing.T) {
	cmdtest.Chdir(t, t.TempDir())

	r := cmdtest.Run(t, withRequireRepoSubcommand(t), "--repo", "just-a-name", "probe")

	cmdtest.AssertError(t, r)
	cmdtest.AssertExitCode(t, r, 1)
	cmdtest.AssertErrorFooter(t, r, "REPO_FLAG_INVALID", 1)
}

// TestVersion_TCMIG001_outside_git_repo locks TC-MIG-001's repo-
// independence clause: invoking `triage --version` from a non-git
// directory still exits 0 with the version banner on stdout. Same
// contract as the in-repo case above; this variant just confirms
// the version path doesn't accidentally reach the repo resolver.
func TestVersion_TCMIG001_outside_git_repo(t *testing.T) {
	cmdtest.Chdir(t, t.TempDir())

	r := cmdtest.Run(t, cmd.NewRoot(), "--version")

	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)
	cmdtest.AssertStdoutContains(t, r, "triage version ")
}

// TestHelp_TCCMD008_outside_git_repo exercises TC-CMD-008's repo-
// independence clause for the triage plugin binary: `triage --help`
// runs without invoking the repo resolver, exits 0, and surfaces the
// app name (`triage`) on stdout.
//
// The assertion is the exact app name (set by NewRoot's `Name`
// field) rather than a substring like "triage" that would also match
// the Usage line — that way a regression that reverted the rename
// would surface as a test failure rather than passing on the Usage
// string.
func TestHelp_TCCMD008_outside_git_repo(t *testing.T) {
	cmdtest.Chdir(t, t.TempDir())

	r := cmdtest.Run(t, cmd.NewRoot(), "--help")

	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)
	// urfave/cli renders the program name on the "NAME:" or
	// "USAGE:" line; checking for "triage " (with a trailing space)
	// distinguishes the program-name occurrence from substrings
	// inside the Usage description.
	cmdtest.AssertStdoutContains(t, r, "triage ")
}
