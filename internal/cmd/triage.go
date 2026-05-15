// triage.go holds the flag constants, shared scope-flag list, and
// joint DB-open + scope-resolve preamble used by every triage verb
// (list, show, accept, dismiss, complete, status, forget). The
// per-verb commands live in their own files in this package.

package cmd

import (
	"context"

	"github.com/danielmastrorillo/tai/internal/storage"
	"github.com/danielmastrorillo/tai/internal/triage/scope"
	"github.com/urfave/cli/v3"
)

// Triage-verb flag names. The two scope flags (--pr / --branch) are
// shared across every triage verb (except `tai forget --repo`), which
// is why they live on this file alongside the helpers.
const (
	prFlag         = "pr"
	branchFlag     = "branch"
	statusFlag     = "status"
	batchFlag      = "batch"
	resolutionFlag = "resolution"
	reasonFlag     = "reason"
	byFlag         = "by"
	allFlag        = "all"
	commentFlag    = "comment"
	yesFlag        = "yes"
)

// scopeFlags returns the standard --pr / --branch pair every triage
// verb wires onto its command.
func scopeFlags() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{Name: prFlag, Usage: "Override scope to a specific PR number"},
		&cli.StringFlag{Name: branchFlag, Usage: "Override scope to a specific branch name"},
	}
}

// openDBAndScope is the joint preamble every triage verb runs:
// resolve the scope (which itself resolves the repo), open the
// database, and run migrations. Returns the resolved scope and a
// close func the caller must defer.
func openDBAndScope(ctx context.Context, c *cli.Command) (scope.Scope, *storage.DB, error) {
	db, err := storage.Open(ctx)
	if err != nil {
		return scope.Scope{}, nil, err
	}
	flags := scope.Flags{
		PR:     int(c.Int(prFlag)),
		Branch: c.String(branchFlag),
	}
	s, err := scope.Resolve(ctx, db, c.String(RepoFlag), flags)
	if err != nil {
		_ = db.Close()
		return scope.Scope{}, nil, err
	}
	return s, db, nil
}
