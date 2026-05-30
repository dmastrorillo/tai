// Package repoinit owns the `tai repo init <path>` scaffold.
//
// The package embeds every file the scaffold writes via go:embed so
// the binary ships self-contained — no runtime dependency on a
// templates/ directory beside it. See
// openspec/changes/pivot-to-ai-as-code/specs/repo-init/spec.md for
// the normative spec.
package repoinit

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

//go:embed templates
var templatesFS embed.FS

// Scaffold writes the source-repo template into dst.
//
// Behaviour per the spec:
//
//   - If dst does not exist, it is created.
//   - If dst exists and is non-empty, returns CONFIG-style error
//     REPO_INIT_TARGET_NOT_EMPTY without modifying anything.
//   - After files are written, `git init` + an initial commit are
//     run. If git is not on PATH, returns REPO_INIT_GIT_UNAVAILABLE
//     after the files are already on disk.
//
// The .gitignore is shipped under the embed name "gitignore" (no
// leading dot) because go:embed silently skips dotfiles; we rename
// at write time.
//
// ctx is threaded through to every subprocess call so Ctrl+C during a
// long scaffold aborts cleanly.
func Scaffold(ctx context.Context, dst string) error {
	if err := checkTargetEmpty(dst); err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return errcode.Wrapf(errcode.InternalError, err,
			"create scaffold directory %s", dst)
	}
	if err := writeTemplates(dst); err != nil {
		return err
	}
	if err := gitInitAndCommit(ctx, dst); err != nil {
		return err
	}
	return nil
}

// NextStepsBlock returns the human-readable next-steps text emitted on
// successful scaffold. The exact strings are part of TC-INIT-007's
// contract: the phrases "Next steps:", "git remote add origin", and
// "tai config set repo-url" MUST appear verbatim.
func NextStepsBlock(dst string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Scaffolded a tai source repo at %s.\n\n", dst)
	b.WriteString("Next steps:\n\n")
	b.WriteString("  1. Push this repo to a remote:\n")
	b.WriteString("       git remote add origin <your-remote-url>\n")
	b.WriteString("       git push -u origin main\n\n")
	b.WriteString("  2. On every developer machine (including this one if it consumes the repo):\n")
	b.WriteString("       tai config set repo-url <your-remote-url>\n")
	b.WriteString("       tai config target add ~/.claude\n")
	b.WriteString("       tai sync\n")
	return b.String()
}

// checkTargetEmpty verifies dst is either non-existent or an empty
// directory. Anything else surfaces REPO_INIT_TARGET_NOT_EMPTY.
func checkTargetEmpty(dst string) error {
	entries, err := os.ReadDir(dst)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return errcode.Wrapf(errcode.InternalError, err,
			"inspect scaffold target %s", dst)
	}
	if len(entries) > 0 {
		return errcode.Newf(errcode.RepoInitTargetNotEmpty,
			"refuse to scaffold into non-empty directory %s", dst).
			WithHelp(
				"move existing files out, or pick a fresh path",
				"the scaffold MUST land in an empty directory to avoid clobbering authored content",
			)
	}
	return nil
}

// writeTemplates walks the embedded templates/ tree and copies every
// entry into dst, applying two name-rewrites:
//
//   - "gitignore" becomes ".gitignore" (go:embed skips dotfiles).
//   - The root "templates/" prefix is stripped.
func writeTemplates(dst string) error {
	return fs.WalkDir(templatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "templates")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return nil
		}
		// Rewrite "gitignore" → ".gitignore" at the top level only.
		if rel == "gitignore" {
			rel = ".gitignore"
		}
		outPath := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(outPath, 0o755)
		}

		data, readErr := templatesFS.ReadFile(path)
		if readErr != nil {
			return errcode.Wrapf(errcode.InternalError, readErr,
				"read embedded template %s", path)
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return errcode.Wrapf(errcode.InternalError, err,
				"create directory for %s", outPath)
		}
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return errcode.Wrapf(errcode.InternalError, err,
				"write template to %s", outPath)
		}
		return nil
	})
}

// gitInitAndCommit runs `git init`, `git add -A`, and `git commit -m`
// with the spec's canonical initial commit message. If git is not on
// PATH the scaffolded files remain on disk; the user re-runs after
// installing git.
//
// The commit author defaults are configured locally on the new repo
// so the commit succeeds even when the running user has no global
// git identity — a fresh CI box or container, for instance, would
// otherwise fail at `git commit`.
//
// The init step prefers `--initial-branch=main` (git >= 2.28; released
// 2020-07) for forward-compatibility with the wider ecosystem. On
// older git versions the flag is silently retried without it and the
// branch is renamed via `git branch -m main` so the resulting repo
// is identical regardless of git version.
func gitInitAndCommit(ctx context.Context, dst string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return errcode.New(errcode.RepoInitGitUnavailable,
			"`git` is not on PATH").
			WithHelp(
				"install git and re-run `cd "+dst+" && git init && git add -A && git commit -m 'Initial TAI source-repo scaffold'`",
				"the scaffolded files are already on disk and are safe to keep",
			)
	}

	// Try the modern --initial-branch flag first. Fall back to the
	// two-step (`git init` + `git branch -m main`) form for git < 2.28.
	if err := runGit(ctx, dst, "init", "--initial-branch=main"); err != nil {
		if err := runGit(ctx, dst, "init"); err != nil {
			return errcode.Wrapf(errcode.InternalError, err,
				"git init failed in %s", dst).
				WithHelp("run `git init` manually inside " + dst + " to see the underlying cause")
		}
		// Best-effort rename to main; ignore failure (the repo is
		// usable even if the branch is still called master).
		_ = runGit(ctx, dst, "branch", "-m", "main")
	}

	steps := [][]string{
		{"config", "user.email", "tai-repo-init@local"},
		{"config", "user.name", "tai repo init"},
		{"add", "-A"},
		{"commit", "-m", "Initial TAI source-repo scaffold"},
	}
	for _, step := range steps {
		if err := runGit(ctx, dst, step...); err != nil {
			return errcode.Wrapf(errcode.InternalError, err,
				"git step `git %s` failed", strings.Join(step, " ")).
				WithHelp("run the failing git command manually inside " + dst + " to see the underlying cause")
		}
	}
	return nil
}

// runGit invokes git inside dir with the given args. Buffers
// CombinedOutput so git's chatty stderr doesn't leak through tai's
// error template; the buffered output is folded into the error
// message on failure.
func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s in %s: %w\n%s",
			strings.Join(args, " "), dir, err, strings.TrimSpace(string(out)))
	}
	return nil
}
