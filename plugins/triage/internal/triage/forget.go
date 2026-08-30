package triage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/storage"
	"github.com/dmastrorillo/tai/plugins/triage/internal/triage/scope"
)

// ForgetPlan is one resolved forget selector: the counts the consent
// summary shows, a human-readable description of the target, and the
// delete to run once consent is granted. Planning (read-only counts)
// and execution (the transactional delete) are deliberately separate
// so the cmd layer can put its consent prompt between them.
type ForgetPlan struct {
	Description  string
	CommentCount int
	BatchCount   int
	RefCount     int

	exec func(ctx context.Context, tx *sql.Tx) error
}

// Execute runs the plan's delete inside the caller's transaction.
func (p *ForgetPlan) Execute(ctx context.Context, tx *sql.Tx) error {
	return p.exec(ctx, tx)
}

// PlanRepoForget handles `tai forget --repo <owner/name> [--status ...]`.
// With statuses, only matching comments across the repo are deleted
// and parent rows survive; without, the whole repo row is deleted and
// CASCADE picks up everything under it.
func PlanRepoForget(ctx context.Context, db *storage.DB, owner string, statuses []string) (*ForgetPlan, error) {
	var repoID int64
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM repos WHERE owner_name = ?`, owner).Scan(&repoID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errcode.Newf(errcode.TriageNotFound,
				"no triage data for repo %q", owner)
		}
		return nil, errcode.Wrap(errcode.InternalError, err, "look up repo")
	}

	if len(statuses) > 0 {
		n, refs, err := countCommentsRepo(ctx, db, repoID, statuses)
		if err != nil {
			return nil, err
		}
		return &ForgetPlan{
			Description:  fmt.Sprintf("%s comments matching status (%s)", owner, strings.Join(statuses, ", ")),
			CommentCount: n, RefCount: refs,
			exec: func(ctx context.Context, tx *sql.Tx) error {
				return deleteRepoComments(ctx, tx, repoID, statuses)
			},
		}, nil
	}

	nComments, nBatches, nRefs, err := countRepoCascade(ctx, db, repoID)
	if err != nil {
		return nil, err
	}
	return &ForgetPlan{
		Description:  owner,
		CommentCount: nComments, BatchCount: nBatches, RefCount: nRefs,
		exec: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DELETE FROM repos WHERE id = ?`, repoID)
			return err
		},
	}, nil
}

