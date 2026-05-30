// Package sync owns the `tai sync` capability: cloning the configured
// source repo into <TAI_DATA_DIR>/source/, fetching updates, copying
// assets into configured targets with M1 existence-based overwrite
// detection, manifest tracking, and the --prune deletion path. The
// background update-poll goroutine consumed by the update-banner
// capability lives in poll.go in the same package.
//
// Normative spec:
// openspec/changes/pivot-to-ai-as-code/specs/repo-sync/spec.md.
package sync

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// CloneDir returns the absolute path to TAI's single source-repo clone.
// Spec: "The system SHALL maintain exactly one git clone of the
// configured source repo, in TAI's data directory at
// <TAI_DATA_DIR>/source/. The location is not configurable."
func CloneDir(dataDir string) string {
	return filepath.Join(dataDir, "source")
}

// EnsureClone clones the source repo on first call and reuses the
// existing clone on subsequent calls. The fetch step is separate (see
// Fetch) so callers can decide whether to ignore fetch failures and
// fall back to the cached state.
//
// Returns the clone directory on success. The error is an
// *errcode.Error{Code: REPO_FETCH_FAILED} when the initial `git clone`
// fails — there is no cache to fall back to on the very first sync.
func EnsureClone(ctx context.Context, dataDir, repoURL string) (string, error) {
	dst := CloneDir(dataDir)
	if existing, err := isGitClone(dst); err != nil {
		return "", err
	} else if existing {
		return dst, nil
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", errcode.Wrapf(errcode.InternalError, err,
			"create data directory %s", filepath.Dir(dst))
	}

	cmd := exec.CommandContext(ctx, "git", "clone", repoURL, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Tidy up: a partial clone shouldn't poison subsequent runs.
		_ = os.RemoveAll(dst)
		return "", errcode.Wrapf(errcode.RepoFetchFailed, err,
			"git clone of %s failed: %s", repoURL, trimSpace(string(out))).
			WithHelp(
				"check that the repo-url is reachable and your git credentials are configured",
				"`git clone "+repoURL+" <some-local-dir>` reproduces the call tai makes",
			)
	}
	return dst, nil
}

// Fetch runs `git fetch` followed by `git reset --hard
// origin/<default-branch>` so the workspace reflects the upstream tip.
// On failure (network, auth, etc.) Fetch returns the wrapped error
// without modifying the clone — callers decide whether to fall back
// to the cached state with a warning.
//
// Every returned error is `*errcode.Error{Code: REPO_FETCH_FAILED}`
// so the contract stays uniform across the fetch step and the reset
// step. Today Sync swallows the error and emits a stderr warning, but
// a future direct caller (e.g. a unit test on Fetch) can rely on the
// stable error type.
//
// "Last successful fetch" timestamps come from os.Stat on
// <clone>/.git/FETCH_HEAD (git updates that file's mtime on each
// successful fetch). LastFetchAttempt provides the lookup helper.
func Fetch(ctx context.Context, cloneDir string) error {
	if err := runGit(ctx, cloneDir, "fetch", "--prune"); err != nil {
		return errcode.Wrapf(errcode.RepoFetchFailed, err, "%s", err.Error()).
			WithHelp(
				"check network connectivity to the source repo's host",
				"verify your git credentials (SSH agent, credential helper, etc.)",
				"confirm the configured `repo-url` with `tai config show`",
			)
	}
	branch, err := defaultBranch(ctx, cloneDir)
	if err != nil {
		return errcode.Wrapf(errcode.RepoFetchFailed, err,
			"resolve default branch in %s", cloneDir)
	}
	if err := runGit(ctx, cloneDir, "reset", "--hard", "origin/"+branch); err != nil {
		return errcode.Wrapf(errcode.RepoFetchFailed, err,
			"git reset --hard origin/%s", branch)
	}
	return nil
}

// LastFetchSuccess returns the mtime of <clone>/.git/FETCH_HEAD. The
// zero time signals "no successful fetch on record" (FETCH_HEAD does
// not exist yet), which only happens between the initial clone and
// the first explicit fetch — the clone itself does not touch
// FETCH_HEAD, so we use the directory's own mtime as a fallback.
func LastFetchSuccess(cloneDir string) time.Time {
	info, err := os.Stat(filepath.Join(cloneDir, ".git", "FETCH_HEAD"))
	if err != nil {
		// FETCH_HEAD missing → fall back to the .git directory's mtime
		// (set at clone time) so callers always have a meaningful
		// "last-success" timestamp to surface.
		if gitInfo, gitErr := os.Stat(filepath.Join(cloneDir, ".git")); gitErr == nil {
			return gitInfo.ModTime()
		}
		return time.Time{}
	}
	return info.ModTime()
}

// isGitClone returns (true, nil) iff dst is a directory containing a
// .git/ subdirectory. Anything else returns (false, nil) without
// error — callers re-clone over an empty directory but refuse to
// clone over an existing non-git directory (handled by os.MkdirAll +
// git clone's own check).
func isGitClone(dst string) (bool, error) {
	info, err := os.Stat(filepath.Join(dst, ".git"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, errcode.Wrapf(errcode.InternalError, err,
			"stat %s/.git", dst)
	}
	if !info.IsDir() {
		return false, nil
	}
	return true, nil
}

// runGit invokes git inside dir with the supplied args and surfaces
// the buffered stderr in any returned error message. Useful for fetch
// / reset paths where the failure-mode reason matters.
func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v in %s: %w\n%s", args, dir, err, trimSpace(string(out)))
	}
	return nil
}

// defaultBranch returns the upstream's HEAD branch name (e.g. "main",
// "master") for the given clone. Reads
// refs/remotes/origin/HEAD via `git symbolic-ref` so the answer
// matches what git itself thinks the default is.
func defaultBranch(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git symbolic-ref: %w", err)
	}
	// Output is `refs/remotes/origin/<branch>\n` — extract the branch.
	s := trimSpace(string(out))
	const prefix = "refs/remotes/origin/"
	if len(s) <= len(prefix) || s[:len(prefix)] != prefix {
		return "", fmt.Errorf("unexpected symbolic-ref output: %q", s)
	}
	return s[len(prefix):], nil
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
