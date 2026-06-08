// `plugins.yml` auto-install hook for `tai sync`.
//
// At the start of every sync, after the clone is in place and the
// (best-effort) fetch has run, we read `<clone>/plugins.yml` (if
// present) and install any plugin it lists that isn't already in
// the local state. The schema is intentionally tiny:
//
//	plugins:
//	  - name: triage
//	  - name: acme-custom
//	    source: github.com/acme/tai-plugin-custom
//	    version: v1.2.0
//
// The list is additive — removing an entry does NOT uninstall the
// plugin from a developer's machine. Removal is exclusively a user
// gesture via `tai plugins remove <name>`.

package sync

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/plugins"
)

// pluginsYAML mirrors the on-disk schema of `<clone>/plugins.yml`.
type pluginsYAML struct {
	Plugins []pluginsYAMLEntry `yaml:"plugins"`
}

// pluginsYAMLEntry is one row of the `plugins:` list. Source/Version
// are optional — when both are empty, the built-in first-party
// registry resolves the name.
type pluginsYAMLEntry struct {
	Name    string `yaml:"name"`
	Source  string `yaml:"source,omitempty"`
	Version string `yaml:"version,omitempty"`
}

// AutoInstallFn is the signature for the auto-install seam tests
// override via AutoInstallForTesting. Production binds it to
// plugins.Install directly; tests substitute a no-network stub that
// records the call.
type AutoInstallFn func(ctx context.Context, name, dataDir string, cfg *config.File, opts plugins.InstallOptions) (*plugins.Entry, error)

// autoInstallFunc is the indirection. Default: plugins.Install.
var autoInstallFunc AutoInstallFn = func(ctx context.Context, name, dataDir string, cfg *config.File, opts plugins.InstallOptions) (*plugins.Entry, error) {
	return plugins.Install(ctx, name, dataDir, cfg, opts)
}

// AutoInstallForTesting swaps the auto-install function for the
// lifetime of t. The strict default is restored via t.Cleanup so
// every test starts from a clean state.
//
// `testing.TB` makes accidental production use a code-review red
// flag — the only path to call this is from a `_test.go` file or a
// binary that deliberately imports `testing`.
func AutoInstallForTesting(t testing.TB, fn AutoInstallFn) {
	t.Helper()
	prev := autoInstallFunc
	autoInstallFunc = fn
	t.Cleanup(func() { autoInstallFunc = prev })
}

// readPluginsYAML returns the parsed entries from
// `<cloneDir>/plugins.yml`, or (nil, nil) when the file is absent.
// Parse errors surface as a plain error; the caller wraps them with
// the appropriate `errcode`.
func readPluginsYAML(cloneDir string) ([]pluginsYAMLEntry, error) {
	p := filepath.Join(cloneDir, "plugins.yml")
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var doc pluginsYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc.Plugins, nil
}

// autoInstallPluginsFromYAML iterates the plugins.yml entries and
// installs each one not already present in plugins.json state.
// Already-installed entries are passed over silently (additive
// semantics). The first install error halts the sequence.
//
// Side effects: writes binaries / assets per plugins.Install. No
// state mutation when the YAML file is absent or empty.
func autoInstallPluginsFromYAML(ctx context.Context, cloneDir, dataDir string, cfg *config.File, stderr io.Writer) error {
	entries, err := readPluginsYAML(cloneDir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	state, err := plugins.LoadState(dataDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if _, idx := state.Find(e.Name); idx >= 0 {
			continue
		}
		_, installErr := autoInstallFunc(ctx, e.Name, dataDir, cfg, plugins.InstallOptions{
			Source:  plugins.ParseSource(e.Source),
			Version: e.Version,
			Stderr:  stderr,
		})
		if installErr != nil {
			return installErr
		}
		// Reload after each install so the next iteration sees a
		// freshly-installed plugin (avoids re-installing if the
		// YAML file references the same name twice).
		state, err = plugins.LoadState(dataDir)
		if err != nil {
			return err
		}
	}
	return nil
}
