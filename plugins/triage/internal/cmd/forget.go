package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/repoctx"
	"github.com/dmastrorillo/tai/plugins/triage/internal/storage"
	"github.com/dmastrorillo/tai/plugins/triage/internal/triage"
	"github.com/dmastrorillo/tai/plugins/triage/internal/triage/scope"
	"github.com/urfave/cli/v3"
)

// forgetEnv is the env-var name that, when truthy, treats the
// destructive prompt as accepted. Scoped to the forget verb — it is
// the only destructive one.
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
// The cmd layer owns flag parsing, selector precedence, the consent
// prompt, and the transaction; the counting and delete SQL lives in
// internal/triage's PlanXxxForget functions, the same split the
// mutate verbs use.
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
	_, _ = io.WriteString(c.Writer, forgetSummary(plan))
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
	if err := plan.Execute(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return errcode.Wrap(errcode.InternalError, err, "commit forget")
	}

	_, _ = io.WriteString(c.Writer, "Done. [exit 0]\n")
	return nil
}

// forgetSummary renders the consent prompt for a plan. Presentation
// only — the counts come from the plan.
func forgetSummary(p *triage.ForgetPlan) string {
	var b strings.Builder
	b.WriteString("You're about to delete:\n")
	b.WriteString("  • " + p.Description + "\n")
	b.WriteString(fmt.Sprintf("  • %d comments\n", p.CommentCount))
	b.WriteString(fmt.Sprintf("  • %d batches\n", p.BatchCount))
	b.WriteString(fmt.Sprintf("  • %d external references\n", p.RefCount))
	b.WriteString("This cannot be undone. Continue? [y/N] ")
	return b.String()
}

// buildForgetPlan dispatches by selector + status modifier. Comment
// and batch selectors win precedence over --pr/--branch (which become
// scope overrides). Whole-repo only fires when no other selector is
// set.
func buildForgetPlan(ctx context.Context, c *cli.Command, db *storage.DB, statuses []string, wholeRepo bool) (*triage.ForgetPlan, error) {
	switch {
	case c.IsSet(commentFlag):
		pos, s, err := resolveCommentSelector(ctx, c, db)
		if err != nil {
			return nil, err
		}
		return triage.PlanCommentForget(ctx, db, s, pos)
	case c.IsSet(batchFlag):
		s, err := resolveOverrideScope(ctx, c, db)
		if err != nil {
			return nil, err
		}
		return triage.PlanBatchForget(ctx, db, s, c.String(batchFlag), statuses)
	case wholeRepo:
		owner := c.String(RepoFlag)
		// Repo mode does NOT require git context (it carries identity).
		if _, err := repoctx.ParseIdentity(owner); err != nil {
			return nil, err
		}
		return triage.PlanRepoForget(ctx, db, owner, statuses)
	case c.IsSet(prFlag):
		s, err := scope.Resolve(ctx, db, c.String(RepoFlag), scope.Flags{PR: int(c.Int(prFlag))})
		if err != nil {
			return nil, err
		}
		return triage.PlanScopedForget(ctx, db, s, statuses)
	case c.IsSet(branchFlag):
		s, err := scope.Resolve(ctx, db, c.String(RepoFlag), scope.Flags{Branch: c.String(branchFlag)})
		if err != nil {
			return nil, err
		}
		return triage.PlanScopedForget(ctx, db, s, statuses)
	}
	return nil, errcode.New(errcode.InternalError, "unreachable: no selector matched")
}

// resolveCommentSelector parses --comment's positive-integer position
// and resolves the (possibly --pr/--branch-overridden) scope it is
// looked up in.
func resolveCommentSelector(ctx context.Context, c *cli.Command, db *storage.DB) (int, scope.Scope, error) {
	posStr := c.String(commentFlag)
	pos, err := strconv.Atoi(posStr)
	if err != nil || pos <= 0 {
		return 0, scope.Scope{}, errcode.Newf(errcode.TriageInvalidFlags,
			"--comment %q is not a positive integer", posStr)
	}
	s, err := resolveOverrideScope(ctx, c, db)
	if err != nil {
		return 0, scope.Scope{}, err
	}
	return pos, s, nil
}

// resolveOverrideScope resolves scope for the scope-local selectors
// (--comment / --batch), where --pr/--branch act as overrides for the
// resolver rather than as selectors.
func resolveOverrideScope(ctx context.Context, c *cli.Command, db *storage.DB) (scope.Scope, error) {
	return scope.Resolve(ctx, db, c.String(RepoFlag), scope.Flags{
		PR:     int(c.Int(prFlag)),
		Branch: c.String(branchFlag),
	})
}

// consentGranted returns true when the user has signalled OK to
// proceed: --yes flag, truthy TAI_ACCEPT_DESTRUCTIVE env, OR
// interactive `y`/`Y` answer at the prompt. Non-interactive without
// either flag/env returns false.
func consentGranted(c *cli.Command) bool {
	if c.Bool(yesFlag) {
		return true
	}
	if isTruthyEnv(forgetEnv) {
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
