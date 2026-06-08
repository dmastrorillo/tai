package cmd_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/core/internal/plugins"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// installFakePlugin stages a real executable under
// `<dataDir>/plugins/<name>/<name>` and records the install in
// state. The executable is a POSIX shell script — Windows is not
// covered by these tests today; a future change can layer a
// PowerShell shim if needed.
//
// `body` is the shell-script content (after the `#!/bin/sh` line).
//
// Not tied to a TC-ID — test fixture helper.
func installFakePlugin(t *testing.T, dataDir, name, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("plugin subprocess tests use a POSIX shell stub")
	}
	installDir := plugins.PluginInstallDir(dataDir, name)
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	binPath := plugins.PluginBinaryPath(dataDir, name)
	script := "#!/bin/sh\n" + body
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write plugin: %v", err)
	}

	// Record the install in state.
	state, _ := plugins.LoadState(dataDir)
	state.Upsert(plugins.Entry{
		Name:        name,
		Source:      plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
		Version:     "v0.0.0-test",
		InstalledAt: time.Now().UTC(),
	})
	if err := plugins.SaveState(dataDir, state); err != nil {
		t.Fatalf("save state: %v", err)
	}
	return binPath
}

// TestPluginInvoke_TCPLG002_passthrough exercises TC-PLG-002.
func TestPluginInvoke_TCPLG002_passthrough(t *testing.T) {
	dataDir := pluginsEnv(t)
	// Plugin echoes argv[1] to stdout, "err" to stderr, exits 7.
	installFakePlugin(t, dataDir, "triage",
		`printf '%s' "$1" >&1
printf '%s' "err" >&2
exit 7
`)

	r := runRoot(t, "triage", "foo")
	if r.exitCode != 7 {
		t.Errorf("exit code: want 7, got %d", r.exitCode)
	}
	if r.stdout != "foo" {
		t.Errorf("stdout passthrough: want %q, got %q", "foo", r.stdout)
	}
	if !strings.Contains(r.stderr, "err") {
		t.Errorf("stderr passthrough: want substring %q, got %q", "err", r.stderr)
	}
	// Critical: the host MUST NOT render its own INTERNAL_ERROR
	// template over a plugin's non-zero exit. The plugin already
	// wrote its own stderr; the host only propagates the code.
	if strings.Contains(r.stderr, "INTERNAL_ERROR") {
		t.Errorf("host should not render INTERNAL_ERROR over a plugin's exit code\nstderr:\n%s", r.stderr)
	}
	if strings.Contains(r.stderr, "[exit ") {
		t.Errorf("host should not render any [exit N: …] footer over a plugin's exit code\nstderr:\n%s", r.stderr)
	}
}

// TestPluginInvoke_TCPLG003_unknown_verb exercises TC-PLG-003.
func TestPluginInvoke_TCPLG003_unknown_verb(t *testing.T) {
	pluginsEnv(t) // no plugins installed

	r := runRoot(t, "nope")
	if r.err == nil {
		t.Fatal("expected error")
	}
	if r.exitCode != 1 {
		t.Errorf("exit code: want 1, got %d", r.exitCode)
	}
	assertCode(t, r.err, errcode.UnknownSubcommand)
	for _, want := range []string{
		"[exit 1: UNKNOWN_SUBCOMMAND]",
		"tai plugins list",
		"tai plugins install nope",
	} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr missing %q\nstderr:\n%s", want, r.stderr)
		}
	}
}

// TestPluginInvoke_TCPLG005_env_var_contract exercises TC-PLG-005.
func TestPluginInvoke_TCPLG005_env_var_contract(t *testing.T) {
	dataDir := pluginsEnv(t)
	target := filepath.Join(t.TempDir(), "claude")

	// Re-write the config so it has a target + a repo-url (so
	// TAI_CLONE_DIR is populated).
	cfgPath, _ := os.LookupEnv("TAI_CONFIG")
	body := fmt.Sprintf("repo-url: git@github.com:acme/repo.git\nupdate-check-interval: 0\ntargets:\n  - root: %s\n", target)
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}

	// Plugin dumps the three contract env vars to stdout as JSON so
	// the test can decode them.
	installFakePlugin(t, dataDir, "triage", `cat <<EOF
{
  "data_dir":   "${TAI_DATA_DIR}",
  "clone_dir":  "${TAI_CLONE_DIR}",
  "targets":    ${TAI_TARGETS}
}
EOF
`)

	r := runRoot(t, "triage")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	var got struct {
		DataDir  string `json:"data_dir"`
		CloneDir string `json:"clone_dir"`
		Targets  []struct {
			Root     string `json:"root"`
			Skills   string `json:"skills"`
			Commands string `json:"commands"`
			Agents   string `json:"agents"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatalf("decode env JSON from plugin: %v\nstdout:\n%s", err, r.stdout)
	}
	if got.DataDir != dataDir {
		t.Errorf("TAI_DATA_DIR: want %q, got %q", dataDir, got.DataDir)
	}
	wantClone := filepath.Join(dataDir, "source")
	if got.CloneDir != wantClone {
		t.Errorf("TAI_CLONE_DIR: want %q, got %q", wantClone, got.CloneDir)
	}
	if len(got.Targets) != 1 {
		t.Fatalf("targets: want 1, got %d", len(got.Targets))
	}
	if got.Targets[0].Root != target {
		t.Errorf("target.root: want %q, got %q", target, got.Targets[0].Root)
	}
	if got.Targets[0].Skills != filepath.Join(target, "skills") {
		t.Errorf("target.skills: %q", got.Targets[0].Skills)
	}
	if got.Targets[0].Commands != filepath.Join(target, "commands") {
		t.Errorf("target.commands: %q", got.Targets[0].Commands)
	}
}
