package plugins

import (
	"context"
	"io"
	"time"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// UpdateOptions mirrors InstallOptions for the update flow. The
// Source recorded at install time is the source of truth — `update`
// re-fetches from there. Callers MAY override Version (e.g. to pin a
// specific tag); leaving it empty means "latest".
type UpdateOptions struct {
	Version string
	Fetcher Fetcher
	Stderr  io.Writer
}

// Update re-fetches the plugin named `name` from the source recorded
// at install time, replaces the install directory, re-syncs assets
// to every configured target, and updates the state file.
//
// Returns `*errcode.Error{Code: PLUGIN_UNKNOWN}` when no install
// record exists for name — update is a "replace what's there"
// operation, not an alternate install path.
//
// Implementation note: Update reads the existing record's source
// then delegates to Install. The freshly-stamped InstalledAt is
// passed through InstallOptions so the state file is written exactly
// once per update — no double-save TOCTOU window between two writes.
func Update(ctx context.Context, name string, dataDir string, cfg *config.File, opts UpdateOptions) (*Entry, error) {
	state, err := LoadState(dataDir)
	if err != nil {
		return nil, err
	}
	prior, idx := state.Find(name)
	if idx < 0 {
		return nil, errcode.Newf(errcode.PluginUnknown,
			"no installed plugin named %q to update", name).
			WithHelp(
				"install it first: `tai plugins "+name+" install`",
				"or run `tai plugins list` to see what's installed",
			)
	}

	src := prior.Source
	if opts.Version != "" {
		src.Version = opts.Version
	} else {
		src.Version = ""
	}

	return Install(ctx, name, dataDir, cfg, InstallOptions{
		Source:      src,
		Version:     opts.Version,
		Fetcher:     opts.Fetcher,
		Stderr:      opts.Stderr,
		InstalledAt: time.Now().UTC(),
	})
}
