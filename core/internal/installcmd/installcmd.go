// Package installcmd ships TAI's own bundled slash-command assets
// into every configured target's `<commands>/tai/` subdirectory.
//
// The `tai/` subdirectory is treated as a TAI-owned namespace —
// re-runs overwrite files within it and remove built-ins the running
// binary no longer bundles, but content outside `tai/` is preserved
// untouched. Because the subdirectory is wholly TAI-owned, no
// manifest is needed: the next run computes target state from the
// currently-embedded bundle.
//
// See openspec/changes/pivot-to-ai-as-code/specs/install-commands/
// spec.md for the normative behaviour.
package installcmd

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// bundledFS is the embedded slash-command bundle. The path
// `assets/commands` is fixed by convention; every `.md` file under
// it ships with the binary. To add a new built-in, drop a `.md` file
// into `assets/commands/`.
//
//go:embed assets/commands/*.md
var bundledFS embed.FS

// subdirName is the TAI-owned subdirectory inside each target's
// configured commands path. Re-runs delete-and-replace files within
// it; content outside it is never touched.
const subdirName = "tai"

// Result describes what Install did. Counts are aggregated across
// every configured target.
type Result struct {
	// Targets is the number of targets that received the bundle
	// (i.e. excluding targets skipped via a falsy `commands`
	// sub-path).
	Targets int
	// Written is the count of bundled-file writes across all
	// targets. Equal to Targets * len(bundled-files) when no target
	// is skipped.
	Written int
	// Removed is the count of stale built-ins deleted across all
	// targets (files present under `<root>/<commands>/tai/` whose
	// names are no longer in the running binary's bundle).
	Removed int
	// Skipped is the count of targets bypassed because their
	// `commands` sub-path is falsy ("").
	Skipped int
}

// Install copies every bundled built-in command into
// `<target.root>/<target.commands>/tai/` for each configured target,
// then deletes any files in that subdirectory whose names are no
// longer in the bundle. Warnings (e.g. falsy-skip notices) are
// written to stderr.
//
// Returns *errcode.Error{Code: TaiNotConfigured} when cfg has no
// targets.
func Install(cfg *config.File, stderr io.Writer) (*Result, error) {
	if cfg == nil || len(cfg.Targets) == 0 {
		return nil, errcode.New(errcode.TaiNotConfigured,
			"tai install-commands requires at least one configured target").
			WithHelp(
				"add a target: `tai config target add ~/.claude`",
				"or inspect the current config: `tai config show`",
			)
	}

	bundled, err := bundledFiles()
	if err != nil {
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"enumerate bundled commands")
	}
	bundledSet := map[string]bool{}
	for _, name := range bundled {
		bundledSet[name] = true
	}

	res := &Result{}
	for _, t := range cfg.Targets {
		commandsDir := effectiveCommandsDir(t)
		if commandsDir == "" {
			_, _ = fmt.Fprintf(stderr,
				"[tai] target %s: commands subdirectory is set to \"\" in config — skipping\n",
				t.Root)
			res.Skipped++
			continue
		}
		taiDir := filepath.Join(commandsDir, subdirName)
		written, removed, err := installToTarget(taiDir, bundled, bundledSet)
		if err != nil {
			return nil, err
		}
		res.Targets++
		res.Written += written
		res.Removed += removed
	}
	return res, nil
}

// effectiveCommandsDir resolves the `commands` sub-path for t by
// delegating to config.Target.EffectiveSubpaths. Returns "" when the
// override is explicitly falsy ("" in YAML); the caller skips such
// targets. The skills/agents return values are intentionally
// discarded — install-commands only writes to the commands sub-path.
func effectiveCommandsDir(t config.Target) string {
	_, commands, _ := t.EffectiveSubpaths()
	return commands
}

// installToTarget writes every bundled file into taiDir and removes
// any pre-existing `.md` whose name is no longer in bundledSet.
// Returns (written, removed).
func installToTarget(taiDir string, bundled []string, bundledSet map[string]bool) (int, int, error) {
	// Step 1: remove stale `.md` files in taiDir. This precedes the
	// write step so a same-name overwrite doesn't get classified as
	// "removed".
	removed, err := removeStaleBuiltins(taiDir, bundledSet)
	if err != nil {
		return 0, 0, err
	}

	// Step 2: write each bundled file. MkdirAll handles the
	// first-time case; subsequent runs reuse the existing directory.
	if err := os.MkdirAll(taiDir, 0o755); err != nil {
		return 0, 0, errcode.Wrapf(errcode.InternalError, err,
			"create %s", taiDir)
	}
	var written int
	for _, name := range bundled {
		src := "assets/commands/" + name
		data, readErr := bundledFS.ReadFile(src)
		if readErr != nil {
			return 0, 0, errcode.Wrapf(errcode.InternalError, readErr,
				"read embedded asset %s", src)
		}
		dst := filepath.Join(taiDir, name)
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return 0, 0, errcode.Wrapf(errcode.InternalError, err,
				"write %s", dst)
		}
		written++
	}
	return written, removed, nil
}

// removeStaleBuiltins deletes every `.md` file in taiDir whose name
// is not in bundledSet. Returns the count of files removed. A missing
// taiDir is fine — nothing to remove.
func removeStaleBuiltins(taiDir string, bundledSet map[string]bool) (int, error) {
	entries, err := os.ReadDir(taiDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, errcode.Wrapf(errcode.InternalError, err,
			"read %s", taiDir)
	}
	var removed int
	for _, e := range entries {
		if e.IsDir() {
			// The bundle is flat .md files; any subdirectory is
			// user-authored and must NOT be touched.
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			// Only `.md` files are TAI's responsibility. Leave any
			// other file alone — they fall outside the documented
			// namespace.
			continue
		}
		if bundledSet[name] {
			continue
		}
		p := filepath.Join(taiDir, name)
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return 0, errcode.Wrapf(errcode.InternalError, err,
				"remove stale %s", p)
		}
		removed++
	}
	return removed, nil
}

// bundledFiles returns the sorted list of `.md` filenames under
// `assets/commands/` in the embedded FS. Sorting makes the install
// order deterministic for tests.
func bundledFiles() ([]string, error) {
	entries, err := bundledFS.ReadDir("assets/commands")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// BundledFiles is the test-only accessor for bundledFiles. Exported
// solely so e2e tests in core/internal/cmd_test (a different
// package, so it cannot reach `bundledFiles` directly) can assert
// what the running binary ships. The `core/internal/` import path
// keeps this symbol unreachable from any module outside `tai`; no
// `tai` production code should call it.
func BundledFiles() ([]string, error) { return bundledFiles() }
