package cmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/core/internal/plugins"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// pluginsEnv stages a fresh TAI_CONFIG + TAI_DATA_DIR pair, with no
// targets configured by default. Returns the resolved dataDir.
//
// Not tied to a TC-ID — test fixture helper.
func pluginsEnv(t *testing.T) string {
	t.Helper()

	dataDir := t.TempDir()
	t.Setenv("TAI_DATA_DIR", dataDir)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	t.Setenv("TAI_CONFIG", cfgPath)
	body := "update-check-interval: 0\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	return dataDir
}

// seedPluginsState writes a plugins.json under dataDir with the
// given entries so `tai plugins list` has something to render.
//
// Not tied to a TC-ID — test fixture helper.
func seedPluginsState(t *testing.T, dataDir string, entries []plugins.Entry) {
	t.Helper()
	statePath := filepath.Join(dataDir, "state", "plugins.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	body, err := json.MarshalIndent(plugins.State{Plugins: entries}, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(statePath, append(body, '\n'), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

// TestPluginsList_TCPLG011_empty exercises TC-PLG-011.
func TestPluginsList_TCPLG011_empty(t *testing.T) {
	pluginsEnv(t)

	r := runRoot(t, "plugins", "list")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stdout, "(no plugins installed)") {
		t.Errorf("stdout: want `(no plugins installed)`, got %q", r.stdout)
	}
}

// TestPluginsList_TCPLG012_renders_table exercises TC-PLG-012.
func TestPluginsList_TCPLG012_renders_table(t *testing.T) {
	dataDir := pluginsEnv(t)
	ts := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	seedPluginsState(t, dataDir, []plugins.Entry{
		{
			Name:        "triage",
			Source:      plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
			Version:     "0.5.0",
			InstalledAt: ts,
		},
	})

	r := runRoot(t, "plugins", "list")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	for _, want := range []string{"name", "version", "installed-at", "triage", "0.5.0"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout missing %q\nstdout:\n%s", want, r.stdout)
		}
	}
}

// TestPluginsInstall_TCPLG004_reserved_name_rejected exercises TC-PLG-004.
func TestPluginsInstall_TCPLG004_reserved_name_rejected(t *testing.T) {
	dataDir := pluginsEnv(t)

	r := runRoot(t, "plugins", "install", "config")
	if r.err == nil {
		t.Fatal("expected error")
	}
	if r.exitCode != 1 {
		t.Errorf("exit code: want 1, got %d", r.exitCode)
	}
	assertCode(t, r.err, errcode.PluginNameReserved)
	if !strings.Contains(r.stderr, "[exit 1: PLUGIN_NAME_RESERVED]") {
		t.Errorf("stderr missing footer, got:\n%s", r.stderr)
	}
	// No directory was created.
	if _, err := os.Stat(filepath.Join(dataDir, "plugins", "config")); !os.IsNotExist(err) {
		t.Errorf("install dir should not exist; stat: %v", err)
	}
}

// TestPluginsInstall_TCPLG008_unknown_plugin exercises TC-PLG-008.
func TestPluginsInstall_TCPLG008_unknown_plugin(t *testing.T) {
	pluginsEnv(t)

	r := runRoot(t, "plugins", "install", "acme-custom")
	if r.err == nil {
		t.Fatal("expected error")
	}
	if r.exitCode != 1 {
		t.Errorf("exit code: want 1, got %d", r.exitCode)
	}
	assertCode(t, r.err, errcode.PluginUnknown)
	if !strings.Contains(r.stderr, "[exit 1: PLUGIN_UNKNOWN]") {
		t.Errorf("stderr missing footer, got:\n%s", r.stderr)
	}
	if !strings.Contains(r.stderr, "--source") {
		t.Errorf("stderr should suggest passing --source, got:\n%s", r.stderr)
	}
}

// TC-PLG-009 / TC-PLG-010 CLI-boundary coverage:
// the engine-layer tests in core/internal/plugins/install_test.go
// pin plugins.Install's return value as the correct
// *errcode.Error{Code: PluginFetch{Unauthorized,Failed}}. The CLI
// returns that error unchanged through urfave/cli — *errcode.Error
// satisfies cli.ExitCoder, so the exit code propagates without
// re-wrapping. The rendered footer is locked by pkg/cliout's
// TC-ERR-004 footer-regex invariant test against every code in
// the taxonomy (extended in Phase 4.2 to include all PLUGIN_*
// codes). Adding a separate httptest-backed CLI test here would
// require a new fetcher-injection seam at the cmd layer with no
// other motivation — defer until one exists.

// TestPluginsRemove_TCPLG014_retained_state_warning_at_cli is the
// CLI-boundary anchor for TC-PLG-014: the stderr warning naming the
// retained state directory must reach the user through the CLI, not
// just the engine.
func TestPluginsRemove_TCPLG014_retained_state_warning_at_cli(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install fixture uses a POSIX shell stub")
	}
	dataDir := pluginsEnv(t)

	// Seed an installed plugin + a runtime-state file.
	installDir := plugins.PluginInstallDir(dataDir, "triage")
	if err := os.MkdirAll(filepath.Join(installDir, "state"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	statePath := filepath.Join(installDir, "state", "triage.db")
	if err := os.WriteFile(statePath, []byte("db"), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	if err := os.WriteFile(plugins.PluginBinaryPath(dataDir, "triage"),
		[]byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	state, _ := plugins.LoadState(dataDir)
	state.Upsert(plugins.Entry{
		Name:        "triage",
		Source:      plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
		Version:     "v0.0.0-test",
		InstalledAt: time.Now().UTC(),
	})
	if err := plugins.SaveState(dataDir, state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	r := runRoot(t, "plugins", "remove", "triage")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	// Spec: stderr names the retained data path.
	stateDir := filepath.Dir(statePath)
	if !strings.Contains(r.stderr, stateDir) {
		t.Errorf("stderr should name the retained state dir %s, got:\n%s", stateDir, r.stderr)
	}
	// Spec: stdout summary mentions removal.
	if !strings.Contains(r.stdout, "removed triage") {
		t.Errorf("stdout should announce the removal, got: %q", r.stdout)
	}
}
