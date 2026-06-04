package sync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// Options carries the per-invocation flags Sync respects.
type Options struct {
	// Yes (the -y / --yes flag) bypasses the overwrite/prune
	// confirmation prompt.
	Yes bool
	// Prune (--prune) instructs Sync to delete orphans (entries in
	// the manifest no longer present in the current source). Without
	// it, orphans persist and are surfaced in the summary.
	Prune bool

	// Stdin / Stdout / Stderr — Sync's I/O. Tests pass buffers /
	// strings.NewReader; main.go passes os.Std*.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Result describes what Sync did. The fields are deliberately
// per-category and per-target-flat (a single []string) so tests can
// assert without re-walking the filesystem.
type Result struct {
	// Written is the count of files written (created or overwritten)
	// across all targets and categories.
	Written int
	// Overwritten is the subset of Written that already existed.
	Overwritten int
	// OrphansPending is the count of manifest entries that no longer
	// exist in the source. Reported regardless of --prune.
	OrphansPending int
	// Pruned is the count of orphan files actually deleted from
	// targets (--prune + confirm only).
	Pruned int
	// Cancelled is true when the user answered N at the prompt.
	Cancelled bool
}

// Sync performs the full sync flow for every configured target. The
// flow per the spec:
//
//  1. EnsureClone — clones on first sync, reuses otherwise.
//  2. Fetch — eager fetch; on failure emits one-line cache-fallback
//     warning to stderr but does NOT abort.
//  3. For each target: walk the source under each category, classify
//     destinations as create / overwrite / up-to-date, load the
//     manifest, compute orphans.
//  4. If any overwrite/orphan needs confirmation and -y is unset,
//     emit one batched prompt to stderr and read stdin.
//  5. Write the planned source files, update the manifest, and
//     (when --prune is on and confirmed) delete orphans.
//
// Pre-conditions enforced before any I/O: both repo-url and at least
// one target MUST be configured. Otherwise Sync exits with
// TAI_NOT_CONFIGURED before touching disk.
func Sync(ctx context.Context, cfg *config.File, dataDir string, opts Options) (*Result, error) {
	if err := checkConfigured(cfg); err != nil {
		return nil, err
	}

	cloneDir, err := EnsureClone(ctx, dataDir, cfg.RepoURL)
	if err != nil {
		return nil, err
	}
	if fetchErr := Fetch(ctx, cloneDir); fetchErr != nil {
		_, _ = fmt.Fprintf(opts.Stderr, "[tai] fetch failed (%s); proceeding against cached clone from %s\n",
			summariseFetchError(fetchErr),
			LastFetchSuccess(cloneDir).Format(time.RFC3339))
	}

	// Phase 4 of pivot-to-ai-as-code: read <clone>/plugins.yml and
	// install any listed plugin that isn't already present. Runs
	// before the asset-sync phase so a plugin's namespaced assets
	// land alongside the source-repo assets in one pass. The hook
	// is additive — removing a YAML entry does NOT uninstall a
	// plugin from the developer's machine (spec).
	if err := autoInstallPluginsFromYAML(ctx, cloneDir, dataDir, cfg, opts.Stderr); err != nil {
		return nil, err
	}

	plan, perTargetPlans, err := buildPlan(cloneDir, cfg.Targets, dataDir)
	if err != nil {
		return nil, err
	}

	// Emit falsy-skip warnings up-front so the user sees what's being
	// skipped before the prompt asks them to confirm overwrites.
	for _, tp := range perTargetPlans {
		for _, s := range tp.skippedCategories {
			_, _ = fmt.Fprintf(opts.Stderr,
				"[tai] target %s: %s category skipped (%d file%s) — sub-path is set to \"\" in config\n",
				s.target, s.category, s.count, plural(s.count))
		}
	}

	res := &Result{
		OrphansPending: len(plan.Orphans),
	}

	// Decide whether we need to prompt.
	needPrompt := !opts.Yes && (plan.HasOverwrites() || (opts.Prune && plan.HasOrphans()))
	if needPrompt {
		ok, err := Prompt(plan, opts.Prune, opts.Stdin, opts.Stderr)
		if err != nil {
			return nil, errcode.Wrap(errcode.InternalError, err, "read overwrite prompt response")
		}
		if !ok {
			NoticeCancelled(opts.Stderr)
			res.Cancelled = true
			// Still surface the orphan count even after a cancel so
			// the user sees the pending state.
			if !opts.Prune {
				NoticeOrphans(opts.Stderr, res.OrphansPending)
			}
			return res, nil
		}
	}

	// Write phase: apply each per-target plan.
	for _, p := range perTargetPlans {
		w, o, pruned, err := applyTargetPlan(p, opts.Prune)
		if err != nil {
			return nil, err
		}
		res.Written += w
		res.Overwritten += o
		res.Pruned += pruned
	}

	// Visibility of bypassed overwrites — only emitted when -y was
	// the bypass mechanism.
	if opts.Yes && plan.HasOverwrites() {
		NoticeOverwritten(opts.Stderr, plan.Overwrites)
	}

	// Without --prune, surface the pending-orphan count as a one-line
	// summary on every sync (spec requirement).
	if !opts.Prune && res.OrphansPending > 0 {
		NoticeOrphans(opts.Stderr, res.OrphansPending)
	}

	return res, nil
}

// checkConfigured rejects the precondition gate: both repo-url and at
// least one target MUST be set for sync to do anything useful.
func checkConfigured(cfg *config.File) error {
	if cfg == nil || strings.TrimSpace(cfg.RepoURL) == "" || len(cfg.Targets) == 0 {
		return errcode.New(errcode.TaiNotConfigured,
			"tai sync requires both `repo-url` and at least one target").
			WithHelp(
				"set the source repo: `tai config set repo-url <url>`",
				"add a target: `tai config target add ~/.claude`",
			)
	}
	return nil
}

// targetPlan is the per-target slice of the global Plan: it carries
// the absolute paths Sync will write or delete for one configured
// target.
type targetPlan struct {
	root     string
	dataDir  string
	manifest *Manifest
	// writes[cat] is the slice of (sourcePath, destPath, manifestEntry).
	writes map[Category][]writePair
	// orphans[absPath] = manifestEntry. Both halves are needed —
	// absPath to delete the file, manifestEntry to remove from the
	// manifest after.
	orphans map[string]string
	// skippedCategories tracks any (target, category) pair where the
	// target's sub-path is falsy ("") AND the source has content for
	// that category. Sync emits a one-line stderr warning per entry
	// so the operator sees what was skipped without re-running.
	skippedCategories []skippedCategory
}

// skippedCategory is one (target, category) pair that was skipped by
// a falsy override despite the source having content. Surfaced as a
// warning in the sync summary.
type skippedCategory struct {
	target   string
	category Category
	count    int
}

type writePair struct {
	srcAbs        string
	dstAbs        string
	manifestEntry string
	existed       bool
}

// buildPlan walks each target and computes the would-write /
// would-overwrite / orphan sets. Returns both the global view (for
// the batched prompt) and the per-target view (for the write step).
func buildPlan(cloneDir string, targets []config.Target, dataDir string) (*Plan, []*targetPlan, error) {
	plan := &Plan{
		Overwrites: map[Category][]string{},
	}
	var perTarget []*targetPlan

	for _, t := range targets {
		tp := &targetPlan{
			root:    t.Root,
			dataDir: dataDir,
			writes:  map[Category][]writePair{},
			orphans: map[string]string{},
		}
		manifest, err := LoadManifest(dataDir, t.Root)
		if err != nil {
			return nil, nil, err
		}
		tp.manifest = manifest

		// Build the current-source set across all three categories so
		// we can compute orphans (manifest - source) at the end.
		currentSource := map[string]bool{}

		for _, cat := range Categories() {
			override := pickOverride(t, cat)
			subRoot := TargetSubpath(t.Root, override, string(cat))
			files, err := SourceFiles(cloneDir, cat)
			if err != nil {
				return nil, nil, errcode.Wrapf(errcode.InternalError, err,
					"walk source category %s", cat)
			}
			if subRoot == "" {
				// Falsy sub-path → skip this category entirely for
				// this target. Record the skip so Sync can emit a
				// one-line stderr warning per skipped category that
				// had content in the source.
				if len(files) > 0 {
					tp.skippedCategories = append(tp.skippedCategories, skippedCategory{
						target:   t.Root,
						category: cat,
						count:    len(files),
					})
				}
				continue
			}

			for _, rel := range files {
				srcAbs := JoinRel(filepath.Join(cloneDir, string(cat)), rel)
				dstAbs := JoinRel(subRoot, rel)
				manifestEntry := string(cat) + "/" + rel
				currentSource[manifestEntry] = true

				existed := fileExists(dstAbs)
				tp.writes[cat] = append(tp.writes[cat], writePair{
					srcAbs:        srcAbs,
					dstAbs:        dstAbs,
					manifestEntry: manifestEntry,
					existed:       existed,
				})
				if existed {
					plan.Overwrites[cat] = append(plan.Overwrites[cat], dstAbs)
				}
			}
		}

		// Orphans for this target: manifest entries not in the
		// current source set.
		for _, entry := range tp.manifest.Orphans(currentSource) {
			absPath, ok := resolveOrphan(t, entry)
			if !ok {
				// Couldn't resolve — the manifest entry points at a
				// category the target no longer covers (falsy
				// override added after the entry was synced). Drop
				// the entry from the in-memory manifest now so it's
				// cleaned up by the SaveManifest at the end of
				// applyTargetPlan; otherwise the manifest accumulates
				// stale entries indefinitely.
				tp.manifest.Remove(entry)
				continue
			}
			tp.orphans[absPath] = entry
			plan.Orphans = append(plan.Orphans, absPath)
			plan.PrunePaths = append(plan.PrunePaths, absPath)
		}

		perTarget = append(perTarget, tp)
	}
	return plan, perTarget, nil
}

// pickOverride returns the *string pointer the config holds for cat
// on t.
func pickOverride(t config.Target, cat Category) *string {
	switch cat {
	case CategorySkills:
		return t.Skills
	case CategoryCommands:
		return t.Commands
	case CategoryAgents:
		return t.Agents
	}
	return nil
}

// resolveOrphan converts a manifest entry ("<cat>/<rel>") into an
// absolute filesystem path on the given target. Returns false when
// the target no longer has a sub-path for the entry's category
// (falsy override) — that orphan can't be reached and should be
// dropped from the manifest by the caller.
func resolveOrphan(t config.Target, entry string) (string, bool) {
	slash := strings.IndexByte(entry, '/')
	if slash < 0 {
		return "", false
	}
	cat := Category(entry[:slash])
	rel := entry[slash+1:]
	override := pickOverride(t, cat)
	sub := TargetSubpath(t.Root, override, string(cat))
	if sub == "" {
		return "", false
	}
	return JoinRel(sub, rel), true
}

// applyTargetPlan executes the writes and (when prune is on)
// deletions for one target. Returns counts: (written, overwritten,
// pruned).
func applyTargetPlan(tp *targetPlan, prune bool) (int, int, int, error) {
	var written, overwritten, pruned int

	for _, cat := range Categories() {
		for _, w := range tp.writes[cat] {
			if err := copyFile(w.srcAbs, w.dstAbs); err != nil {
				return 0, 0, 0, err
			}
			tp.manifest.Add(w.manifestEntry)
			written++
			if w.existed {
				overwritten++
			}
		}
	}

	if prune {
		for abs, entry := range tp.orphans {
			if err := os.Remove(abs); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return 0, 0, 0, errcode.Wrapf(errcode.InternalError, err,
					"prune %s", abs)
			}
			tp.manifest.Remove(entry)
			pruned++
		}
	}

	if err := SaveManifest(tp.dataDir, tp.root, tp.manifest); err != nil {
		return 0, 0, 0, err
	}
	return written, overwritten, pruned, nil
}

// copyFile creates dst's parent dir if needed, then writes src's
// bytes to dst with the same permission as src.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return errcode.Wrapf(errcode.InternalError, err, "read source %s", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return errcode.Wrapf(errcode.InternalError, err, "mkdir %s", filepath.Dir(dst))
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return errcode.Wrapf(errcode.InternalError, err, "write %s", dst)
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// summariseFetchError reduces a noisy git error message to a short
// human-readable category for the cache-fallback warning. Today we
// just take the first line of the message — git's first line is
// usually the most actionable part.
func summariseFetchError(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

// plural is shared with prompt.go in the same package.
