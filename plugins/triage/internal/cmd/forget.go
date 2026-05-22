package cmd

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/installer"
	"github.com/dmastrorillo/tai/plugins/triage/internal/repoctx"
	"github.com/dmastrorillo/tai/plugins/triage/internal/storage"
	"github.com/dmastrorillo/tai/plugins/triage/internal/triage"
	"github.com/dmastrorillo/tai/plugins/triage/internal/triage/scope"
	"github.com/urfave/cli/v3"
)

// forgetEnv is the env-var name that, when truthy, treats the
// destructive prompt as accepted. Mirrors installer.AcceptEnv but
// applies to the triage forget verb only.
const forgetEnv = "TAI_ACCEPT_DESTRUCTIVE"

func newForgetCommand() *cli.Command {
	return &cli.Command{
		Name:  "forget",
		Usage: "Delete a comment, batch, PR, branch, or whole repo (destructive)",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: prFlag, Usage: "Delete a single PR (selector)"},
			&cli.StringFlag{Name: branchFlag, Usage: "Delete a single branch (selector)"},
			&cli.StringFlag{Name: commentFlag, Usage: "Delete a single comment by position (selector)"},
			&cli.StringFlag{Name: batchFlag, Usage: "Delete a single batch (selector)"},
			&cli.StringSliceFlag{Name: statusFlag, Usage: "When combined with --pr/--branch/--repo/--batch, only delete matching comments"},
			&cli.BoolFlag{Name: yesFlag, Usage: "Skip the destructive confirmation prompt"},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return runForget(ctx, c)
		},
	}
}

