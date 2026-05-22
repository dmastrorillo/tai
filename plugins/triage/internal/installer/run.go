package installer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdframework"
	"github.com/dmastrorillo/tai/plugins/triage/internal/version"
)

// AcceptEnv is the environment variable that, when truthy, makes both
// `tai install` and `tai uninstall` overwrite/remove user-modified
// files without prompting (same effect as `--force`).
const AcceptEnv = "TAI_ACCEPT_COMMAND_UPDATES"

// Outcome is the recorded result for a single verb after install or
// uninstall processes it. Stable string values — they appear verbatim
// in the summary block on stdout.
type Outcome string

const (
	OutcomeInstalled       Outcome = "installed"
	OutcomeUpdated         Outcome = "updated"
	OutcomeSkippedUpToDate Outcome = "skipped"
	OutcomePromptedSkipped Outcome = "prompted-skipped"
	OutcomeRemoved         Outcome = "removed"
	OutcomeNotFound        Outcome = "not-found"
)

// Result is one verb's per-run outcome.
type Result struct {
	Verb    string
	Outcome Outcome
}

// Bundle is the read-only view of the bundled slash-commands that
// install and uninstall reconcile against. Production wires this to
// cmdframework.Verbs / BundleSource / LedgerStrict; tests inject a
// fake to exercise scenarios without bundling real commands.
//
// Source returns the FULL `<verb>.md` bytes (frontmatter + body) — the
// installed file's on-disk shape mirrors the bundle's source shape so a
// user reading the file sees the same frontmatter the binary embedded.
// The hash classifier strips the frontmatter and compares body hashes.
type Bundle interface {
	Verbs() []string
	Source(verb string) ([]byte, error)
	Ledger(verb string) ([]string, error)
}

// Options bundles the inputs both install and uninstall need.
type Options struct {
	// TargetDir is the resolved on-disk directory tai writes into
	// (default `~/.claude/commands/tai/`).
	TargetDir string

	// Force, when true, bypasses the user-modified prompt and
	// overwrites (install) or removes (uninstall) without asking.
	Force bool

	// IsTTY indicates whether stdin is a terminal. Only consulted by
	// Install — Uninstall never prompts (per spec D5), so the field is
	// ignored on that path. When false, Install suppresses the prompt
	// and skips user-modified files.
	IsTTY bool

	// Stdin and Stdout are the streams for prompt I/O. The caller wires
	// them to the urfave/cli Reader/Writer in production.
	Stdin  io.Reader
	Stdout io.Writer

	// Bundle is the bundled-command source of truth. If nil, the
	// production cmdframework bundle is used.
	Bundle Bundle
}

// bundleOrDefault returns opts.Bundle when set, or the production
// cmdframework-backed bundle otherwise.
func bundleOrDefault(opts Options) Bundle {
	if opts.Bundle != nil {
		return opts.Bundle
	}
	return cmdframeworkBundle{}
}

// cmdframeworkBundle is the production Bundle implementation. It is
// a value type with no fields — all data lives in cmdframework's
// package-level embed.FS.
type cmdframeworkBundle struct{}

func (cmdframeworkBundle) Verbs() []string                    { return cmdframework.Verbs() }
func (cmdframeworkBundle) Source(verb string) ([]byte, error) { return cmdframework.BundleSource(verb) }
func (cmdframeworkBundle) Ledger(verb string) ([]string, error) {
	return cmdframework.LedgerStrict(verb)
}

// PromptDecision is the answer the user gave to the overwrite prompt.
type PromptDecision int

const (
	PromptOverwrite PromptDecision = iota
	PromptSkip
)

