package plugins

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// ValidateAssetNamespace walks the plugin's downloaded `assets/`
// directory under pluginDir and returns the first
// `*errcode.Error{Code: PLUGIN_ASSET_NAMING}` it encounters.
//
// Rules (per spec):
//
//   - Every entry directly under `assets/skills/` MUST start with
//     `tai-<plugin>-`.
//   - Every file directly under `assets/agents/` MUST start with
//     `tai-<plugin>-`.
//   - Commands (`assets/commands/`) are unconstrained — TAI routes
//     them into `<commands>/tai-<plugin>/` at install time.
//
// A missing `assets/skills` or `assets/agents` is fine (the plugin
// just doesn't ship that category).
func ValidateAssetNamespace(pluginDir, pluginName string) error {
	prefix := "tai-" + pluginName + "-"
	for _, sub := range []string{"skills", "agents"} {
		root := filepath.Join(pluginDir, "assets", sub)
		entries, err := os.ReadDir(root)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return errcode.Wrapf(errcode.InternalError, err,
				"read %s", root)
		}
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), prefix) {
				rel := filepath.Join("assets", sub, e.Name())
				return errcode.Newf(errcode.PluginAssetNaming,
					"plugin %s ships %s without the mandatory %q prefix",
					pluginName, rel, prefix).
					WithHelp(
						"rename the file/folder to start with `"+prefix+"`",
						"plugin asset namespacing is documented in CLAUDE.md",
					)
			}
		}
	}
	return nil
}

// SyncAssetsToTargets copies the plugin's `assets/` content into
// every configured target, applying the namespacing rules. The order
// per category is copy-then-prune: new files are copied in first
// (overwriting same-name entries in place), and only after every
// copy succeeded are namespace entries the new version no longer
// ships removed. A copy failure partway therefore leaves the
// previously-installed files on disk instead of a wiped, half-copied
// namespace — the same partial-failure discipline atomicReplaceDir
// applies to the plugin's own install directory.
//
// Falsy sub-paths (`skills: ""` / `commands: ""` / `agents: ""` in
// config) skip that category for that target, with a one-line
// stderr warning per skipped category that actually has content.
//
// The function is the same shape `tai sync` uses for source-repo
// assets, but the namespace scope is wholly TAI-owned (no overwrite
// prompts, no manifest — the namespace IS the manifest).
func SyncAssetsToTargets(pluginDir, pluginName string, targets []config.Target, stderr io.Writer) error {
	for _, t := range targets {
		for _, c := range categoriesFor(pluginName, t) {
			if c.target == "" {
				// Falsy → only warn when the plugin actually has
				// content for this category. Avoids noise on the
				// majority of plugins that ship two of three
				// categories.
				if categoryHasContent(pluginDir, c.sub) {
					_, _ = fmt.Fprintf(stderr,
						"[tai] target %s: %s subdirectory skipped — sub-path is set to \"\" in config\n",
						c.root, c.sub)
				}
				continue
			}
			shipped, err := copyPluginCategory(pluginDir, c)
			if err != nil {
				return err
			}
			if err := prunePluginNamespace(c, shipped); err != nil {
				return err
			}
		}
	}
	return nil
}

// WipePluginFromTargets removes the plugin's namespace from every
// target without copying anything in. Used by `tai plugins <name>
// remove` and by the update flow before the new assets are copied.
func WipePluginFromTargets(pluginName string, targets []config.Target) error {
	for _, t := range targets {
		for _, c := range categoriesFor(pluginName, t) {
			if c.target == "" {
				continue
			}
			if err := wipePluginNamespace(c); err != nil {
				return err
			}
		}
	}
	return nil
}

// categoriesFor builds the three assetCategory rows for one target —
// the single source of truth for the category/sub-path wiring shared
// by SyncAssetsToTargets and WipePluginFromTargets.
func categoriesFor(pluginName string, t config.Target) []assetCategory {
	skills, commands, agents := t.EffectiveSubpaths()
	return []assetCategory{
		{sub: "skills", target: skills, plugin: pluginName, root: t.Root, isCommands: false},
		{sub: "agents", target: agents, plugin: pluginName, root: t.Root, isCommands: false},
		{sub: "commands", target: commands, plugin: pluginName, root: t.Root, isCommands: true},
	}
}

// assetCategory carries the (source category, target absolute path,
// plugin name) tuple SyncAssetsToTargets passes around.
type assetCategory struct {
	sub        string // "skills" / "commands" / "agents"
	target     string // absolute path under the target's root
	plugin     string
	root       string // target.Root, for warning messages
	isCommands bool   // commands route into `tai-<plugin>/` subdir; skills/agents are file-prefixed
}

