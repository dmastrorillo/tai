package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/dmastrorillo/tai/plugins/triage/internal/triage"
	"github.com/urfave/cli/v3"
)

func newStatusCommand() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Print a compact summary of the resolved scope (counts, batches)",
		Flags: scopeFlags(),
		Action: func(ctx context.Context, c *cli.Command) error {
			return runStatus(ctx, c)
		},
	}
}

func runStatus(ctx context.Context, c *cli.Command) error {
	s, db, err := openDBAndScope(ctx, c)
	if err != nil {
		return err
	}
	defer db.Close()

	counts, err := triage.CountByStatus(ctx, db, s)
	if err != nil {
		return err
	}
	batches, err := triage.BatchSummaries(ctx, db, s)
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Repo: %s\n", s.OwnerName)
	fmt.Fprintf(&b, "Scope: %s\n\n", s.LongLabel())

	fmt.Fprintln(&b, "Counts:")
	fmt.Fprintf(&b, "  Total:      %d\n", counts.Total)
	if counts.Total > 0 {
		if counts.Pending > 0 {
			fmt.Fprintf(&b, "  Pending:    %d\n", counts.Pending)
		}
		if counts.Accepted > 0 {
			fmt.Fprintf(&b, "  Accepted:   %d\n", counts.Accepted)
		}
		if counts.Dismissed > 0 {
			fmt.Fprintf(&b, "  Dismissed:  %d\n", counts.Dismissed)
		}
		if counts.Completed > 0 {
			fmt.Fprintf(&b, "  Completed:  %d\n", counts.Completed)
		}
	}

	if len(batches) > 0 {
		fmt.Fprintf(&b, "\nBatches: %d\n", len(batches))
		for _, br := range batches {
			fmt.Fprintf(&b, "  %s (%d comments — %s) — %s\n",
				br.Key, br.Count, br.Status, br.Title)
		}
	}

	fmt.Fprintln(&b, "\n[exit 0]")
	_, _ = io.WriteString(c.Writer, b.String())
	return nil
}
