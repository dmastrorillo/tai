package triage

import (
	"context"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/storage"
	"github.com/dmastrorillo/tai/plugins/triage/internal/triage/scope"
)

// StatusCounts is the per-status comment tally `tai status` renders
// for a scope.
type StatusCounts struct {
	Total, Pending, Accepted, Dismissed, Completed int
}

// CountByStatus tallies the scope's comments by status.
func CountByStatus(ctx context.Context, db *storage.DB, s scope.Scope) (StatusCounts, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM comments WHERE `+s.ParentColumn()+` = ? GROUP BY status`,
		s.TargetID)
	if err != nil {
		return StatusCounts{}, errcode.Wrap(errcode.InternalError, err, "count comments by status")
	}
	defer func() { _ = rows.Close() }()
	var r StatusCounts
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return StatusCounts{}, errcode.Wrap(errcode.InternalError, err, "scan status counts")
		}
		r.Total += n
		switch st {
		case "pending":
			r.Pending = n
		case "accepted":
			r.Accepted = n
		case "dismissed":
			r.Dismissed = n
		case "completed":
			r.Completed = n
		}
	}
	return r, rows.Err()
}

// BatchSummary is one batch row in `tai status`'s output: the batch
// key, title, status, and member-comment count.
type BatchSummary struct {
	Key, Title, Status string
	Count              int
}

// BatchSummaries lists the scope's batches with member counts,
// ordered by batch key.
func BatchSummaries(ctx context.Context, db *storage.DB, s scope.Scope) ([]BatchSummary, error) {
	q := `SELECT b.batch_key, b.title, b.status, COUNT(c.id)
	      FROM batches b
	      LEFT JOIN comments c ON c.batch_id = b.id
	      WHERE b.` + s.ParentColumn() + ` = ?
	      GROUP BY b.id, b.batch_key, b.title, b.status
	      ORDER BY b.batch_key ASC`
	rows, err := db.QueryContext(ctx, q, s.TargetID)
	if err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "list batches")
	}
	defer func() { _ = rows.Close() }()
	var out []BatchSummary
	for rows.Next() {
		var br BatchSummary
		if err := rows.Scan(&br.Key, &br.Title, &br.Status, &br.Count); err != nil {
			return nil, errcode.Wrap(errcode.InternalError, err, "scan batch row")
		}
		out = append(out, br)
	}
	return out, rows.Err()
}
