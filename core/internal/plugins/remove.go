package plugins

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

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

	// Move the state subdir aside, wipe the install dir, then put it
	// back. Parking before the wipe rather than re-creating after it
	// avoids a window where a concurrent plugin invocation sees a
	// missing state path.
	parked, err := parkPluginState(installDir)
	if err != nil {
		return nil, err
	}
	retained := ""
	if parked.held() {
		retained = parked.statePath
	}

	if err := os.RemoveAll(installDir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"remove %s", installDir)
	}
	if err := parked.restore(); err != nil {
		return nil, err
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
