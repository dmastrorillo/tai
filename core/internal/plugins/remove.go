package plugins

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// RemoveOptions carries the io sink and target list for Remove.
type RemoveOptions struct {
	Stderr io.Writer
}

// RemoveResult records what Remove did so the CLI verb can render a
// summary. RetainedState is the absolute path of the preserved
// `state/` subdirectory (empty when the plugin had no state).
type RemoveResult struct {
	WipedTargets  int
	RetainedState string
}

// Remove uninstalls the plugin named `name`. Per spec, the plugin's
// own runtime state under `<dataDir>/plugins/<name>/state/` is
// preserved; everything else (the binary, the `assets/` folder,
// every namespaced asset in every configured target, and the
// plugins.json entry) is deleted. The stderr writer receives a
// reminder naming the retained path when one exists.
//
// Returns `*errcode.Error{Code: PluginUnknown}` when no install
// record exists for name.
func Remove(name string, dataDir string, cfg *config.File, opts RemoveOptions) (*RemoveResult, error) {
	state, err := LoadState(dataDir)
	if err != nil {
		return nil, err
	}
	if _, idx := state.Find(name); idx < 0 {
		return nil, errcode.Newf(errcode.PluginUnknown,
			"no installed plugin named %q to remove", name).
			WithHelp(
				"check `tai plugins list` to see what's installed",
			)
	}

	// Wipe target namespace first; if a target wipe fails, the
	// install dir is still intact so the user can retry.
	if err := WipePluginFromTargets(name, cfg.Targets); err != nil {
		return nil, err
	}

	installDir := PluginInstallDir(dataDir, name)
	statePath := filepath.Join(installDir, "state")
	retained := ""
	if info, statErr := os.Stat(statePath); statErr == nil && info.IsDir() {
		retained = statePath
	}

	// Move the install dir's state subdir aside (if any), then wipe
	// the install dir, then put state back. This sequence avoids a
	// "remove + then re-create" race where a concurrent plugin
	// invocation might see a missing state path mid-flight.
	//
	// The wrapper temp dir is only cleaned up after the state was
	// successfully restored — if the restore Rename fails, the
	// wrapper is the only surviving copy of the plugin's runtime
	// state and MUST NOT be wiped.
	parked := ""
	restored := false
	if retained != "" {
		tmp, err := os.MkdirTemp(filepath.Dir(installDir), "tai-state-keep-")
		if err != nil {
			return nil, errcode.Wrapf(errcode.InternalError, err,
				"create state-keep tmp dir")
		}
		parked = filepath.Join(tmp, "state")
		if err := os.Rename(statePath, parked); err != nil {
			return nil, errcode.Wrapf(errcode.InternalError, err,
				"park state %s", statePath)
		}
		defer func() {
			if restored {
				_ = os.RemoveAll(filepath.Dir(parked))
			}
		}()
	}
	if err := os.RemoveAll(installDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"remove %s", installDir)
	}
	if parked != "" {
		if err := os.MkdirAll(installDir, 0o755); err != nil {
			return nil, errcode.Wrapf(errcode.InternalError, err,
				"recreate %s for state restore", installDir)
		}
		if err := os.Rename(parked, statePath); err != nil {
			return nil, errcode.Wrapf(errcode.InternalError, err,
				"restore state %s", statePath).
				WithHelp(
					"the plugin's runtime state is parked at "+parked,
					"recover it manually before re-running `tai plugins remove`",
				)
		}
		restored = true
	}

	// Update state last so a failure above leaves the listing
	// consistent with disk.
	state.Remove(name)
	if err := SaveState(dataDir, state); err != nil {
		return nil, err
	}

	if retained != "" && opts.Stderr != nil {
		_, _ = fmt.Fprintf(opts.Stderr,
			"[tai] kept %s — plugin's own runtime state; delete manually if no longer needed\n",
			retained)
	}
	return &RemoveResult{
		WipedTargets:  len(cfg.Targets),
		RetainedState: retained,
	}, nil
}