// Prompt asks the user whether to overwrite a user-modified file at
// path, identifying tai's binary version in the prompt text so the user
// knows which release they would be installing. Default answer (empty
// input) is skip. Only `y` or `Y` accepts. The prompt is written to
// stdout; the answer is read from stdin.
//
// The opts.IsTTY=false case is the caller's responsibility — Prompt
// itself always reads from stdin and never inspects the TTY status.
//
// The version is passed in as a parameter so this function is testable
// without mutating package-level state and so the caller (which already
// knows the binary's identity) owns the threading.
func Prompt(opts Options, path, taiVersion string) (PromptDecision, error) {
	if _, err := fmt.Fprintf(opts.Stdout,
		"The file at %s has been modified locally.\n"+
			"Overwrite with the version bundled in tai %s? [y/N] ",
		path, taiVersion); err != nil {
		return PromptSkip, err
	}

	reader := bufio.NewReader(opts.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return PromptSkip, fmt.Errorf("read prompt response: %w", err)
	}
	answer := strings.TrimSpace(line)
	if answer == "y" || answer == "Y" {
		return PromptOverwrite, nil
	}
	return PromptSkip, nil
}

// Install resolves the bundle, classifies each verb's target file,
// executes the resulting action, and returns the per-verb outcomes.
//
// The function never panics; user-modified files with no overrides are
// reported under OutcomePromptedSkipped (exit code 0 — failing to
// update a user-modified file is not an error).
func Install(opts Options) ([]Result, error) {
	if err := ensureTargetDir(opts.TargetDir); err != nil {
		return nil, err
	}

	b := bundleOrDefault(opts)
	verbs := b.Verbs()
	results := make([]Result, 0, len(verbs))
	envAccept := IsTruthyEnv(AcceptEnv)

	for _, verb := range verbs {
		// Source-read failure for a verb that Verbs() just returned is
		// an internal invariant violation, NOT a corrupt ledger — surface
		// as INTERNAL_ERROR so the user is not nudged toward the wrong
		// remediation (re-running `make ledger-update`).
		src, err := b.Source(verb)
		if err != nil {
			return nil, errcode.Wrapf(errcode.InternalError, err,
				"cannot read bundled source for %q", verb)
		}
		ledger, err := b.Ledger(verb)
		if err != nil {
			return nil, err
		}

		targetPath := filepath.Join(opts.TargetDir, verb+".md")
		class, err := Classify(targetPath, ledger)
		if err != nil {
			return nil, err
		}

		switch class {
		case ClassMissing:
			if err := writeFile(targetPath, src); err != nil {
				return nil, err
			}
			results = append(results, Result{Verb: verb, Outcome: OutcomeInstalled})

		case ClassUpToDate:
			results = append(results, Result{Verb: verb, Outcome: OutcomeSkippedUpToDate})

		case ClassStaleButUntouched:
			if err := writeFile(targetPath, src); err != nil {
				return nil, err
			}
			results = append(results, Result{Verb: verb, Outcome: OutcomeUpdated})

		case ClassUserModified:
			overwrite := opts.Force || envAccept
			if !overwrite && opts.IsTTY {
				decision, err := Prompt(opts, targetPath, version.String)
				if err != nil {
					return nil, err
				}
				overwrite = decision == PromptOverwrite
			}
			if overwrite {
				if err := writeFile(targetPath, src); err != nil {
					return nil, err
				}
				results = append(results, Result{Verb: verb, Outcome: OutcomeUpdated})
			} else {
				results = append(results, Result{Verb: verb, Outcome: OutcomePromptedSkipped})
			}
		}
	}

	return results, nil
}

