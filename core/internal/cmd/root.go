// Package cmd assembles the tai core CLI's urfave/cli command tree.
//
// In Phase 0 of the AI-as-code pivot this is a thin scaffold: the root
// command carries --version and --help, and the OnUsageError/Action
// hooks route any unrecognised input through the foundation's
// UnknownSubcommand error contract. The new top-level verbs (tai config,
// tai sync, tai repo init, ...) graft onto this root in Phase 1 as their
// OpenSpec proposals are applied.
//
// NewRoot is the single seam every test and the production binary use to
// build the command — there is no package-level state, and the function
// is cheap to call repeatedly.
package cmd

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/dmastrorillo/tai/core/internal/version"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// NewRoot returns a freshly-assembled tai core root command.
//
// Writer / ErrWriter / Reader default to os.Stdout/Stderr/Stdin on the
// returned value; tests swap them for buffers via the pkg/cliexec.Run
// wrapper.
//
// Three hooks make stderr single-source and route unknown subcommands
// through the foundation's error contract:
//
//   - Action: a root-level fallback that fires when no subcommand
//     matched. If positional arguments are present (i.e. `tai bogus`),
//     returns UnknownSubcommand. If none are present (`tai`), shows help.
//   - OnUsageError: converts urfave/cli's parser/usage failures into
//     *errcode.Error{Code: UnknownSubcommand} so the printer in
//     pkg/cliout owns the rendered template.
//   - ExitErrHandler: a no-op. urfave/cli's default HandleExitCoder
//     prints the error to its package-level ErrWriter and calls os.Exit;
//     we want neither. Setting a no-op handler lets the error flow back
//     to cliexec.Run, where core/cmd/tai/main.go takes over.
func NewRoot() *cli.Command {
	return &cli.Command{
		Name:    "tai",
		Usage:   "AI-as-code CLI: distribute Claude Code assets and run AI plugins",
		Version: version.String,

		Action: func(_ context.Context, c *cli.Command) error {
			if args := c.Args(); args.Present() {
				return errcode.Newf(errcode.UnknownSubcommand,
					"unknown command: %q", args.First()).
					WithHelp("run `tai --help` to see available commands and flags")
			}
			return cli.ShowAppHelp(c)
		},

		OnUsageError: func(_ context.Context, _ *cli.Command, err error, _ bool) error {
			return errcode.Wrap(errcode.UnknownSubcommand, err, err.Error()).
				WithHelp("run `tai --help` to see available commands and flags")
		},

		ExitErrHandler: func(_ context.Context, _ *cli.Command, _ error) {
			// Intentionally empty. core/cmd/tai/main.go owns stderr
			// formatting and exit-code mapping.
		},
	}
}
