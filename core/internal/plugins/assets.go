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
// every configured target, applying the namespacing rules. Before
// each copy it removes the plugin's existing namespace in that
// target so stale entries from prior installs are cleaned up.
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
		skills, commands, agents := t.EffectiveSubpaths()
		categories := []assetCategory{
			{sub: "skills", target: skills, plugin: pluginName, root: t.Root, isCommands: false},
			{sub: "agents", target: agents, plugin: pluginName, root: t.Root, isCommands: false},
			{sub: "commands", target: commands, plugin: pluginName, root: t.Root, isCommands: true},
		}
		for _, c := range categories {
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
			if err := wipePluginNamespace(c); err != nil {
				return err
			}
			if err := copyPluginCategory(pluginDir, c); err != nil {
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
		skills, commands, agents := t.EffectiveSubpaths()
		for _, c := range []assetCategory{
			{sub: "skills", target: skills, plugin: pluginName, root: t.Root, isCommands: false},
			{sub: "agents", target: agents, plugin: pluginName, root: t.Root, isCommands: false},
			{sub: "commands", target: commands, plugin: pluginName, root: t.Root, isCommands: true},
		} {
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
		dir := filepath.Join(c.target, "tai-"+c.plugin)
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
// destination is `c.target/` directly.
func copyPluginCategory(pluginDir string, c assetCategory) error {
	src := filepath.Join(pluginDir, "assets", c.sub)
	entries, err := os.ReadDir(src)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return errcode.Wrapf(errcode.InternalError, err, "read %s", src)
	}
	var dstBase string
	if c.isCommands {
		dstBase = filepath.Join(c.target, "tai-"+c.plugin)
	} else {
		dstBase = c.target
	}
	if err := os.MkdirAll(dstBase, 0o755); err != nil {
		return errcode.Wrapf(errcode.InternalError, err, "mkdir %s", dstBase)
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dstBase, e.Name())
		if e.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(s, d); err != nil {
			return err
		}
	}
	return nil
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
