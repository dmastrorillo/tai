package cmd_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/installcmd"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// installCommandsEnv stages a fresh TAI_CONFIG with the given target
// roots (each gets default sub-paths) plus a deterministic empty
// data dir. Returns the resolved target roots in declaration order.
// Each target's directory tree is created so the test can assert on
// it without depending on installcmd's MkdirAll behaviour.
//
// Not tied to a TC-ID — test fixture helper.
func installCommandsEnv(t *testing.T, opts ...installEnvOpt) []string {
	t.Helper()

	t.Setenv("TAI_DATA_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	t.Setenv("TAI_CONFIG", cfgPath)

	cfg := installEnvConfig{}
	for _, o := range opts {
		o(&cfg)
	}

	body := "update-check-interval: 0\n"
	if len(cfg.targets) > 0 {
		body += "targets:\n"
		for _, tgt := range cfg.targets {
			body += fmt.Sprintf("  - root: %s\n", tgt.root)
			if tgt.commands != nil {
				body += fmt.Sprintf("    commands: %q\n", *tgt.commands)
			}
		}
	}
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	var roots []string
	for _, tgt := range cfg.targets {
		roots = append(roots, tgt.root)
	}
	return roots
}

type installEnvOpt func(*installEnvConfig)

type installEnvConfig struct {
	targets []installEnvTarget
}

type installEnvTarget struct {
	root     string
	commands *string // nil = default, &"" = falsy/skip, &"x" = override
}

func withTarget(t *testing.T) installEnvOpt {
	t.Helper()
	root := filepath.Join(t.TempDir(), "claude")
	return func(c *installEnvConfig) {
		c.targets = append(c.targets, installEnvTarget{root: root})
	}
}

func withFalsyCommandsTarget(t *testing.T) installEnvOpt {
	t.Helper()
	root := filepath.Join(t.TempDir(), "claude")
	empty := ""
	return func(c *installEnvConfig) {
		c.targets = append(c.targets, installEnvTarget{root: root, commands: &empty})
	}
}

// TestInstallCommands_TCIC001_single_target_writes_bundle exercises TC-IC-001.
func TestInstallCommands_TCIC001_single_target_writes_bundle(t *testing.T) {
	roots := installCommandsEnv(t, withTarget(t))

	r := runRoot(t, "install-commands")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	bundled, err := installcmd.BundledFiles()
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	if len(bundled) == 0 {
		t.Fatal("the embedded bundle is empty — nothing to install")
	}
	for _, name := range bundled {
		p := filepath.Join(roots[0], "commands", "tai", name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing installed file %s: %v", p, err)
		}
	}

	// Stdout summary contract: the no-stale branch reads
	// "installed N command(s) into 1 target" (no parenthetical).
	wantStdout := fmt.Sprintf("installed %d command", len(bundled))
	if !strings.Contains(r.stdout, wantStdout) {
		t.Errorf("stdout missing %q\nstdout:\n%s", wantStdout, r.stdout)
	}
	if !strings.Contains(r.stdout, "into 1 target") {
		t.Errorf("stdout should name `into 1 target`, got: %q", r.stdout)
	}
	if strings.Contains(r.stdout, "stale") {
		t.Errorf("no-stale run should NOT print a stale-removed parenthetical, got: %q", r.stdout)
	}
}

// TestInstallCommands_TCIC002_multi_target_fan_out exercises TC-IC-002.
func TestInstallCommands_TCIC002_multi_target_fan_out(t *testing.T) {
	roots := installCommandsEnv(t, withTarget(t), withTarget(t))

	r := runRoot(t, "install-commands")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	bundled, err := installcmd.BundledFiles()
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	for _, root := range roots {
		for _, name := range bundled {
			p := filepath.Join(root, "commands", "tai", name)
			if _, err := os.Stat(p); err != nil {
				t.Errorf("missing %s under %s: %v", name, root, err)
			}
		}
	}
}

// TestInstallCommands_TCIC003_no_targets exercises TC-IC-003.
func TestInstallCommands_TCIC003_no_targets(t *testing.T) {
	installCommandsEnv(t) // no targets

	r := runRoot(t, "install-commands")
	if r.err == nil {
		t.Fatal("expected error")
	}
	if r.exitCode != 2 {
		t.Errorf("exit code: want 2, got %d", r.exitCode)
	}
	assertCode(t, r.err, errcode.TaiNotConfigured)
	if !strings.Contains(r.stderr, "[exit 2: TAI_NOT_CONFIGURED]") {
		t.Errorf("stderr missing footer, got:\n%s", r.stderr)
	}
	if !strings.Contains(r.stderr, "tai config target add") {
		t.Errorf("stderr should name the resolution `tai config target add`, got:\n%s", r.stderr)
	}
}

// TestInstallCommands_TCIC004_falsy_commands_skipped exercises TC-IC-004.
func TestInstallCommands_TCIC004_falsy_commands_skipped(t *testing.T) {
	roots := installCommandsEnv(t, withFalsyCommandsTarget(t))

	r := runRoot(t, "install-commands")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	// No file at the falsy target.
	if _, err := os.Stat(filepath.Join(roots[0], "commands")); err == nil {
		t.Errorf("commands dir should not have been created under falsy target %s", roots[0])
	}
	if !strings.Contains(r.stderr, roots[0]) {
		t.Errorf("stderr should name the skipped target %s, got:\n%s", roots[0], r.stderr)
	}
	if !strings.Contains(r.stderr, "skipping") {
		t.Errorf("stderr should explain the skip, got:\n%s", r.stderr)
	}

	// All-skipped stdout contract: distinct from the success path
	// so an operator (or pipeline) sees that nothing was installed.
	if !strings.Contains(r.stdout, "all 1 target skipped") {
		t.Errorf("stdout should announce the all-skipped result, got: %q", r.stdout)
	}
	if strings.Contains(r.stdout, "installed 0") {
		t.Errorf("stdout should NOT read as a zero-count install, got: %q", r.stdout)
	}
}

// TestInstallCommands_TCIC005_rerun_idempotent exercises TC-IC-005.
func TestInstallCommands_TCIC005_rerun_idempotent(t *testing.T) {
	roots := installCommandsEnv(t, withTarget(t))

	if r := runRoot(t, "install-commands"); r.err != nil {
		t.Fatalf("first run: %v\nstderr:\n%s", r.err, r.stderr)
	}
	first := snapshotDir(t, filepath.Join(roots[0], "commands", "tai"))

	if r := runRoot(t, "install-commands"); r.err != nil {
		t.Fatalf("second run: %v\nstderr:\n%s", r.err, r.stderr)
	}
	second := snapshotDir(t, filepath.Join(roots[0], "commands", "tai"))

	if len(first) != len(second) {
		t.Fatalf("file count changed across runs: first=%d second=%d", len(first), len(second))
	}
	for name, body := range first {
		if second[name] != body {
			t.Errorf("file %s changed across runs", name)
		}
	}
}

// TestInstallCommands_TCIC006_stale_builtin_removed exercises TC-IC-006.
func TestInstallCommands_TCIC006_stale_builtin_removed(t *testing.T) {
	roots := installCommandsEnv(t, withTarget(t))
	taiDir := filepath.Join(roots[0], "commands", "tai")

	// Pre-seed a stale built-in that is NOT in the current bundle.
	if err := os.MkdirAll(taiDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stale := filepath.Join(taiDir, "legacy.md")
	if err := os.WriteFile(stale, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("seed stale: %v", err)
	}

	r := runRoot(t, "install-commands")
	if r.err != nil {
		t.Fatalf("install: %v\nstderr:\n%s", r.err, r.stderr)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale built-in should have been removed; stat returned: %v", err)
	}

	// Sanity: every currently-bundled file is still present.
	bundled, err := installcmd.BundledFiles()
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	for _, name := range bundled {
		p := filepath.Join(taiDir, name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing bundled file %s: %v", name, err)
		}
	}

	// Stdout summary contract: the stale-removal branch reads
	// "installed N command(s) into 1 target (1 stale built-in removed)".
	if !strings.Contains(r.stdout, "1 stale built-in removed") {
		t.Errorf("stdout should report 1 stale built-in removed, got: %q", r.stdout)
	}
	wantStdout := fmt.Sprintf("installed %d command", len(bundled))
	if !strings.Contains(r.stdout, wantStdout) {
		t.Errorf("stdout missing %q\nstdout:\n%s", wantStdout, r.stdout)
	}
}

// TestInstallCommands_TCIC007_outside_tai_untouched exercises TC-IC-007.
func TestInstallCommands_TCIC007_outside_tai_untouched(t *testing.T) {
	roots := installCommandsEnv(t, withTarget(t))
	commandsDir := filepath.Join(roots[0], "commands")

	// Pre-seed user content OUTSIDE the `tai/` namespace.
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	userFile := filepath.Join(commandsDir, "my-own.md")
	const userBytes = "# user-authored, hands off\n"
	if err := os.WriteFile(userFile, []byte(userBytes), 0o644); err != nil {
		t.Fatalf("seed user content: %v", err)
	}

	if r := runRoot(t, "install-commands"); r.err != nil {
		t.Fatalf("install: %v\nstderr:\n%s", r.err, r.stderr)
	}

	got, err := os.ReadFile(userFile)
	if err != nil {
		t.Fatalf("read user file after install: %v", err)
	}
	if string(got) != userBytes {
		t.Errorf("user file mutated\nbefore: %q\nafter:  %q", userBytes, got)
	}
}

// snapshotDir returns a name → bytes map for every regular file in
// dir (one level deep). Used by TC-IC-005 to assert re-run
// idempotency.
//
// Not tied to a TC-ID — test fixture helper.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}
	out := map[string]string{}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, n := range names {
		body, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatalf("read %s: %v", n, err)
		}
		out[n] = string(body)
	}
	return out
}