// Uninstall walks opts.TargetDir, removes every file matching a known
// verb whose body hash is in that verb's ledger, and returns the
// per-verb outcomes. Files whose filename does not match any known verb
// are left untouched. After processing, the directory is removed iff it
// is empty.
func Uninstall(opts Options) ([]Result, error) {
	// If the directory doesn't exist, there is nothing to do — report
	// every bundled verb as not-found and return.
	if _, err := os.Stat(opts.TargetDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			b := bundleOrDefault(opts)
			var out []Result
			for _, verb := range b.Verbs() {
				out = append(out, Result{Verb: verb, Outcome: OutcomeNotFound})
			}
			return out, nil
		}
		return nil, errcode.Wrapf(errcode.InstallTargetUnwritable, err,
			"cannot stat target directory %q", opts.TargetDir).
			WithHelp("check directory permissions")
	}

	b := bundleOrDefault(opts)
	verbs := b.Verbs()
	results := make([]Result, 0, len(verbs))
	envAccept := IsTruthyEnv(AcceptEnv)

	for _, verb := range verbs {
		targetPath := filepath.Join(opts.TargetDir, verb+".md")
		ledger, err := b.Ledger(verb)
		if err != nil {
			return nil, err
		}
		class, err := Classify(targetPath, ledger)
		if err != nil {
			return nil, err
		}
		switch class {
		case ClassMissing:
			results = append(results, Result{Verb: verb, Outcome: OutcomeNotFound})
		case ClassUpToDate, ClassStaleButUntouched:
			if err := os.Remove(targetPath); err != nil {
				return nil, errcode.Wrapf(errcode.InstallTargetUnwritable, err,
					"cannot remove %q", targetPath).
					WithHelp("check directory permissions")
			}
			results = append(results, Result{Verb: verb, Outcome: OutcomeRemoved})
		case ClassUserModified:
			if opts.Force || envAccept {
				if err := os.Remove(targetPath); err != nil {
					return nil, errcode.Wrapf(errcode.InstallTargetUnwritable, err,
						"cannot remove %q", targetPath).
						WithHelp("check directory permissions")
				}
				results = append(results, Result{Verb: verb, Outcome: OutcomeRemoved})
			} else {
				results = append(results, Result{Verb: verb, Outcome: OutcomePromptedSkipped})
			}
		}
	}

	// Remove the directory iff empty (no unrelated files survive).
	entries, err := os.ReadDir(opts.TargetDir)
	if err != nil {
		return nil, errcode.Wrapf(errcode.InstallTargetUnwritable, err,
			"cannot read target directory %q", opts.TargetDir)
	}
	if len(entries) == 0 {
		if err := os.Remove(opts.TargetDir); err != nil {
			return nil, errcode.Wrapf(errcode.InstallTargetUnwritable, err,
				"cannot remove empty target directory %q", opts.TargetDir)
		}
	}

	return results, nil
}

// ensureTargetDir creates the target directory (and any missing
// parents) and probes for writability. Errors map to
// INSTALL_TARGET_UNWRITABLE.
func ensureTargetDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errcode.Wrapf(errcode.InstallTargetUnwritable, err,
			"cannot create target directory %q", dir).
			WithHelp(
				"check directory permissions",
				"override the target with `--commands-dir <path>`",
			)
	}
	// Probe writability — MkdirAll succeeds on an existing-but-read-only
	// directory, so an actual write is the only way to be sure.
	probe, err := os.CreateTemp(dir, ".tai-install-probe-*")
	if err != nil {
		return errcode.Wrapf(errcode.InstallTargetUnwritable, err,
			"target directory %q is not writable", dir).
			WithHelp("check directory permissions")
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		removeOrLog(name)
		return errcode.Wrapf(errcode.InstallTargetUnwritable, err,
			"target directory %q probe close failed", dir)
	}
	if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errcode.Wrapf(errcode.InstallTargetUnwritable, err,
			"target directory %q probe cleanup failed", dir).
			WithHelp("check directory permissions",
				fmt.Sprintf("remove the stray file at %q", name))
	}
	return nil
}

