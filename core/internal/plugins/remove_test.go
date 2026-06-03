package plugins_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/plugins"
)

// TestRemove_TCPLG014_preserves_runtime_state exercises TC-PLG-014.
func TestRemove_TCPLG014_preserves_runtime_state(t *testing.T) {
	dataDir := t.TempDir()
	tgt := stageTarget(t)
	cfg := &config.File{Targets: []config.Target{tgt}}

	// Install a plugin so we have something to remove.
	bundle := stagePluginBundle(t, "triage", map[string]string{
		"skills/tai-triage-pulse.md": "x",
		"commands/import.md":         "x",
	})
	if _, err := plugins.Install(context.Background(), "triage", dataDir, cfg, plugins.InstallOptions{
		Source:  plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
		Fetcher: &fakeFetcher{source: bundle, version: "v0.5.0"},
	}); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Pre-seed a runtime-state file the plugin would have created.
	statePath := filepath.Join(plugins.PluginInstallDir(dataDir, "triage"), "state", "triage.db")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	const dbBody = "fake-sqlite-bytes"
	if err := os.WriteFile(statePath, []byte(dbBody), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	var stderr bytes.Buffer
	res, err := plugins.Remove("triage", dataDir, cfg, plugins.RemoveOptions{Stderr: &stderr})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}

	// Binary + assets/ are gone.
	binPath := plugins.PluginBinaryPath(dataDir, "triage")
	if _, err := os.Stat(binPath); !os.IsNotExist(err) {
		t.Errorf("binary should be removed; stat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(plugins.PluginInstallDir(dataDir, "triage"), "assets")); !os.IsNotExist(err) {
		t.Errorf("assets/ should be removed; stat: %v", err)
	}

	// Target namespace is wiped.
	if _, err := os.Stat(filepath.Join(tgt.Root, "skills", "tai-triage-pulse.md")); !os.IsNotExist(err) {
		t.Errorf("target skill should be removed; stat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tgt.Root, "commands", "tai-triage")); !os.IsNotExist(err) {
		t.Errorf("target commands/tai-triage should be removed; stat: %v", err)
	}

	// State entry gone.
	st, _ := plugins.LoadState(dataDir)
	if _, idx := st.Find("triage"); idx >= 0 {
		t.Errorf("state should no longer reference triage")
	}

	// Runtime state file survives.
	body, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("runtime state should survive: %v", err)
	}
	if string(body) != dbBody {
		t.Errorf("runtime state bytes changed: %q", body)
	}

	// stderr names the retained `state/` directory (the spec says
	// "MUST name the retained data path" — granularity is the
	// directory, since plugins put arbitrary files under state/).
	stateDir := filepath.Dir(statePath)
	if !strings.Contains(stderr.String(), stateDir) {
		t.Errorf("stderr should name retained state dir %s; got: %q", stateDir, stderr.String())
	}
	if res.RetainedState != stateDir {
		t.Errorf("RetainedState: want %s, got %s", stateDir, res.RetainedState)
	}
}