// PlanScopedForget handles a --pr / --branch selector on an
// already-resolved scope, with or without --status.
func PlanScopedForget(ctx context.Context, db *storage.DB, s scope.Scope, statuses []string) (*ForgetPlan, error) {
	col := s.ParentColumn()

	if len(statuses) > 0 {
		n, refs, err := countCommentsScope(ctx, db, col, s.TargetID, statuses)
		if err != nil {
			return nil, err
		}
		return &ForgetPlan{
			Description: fmt.Sprintf("%s comments matching status (%s)",
				s.OwnerName+" "+s.TargetLabel(), strings.Join(statuses, ", ")),
			CommentCount: n, RefCount: refs,
			exec: func(ctx context.Context, tx *sql.Tx) error {
				return deleteScopeComments(ctx, tx, col, s.TargetID, statuses)
			},
		}, nil
	}

	// Whole-target delete (CASCADE picks up comments + batches).
	var nComments, nBatches, nRefs int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comments WHERE `+col+` = ?`, s.TargetID).Scan(&nComments); err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "count scope comments")
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM batches WHERE `+col+` = ?`, s.TargetID).Scan(&nBatches); err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "count scope batches")
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comment_external_refs r
		 JOIN comments c ON r.comment_id = c.id
		 WHERE c.`+col+` = ?`, s.TargetID).Scan(&nRefs); err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "count scope refs")
	}
	parentTable := s.ParentTable()
	return &ForgetPlan{
		Description:  s.OwnerName + " " + s.TargetLabel(),
		CommentCount: nComments, BatchCount: nBatches, RefCount: nRefs,
		exec: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, parentTable), s.TargetID)
			return err
		},
	}, nil
}

// PlanCommentForget handles `--comment <position>` on an
// already-resolved scope.
func PlanCommentForget(ctx context.Context, db *storage.DB, s scope.Scope, pos int) (*ForgetPlan, error) {
	id, err := LookupByPosition(ctx, db, s, pos)
	if err != nil {
		return nil, err
	}
	var nRefs int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comment_external_refs WHERE comment_id = ?`, id).Scan(&nRefs); err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "count comment refs")
	}
	return &ForgetPlan{
		Description:  fmt.Sprintf("%s %s comment %d", s.OwnerName, s.TargetLabel(), pos),
		CommentCount: 1, RefCount: nRefs,
		exec: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DELETE FROM comments WHERE id = ?`, id)
			return err
		},
	}, nil
}

// PlanBatchForget handles `--batch <key>` on an already-resolved
// scope, with optional --status. With statuses, only matching member
// comments are deleted and the batch's status is recomputed; without,
// the batch row alone is deleted and members survive (cascade
// set-null per storage schema).
func PlanBatchForget(ctx context.Context, db *storage.DB, s scope.Scope, key string, statuses []string) (*ForgetPlan, error) {
	batchID, err := LookupBatchID(ctx, db, s, key)
	if err != nil {
		return nil, err
	}

	if len(statuses) > 0 {
		n, refs, err := countBatchComments(ctx, db, batchID, statuses)
		if err != nil {
			return nil, err
		}
		return &ForgetPlan{
			Description:  fmt.Sprintf("batch %s members matching status (%s)", key, strings.Join(statuses, ", ")),
			CommentCount: n, RefCount: refs,
			exec: func(ctx context.Context, tx *sql.Tx) error {
				if err := deleteBatchComments(ctx, tx, batchID, statuses); err != nil {
					return err
				}
				_, err := RecomputeBatch(ctx, tx, batchID)
				return err
			},
		}, nil
	}

	var nMembers int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comments WHERE batch_id = ?`, batchID).Scan(&nMembers); err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "count batch members")
	}
	return &ForgetPlan{
		Description: fmt.Sprintf("batch %s (%d member comments survive)", key, nMembers),
		BatchCount:  1,
		exec: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DELETE FROM batches WHERE id = ?`, batchID)
			return err
		},
	}, nil
}

// countRepoCascade returns the (#comments, #batches, #refs) a
// whole-repo delete will cascade over.
func countRepoCascade(ctx context.Context, db *storage.DB, repoID int64) (int, int, int, error) {
	var nComments, nBatches, nRefs int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comments c
		 LEFT JOIN prs p ON c.pr_id = p.id
		 LEFT JOIN branches b ON c.branch_id = b.id
		 WHERE p.repo_id = ? OR b.repo_id = ?`, repoID, repoID).Scan(&nComments); err != nil {
		return 0, 0, 0, errcode.Wrap(errcode.InternalError, err, "count repo comments")
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM batches ba
		 LEFT JOIN prs p ON ba.pr_id = p.id
		 LEFT JOIN branches b ON ba.branch_id = b.id
		 WHERE p.repo_id = ? OR b.repo_id = ?`, repoID, repoID).Scan(&nBatches); err != nil {
		return 0, 0, 0, errcode.Wrap(errcode.InternalError, err, "count repo batches")
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comment_external_refs r
		 JOIN comments c ON r.comment_id = c.id
		 LEFT JOIN prs p ON c.pr_id = p.id
		 LEFT JOIN branches b ON c.branch_id = b.id
		 WHERE p.repo_id = ? OR b.repo_id = ?`, repoID, repoID).Scan(&nRefs); err != nil {
		return 0, 0, 0, errcode.Wrap(errcode.InternalError, err, "count repo refs")
	}
	return nComments, nBatches, nRefs, nil
}

// countCommentsRepo / countCommentsScope / countBatchComments return
// (#comments, #refs) for the matching subset.

