// `tai repo` verb tree. Phase 2 wires only `tai repo init`; the verb
// stays a coherent home for future repo-management subcommands (e.g.
// `tai repo doctor`, `tai repo lint`) without re-architecting the
// surface.

package cmd

import (
	"context"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/dmastrorillo/tai/core/internal/repoinit"
)

func newRepoCommand() *cli.Command {
	return &cli.Command{
		Name:  "repo",
		Usage: "Manage tai source repos (scaffold, lint, ...)",

		Commands: []*cli.Command{
			newRepoInitCommand(),
		},

		Action: func(_ context.Context, c *cli.Command) error {
			return cli.ShowSubcommandHelp(c)
		},
	}
}

func newRepoInitCommand() *cli.Command {
	return &cli.Command{
		Name:      "init",
		Usage:     "Scaffold a new tai source repo at <path>",
		ArgsUsage: "<path>",
		Action:    runRepoInit,
	}
}

func runRepoInit(ctx context.Context, c *cli.Command) error {
	dst, err := requireOneArg(c, "tai repo init", "<path>",
		"example: `tai repo init ./my-tai-source-repo`")
	if err != nil {
		return err
	}

	if err := repoinit.Scaffold(ctx, dst); err != nil {
		return err
	}

	// Emit the next-steps block to stdout in a single Write — this IS
	// the data product of `repo init` (the user asked "what do I do
	// now?"), so stdout per stdout-discipline. NextStepsBlock returns
	// a trailing newline so the terminal stays readable when the next
	// prompt lands.
	_, _ = io.WriteString(c.Writer, repoinit.NextStepsBlock(dst)+"\n")
	return nil
}