// categoryHasContent reports whether `<pluginDir>/assets/<sub>/`
// exists and contains at least one entry. Used to decide whether a
// falsy-skip warning is worth emitting.
func categoryHasContent(pluginDir, sub string) bool {
	root := filepath.Join(pluginDir, "assets", sub)
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

// wipePluginNamespace deletes the plugin's namespace inside c.target.
// For commands, that's the `tai-<plugin>/` subdirectory. For
// skills/agents, that's every entry whose name starts with
// `tai-<plugin>-`.
func wipePluginNamespace(c assetCategory) error {
	if c.isCommands {
		dir := c.namespaceBase()
		if err := os.RemoveAll(dir); err != nil {
			return errcode.Wrapf(errcode.InternalError, err,
				"remove %s", dir)
		}
		return nil
	}
	entries, err := os.ReadDir(c.target)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return errcode.Wrapf(errcode.InternalError, err,
			"read %s", c.target)
	}
	prefix := "tai-" + c.plugin + "-"
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		p := filepath.Join(c.target, e.Name())
		if err := os.RemoveAll(p); err != nil {
			return errcode.Wrapf(errcode.InternalError, err,
				"remove %s", p)
		}
	}
	return nil
}

// copyPluginCategory copies every entry in `<pluginDir>/assets/<sub>/`
// into the appropriate place under c.target. For commands the
// destination is `c.target/tai-<plugin>/`; for skills/agents the
// destination is `c.target/` directly. Existing same-name entries
// are overwritten in place; nothing outside the shipped set is
// touched (pruning is the caller's separate, after-success step).
//
// Returns the set of entry names shipped by this version, for the
// prune step. A missing source category returns an empty (non-nil)
// set — every previously-shipped entry is then stale.
func copyPluginCategory(pluginDir string, c assetCategory) (map[string]bool, error) {
	shipped := map[string]bool{}
	src := filepath.Join(pluginDir, "assets", c.sub)
	entries, err := os.ReadDir(src)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return shipped, nil
		}
		return nil, errcode.Wrapf(errcode.InternalError, err, "read %s", src)
	}
	dstBase := c.namespaceBase()
	if len(entries) > 0 {
		if err := os.MkdirAll(dstBase, 0o755); err != nil {
			return nil, errcode.Wrapf(errcode.InternalError, err, "mkdir %s", dstBase)
		}
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dstBase, e.Name())
		if e.IsDir() {
			// Replace directory entries wholesale so files a prior
			// version nested inside the same-named folder don't
			// linger. Blast radius on failure is this one entry.
			if err := os.RemoveAll(d); err != nil {
				return nil, errcode.Wrapf(errcode.InternalError, err, "remove %s", d)
			}
			if err := copyDir(s, d); err != nil {
				return nil, err
			}
		} else if err := copyFile(s, d); err != nil {
			return nil, err
		}
		shipped[e.Name()] = true
	}
	return shipped, nil
}

// prunePluginNamespace removes entries in the plugin's namespace that
// the just-copied version no longer ships. Runs only after every copy
// succeeded, so a failed sync never deletes anything.
func prunePluginNamespace(c assetCategory, shipped map[string]bool) error {
	base := c.namespaceBase()
	if c.isCommands && len(shipped) == 0 {
		// The version ships no commands — the wholly-owned
		// `tai-<plugin>/` dir is stale in its entirety.
		if err := os.RemoveAll(base); err != nil {
			return errcode.Wrapf(errcode.InternalError, err, "remove %s", base)
		}
		return nil
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return errcode.Wrapf(errcode.InternalError, err, "read %s", base)
	}
	prefix := "tai-" + c.plugin + "-"
	for _, e := range entries {
		if shipped[e.Name()] {
			continue
		}
		// Inside `tai-<plugin>/` every entry is plugin-owned; in the
		// shared skills/agents dir only prefixed entries are.
		if !c.isCommands && !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		p := filepath.Join(base, e.Name())
		if err := os.RemoveAll(p); err != nil {
			return errcode.Wrapf(errcode.InternalError, err, "remove %s", p)
		}
	}
	return nil
}

// namespaceBase returns the directory the plugin's namespace lives
// in for this category: `<target>/tai-<plugin>/` for commands, the
// shared category dir itself for skills/agents.
func (c assetCategory) namespaceBase() string {
	if c.isCommands {
		return filepath.Join(c.target, "tai-"+c.plugin)
	}
	return c.target
}

// copyFile reads src bytes and writes them to dst, preserving the
// source file's permission bits (0o755 for executable scripts,
// 0o644 for regular files, etc.). Parent directory must exist;
// copyDir handles that for nested trees.
//
// Preserving the mode is load-bearing for plugin bundles: a plugin
// that ships an executable helper under `assets/skills/` MUST keep
// the executable bit through the cross-device copy path
// (atomicReplaceDir's fallback). Pre-Phase-4 the copy unconditionally
// wrote 0o644 and silently stripped the bit.
func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return errcode.Wrapf(errcode.InternalError, err, "stat %s", src)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return errcode.Wrapf(errcode.InternalError, err, "read %s", src)
	}
	if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		return errcode.Wrapf(errcode.InternalError, err, "write %s", dst)
	}
	return nil
}

// copyDir recursively copies src → dst. Used for skill/agent
// directories that ship as a folder (e.g. a skill bundled with its
// reference files).
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}
