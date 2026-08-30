package plugins

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/verbs"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// InstallOptions carries everything Install needs that does not flow
// from cfg/dataDir. Source comes from `--source` on the CLI; Version
// from `--version`; Fetcher is injected so tests can avoid the
// network.
type InstallOptions struct {
	// Source overrides the built-in registry. Optional. When empty,
	// Install consults the built-in registry; an unresolved name in
	// that case surfaces as PLUGIN_UNKNOWN.
	Source Source

	// Version overrides the installed version. Optional. Empty means
	// "latest" (the host's Releases API resolves the symbol).
	Version string

	// Fetcher is the implementation that downloads + unpacks the
	// release asset. Nil falls back to a default HTTPFetcher.
	Fetcher Fetcher

	// Stderr receives non-fatal warnings (e.g. falsy-skip notices
	// during the asset-sync phase).
	Stderr io.Writer

	// InstalledAt overrides the InstalledAt timestamp recorded in
	// the state file. Zero (default) means Install stamps it with
	// the current UTC time. Update sets this so the state save
	// happens exactly once per update.
	InstalledAt time.Time
}

// Install installs the plugin named `name` per
// `specs/plugin-host/spec.md` §"Plugin install". The flow:
//
//  1. Refuse reserved verb names (PLUGIN_NAME_RESERVED).
//  2. Resolve the source: explicit opts.Source > built-in registry.
//     A miss with no explicit source surfaces PLUGIN_UNKNOWN.
//  3. Stage a temp directory; fetch + unpack the release asset.
//  4. Validate the bundle's asset namespacing (PLUGIN_ASSET_NAMING).
//  5. Atomically replace any prior install under
//     `<dataDir>/plugins/<name>/`.
//  6. Sync the bundle's `assets/` into every configured target,
//     applying the namespacing rules. Falsy sub-paths warn and
//     skip.
//  7. Upsert the entry in `<dataDir>/state/plugins.json`.
//
// Returns the *Entry that was recorded so callers (the CLI verb)
// can render a summary.
func Install(ctx context.Context, name string, dataDir string, cfg *config.File, opts InstallOptions) (*Entry, error) {
	if verbs.IsReserved(name) {
		return nil, errcode.Newf(errcode.PluginNameReserved,
			"plugin name %q collides with a reserved top-level verb", name).
			WithHelp(
				"pick a different name for the plugin",
				"reserved verbs: "+strings.Join(verbs.Reserved(), ", "),
			)
	}

	src, err := resolveSource(name, opts.Source, opts.Version)
	if err != nil {
		return nil, err
	}

	fetcher := opts.Fetcher
	if fetcher == nil {
		fetcher = &HTTPFetcher{}
	}

	stagingDir, err := os.MkdirTemp("", "tai-plugin-install-")
	if err != nil {
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"create staging dir")
	}
	// Always clean staging — either it got promoted, or it stays
	// behind as garbage. Best-effort RemoveAll handles both.
	defer func() { _ = os.RemoveAll(stagingDir) }()

	resolvedTag, err := fetcher.Fetch(ctx, name, src, stagingDir)
	if err != nil {
		return nil, err
	}

	// Contract checks run against the staging directory, before
	// anything is promoted: a bundle that fails either one leaves no
	// trace, and any prior install of the same plugin stays intact.
	if err := RequireAssetsDir(stagingDir, name); err != nil {
		return nil, err
	}
	if err := ValidateAssetNamespace(stagingDir, name); err != nil {
		return nil, err
	}
	description, err := ReadHelpSummary(ctx, stagingDir, name)
	if err != nil {
		return nil, err
	}

	finalDir := filepath.Join(dataDir, "plugins", name)
	if err := atomicReplaceDir(stagingDir, finalDir); err != nil {
		return nil, err
	}

	// Ensure the binary is executable. The tarball SHOULD already
	// have the mode set, but the most common cause of "plugin
	// silently fails to run" is a missing +x bit; an extra chmod is
	// cheap insurance.
	binName := name
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(finalDir, binName)
	if info, statErr := os.Stat(binPath); statErr == nil && !info.IsDir() {
		_ = os.Chmod(binPath, info.Mode()|0o111)
	}

	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	if err := SyncAssetsToTargets(finalDir, name, cfg.Targets, stderr); err != nil {
		return nil, err
	}

	src.Version = resolvedTag
	installedAt := opts.InstalledAt
	if installedAt.IsZero() {
		installedAt = time.Now().UTC()
	}
	entry := Entry{
		Name:        name,
		Source:      src,
		Version:     resolvedTag,
		InstalledAt: installedAt,
		Description: description,
	}
	state, err := LoadState(dataDir)
	if err != nil {
		return nil, err
	}
	state.Upsert(entry)
	if err := SaveState(dataDir, state); err != nil {
		return nil, err
	}
	return &entry, nil
}

