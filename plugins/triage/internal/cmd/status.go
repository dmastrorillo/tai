package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/storage"
	"github.com/dmastrorillo/tai/plugins/triage/internal/triage/scope"
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

	counts, err := statusCounts(ctx, db, s)
	if err != nil {
		return err
	}
	batches, err := statusBatches(ctx, db, s)
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Repo: %s\n", s.OwnerName)
	fmt.Fprintf(&b, "Scope: %s\n\n", s.LongLabel())

	fmt.Fprintln(&b, "Counts:")
	fmt.Fprintf(&b, "  Total:      %d\n", counts.total)
	if counts.total > 0 {
		if counts.pending > 0 {
			fmt.Fprintf(&b, "  Pending:    %d\n", counts.pending)
		}
		if counts.accepted > 0 {
			fmt.Fprintf(&b, "  Accepted:   %d\n", counts.accepted)
		}
		if counts.dismissed > 0 {
			fmt.Fprintf(&b, "  Dismissed:  %d\n", counts.dismissed)
		}
		if counts.completed > 0 {
			fmt.Fprintf(&b, "  Completed:  %d\n", counts.completed)
		}
	}

	if len(batches) > 0 {
		fmt.Fprintf(&b, "\nBatches: %d\n", len(batches))
		for _, br := range batches {
			fmt.Fprintf(&b, "  %s (%d comments — %s) — %s\n",
				br.key, br.count, br.status, br.title)
		}
	}

	fmt.Fprintln(&b, "\n[exit 0]")
	_, _ = io.WriteString(c.Writer, b.String())
	return nil
}

type statusCountsRow struct {
	total, pending, accepted, dismissed, completed int
}

func statusCounts(ctx context.Context, db *storage.DB, s scope.Scope) (statusCountsRow, error) {
	col := "pr_id"
	if s.Kind == scope.KindBranch {
		col = "branch_id"
	}
	rows, err := db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM comments WHERE `+col+` = ? GROUP BY status`,
		s.TargetID)
	if err != nil {
		return statusCountsRow{}, errcode.Wrap(errcode.InternalError, err, "count comments by status")
	}
	defer rows.Close()
	var r statusCountsRow
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return statusCountsRow{}, errcode.Wrap(errcode.InternalError, err, "scan status counts")
		}
		r.total += n
		switch st {
		case "pending":
			r.pending = n
		case "accepted":
			r.accepted = n
		case "dismissed":
			r.dismissed = n
		case "completed":
			r.completed = n
		}
	}
	return r, rows.Err()
}

type batchRow struct {
	key, title, status string
	count              int
}

func statusBatches(ctx context.Context, db *storage.DB, s scope.Scope) ([]batchRow, error) {
	col := "pr_id"
	if s.Kind == scope.KindBranch {
		col = "branch_id"
	}
	q := `SELECT b.batch_key, b.title, b.status, COUNT(c.id)
	      FROM batches b
	      LEFT JOIN comments c ON c.batch_id = b.id
	      WHERE b.` + col + ` = ?
	      GROUP BY b.id, b.batch_key, b.title, b.status
	      ORDER BY b.batch_key ASC`
	rows, err := db.QueryContext(ctx, q, s.TargetID)
	if err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "list batches")
	}
	defer rows.Close()
	var out []batchRow
	for rows.Next() {
		var br batchRow
		if err := rows.Scan(&br.key, &br.title, &br.status, &br.count); err != nil {
			return nil, errcode.Wrap(errcode.InternalError, err, "scan batch row")
		}
		out = append(out, br)
	}
	return out, rows.Err()
}
