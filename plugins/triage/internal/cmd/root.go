// Package cmd assembles the tai CLI's urfave/cli command tree.
//
// Subcommands land here as their OpenSpec changes are applied. The package
// is the single seam every test and the production binary uses to build
// the command — there is no global state, and the function is cheap to
// call repeatedly.
package cmd

import (
	"context"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/repoctx"
	"github.com/dmastrorillo/tai/plugins/triage/internal/version"
	"github.com/urfave/cli/v3"
)

// RepoFlag is the canonical name of the global `--repo` flag. Exported
// so subcommand wiring (and tests) can reference it without re-stating
// the literal.
const RepoFlag = "repo"

// RequireRepo resolves the repo identity for a subcommand action. It is
// the canonical entry point — subcommands that need repo context call
// it from their Action and either get a valid Identity back or surface
// the structured error to the user.
//
// The resolver applies the foundation's precedence: --repo flag value
// (validated via repoctx.ParseIdentity) > working-directory auto-detect
// via `git config --get remote.origin.url`.
//
// "Repo-independent" subcommands (e.g. tai install, tai --help,
// tai --version) are simply those whose Action does NOT call RequireRepo.
// The opt-in shape keeps the contract explicit at the call site rather
// than hidden in a registry: a reader of the subcommand's Action sees
// whether it needs repo context.
func RequireRepo(ctx context.Context, cmd *cli.Command) (repoctx.Identity, error) {
	return repoctx.Resolve(ctx, cmd.String(RepoFlag))
}

// NewRoot returns a freshly-assembled tai root command.
//
// Writer / ErrWriter / Reader default to os.Stdout/Stderr/Stdin on the
// returned value; tests in plugins/triage/internal/cmdtest swap them
// for buffers.
//
// Three hooks are wired here to make stderr single-source and to ensure
// unknown subcommands route through the foundation's error contract:
//
//   - Action: a root-level fallback that fires when no subcommand matched.
//     If positional arguments are present (i.e. `tai bogus`), returns
//     UnknownSubcommand. If none are present (`tai`), shows help.
//   - OnUsageError: converts urfave/cli's parser/usage failures into
//     *errcode.Error{Code: UnknownSubcommand} so the printer in
//     pkg/cliout owns the rendered template.
//   - ExitErrHandler: a no-op. urfave/cli's default HandleExitCoder
//     prints the error to its package-level ErrWriter and calls os.Exit;
//     we want neither. Setting a no-op handler lets the error flow back
//     to cliexec.Run, where main.go takes over.
func NewRoot() *cli.Command {
	return &cli.Command{
		Name:    "triage",
		Usage:   "Triage AI — store, walk, and verify code-review comments",
		Version: version.String,

		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  RepoFlag,
				Usage: "Override repo identity (format: <owner>/<name>)",
			},
		},

		Commands: []*cli.Command{
			newImportCommand(),
			newListCommand(),
			newShowCommand(),
			newAcceptCommand(),
			newDismissCommand(),
			newCompleteCommand(),
			newStatusCommand(),
			newForgetCommand(),
		},

		Action: func(_ context.Context, c *cli.Command) error {
			if args := c.Args(); args.Present() {
				return errcode.Newf(errcode.UnknownSubcommand,
					"unknown command: %q", args.First()).
					WithHelp("run `triage --help` to see available commands and flags")
			}
			// No positional and no subcommand → render help.
			return cli.ShowAppHelp(c)
		},

		OnUsageError: func(_ context.Context, _ *cli.Command, err error, _ bool) error {
			return errcode.Wrap(errcode.UnknownSubcommand, err, err.Error()).
				WithHelp("run `triage --help` to see available commands and flags")
		},

		ExitErrHandler: func(_ context.Context, _ *cli.Command, _ error) {
			// Intentionally empty. main.go owns stderr formatting and
			// exit-code mapping.
		},
	}
}
