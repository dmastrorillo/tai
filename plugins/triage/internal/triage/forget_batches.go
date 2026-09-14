// Batch maintenance for a status-filtered forget.
//
// Deleting comments by status leaves their batches behind. A batch the
// delete emptied is dead weight in `tai triage status`; one that kept
// members carries a status computed from comments that no longer
// exist. Both are this file's problem.
//
// Everything here works from one snapshot: the batches whose
// membership the delete actually reduces, taken before it runs. That
// set is what the consent summary counts and what the maintenance
// touches, so the two can never disagree — and a batch the prune never
// touched, including one that was already empty, is left alone.

package triage

import (
	"context"
	"database/sql"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/storage"
)

// querier is the read surface shared by *storage.DB and *sql.Tx, so
// the snapshot can be taken read-only at plan time and again inside
// the transaction at execute time.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// commentScope is the WHERE fragment naming the comments a prune
// deletes — an expression over the `comments` alias `c`, plus its
// arguments. The scope and repo selectors reach comments differently,
// one by a direct column and the other through prs/branches, so the
// helpers below take the fragment rather than duplicating themselves.
type commentScope struct {
	where string
	args  []any
}

func scopeComments(col string, targetID int64) commentScope {
	return commentScope{where: "c." + col + " = ?", args: []any{targetID}}
}

func repoComments(repoID int64) commentScope {
	return commentScope{
		where: `c.id IN (SELECT c2.id FROM comments c2
		          LEFT JOIN prs p ON c2.pr_id = p.id
		          LEFT JOIN branches b ON c2.branch_id = b.id
		          WHERE p.repo_id = ? OR b.repo_id = ?)`,
		args: []any{repoID, repoID},
	}
}

// batchesLosingMembers returns the batches that will lose at least one
// comment to this prune.
//
// Taken before the delete, this is the only set the prune is entitled
// to touch. A batch with no members already — which import creates,
// since it inserts every `batches[]` entry whether or not a comment
// references it — is absent from the set and therefore survives.
func batchesLosingMembers(ctx context.Context, q querier, own commentScope, statuses []string) ([]int64, error) {
	statusList, statusArgs := inPlaceholders(statuses)
	args := append(append([]any{}, own.args...), statusArgs...)
	rows, err := q.QueryContext(ctx,
		`SELECT DISTINCT c.batch_id FROM comments c
		  WHERE `+own.where+`
		    AND c.status IN (`+statusList+`)
		    AND c.batch_id IS NOT NULL`, args...)
	if err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "snapshot batches losing members")
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, errcode.Wrap(errcode.InternalError, err, "scan batch id")
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "read batches losing members")
	}
	return ids, nil
}

// countEmptiedBatches returns how many of the snapshotted batches the
// prune will leave with no members. The consent summary needs it
// before anything is deleted, because a batch the prune removes is
// part of what the user is agreeing to.
func countEmptiedBatches(ctx context.Context, db *storage.DB, own commentScope, statuses []string) (int, error) {
	affected, err := batchesLosingMembers(ctx, db, own, statuses)
	if err != nil {
		return 0, err
	}
	if len(affected) == 0 {
		return 0, nil
	}
	statusList, statusArgs := inPlaceholders(statuses)
	args := append(int64Args(affected), statusArgs...)
	q := `SELECT COUNT(*) FROM batches ba
	      WHERE ba.id IN (` + int64Placeholders(affected) + `)
	        AND NOT EXISTS (SELECT 1 FROM comments c
	                        WHERE c.batch_id = ba.id
	                          AND c.status NOT IN (` + statusList + `))`
	var n int
	if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, errcode.Wrap(errcode.InternalError, err, "count emptied batches")
	}
	return n, nil
}

// maintainBatches deletes the snapshotted batches the prune emptied and
// recomputes the status of those that kept members.
//
// `affected` MUST be the snapshot taken before the delete: afterwards
// there is no way to tell a batch this prune emptied from one that was
// empty all along, and sweeping both is the bug this design exists to
// prevent.
func maintainBatches(ctx context.Context, tx *sql.Tx, affected []int64) error {
	if len(affected) == 0 {
		return nil
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT ba.id, COUNT(c.id) FROM batches ba
		   LEFT JOIN comments c ON c.batch_id = ba.id
		  WHERE ba.id IN (`+int64Placeholders(affected)+`)
		  GROUP BY ba.id`, int64Args(affected)...)
	if err != nil {
		return errcode.Wrap(errcode.InternalError, err, "count surviving batch members")
	}
	var emptied, kept []int64
	for rows.Next() {
		var id int64
		var members int
		if err := rows.Scan(&id, &members); err != nil {
			_ = rows.Close()
			return errcode.Wrap(errcode.InternalError, err, "scan batch member count")
		}
		if members == 0 {
			emptied = append(emptied, id)
			continue
		}
		kept = append(kept, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return errcode.Wrap(errcode.InternalError, err, "read batch member counts")
	}
	_ = rows.Close()

	if len(emptied) > 0 {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM batches WHERE id IN (`+int64Placeholders(emptied)+`)`,
			int64Args(emptied)...); err != nil {
			return errcode.Wrap(errcode.InternalError, err, "delete emptied batches")
		}
	}
	for _, id := range kept {
		if _, err := RecomputeBatch(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

// int64Placeholders and int64Args render an id slice for an IN clause.
// Callers MUST check the slice is non-empty first — `IN ()` is a
// syntax error, not an empty match.

func int64Placeholders(ids []int64) string {
	return strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
}

func int64Args(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}