// writeFile writes body atomically. A uniquely-named temp file in the
// target's directory is created via os.CreateTemp (so two concurrent
// `tai install` invocations cannot interleave writes into the same
// path), the body is written and fsynced-via-close, and the temp is
// renamed onto targetPath. Rename is atomic on POSIX and on same-volume
// Windows, which is the case here because both files share dir.
//
// On any failure after the temp is created the temp is best-effort
// removed; the cleanup error is appended to the returned error so a
// stray temp file is at least visible to the user.
func writeFile(targetPath string, body []byte) error {
	dir := filepath.Dir(targetPath)
	base := filepath.Base(targetPath)

	tmp, err := os.CreateTemp(dir, base+".*.tmp")
	if err != nil {
		return errcode.Wrapf(errcode.InstallTargetUnwritable, err,
			"cannot create temp file for %q", targetPath).
			WithHelp("check directory permissions")
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		removeOrLog(tmpName)
		return errcode.Wrapf(errcode.InstallTargetUnwritable, err,
			"cannot write %q", tmpName)
	}
	if err := tmp.Close(); err != nil {
		removeOrLog(tmpName)
		return errcode.Wrapf(errcode.InstallTargetUnwritable, err,
			"cannot close %q", tmpName)
	}
	// Match the world-readable file mode of every other slash-command
	// markdown the user touches. CreateTemp creates 0o600 by default.
	if err := os.Chmod(tmpName, 0o644); err != nil {
		removeOrLog(tmpName)
		return errcode.Wrapf(errcode.InstallTargetUnwritable, err,
			"cannot chmod %q", tmpName)
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		removeOrLog(tmpName)
		return errcode.Wrapf(errcode.InstallTargetUnwritable, err,
			"cannot rename %q to %q", tmpName, targetPath)
	}
	return nil
}

// removeOrLog removes path and writes a log entry on failure. Used by
// writeFile and ensureTargetDir to avoid silently leaking temp files
// while still letting the caller propagate the primary error.
func removeOrLog(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		// We have no logger plumbed into installer; surface to stderr so
		// the user at least sees the orphan path. This is best-effort.
		fmt.Fprintf(os.Stderr, "warning: could not remove temp file %q: %v\n", path, err)
	}
}

// FormatSummary renders the human-readable summary block per the spec.
// install=true emits Installed/Updated/Skipped/Prompted-skipped;
// install=false emits Removed/Prompted-skipped/Not-found.
//
// Outcome buckets with zero entries are omitted. Verb lists are
// collapsed to `…` once N>5.
func FormatSummary(results []Result, install bool) string {
	buckets := map[Outcome][]string{}
	for _, r := range results {
		buckets[r.Outcome] = append(buckets[r.Outcome], r.Verb)
	}
	for _, vs := range buckets {
		sort.Strings(vs)
	}

	var lines []string
	emit := func(o Outcome, label, suffix string) {
		vs := buckets[o]
		if len(vs) == 0 {
			return
		}
		line := fmt.Sprintf("%s: %d %s%s", label, len(vs), pluralCommand(len(vs)), formatVerbs(vs, suffix))
		lines = append(lines, line)
	}

	if install {
		emit(OutcomeInstalled, "Installed", "")
		emit(OutcomeUpdated, "Updated", "")
		emit(OutcomeSkippedUpToDate, "Skipped", " (up to date)")
		emit(OutcomePromptedSkipped, "Prompted-skipped", "")
	} else {
		emit(OutcomeRemoved, "Removed", "")
		emit(OutcomePromptedSkipped, "Prompted-skipped", "")
		emit(OutcomeNotFound, "Not-found", "")
	}

	if len(lines) == 0 {
		lines = append(lines, "No bundled commands in this build.")
	}

	return strings.Join(lines, "\n") + "\n\n[exit 0]\n"
}

// pluralCommand picks the right English form so the summary reads
// naturally: "1 command" vs "2 commands". The earlier "command(s)"
// portmanteau diverged from the design doc's example output.
func pluralCommand(n int) string {
	if n == 1 {
		return "command"
	}
	return "commands"
}

// formatVerbs renders the parenthesised verb list shown after each
// count line. Returns the leading suffix (e.g. " (up to date)") when no
// verb names are needed, or " (a, b, c)" / " (… more)" otherwise.
func formatVerbs(verbs []string, fixedSuffix string) string {
	if fixedSuffix != "" {
		return fixedSuffix
	}
	if len(verbs) == 0 {
		return ""
	}
	if len(verbs) > 5 {
		return " (…)"
	}
	return " (" + strings.Join(verbs, ", ") + ")"
}
