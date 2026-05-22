// Package scope resolves the operating scope for every triage verb
// (except `tai forget --repo`, which carries its own identity). The
// rule is precedence-based: explicit `--pr` flag wins, then explicit
// `--branch`, otherwise the current git branch auto-detects to a PR
// row (via `prs.head_branch`) or a `branches` row.
//
// See openspec/changes/add-triage-state/specs/triage/spec.md
// (Requirement: Scope resolution) for the normative contract.
package scope

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/repoctx"
	"github.com/dmastrorillo/tai/plugins/triage/internal/storage"
)

// Kind discriminates a Scope: a PR row or a branch row.
type Kind string

const (
	KindPR     Kind = "pr"
	KindBranch Kind = "branch"
)

// Scope is a fully-resolved scope: the repo row, the kind, the target
// row id, and presentation-friendly fields the verbs need to render
// headers without re-querying.
type Scope struct {
	Kind       Kind
	RepoID     int64
	OwnerName  string // <owner>/<name>
	TargetID   int64  // prs.id or branches.id
	PRNumber   int    // 0 for branch scope
	Title      string // PR title (empty for branch)
	HeadBranch string // PR head branch (empty for branch)
	BranchName string // branch name (empty for PR)
}

// TargetLabel returns the human-readable label used in headers:
// `PR #<n>` for PR scope, `branch <name>` for branch scope. Used by
// `tai list`, `tai show`, `tai status`.
func (s Scope) TargetLabel() string {
	if s.Kind == KindPR {
		return fmt.Sprintf("PR #%d", s.PRNumber)
	}
	return fmt.Sprintf("branch %s", s.BranchName)
}

// LongLabel is `tai status`'s richer scope line (includes PR title and
// head_branch for PR scope).
func (s Scope) LongLabel() string {
	if s.Kind == KindPR {
		return fmt.Sprintf("PR #%d — %s (branch: %s)", s.PRNumber, s.Title, s.HeadBranch)
	}
	return fmt.Sprintf("branch %s", s.BranchName)
}

// Flags carries the per-invocation overrides — exactly one of PR /
// Branch may be non-zero (mutex is enforced by Resolve).
type Flags struct {
	PR     int    // 0 → unset
	Branch string // "" → unset
}

// Resolve runs the precedence rule. The repo is resolved through
// repoctx.Resolve(ctx, repoFlag), where repoFlag is the foundation's
// global --repo override (empty means auto-detect).
func Resolve(ctx context.Context, db *storage.DB, repoFlag string, flags Flags) (Scope, error) {
	if flags.PR > 0 && flags.Branch != "" {
		return Scope{}, errcode.New(errcode.TriageInvalidFlags,
			"--pr and --branch are mutually exclusive").
			WithHelp("pass only one of --pr <number> or --branch <name>")
	}

	id, err := repoctx.Resolve(ctx, repoFlag)
	if err != nil {
		return Scope{}, err
	}

	repoID, err := lookupRepoID(ctx, db, id.String())
	if err != nil {
		return Scope{}, err
	}

	switch {
	case flags.PR > 0:
		return resolvePR(ctx, db, repoID, id.String(), flags.PR)
	case flags.Branch != "":
		return resolveBranch(ctx, db, repoID, id.String(), flags.Branch)
	default:
		return autoDetect(ctx, db, repoID, id.String())
	}
}

func lookupRepoID(ctx context.Context, db *storage.DB, ownerName string) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx,
		`SELECT id FROM repos WHERE owner_name = ?`, ownerName).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errcode.Newf(errcode.TriageNotFound,
			"no triage data for repo %q (nothing has been imported yet)", ownerName).
			WithHelp("run `tai import -` to populate the database, or check the repo identity")
	}
	if err != nil {
		return 0, errcode.Wrap(errcode.InternalError, err, "look up repo id")
	}
	return id, nil
}

func resolvePR(ctx context.Context, db *storage.DB, repoID int64, ownerName string, number int) (Scope, error) {
	var s Scope
	s.Kind = KindPR
	s.RepoID = repoID
	s.OwnerName = ownerName
	s.PRNumber = number
	err := db.QueryRowContext(ctx,
		`SELECT id, title, head_branch FROM prs WHERE repo_id = ? AND number = ?`,
		repoID, number).Scan(&s.TargetID, &s.Title, &s.HeadBranch)
	if errors.Is(err, sql.ErrNoRows) {
		return Scope{}, errcode.Newf(errcode.TriageNotFound,
			"no PR #%d in %s", number, ownerName).
			WithHelp("check `tai status` for the PRs that have been imported")
	}
	if err != nil {
		return Scope{}, errcode.Wrap(errcode.InternalError, err, "look up pr")
	}
	return s, nil
}

