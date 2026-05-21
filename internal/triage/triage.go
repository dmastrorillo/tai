package triage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dmastrorillo/tai/internal/errcode"
	"github.com/dmastrorillo/tai/internal/storage"
	"github.com/dmastrorillo/tai/internal/triage/scope"
)

// Comment is the projection of a `comments` row plus its computed
// per-target position and (optional) batch metadata. Used by `tai
// list`, `tai show`, and the mutation verbs that need to display
// what they touched.
type Comment struct {
	ID            int64
	Position      int
	Severity      string
	Category      string
	File          string
	Lines         string
	Source        string
	Title         string
	Description   string
	WhyFix        string
	SuggestedFix  string
	Consequences  string
	Status        string
	Resolution    sql.NullString
	DismissedBy   sql.NullString
	DismissReason sql.NullString
	BatchID       sql.NullInt64
	BatchKey      sql.NullString
	BatchTitle    sql.NullString
}

// listSQL is the canonical SELECT used for list/show. ROW_NUMBER over
// `id ASC` partitioned by parent gives the user-facing position; the
// LEFT JOIN onto batches provides the optional batch_key/title meta.
const listSQL = `
SELECT
  c.id,
  ROW_NUMBER() OVER (PARTITION BY %s ORDER BY c.id ASC) AS position,
  c.severity, c.category, c.file, c.lines, c.source,
  c.title, c.description, c.why_fix, c.suggested_fix, c.consequences,
  c.status, c.resolution, c.dismissed_by, c.dismiss_reason,
  c.batch_id, b.batch_key, b.title
FROM comments c
LEFT JOIN batches b ON c.batch_id = b.id
WHERE %s = ?
`

// ListComments returns every comment under the scope, ordered by
// position ascending. Pass statusFilter == nil (or empty) to disable
// the per-status filter.
func ListComments(ctx context.Context, db *storage.DB, s scope.Scope, statusFilter []string) ([]Comment, error) {
	q, args := buildListQuery(s, statusFilter)
	return runListQuery(ctx, db, q, args)
}

// LookupByPosition translates a user-facing per-target position to
// the comment's internal ID. Returns TRIAGE_NOT_FOUND when no comment
// at that position exists in the scope.
func LookupByPosition(ctx context.Context, db *storage.DB, s scope.Scope, position int) (int64, error) {
	if position <= 0 {
		return 0, errcode.Newf(errcode.TriageNotFound,
			"comment id %d is out of range (positions start at 1)", position)
	}
	q, args := buildListQuery(s, nil)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return 0, errcode.Wrap(errcode.InternalError, err, "look up comment by position")
	}
	defer rows.Close()
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return 0, err
		}
		if c.Position == position {
			return c.ID, nil
		}
	}
	if err := rows.Err(); err != nil {
		return 0, errcode.Wrap(errcode.InternalError, err, "iterate comments")
	}
	return 0, errcode.Newf(errcode.TriageNotFound,
		"no comment with id %d in this scope", position)
}

// Show returns the projection for one specific comment, identified
// by its user-facing position.
func Show(ctx context.Context, db *storage.DB, s scope.Scope, position int) (Comment, int, error) {
	q, args := buildListQuery(s, nil)
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return Comment{}, 0, errcode.Wrap(errcode.InternalError, err, "look up comment for show")
	}
	defer rows.Close()
	var found Comment
	total := 0
	got := false
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return Comment{}, 0, err
		}
		total++
		if c.Position == position {
			found = c
			got = true
		}
	}
	if err := rows.Err(); err != nil {
		return Comment{}, 0, errcode.Wrap(errcode.InternalError, err, "iterate comments")
	}
	if !got {
		return Comment{}, 0, errcode.Newf(errcode.TriageNotFound,
			"no comment with id %d in this scope", position)
	}
	return found, total, nil
}

// buildListQuery returns the SELECT and the args for the scope's
// comments.
func buildListQuery(s scope.Scope, statusFilter []string) (string, []any) {
	var parent string
	var args []any
	if s.Kind == scope.KindPR {
		parent = "c.pr_id"
		args = append(args, s.TargetID)
	} else {
		parent = "c.branch_id"
		args = append(args, s.TargetID)
	}
	q := fmt.Sprintf(listSQL, parent, parent)
	if len(statusFilter) > 0 {
		placeholders := ""
		for i, st := range statusFilter {
			if i > 0 {
				placeholders += ", "
			}
			placeholders += "?"
			args = append(args, st)
		}
		q += " AND c.status IN (" + placeholders + ")"
	}
	q += " ORDER BY position ASC"
	return q, args
}