func countCommentsRepo(ctx context.Context, db *storage.DB, repoID int64, statuses []string) (int, int, error) {
	statusList, args := inPlaceholders(statuses)
	args = append([]any{repoID, repoID}, args...)
	q := `SELECT COUNT(*) FROM comments c
	      LEFT JOIN prs p ON c.pr_id = p.id
	      LEFT JOIN branches b ON c.branch_id = b.id
	      WHERE (p.repo_id = ? OR b.repo_id = ?) AND c.status IN (` + statusList + `)`
	var n int
	if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, 0, errcode.Wrap(errcode.InternalError, err, "count repo comments")
	}
	qr := `SELECT COUNT(*) FROM comment_external_refs r
	       JOIN comments c ON r.comment_id = c.id
	       LEFT JOIN prs p ON c.pr_id = p.id
	       LEFT JOIN branches b ON c.branch_id = b.id
	       WHERE (p.repo_id = ? OR b.repo_id = ?) AND c.status IN (` + statusList + `)`
	var refs int
	if err := db.QueryRowContext(ctx, qr, args...).Scan(&refs); err != nil {
		return 0, 0, errcode.Wrap(errcode.InternalError, err, "count repo refs")
	}
	return n, refs, nil
}

func deleteRepoComments(ctx context.Context, tx *sql.Tx, repoID int64, statuses []string) error {
	statusList, args := inPlaceholders(statuses)
	args = append([]any{repoID, repoID}, args...)
	_, err := tx.ExecContext(ctx,
		`DELETE FROM comments WHERE id IN (
		   SELECT c.id FROM comments c
		   LEFT JOIN prs p ON c.pr_id = p.id
		   LEFT JOIN branches b ON c.branch_id = b.id
		   WHERE (p.repo_id = ? OR b.repo_id = ?) AND c.status IN (`+statusList+`)
		 )`, args...)
	return err
}

func countCommentsScope(ctx context.Context, db *storage.DB, col string, targetID int64, statuses []string) (int, int, error) {
	statusList, args := inPlaceholders(statuses)
	args = append([]any{targetID}, args...)
	q := `SELECT COUNT(*) FROM comments WHERE ` + col + ` = ? AND status IN (` + statusList + `)`
	var n int
	if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, 0, errcode.Wrap(errcode.InternalError, err, "count scoped comments")
	}
	qr := `SELECT COUNT(*) FROM comment_external_refs r
	       JOIN comments c ON r.comment_id = c.id
	       WHERE c.` + col + ` = ? AND c.status IN (` + statusList + `)`
	var refs int
	if err := db.QueryRowContext(ctx, qr, args...).Scan(&refs); err != nil {
		return 0, 0, errcode.Wrap(errcode.InternalError, err, "count scoped refs")
	}
	return n, refs, nil
}

func deleteScopeComments(ctx context.Context, tx *sql.Tx, col string, targetID int64, statuses []string) error {
	statusList, args := inPlaceholders(statuses)
	args = append([]any{targetID}, args...)
	_, err := tx.ExecContext(ctx,
		`DELETE FROM comments WHERE `+col+` = ? AND status IN (`+statusList+`)`, args...)
	return err
}

func countBatchComments(ctx context.Context, db *storage.DB, batchID int64, statuses []string) (int, int, error) {
	statusList, args := inPlaceholders(statuses)
	args = append([]any{batchID}, args...)
	q := `SELECT COUNT(*) FROM comments WHERE batch_id = ? AND status IN (` + statusList + `)`
	var n int
	if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, 0, errcode.Wrap(errcode.InternalError, err, "count batch comments")
	}
	qr := `SELECT COUNT(*) FROM comment_external_refs r
	       JOIN comments c ON r.comment_id = c.id
	       WHERE c.batch_id = ? AND c.status IN (` + statusList + `)`
	var refs int
	if err := db.QueryRowContext(ctx, qr, args...).Scan(&refs); err != nil {
		return 0, 0, errcode.Wrap(errcode.InternalError, err, "count batch refs")
	}
	return n, refs, nil
}

func deleteBatchComments(ctx context.Context, tx *sql.Tx, batchID int64, statuses []string) error {
	statusList, args := inPlaceholders(statuses)
	args = append([]any{batchID}, args...)
	_, err := tx.ExecContext(ctx,
		`DELETE FROM comments WHERE batch_id = ? AND status IN (`+statusList+`)`, args...)
	return err
}

func inPlaceholders(values []string) (string, []any) {
	if len(values) == 0 {
		return "''", nil
	}
	parts := make([]string, len(values))
	args := make([]any, len(values))
	for i, v := range values {
		parts[i] = "?"
		args[i] = v
	}
	return strings.Join(parts, ","), args
}