func resolveBranch(ctx context.Context, db *storage.DB, repoID int64, ownerName, name string) (Scope, error) {
	var s Scope
	s.Kind = KindBranch
	s.RepoID = repoID
	s.OwnerName = ownerName
	s.BranchName = name
	err := db.QueryRowContext(ctx,
		`SELECT id FROM branches WHERE repo_id = ? AND name = ?`,
		repoID, name).Scan(&s.TargetID)
	if errors.Is(err, sql.ErrNoRows) {
		return Scope{}, errcode.Newf(errcode.TriageNotFound,
			"no branch %q in %s", name, ownerName).
			WithHelp("check `tai status` for the branches that have been imported")
	}
	if err != nil {
		return Scope{}, errcode.Wrap(errcode.InternalError, err, "look up branch")
	}
	return s, nil
}

// autoDetect reads the current git branch and looks for a matching
// `prs.head_branch` or `branches.name` row in the resolved repo.
func autoDetect(ctx context.Context, db *storage.DB, repoID int64, ownerName string) (Scope, error) {
	current, err := currentBranch(ctx)
	if err != nil {
		return Scope{}, errcode.Wrap(errcode.TriageNoScope, err,
			"cannot read the current git branch").
			WithHelp(
				"pass --pr <number> or --branch <name> to identify the scope explicitly",
				"or check out the branch you imported review comments against",
			)
	}
	if current == "" {
		return Scope{}, errcode.New(errcode.TriageNoScope,
			"not on a named branch (detached HEAD or empty repo)").
			WithHelp("pass --pr <number> or --branch <name> to identify the scope")
	}

	prID, prTitle, prHead, prFound, err := findPRByHead(ctx, db, repoID, current)
	if err != nil {
		return Scope{}, err
	}
	brID, brFound, err := findBranchByName(ctx, db, repoID, current)
	if err != nil {
		return Scope{}, err
	}

	switch {
	case prFound && brFound:
		return Scope{}, errcode.Newf(errcode.TriageAmbiguousScope,
			"current branch %q matches both a PR and a standalone branch in %s",
			current, ownerName).
			WithHelp(
				"pass --pr <number> to act on the PR scope",
				"or --branch "+current+" to act on the branch scope",
			)
	case prFound:
		return Scope{
			Kind: KindPR, RepoID: repoID, OwnerName: ownerName,
			TargetID: prID, PRNumber: detectPRNumber(ctx, db, prID),
			Title: prTitle, HeadBranch: prHead,
		}, nil
	case brFound:
		return Scope{
			Kind: KindBranch, RepoID: repoID, OwnerName: ownerName,
			TargetID: brID, BranchName: current,
		}, nil
	default:
		return Scope{}, errcode.Newf(errcode.TriageNoScope,
			"current branch %q matches no imported PR or branch in %s",
			current, ownerName).
			WithHelp(
				"pass --pr <number> or --branch <name> to identify the scope",
				"or run `tai status` to see what has been imported",
			)
	}
}

func detectPRNumber(ctx context.Context, db *storage.DB, prID int64) int {
	var n int
	_ = db.QueryRowContext(ctx, `SELECT number FROM prs WHERE id = ?`, prID).Scan(&n)
	return n
}

func findPRByHead(ctx context.Context, db *storage.DB, repoID int64, head string) (int64, string, string, bool, error) {
	var id int64
	var title, headBranch string
	err := db.QueryRowContext(ctx,
		`SELECT id, title, head_branch FROM prs WHERE repo_id = ? AND head_branch = ?`,
		repoID, head).Scan(&id, &title, &headBranch)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", "", false, nil
	}
	if err != nil {
		return 0, "", "", false, errcode.Wrap(errcode.InternalError, err, "look up pr by head_branch")
	}
	return id, title, headBranch, true, nil
}

func findBranchByName(ctx context.Context, db *storage.DB, repoID int64, name string) (int64, bool, error) {
	var id int64
	err := db.QueryRowContext(ctx,
		`SELECT id FROM branches WHERE repo_id = ? AND name = ?`,
		repoID, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, errcode.Wrap(errcode.InternalError, err, "look up branch by name")
	}
	return id, true, nil
}

// currentBranch returns the name of the currently checked-out branch,
// or empty string when in a detached HEAD state. We shell out to
// `git rev-parse --abbrev-ref HEAD`; if git is missing or this isn't a
// repo, return an error.
func currentBranch(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(out))
	// "HEAD" means detached. Treat as "no current branch" — the
	// auto-detect path falls back to a clean TRIAGE_NO_SCOPE.
	if name == "HEAD" {
		return "", nil
	}
	return name, nil
}
