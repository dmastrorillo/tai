package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/danielmastrorillo/tai/internal/errcode"
	"github.com/danielmastrorillo/tai/internal/triage"
	"github.com/urfave/cli/v3"
)

// runTransition implements `tai accept`, `tai dismiss`, `tai complete`.
// status is the target status string (`accepted`/`dismissed`/`completed`).
//
// Selection: exactly one of <id> (positional) or --batch <key> MUST be
// present. Mutating both is TRIAGE_INVALID_FLAGS.
//
// For dismiss, --reason is REQUIRED. The dismissed-by attribution
// resolves to (--by | git user.name | $USER | "unknown") in that
// order.
//
// After the mutation, any affected batches have their status
// recomputed so the spec's "mixed" semantics hold.
func runTransition(ctx context.Context, c *cli.Command, target string) error {
	args := c.Args().Slice()
	batch := c.String(batchFlag)
	hasID := len(args) > 0
	hasBatch := batch != ""

	if !hasID && !hasBatch {
		return errcode.Newf(errcode.TriageInvalidFlags,
			"tai %s requires a comment id or --batch <key>", c.Name)
	}
	if hasID && hasBatch {
		return errcode.New(errcode.TriageInvalidFlags,
			"<id> and --batch are mutually exclusive").
			WithHelp("pass either a comment id or --batch <key>, not both")
	}
	if len(args) > 1 {
		return errcode.Newf(errcode.TriageInvalidFlags,
			"tai %s takes at most one comment id, got %d", c.Name, len(args))
	}

	// Per-verb required-flag validation.
	resolutionPtr := optionalString(c, resolutionFlag)
	var dismissReason, dismissedBy string
	if target == "dismissed" {
		dismissReason = strings.TrimSpace(c.String(reasonFlag))
		if dismissReason == "" {
			return errcode.New(errcode.TriageInvalidFlags,
				"--reason is required for `tai dismiss`").
				WithHelp("pass --reason \"<short explanation>\"")
		}
		dismissedBy = resolveDismissBy(ctx, c.String(byFlag))
	}

	s, db, err := openDBAndScope(ctx, c)
	if err != nil {
		return err
	}
	defer db.Close()

	// Resolve the target comment IDs BEFORE opening the write
	// transaction. The DB pool is sized to 1 (per storage.OpenAt) so
	// running a read against the parent DB while a tx is held would
	// deadlock waiting for the single connection.
	var ids []int64
	var label string
	if hasID {
		pos, err := strconv.Atoi(args[0])
		if err != nil || pos <= 0 {
			return errcode.Newf(errcode.TriageInvalidFlags,
				"comment id %q is not a positive integer", args[0])
		}
		id, err := triage.LookupByPosition(ctx, db, s, pos)
		if err != nil {
			return err
		}
		ids = []int64{id}
		label = fmt.Sprintf("comment %d", pos)
	} else {
		batchID, err := triage.LookupBatchID(ctx, db, s, batch)
		if err != nil {
			return err
		}
		members, err := triage.MemberIDs(ctx, db, batchID)
		if err != nil {
			return err
		}
		if len(members) == 0 {
			return errcode.Newf(errcode.TriageNotFound,
				"batch %q has no members", batch)
		}
		ids = members
		label = fmt.Sprintf("batch %s (%d members)", batch, len(members))
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errcode.Wrap(errcode.InternalError, err, "begin transaction")
	}
	defer func() { _ = tx.Rollback() }()

	t := triage.Transition{
		Status:        target,
		Resolution:    resolutionPtr,
		DismissReason: dismissReason,
		DismissedBy:   dismissedBy,
	}
	now := time.Now().Unix()
	changed, err := triage.Apply(ctx, tx, ids, t, now)
	if err != nil {
		return err
	}

	// Recompute affected batches inside the same transaction.
	affected, err := triage.BatchesAffected(ctx, tx, ids)
	if err != nil {
		return err
	}
	for _, bid := range affected {
		if _, err := triage.RecomputeBatch(ctx, tx, bid); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return errcode.Wrap(errcode.InternalError, err, "commit transition")
	}

	verb := pastTense(target)
	fmt.Fprintf(c.Writer, "%s %s (%d row(s) changed)\n[exit 0]\n", verb, label, changed)
	return nil
}

func pastTense(status string) string {
	switch status {
	case "accepted":
		return "Accepted"
	case "dismissed":
		return "Dismissed"
	case "completed":
		return "Completed"
	}
	return status
}

func optionalString(c *cli.Command, name string) *string {
	if !c.IsSet(name) {
		return nil
	}
	v := c.String(name)
	return &v
}

// resolveDismissBy implements the spec's attribution fallback:
// --by > `git config --get user.name` > $USER > "unknown".
func resolveDismissBy(ctx context.Context, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if name, err := gitUserName(ctx); err == nil && name != "" {
		return name
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "unknown"
}

func gitUserName(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "config", "--get", "user.name").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