// runForget implements the spec's selector / consent / delete flow.
//
// Selector counting:
//
//	The selector is the thing being deleted. Valid forms (one per
//	invocation): --comment <id>, --batch <key>, --pr <N>, --branch
//	<name>, or --repo <owner/name>. The first two (--comment, --batch)
//	are scope-local selectors: when they're set, --pr/--branch are
//	interpreted as scope overrides for the resolver, not as
//	additional selectors. --pr/--branch on their own are selectors.
//	--repo is the selector iff no other selector is set.
//
// Detecting whether --repo was passed: we check `c.String(RepoFlag) != ""`
// rather than `c.IsSet(RepoFlag)` because urfave/cli v3's `IsSet`
// semantics for a root-level flag accessed from a subcommand are
// version-fragile. `c.String` documentably walks the ancestor chain
// for inherited flag values, which is what we want.
func runForget(ctx context.Context, c *cli.Command) error {
	hasComment := c.IsSet(commentFlag)
	hasBatch := c.IsSet(batchFlag)
	hasPR := c.IsSet(prFlag)
	hasBranch := c.IsSet(branchFlag)
	hasRepo := c.String(RepoFlag) != ""

	// Count the primary selector. --pr/--branch are selectors ONLY
	// when --comment and --batch are absent.
	selectorCount := 0
	if hasComment {
		selectorCount++
	}
	if hasBatch {
		selectorCount++
	}
	if !hasComment && !hasBatch {
		if hasPR {
			selectorCount++
		}
		if hasBranch {
			selectorCount++
		}
	}
	wholeRepo := hasRepo && selectorCount == 0
	if wholeRepo {
		selectorCount++
	}

	if selectorCount == 0 {
		return errcode.New(errcode.TriageInvalidFlags,
			"tai forget requires exactly one selector").
			WithHelp("pass exactly one of --comment <id>, --batch <key>, --pr <number>, --branch <name>, or --repo <owner/name>")
	}
	if selectorCount > 1 {
		return errcode.New(errcode.TriageInvalidFlags,
			"tai forget accepts only one selector at a time").
			WithHelp("pass exactly one of --comment, --batch, --pr, --branch")
	}

	statuses := c.StringSlice(statusFlag)
	if err := triage.ValidateStatuses(statuses); err != nil {
		return err
	}
	if c.IsSet(commentFlag) && len(statuses) > 0 {
		return errcode.New(errcode.TriageInvalidFlags,
			"--status cannot be combined with --comment (a specific id is already precise)").
			WithHelp("drop --status, or switch to --pr/--branch/--repo/--batch")
	}

	db, err := storage.Open(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	plan, err := buildForgetPlan(ctx, c, db, statuses, wholeRepo)
	if err != nil {
		return err
	}

	// Print summary and gate on consent.
	_, _ = io.WriteString(c.Writer, plan.summary())
	if !consentGranted(c) {
		_, _ = io.WriteString(c.Writer, "Aborted (no consent).\n")
		return errcode.New(errcode.TriageConfirmationRequired,
			"destructive operation requires explicit consent").
			WithHelp(
				"pass --yes to skip the prompt",
				"or set TAI_ACCEPT_DESTRUCTIVE=1 in the environment",
				"or run interactively and answer y/Y at the prompt",
			)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errcode.Wrap(errcode.InternalError, err, "begin forget transaction")
	}
	defer func() { _ = tx.Rollback() }()
	if err := plan.execute(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return errcode.Wrap(errcode.InternalError, err, "commit forget")
	}

	_, _ = io.WriteString(c.Writer, "Done. [exit 0]\n")
	return nil
}

// forgetPlan is the resolved selector + cascade counts + executor.
type forgetPlan struct {
	description  string
	commentCount int
	batchCount   int
	refCount     int

	exec func(ctx context.Context, tx *sql.Tx) error
}

func (p *forgetPlan) summary() string {
	var b strings.Builder
	b.WriteString("You're about to delete:\n")
	b.WriteString("  • " + p.description + "\n")
	b.WriteString(fmt.Sprintf("  • %d comments\n", p.commentCount))
	b.WriteString(fmt.Sprintf("  • %d batches\n", p.batchCount))
	b.WriteString(fmt.Sprintf("  • %d external references\n", p.refCount))
	b.WriteString("This cannot be undone. Continue? [y/N] ")
	return b.String()
}

func (p *forgetPlan) execute(ctx context.Context, tx *sql.Tx) error {
	return p.exec(ctx, tx)
}

// buildForgetPlan dispatches by selector + status modifier. Comment
// and batch selectors win precedence over --pr/--branch (which become
// scope overrides). Whole-repo only fires when no other selector is
// set.
func buildForgetPlan(ctx context.Context, c *cli.Command, db *storage.DB, statuses []string, wholeRepo bool) (*forgetPlan, error) {
	switch {
	case c.IsSet(commentFlag):
		return planCommentForget(ctx, c, db)
	case c.IsSet(batchFlag):
		return planBatchForget(ctx, c, db, statuses)
	case wholeRepo:
		return planRepoForget(ctx, c, db, statuses)
	case c.IsSet(prFlag):
		return planScopedForget(ctx, c, db, statuses, true)
	case c.IsSet(branchFlag):
		return planScopedForget(ctx, c, db, statuses, false)
	}
	return nil, errcode.New(errcode.InternalError, "unreachable: no selector matched")
}

// planRepoForget handles `tai forget --repo <owner/name> [--status ...]`.
// Repo mode does NOT require git context (it carries identity).
func planRepoForget(ctx context.Context, c *cli.Command, db *storage.DB, statuses []string) (*forgetPlan, error) {
	owner := c.String(RepoFlag)
	if _, err := repoctx.ParseIdentity(owner); err != nil {
		return nil, err
	}
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
		// Prune comments only across the repo; preserve parent rows.
		n, refs, err := countCommentsRepo(ctx, db, repoID, statuses)
		if err != nil {
			return nil, err
		}
		return &forgetPlan{
			description:  fmt.Sprintf("%s comments matching status (%s)", owner, strings.Join(statuses, ", ")),
			commentCount: n, refCount: refs,
			exec: func(ctx context.Context, tx *sql.Tx) error {
				return deleteRepoComments(ctx, tx, repoID, statuses)
			},
		}, nil
	}

	// Whole-repo delete: rely on CASCADE for everything.
	var nComments, nBatches, nRefs int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comments c
		 LEFT JOIN prs p ON c.pr_id = p.id
		 LEFT JOIN branches b ON c.branch_id = b.id
		 WHERE p.repo_id = ? OR b.repo_id = ?`, repoID, repoID).Scan(&nComments)
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM batches ba
		 LEFT JOIN prs p ON ba.pr_id = p.id
		 LEFT JOIN branches b ON ba.branch_id = b.id
		 WHERE p.repo_id = ? OR b.repo_id = ?`, repoID, repoID).Scan(&nBatches)
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comment_external_refs r
		 JOIN comments c ON r.comment_id = c.id
		 LEFT JOIN prs p ON c.pr_id = p.id
		 LEFT JOIN branches b ON c.branch_id = b.id
		 WHERE p.repo_id = ? OR b.repo_id = ?`, repoID, repoID).Scan(&nRefs)
	return &forgetPlan{
		description:  owner,
		commentCount: nComments, batchCount: nBatches, refCount: nRefs,
		exec: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DELETE FROM repos WHERE id = ?`, repoID)
			return err
		},
	}, nil
}

