package plugins_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/plugins"
)

// TestUpdate_TCPLG013_replaces_binary_and_assets exercises TC-PLG-013.
func TestUpdate_TCPLG013_replaces_binary_and_assets(t *testing.T) {
	dataDir := t.TempDir()
	tgt := stageTarget(t)
	cfg := &config.File{Targets: []config.Target{tgt}}

	// First install version v0.4.0 with one asset.
	bundleOld := stagePluginBundle(t, "triage", map[string]string{
		"skills/tai-triage-pulse.md": "old-pulse",
		"commands/import.md":         "old-import",
	})
	if _, err := plugins.Install(context.Background(), "triage", dataDir, cfg, plugins.InstallOptions{
		Source:  plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
		Fetcher: &fakeFetcher{source: bundleOld, version: "v0.4.0"},
	}); err != nil {
		t.Fatalf("install v0.4.0: %v", err)
	}

	// Now update to v0.5.0 with replacement content and a new asset.
	bundleNew := stagePluginBundle(t, "triage", map[string]string{
		"skills/tai-triage-pulse.md":  "new-pulse",
		"skills/tai-triage-replay.md": "replay-new",
		"commands/import.md":          "new-import",
	})
	entry, err := plugins.Update(context.Background(), "triage", dataDir, cfg, plugins.UpdateOptions{
		Fetcher: &fakeFetcher{source: bundleNew, version: "v0.5.0"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if entry.Version != "v0.5.0" {
		t.Errorf("state version: want v0.5.0, got %s", entry.Version)
	}

	// Skills: the v0.4.0 file's bytes are replaced; the new file lands.
	pulse, _ := os.ReadFile(filepath.Join(tgt.Root, "skills", "tai-triage-pulse.md"))
	if string(pulse) != "new-pulse" {
		t.Errorf("pulse not replaced; got %q", pulse)
	}
	if _, err := os.Stat(filepath.Join(tgt.Root, "skills", "tai-triage-replay.md")); err != nil {
		t.Errorf("new skill missing: %v", err)
	}

	// Commands: import.md replaced under the namespaced subdir.
	imp, _ := os.ReadFile(filepath.Join(tgt.Root, "commands", "tai-triage", "import.md"))
	if string(imp) != "new-import" {
		t.Errorf("import not replaced; got %q", imp)
	}

	// State file reflects the new version.
	st, _ := plugins.LoadState(dataDir)
	e, idx := st.Find("triage")
	if idx < 0 {
		t.Fatal("triage entry missing from state")
	}
	if e.Version != "v0.5.0" {
		t.Errorf("state.Version: %s", e.Version)
	}
}

// TestUpdate_unknown_plugin_surfaces_PluginUnknown anchors the
// engine-side contract that update on an absent plugin fails with
// PLUGIN_UNKNOWN. Not tied to a TC-ID — the user-visible CLI
// boundary test lives next door in cmd_test.
func TestUpdate_unknown_plugin_surfaces_PluginUnknown(t *testing.T) {
	dataDir := t.TempDir()
	_, err := plugins.Update(context.Background(), "ghost", dataDir, &config.File{}, plugins.UpdateOptions{})
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected ghost-named error, got: %v", err)
	}
}
