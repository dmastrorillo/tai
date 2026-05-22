package cmd

import (
	"context"

	"github.com/urfave/cli/v3"
)

func newAcceptCommand() *cli.Command {
	return &cli.Command{
		Name:  "accept",
		Usage: "Mark a comment (or every member of a batch) as accepted",
		Flags: append(scopeFlags(),
			&cli.StringFlag{Name: batchFlag, Usage: "Apply to every member of <batch_key>"},
			&cli.StringFlag{Name: resolutionFlag, Usage: "Record a resolution note"},
		),
		Action: func(ctx context.Context, c *cli.Command) error {
			return runTransition(ctx, c, "accepted")
		},
	}
}

func newCompleteCommand() *cli.Command {
	return &cli.Command{
		Name:  "complete",
		Usage: "Mark a comment (or batch) as completed (typically: already fixed in code)",
		Flags: append(scopeFlags(),
			&cli.StringFlag{Name: batchFlag, Usage: "Apply to every member of <batch_key>"},
			&cli.StringFlag{Name: resolutionFlag, Usage: "Record a resolution note"},
		),
		Action: func(ctx context.Context, c *cli.Command) error {
			return runTransition(ctx, c, "completed")
		},
	}
}

func newDismissCommand() *cli.Command {
	return &cli.Command{
		Name:  "dismiss",
		Usage: "Mark a comment (or batch) as dismissed; --reason is required",
		Flags: append(scopeFlags(),
			&cli.StringFlag{Name: batchFlag, Usage: "Apply to every member of <batch_key>"},
			&cli.StringFlag{Name: reasonFlag, Usage: "Reason for dismissal (REQUIRED)"},
			&cli.StringFlag{Name: byFlag, Usage: "Attribute the dismissal to <name> (defaults to git user.name or $USER)"},
		),
		Action: func(ctx context.Context, c *cli.Command) error {
			return runTransition(ctx, c, "dismissed")
		},
	}
}