func runListQuery(ctx context.Context, db *storage.DB, q string, args []any) ([]Comment, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "list comments")
	}
	defer rows.Close()
	var out []Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "iterate comments")
	}
	return out, nil
}

// rowScanner abstracts *sql.Rows so scanComment can be used uniformly.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanComment(rs rowScanner) (Comment, error) {
	var c Comment
	err := rs.Scan(
		&c.ID, &c.Position,
		&c.Severity, &c.Category, &c.File, &c.Lines, &c.Source,
		&c.Title, &c.Description, &c.WhyFix, &c.SuggestedFix, &c.Consequences,
		&c.Status, &c.Resolution, &c.DismissedBy, &c.DismissReason,
		&c.BatchID, &c.BatchKey, &c.BatchTitle,
	)
	if err != nil {
		return Comment{}, errcode.Wrap(errcode.InternalError, err, "scan comment row")
	}
	return c, nil
}

// LookupBatchID returns the internal batches.id for a (scope, batch
// key) pair, or TRIAGE_NOT_FOUND when no such batch exists in the
// scope.
func LookupBatchID(ctx context.Context, db *storage.DB, s scope.Scope, key string) (int64, error) {
	var id int64
	var err error
	if s.Kind == scope.KindPR {
		err = db.QueryRowContext(ctx,
			`SELECT id FROM batches WHERE pr_id = ? AND batch_key = ?`,
			s.TargetID, key).Scan(&id)
	} else {
		err = db.QueryRowContext(ctx,
			`SELECT id FROM batches WHERE branch_id = ? AND batch_key = ?`,
			s.TargetID, key).Scan(&id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errcode.Newf(errcode.TriageNotFound,
			"no batch %q in this scope", key)
	}
	if err != nil {
		return 0, errcode.Wrap(errcode.InternalError, err, "look up batch by key")
	}
	return id, nil
}

// MemberIDs returns every comment_id under the named batch, in
// position order.
func MemberIDs(ctx context.Context, db *storage.DB, batchID int64) ([]int64, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id FROM comments WHERE batch_id = ? ORDER BY id ASC`, batchID)
	if err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "list batch members")
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, errcode.Wrap(errcode.InternalError, err, "scan member id")
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "iterate batch members")
	}
	return out, nil
}

// Transition specifies a state mutation to apply via Apply().
type Transition struct {
	// Status is the new status: accepted / dismissed / completed.
	Status string
	// Resolution, if non-nil, overrides the comment's resolution. nil
	// means "leave existing value" (used by idempotent re-runs).
	Resolution *string
	// DismissReason and DismissedBy are written only for status
	// "dismissed". Both are required when Status is dismissed.
	DismissReason string
	DismissedBy   string
}

// Apply transitions every comment in commentIDs to t.Status, clearing
// the unrelated state fields per the spec's "fields follow the verb"
// rule (D4). The work runs inside tx (caller owns the transaction so
// downstream batch recompute can share it).
//
// Returns the number of rows whose status was actually changed (used
// by mutation verbs to report idempotent no-ops).
func Apply(ctx context.Context, tx *sql.Tx, commentIDs []int64, t Transition, now int64) (int, error) {
	changed := 0
	for _, id := range commentIDs {
		// Read current status for idempotency check.
		var cur string
		if err := tx.QueryRowContext(ctx,
			`SELECT status FROM comments WHERE id = ?`, id).Scan(&cur); err != nil {
			return 0, errcode.Wrap(errcode.InternalError, err, "read current status")
		}

		// Idempotent on same-status when no resolution/reason update.
		// Skipping the UPDATE here is safe wrt D4's "fields must tell
		// the truth about current state" rationale: every prior
		// transition into accepted/completed runs the full UPDATE that
		// clears dismissed_by/dismiss_reason; every transition into
		// dismissed clears resolution. The state machine therefore
		// makes stale-field-on-same-status structurally unreachable.
		if cur == t.Status && t.Resolution == nil && t.DismissReason == "" {
			continue
		}

		switch t.Status {
		case "accepted", "completed":
			if t.Resolution != nil {
				if _, err := tx.ExecContext(ctx,
					`UPDATE comments
					   SET status = ?, resolution = ?,
					       dismissed_by = NULL, dismiss_reason = NULL,
					       updated_at = ?
					   WHERE id = ?`,
					t.Status, *t.Resolution, now, id); err != nil {
					return 0, mapDBError(err, "update comment")
				}
			} else {
				if _, err := tx.ExecContext(ctx,
					`UPDATE comments
					   SET status = ?,
					       dismissed_by = NULL, dismiss_reason = NULL,
					       updated_at = ?
					   WHERE id = ?`,
					t.Status, now, id); err != nil {
					return 0, mapDBError(err, "update comment")
				}
			}
		case "dismissed":
			if _, err := tx.ExecContext(ctx,
				`UPDATE comments
				   SET status = 'dismissed',
				       dismiss_reason = ?, dismissed_by = ?,
				       resolution = NULL,
				       updated_at = ?
				   WHERE id = ?`,
				t.DismissReason, t.DismissedBy, now, id); err != nil {
				return 0, mapDBError(err, "update comment")
			}
		default:
			return 0, errcode.Newf(errcode.InternalError,
				"unsupported transition target status %q", t.Status)
		}
		changed++
	}
	return changed, nil
}

// RecomputeBatch reads every member's current status and updates the
// batches row to one of: pending / accepted / dismissed / completed /
// mixed. Returns the new status.
//
// Per spec: all-pending → pending; uniform terminal → that terminal;
// split across two-or-more distinct states (terminal or pending) →
// mixed.
func RecomputeBatch(ctx context.Context, tx *sql.Tx, batchID int64) (string, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT status FROM comments WHERE batch_id = ?`, batchID)
	if err != nil {
		return "", errcode.Wrap(errcode.InternalError, err, "read batch members")
	}
	defer rows.Close()
	seen := map[string]struct{}{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return "", errcode.Wrap(errcode.InternalError, err, "scan member status")
		}
		seen[s] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return "", errcode.Wrap(errcode.InternalError, err, "iterate member statuses")
	}

	var newStatus string
	switch len(seen) {
	case 0:
		// No members left (e.g. all deleted). Leave the batch at its
		// existing status — no useful information about the new state.
		var cur string
		_ = tx.QueryRowContext(ctx, `SELECT status FROM batches WHERE id = ?`, batchID).Scan(&cur)
		return cur, nil
	case 1:
		for s := range seen {
			newStatus = s
		}
	default:
		newStatus = "mixed"
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE batches SET status = ? WHERE id = ?`, newStatus, batchID); err != nil {
		return "", mapDBError(err, "update batch status")
	}
	return newStatus, nil
}

// BatchesAffected returns every batch_id covered by the given
// comment_ids (excluding NULL batch_id). Used by mutation verbs that
// touch comments and need to recompute each affected batch.
func BatchesAffected(ctx context.Context, tx *sql.Tx, commentIDs []int64) ([]int64, error) {
	if len(commentIDs) == 0 {
		return nil, nil
	}
	q := "SELECT DISTINCT batch_id FROM comments WHERE batch_id IS NOT NULL AND id IN ("
	args := make([]any, 0, len(commentIDs))
	for i, id := range commentIDs {
		if i > 0 {
			q += ","
		}
		q += "?"
		args = append(args, id)
	}
	q += ")"
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err, "look up affected batches")
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, errcode.Wrap(errcode.InternalError, err, "scan affected batch")
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// mapDBError mirrors importer.mapDBError: constraint violations →
// DBConstraintViolation, anything else → INTERNAL_ERROR.
func mapDBError(err error, ctx string) error {
	wrapped := storage.ErrConstraint(err)
	if _, ok := errcode.As(wrapped); ok {
		return wrapped
	}
	return errcode.Wrap(errcode.InternalError, err, ctx)
}

// validStatuses is the set of comment statuses accept/dismiss/
// complete/list/forget understand. Used by ValidateStatuses for
// --status flag validation. Unexported to keep the canonical set
// owned by this package — callers should validate via the function,
// not by reading the map.
var validStatuses = map[string]struct{}{
	"pending":   {},
	"accepted":  {},
	"dismissed": {},
	"completed": {},
}

// ValidateStatuses returns TriageInvalidFlags if any value isn't a
// recognised status.
func ValidateStatuses(values []string) error {
	for _, v := range values {
		if _, ok := validStatuses[v]; !ok {
			return errcode.Newf(errcode.TriageInvalidFlags,
				"--status %q is not one of (pending, accepted, dismissed, completed)", v).
				WithHelp("pass --status with one of: pending, accepted, dismissed, completed")
		}
	}
	return nil
}
