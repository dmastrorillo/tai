package plugins_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/plugins"
	"github.com/dmastrorillo/tai/pkg/errcode"
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

// TC-PLG-024 — the same guarantee on the update path. Update
// currently delegates to Install, so this is a thin delegate today;
// it is asserted directly because the reported symptom was
// `tai plugins update`, and a future change that stops delegating
// cleanly would otherwise pass the suite while destroying the
// database again.
func TestUpdate_TCPLG024_preserves_plugin_state(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.File{Targets: []config.Target{{Root: t.TempDir()}}}
	bundle := stageContractBundle(t, "demo", answersHelpSummary("Does the thing."), "populated")
	fetch := &fakeFetcher{source: bundle, version: "v1.0.0"}

	if _, err := plugins.Install(context.Background(), "demo", dataDir, cfg, plugins.InstallOptions{
		Source:  plugins.Source{Host: "github.com", Repo: "acme/demo"},
		Fetcher: fetch,
	}); err != nil {
		t.Fatalf("install: %v", err)
	}

	db := filepath.Join(dataDir, "plugins", "demo", "state", "demo.db")
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db, []byte("irreplaceable"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := plugins.Update(context.Background(), "demo", dataDir, cfg, plugins.UpdateOptions{
		Fetcher: fetch,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := os.ReadFile(db)
	if err != nil {
		t.Fatalf("plugin database destroyed by update: %v", err)
	}
	if string(got) != "irreplaceable" {
		t.Errorf("database contents = %q, want %q", got, "irreplaceable")
	}
}

// TC-PLG-025 — when the restore itself cannot complete, the parked
// copy is the only one left, so it must survive and the error must
// say where it is.
//
// The trigger needs no injection: RequireAssetsDir and
// ValidateAssetNamespace only inspect `assets/`, so a bundle may ship
// its own top-level `state/`. atomicReplaceDir promotes it, and the
// restore rename then lands on a non-empty directory, which POSIX
// refuses.
func TestInstall_TCPLG025_failed_restore_keeps_the_parked_copy(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.File{Targets: []config.Target{{Root: t.TempDir()}}}

	first := stageContractBundle(t, "demo", answersHelpSummary("Does the thing."), "populated")
	if _, err := plugins.Install(context.Background(), "demo", dataDir, cfg, plugins.InstallOptions{
		Source:  plugins.Source{Host: "github.com", Repo: "acme/demo"},
		Fetcher: &fakeFetcher{source: first, version: "v1.0.0"},
	}); err != nil {
		t.Fatalf("first install: %v", err)
	}
	db := filepath.Join(dataDir, "plugins", "demo", "state", "demo.db")
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db, []byte("irreplaceable"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A second bundle that ships its own state/, occupying the path
	// the parked copy has to return to.
	second := stageContractBundle(t, "demo", answersHelpSummary("Does the thing."), "populated")
	occupied := filepath.Join(second, "state", "theirs.txt")
	if err := os.MkdirAll(filepath.Dir(occupied), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(occupied, []byte("from the tarball"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := plugins.Install(context.Background(), "demo", dataDir, cfg, plugins.InstallOptions{
		Source:  plugins.Source{Host: "github.com", Repo: "acme/demo"},
		Fetcher: &fakeFetcher{source: second, version: "v2.0.0"},
	})
	if err == nil {
		t.Fatal("want an error when the state cannot be restored")
	}

	e, ok := errcode.As(err)
	if !ok {
		t.Fatalf("want *errcode.Error, got %T", err)
	}
	help := strings.Join(e.Help, " ")
	if !strings.Contains(help, "tai-state-keep-") {
		t.Errorf("error must name the parked location, got help: %q", help)
	}

	// The parked copy must still be on disk, contents intact.
	parked := findParkedState(t, filepath.Join(dataDir, "plugins"))
	got, readErr := os.ReadFile(filepath.Join(parked, "demo.db"))
	if readErr != nil {
		t.Fatalf("parked state was cleaned up despite the failed restore: %v", readErr)
	}
	if string(got) != "irreplaceable" {
		t.Errorf("parked contents = %q, want %q", got, "irreplaceable")
	}
}

// findParkedState locates the single tai-state-keep-*/state directory
// left behind under root.
func findParkedState(t *testing.T, root string) string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "tai-state-keep-") {
			return filepath.Join(root, e.Name(), "state")
		}
	}
	t.Fatalf("no parked state directory under %s", root)
	return ""
}

// A `state` entry that is not a directory is out of contract — the
// wire contract specifies `state/` — and is deliberately replaced
// with the rest of the tarball's namespace rather than preserved.
// Asserted so the behaviour reads as a decision rather than an
// oversight.
func TestInstall_TCPLG024_non_directory_state_entry_is_not_preserved(t *testing.T) {
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

	stateFile := filepath.Join(dataDir, "plugins", "demo", "state")
	if err := os.WriteFile(stateFile, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := install(); err != nil {
		t.Fatalf("reinstall over a non-directory state entry: %v", err)
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Errorf("a non-directory `state` entry is out of contract and should not survive, stat: %v", err)
	}
}
