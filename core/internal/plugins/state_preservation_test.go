package plugins_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/plugins"
)

// A plugin's runtime state lives at
// <dataDir>/plugins/<name>/state/ — inside the very directory an
// install replaces. Reinstalling or updating a plugin must not
// destroy it: for triage that directory holds the SQLite database
// with every imported review comment and every triage decision made
// against it, none of which is recoverable from the release tarball.
//
// Remove already parks and restores state/ around its wipe. Install
// did not, so `tai plugins update <name>` silently deleted the
// plugin's entire database.
func TestInstall_TCPLG024_preserves_plugin_state_across_reinstall(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.File{Targets: []config.Target{{Root: t.TempDir()}}}
	bundle := stageContractBundle(t, "demo", answersHelpSummary("Does the thing."), "populated")

	install := func() error {
		_, err := plugins.Install(context.Background(), "demo", dataDir, cfg, plugins.InstallOptions{
			Source:  plugins.Source{Host: "github.com", Repo: "acme/demo"},
			Fetcher: &fakeFetcher{source: bundle, version: "v1.0.0"},
		})
		return err
	}

	if err := install(); err != nil {
		t.Fatalf("first install: %v", err)
	}

	// Stand in for the plugin's database plus a nested file, so the
	// test fails the same way whether the bug loses the directory or
	// only its contents.
	stateDir := filepath.Join(dataDir, "plugins", "demo", "state")
	if err := os.MkdirAll(filepath.Join(stateDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(stateDir, "demo.db")
	deep := filepath.Join(stateDir, "nested", "keep.txt")
	if err := os.WriteFile(db, []byte("irreplaceable"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deep, []byte("also irreplaceable"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := install(); err != nil {
		t.Fatalf("reinstall: %v", err)
	}

	got, err := os.ReadFile(db)
	if err != nil {
		t.Fatalf("plugin database destroyed by reinstall: %v", err)
	}
	if string(got) != "irreplaceable" {
		t.Errorf("database contents = %q, want %q", got, "irreplaceable")
	}
	if deepGot, err := os.ReadFile(deep); err != nil || string(deepGot) != "also irreplaceable" {
		t.Errorf("nested state file not preserved: (%q, %v)", deepGot, err)
	}

	// The rest of the install must still have been replaced.
	if _, err := os.Stat(filepath.Join(dataDir, "plugins", "demo", "demo")); err != nil {
		t.Errorf("binary missing after reinstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "plugins", "demo", "assets", "commands", "go.md")); err != nil {
		t.Errorf("assets missing after reinstall: %v", err)
	}
}

// A first install has no state to preserve — the preservation step
// must be a no-op rather than an error.
func TestInstall_TCPLG024_first_install_without_state_is_fine(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.File{Targets: []config.Target{{Root: t.TempDir()}}}
	bundle := stageContractBundle(t, "demo", answersHelpSummary("Does the thing."), "populated")

	if _, err := plugins.Install(context.Background(), "demo", dataDir, cfg, plugins.InstallOptions{
		Source:  plugins.Source{Host: "github.com", Repo: "acme/demo"},
		Fetcher: &fakeFetcher{source: bundle, version: "v1.0.0"},
	}); err != nil {
		t.Fatalf("install with no prior state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "plugins", "demo", "demo")); err != nil {
		t.Errorf("binary missing: %v", err)
	}
}