// planScopedForget handles --pr / --branch with or without --status.
func planScopedForget(ctx context.Context, c *cli.Command, db *storage.DB, statuses []string, isPR bool) (*forgetPlan, error) {
	flags := scope.Flags{}
	if isPR {
		flags.PR = int(c.Int(prFlag))
	} else {
		flags.Branch = c.String(branchFlag)
	}
	s, err := scope.Resolve(ctx, db, c.String(RepoFlag), flags)
	if err != nil {
		return nil, err
	}

	parentCol := "pr_id"
	if !isPR {
		parentCol = "branch_id"
	}

	if len(statuses) > 0 {
		n, refs, err := countCommentsScope(ctx, db, parentCol, s.TargetID, statuses)
		if err != nil {
			return nil, err
		}
		return &forgetPlan{
			description: fmt.Sprintf("%s comments matching status (%s)",
				s.OwnerName+" "+s.TargetLabel(), strings.Join(statuses, ", ")),
			commentCount: n, refCount: refs,
			exec: func(ctx context.Context, tx *sql.Tx) error {
				return deleteScopeComments(ctx, tx, parentCol, s.TargetID, statuses)
			},
		}, nil
	}

	// Whole-target delete (CASCADE picks up comments + batches).
	var nComments, nBatches, nRefs int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comments WHERE `+parentCol+` = ?`, s.TargetID).Scan(&nComments)
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM batches WHERE `+parentCol+` = ?`, s.TargetID).Scan(&nBatches)
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comment_external_refs r
		 JOIN comments c ON r.comment_id = c.id
		 WHERE c.`+parentCol+` = ?`, s.TargetID).Scan(&nRefs)
	parentTable := "prs"
	if !isPR {
		parentTable = "branches"
	}
	return &forgetPlan{
		description:  s.OwnerName + " " + s.TargetLabel(),
		commentCount: nComments, batchCount: nBatches, refCount: nRefs,
		exec: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, parentTable), s.TargetID)
			return err
		},
	}, nil
}

// planCommentForget handles --comment <position>.
func planCommentForget(ctx context.Context, c *cli.Command, db *storage.DB) (*forgetPlan, error) {
	posStr := c.String(commentFlag)
	pos, err := strconv.Atoi(posStr)
	if err != nil || pos <= 0 {
		return nil, errcode.Newf(errcode.TriageInvalidFlags,
			"--comment %q is not a positive integer", posStr)
	}
	flags := scope.Flags{
		PR:     int(c.Int(prFlag)),
		Branch: c.String(branchFlag),
	}
	s, err := scope.Resolve(ctx, db, c.String(RepoFlag), flags)
	if err != nil {
		return nil, err
	}
	id, err := triage.LookupByPosition(ctx, db, s, pos)
	if err != nil {
		return nil, err
	}
	var nRefs int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comment_external_refs WHERE comment_id = ?`, id).Scan(&nRefs)
	return &forgetPlan{
		description:  fmt.Sprintf("%s %s comment %d", s.OwnerName, s.TargetLabel(), pos),
		commentCount: 1, refCount: nRefs,
		exec: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DELETE FROM comments WHERE id = ?`, id)
			return err
		},
	}, nil
}

// planBatchForget handles --batch <key> with optional --status.
func planBatchForget(ctx context.Context, c *cli.Command, db *storage.DB, statuses []string) (*forgetPlan, error) {
	flags := scope.Flags{
		PR:     int(c.Int(prFlag)),
		Branch: c.String(branchFlag),
	}
	s, err := scope.Resolve(ctx, db, c.String(RepoFlag), flags)
	if err != nil {
		return nil, err
	}
	key := c.String(batchFlag)
	batchID, err := triage.LookupBatchID(ctx, db, s, key)
	if err != nil {
		return nil, err
	}

	if len(statuses) > 0 {
		// Delete only matching member comments; preserve batch row,
		// recompute its status after.
		n, refs, err := countBatchComments(ctx, db, batchID, statuses)
		if err != nil {
			return nil, err
		}
		return &forgetPlan{
			description:  fmt.Sprintf("batch %s members matching status (%s)", key, strings.Join(statuses, ", ")),
			commentCount: n, refCount: refs,
			exec: func(ctx context.Context, tx *sql.Tx) error {
				if err := deleteBatchComments(ctx, tx, batchID, statuses); err != nil {
					return err
				}
				_, err := triage.RecomputeBatch(ctx, tx, batchID)
				return err
			},
		}, nil
	}

	// Delete the batch row only; member comments survive (cascade
	// set-null per storage schema).
	var nMembers int
	_ = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comments WHERE batch_id = ?`, batchID).Scan(&nMembers)
	return &forgetPlan{
		description: fmt.Sprintf("batch %s (%d member comments survive)", key, nMembers),
		batchCount:  1,
		exec: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DELETE FROM batches WHERE id = ?`, batchID)
			return err
		},
	}, nil
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

// consentGranted returns true when the user has signalled OK to
// proceed: --yes flag, truthy TAI_ACCEPT_DESTRUCTIVE env, OR
// interactive `y`/`Y` answer at the prompt. Non-interactive without
// either flag/env returns false.
func consentGranted(c *cli.Command) bool {
	if c.Bool(yesFlag) {
		return true
	}
	if installer.IsTruthyEnv(forgetEnv) {
		return true
	}
	if !stdinIsTTY(c.Reader) {
		return false
	}
	reader := bufio.NewReader(c.Reader)
	line, _ := reader.ReadString('\n')
	answer := strings.TrimSpace(line)
	return answer == "y" || answer == "Y"
}