// resolveSource derives the fetch Source for `name`. Precedence:
//
//  1. Explicit opts.Source (CLI `--source` flag).
//  2. Built-in registry hit.
//  3. PLUGIN_UNKNOWN.
//
// The version from opts is layered onto whichever wins, so e.g. a
// registry hit with a `--version` flag uses the registry's host/repo
// at the user-specified tag.
func resolveSource(name string, override Source, version string) (Source, error) {
	if !override.Empty() {
		if version != "" {
			override.Version = version
		}
		return override, nil
	}
	if src, ok := Lookup(name); ok {
		if version != "" {
			src.Version = version
		}
		return src, nil
	}
	return Source{}, errcode.Newf(errcode.PluginUnknown,
		"no plugin named %q in the built-in registry and no `--source` supplied", name).
		WithHelp(
			"pass an explicit source: `tai plugins install "+name+" --source <host>/<org>/<repo>`",
			"or check the spelling against `tai plugins list` (for already-installed plugins)",
		)
}

// atomicReplaceDir replaces dst with the contents of src on disk.
//
// The flow:
//
//  1. Stage src into a sibling temp directory inside dst's parent so
//     the final hop is guaranteed same-device. When src is already
//     on the same device as dst, os.Rename moves the inode; when it
//     isn't (e.g. /tmp on tmpfs vs $HOME on ext4 in many container
//     images), we fall back to copyDir into the sibling.
//  2. RemoveAll(dst) — this happens AFTER the new content is staged
//     adjacent to dst so a failure before this point leaves the
//     prior install intact.
//  3. Rename the staged sibling onto dst. Same-device, atomic on
//     POSIX.
//
// If any step fails, atomicReplaceDir cleans up the staged sibling
// before returning so the disk isn't littered with retry debris.
//
// Pre-Phase-4 behaviour: removed dst first, then tried rename, then
// fell back to a copy that wrote on top of the empty dst. That
// design had a data-loss window if the copy failed — review
// surfaced it. The new flow eliminates that window.
func atomicReplaceDir(src, dst string) error {
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return errcode.Wrapf(errcode.InternalError, err, "mkdir %s", parent)
	}

	// Stage on dst's volume so the final rename is same-device.
	staged, err := os.MkdirTemp(parent, ".tai-plugin-staging-")
	if err != nil {
		return errcode.Wrapf(errcode.InternalError, err,
			"create staging dir under %s", parent)
	}
	stagedInner := filepath.Join(staged, "payload")

	cleanup := func() { _ = os.RemoveAll(staged) }

	// Try moving src wholesale onto the dst-volume staging slot.
	// Same-device → atomic rename. Cross-device → manual copy.
	if err := os.Rename(src, stagedInner); err != nil {
		if !isCrossDeviceErr(err) {
			cleanup()
			return errcode.Wrapf(errcode.InternalError, err,
				"stage %s -> %s", src, stagedInner)
		}
		if err := copyDir(src, stagedInner); err != nil {
			cleanup()
			return err
		}
	}

	// New content is on dst's volume — only NOW remove the prior
	// install. From here the final Rename is same-device and
	// effectively atomic.
	if err := os.RemoveAll(dst); err != nil {
		cleanup()
		return errcode.Wrapf(errcode.InternalError, err, "remove %s", dst)
	}
	if err := os.Rename(stagedInner, dst); err != nil {
		cleanup()
		return errcode.Wrapf(errcode.InternalError, err,
			"rename %s -> %s", stagedInner, dst)
	}
	cleanup()
	return nil
}

// isCrossDeviceErr reports whether err is the EXDEV-flavoured error
// `os.Rename` returns when src and dst live on different
// filesystems. errors.Is doesn't currently recognise the platform-
// specific code reliably across systems; this helper falls back to a
// substring match as a portable approximation.
func isCrossDeviceErr(err error) bool {
	var le *os.LinkError
	if errors.As(err, &le) {
		errStr := strings.ToLower(le.Err.Error())
		return strings.Contains(errStr, "cross-device") ||
			strings.Contains(errStr, "invalid cross-device link") ||
			strings.Contains(errStr, "exdev")
	}
	return false
}

// PluginInstallDir returns the canonical install location for a
// plugin. Exported because update/remove/invoke all need the same
// path and centralising the join keeps them consistent.
func PluginInstallDir(dataDir, name string) string {
	return filepath.Join(dataDir, "plugins", name)
}

// PluginBinaryPath returns the absolute path of the plugin's binary
// inside its install directory. Adds `.exe` on Windows.
func PluginBinaryPath(dataDir, name string) string {
	bin := name
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	return filepath.Join(PluginInstallDir(dataDir, name), bin)
}
