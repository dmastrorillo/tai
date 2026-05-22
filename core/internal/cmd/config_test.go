package cmd_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// configEnv stages a temp file as $TAI_CONFIG and returns the path.
// The file is NOT created — callers exercise the lazy-creation
// behaviour by checking whether the path exists after a command runs.
//
// All TC-CONF tests go through this helper so each test runs against
// an isolated config file with no leakage between tests.
func configEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	t.Setenv("TAI_CONFIG", path)
	// Belt-and-braces: blank XDG and HOME so a stray code path can't
	// fall back to a real user directory.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", t.TempDir())
	return path
}

// writeConfig seeds the file at path with body. Convenience for tests
// that start from a non-empty config.
func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}
}

// readConfig returns the bytes at path, failing the test if read errors.
func readConfig(t *testing.T, path string) string {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	return string(got)
}

// fakeEditor writes a shell script that records argv to a file then
// exits 0. The recorded-argv file is returned so the test can assert
// on what the editor was called with. Skips the test on Windows where
// `#!` scripts don't run.
func fakeEditor(t *testing.T) (editorPath, argvLogPath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-editor relies on POSIX shebang; Windows path covered by manual smoke test")
	}
	dir := t.TempDir()
	editorPath = filepath.Join(dir, "fake-editor")
	argvLogPath = filepath.Join(dir, "argv.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argvLogPath + "\n"
	if err := os.WriteFile(editorPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake editor: %v", err)
	}
	return editorPath, argvLogPath
}

// noopEditor writes a /usr/bin/true-equivalent — a shell script that
// exits 0 without touching anything. Used for the round-trip test.
func noopEditor(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("noop-editor relies on POSIX shebang")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "noop-editor")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write noop editor: %v", err)
	}
	return path
}

// ─── TC-CONF-003 — `--help` / `--version` do not create the config ────

// TestRoot_TCCONF003_help_does_not_create_config exercises TC-CONF-003
// for the --help variant: running tai --help against a fresh user
// state MUST leave the filesystem untouched at the resolved config
// path.
func TestRoot_TCCONF003_help_does_not_create_config(t *testing.T) {
	path := configEnv(t)
	r := runRoot(t, "--help")
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("config file should not exist, but stat returned: %v", err)
	}
}

// TestRoot_TCCONF003_version_does_not_create_config exercises the
// --version half of TC-CONF-003.
func TestRoot_TCCONF003_version_does_not_create_config(t *testing.T) {
	path := configEnv(t)
	r := runRoot(t, "--version")
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("config file should not exist, but stat returned: %v", err)
	}
}

// ─── TC-CONF-004 — lazy creation on first write ────────────────────────

func TestConfig_TCCONF004_lazy_create_on_first_write(t *testing.T) {
	path := configEnv(t)
	r := runRoot(t, "config", "target", "add", "/tmp/example")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if r.exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d", r.exitCode)
	}
	body := readConfig(t, path)
	if !strings.Contains(body, "/tmp/example") {
		t.Fatalf("config missing new target, got:\n%s", body)
	}
}

// ─── TC-CONF-008 / 009 — `tai config show` ─────────────────────────────

func TestConfigShow_TCCONF008_prints_yaml(t *testing.T) {
	path := configEnv(t)
	writeConfig(t, path, `repo-url: git@github.com:acme/repo.git
targets:
  - root: /tmp/claude
`)
	r := runRoot(t, "config", "show")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	for _, want := range []string{"git@github.com:acme/repo.git", "/tmp/claude"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout missing %q\nfull stdout:\n%s", want, r.stdout)
		}
	}
	if r.stderr != "" {
		t.Errorf("stderr should be empty, got %q", r.stderr)
	}
}

func TestConfigShow_TCCONF009_no_config_message(t *testing.T) {
	path := configEnv(t)
	r := runRoot(t, "config", "show")
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if !strings.Contains(r.stdout, path) {
		t.Errorf("stdout should name the resolved path %q, got:\n%s", path, r.stdout)
	}
	for _, want := range []string{"tai config target add", "tai config edit"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout should mention %q, got:\n%s", want, r.stdout)
		}
	}
	// Even though the spec for TC-CONF-009 doesn't explicitly require
	// "stderr is empty", the stdout-discipline rule from
	// specs/cli-framework/spec.md forbids the informational text from
	// leaking onto stderr. Lock it down so a routing regression surfaces.
	if r.stderr != "" {
		t.Errorf("stderr should be empty, got %q", r.stderr)
	}
}

// ─── TC-CONF-010 / 011 / 012 — `tai config edit` ───────────────────────

func TestConfigEdit_TCCONF010_creates_template_and_opens_editor(t *testing.T) {
	path := configEnv(t)
	editor, argvLog := fakeEditor(t)
	t.Setenv("EDITOR", editor)

	r := runRoot(t, "config", "edit")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	body := readConfig(t, path)
	for _, want := range []string{"repo-url", "targets", "update-check-interval"} {
		if !strings.Contains(body, want) {
			t.Errorf("template missing %q, got:\n%s", want, body)
		}
	}
	logged := readConfig(t, argvLog)
	if !strings.Contains(logged, path) {
		t.Errorf("editor argv did not include config path; got:\n%s", logged)
	}
}

// TestConfigEdit_TCCONF011_roundtrip_unchanged exercises TC-CONF-011.
//
// Implementation note: the byte-equality guarantee comes from
// runConfigEdit having NO post-editor re-marshal step — tai hands the
// path to $EDITOR and returns. Today's noopEditor exits without
// touching the file, so the assertion holds trivially against the
// current implementation. A future change that adds a re-validate /
// re-save step (e.g., to normalise YAML on exit) would silently break
// byte-fidelity and this test would not catch it. If that step lands,
// extend this test with an editor that touches the file but emits the
// same bytes back (e.g. ed-style 'wq') so the assertion exercises the
// re-write path.
func TestConfigEdit_TCCONF011_roundtrip_unchanged(t *testing.T) {
	path := configEnv(t)
	editor := noopEditor(t)
	t.Setenv("EDITOR", editor)

	original := `repo-url: git@github.com:acme/repo.git
targets:
  - root: /tmp/claude
`
	writeConfig(t, path, original)
	before := readConfig(t, path)

	r := runRoot(t, "config", "edit")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	after := readConfig(t, path)
	if before != after {
		t.Fatalf("config bytes changed across a no-op edit:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestConfigEdit_TCCONF012_no_editor(t *testing.T) {
	path := configEnv(t)
	t.Setenv("EDITOR", "")

	r := runRoot(t, "config", "edit")
	if r.err == nil {
		t.Fatal("expected error, got nil")
	}
	if r.exitCode != 1 {
		t.Errorf("exit code: want 1, got %d", r.exitCode)
	}
	if !strings.Contains(r.stderr, "[exit 1: CONFIG_EDITOR_UNSET]") {
		t.Errorf("stderr missing CONFIG_EDITOR_UNSET footer; got:\n%s", r.stderr)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("config file should not exist after a failed edit; stat returned: %v", err)
	}
}

// ─── TC-CONF-006 / 013 / 014 — `tai config set` ────────────────────────

func TestConfigSet_TCCONF006_rejects_file_url(t *testing.T) {
	configEnv(t)
	r := runRoot(t, "config", "set", "repo-url", "file:///tmp/repo")
	if r.err == nil {
		t.Fatal("expected error, got nil")
	}
	if r.exitCode != 1 {
		t.Errorf("exit code: want 1, got %d", r.exitCode)
	}
	if !strings.Contains(r.stderr, "[exit 1: CONFIG_INVALID_REPO_URL]") {
		t.Errorf("stderr missing CONFIG_INVALID_REPO_URL footer; got:\n%s", r.stderr)
	}
}

func TestConfigSet_TCCONF006_rejects_local_path(t *testing.T) {
	configEnv(t)
	r := runRoot(t, "config", "set", "repo-url", "/tmp/local-repo")
	if r.err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(r.stderr, "[exit 1: CONFIG_INVALID_REPO_URL]") {
		t.Errorf("stderr missing CONFIG_INVALID_REPO_URL footer; got:\n%s", r.stderr)
	}
}

func TestConfigSet_TCCONF013_updates_repo_url(t *testing.T) {
	path := configEnv(t)
	writeConfig(t, path, `targets:
  - root: /tmp/claude
`)
	r := runRoot(t, "config", "set", "repo-url", "git@github.com:acme/repo.git")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	body := readConfig(t, path)
	if !strings.Contains(body, "repo-url: git@github.com:acme/repo.git") {
		t.Errorf("config missing new repo-url, got:\n%s", body)
	}
	if !strings.Contains(body, "/tmp/claude") {
		t.Errorf("config lost pre-existing target, got:\n%s", body)
	}
}

func TestConfigSet_TCCONF014_rejects_nested_key(t *testing.T) {
	path := configEnv(t)
	writeConfig(t, path, "targets:\n  - root: /tmp/keep\n")
	before := readConfig(t, path)

	r := runRoot(t, "config", "set", "targets[0].root", "/tmp/hack")
	if r.err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(r.stderr, "[exit 1: CONFIG_KEY_NOT_SCRIPTABLE]") {
		t.Errorf("stderr missing CONFIG_KEY_NOT_SCRIPTABLE footer; got:\n%s", r.stderr)
	}
	if got := readConfig(t, path); got != before {
		t.Fatalf("config file mutated despite rejection:\nbefore:\n%s\nafter:\n%s", before, got)
	}
}

// ─── TC-CONF-015 / 016 — `tai config target add` ───────────────────────

func TestConfigTargetAdd_TCCONF015_appends_new(t *testing.T) {
	path := configEnv(t)
	r := runRoot(t, "config", "target", "add", "/tmp/claude", "--skills", "custom-skills")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	body := readConfig(t, path)
	if !strings.Contains(body, "root: /tmp/claude") {
		t.Errorf("config missing target root, got:\n%s", body)
	}
	if !strings.Contains(body, "skills: custom-skills") {
		t.Errorf("config missing skills override, got:\n%s", body)
	}
	// commands/agents must be absent (defaults apply at sync time).
	for _, forbidden := range []string{"commands:", "agents:"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("config should not contain %q (sub-path absent in YAML), got:\n%s", forbidden, body)
		}
	}
}

func TestConfigTargetAdd_TCCONF016_duplicate_rejected(t *testing.T) {
	path := configEnv(t)
	writeConfig(t, path, "targets:\n  - root: /tmp/claude\n")
	before := readConfig(t, path)

	r := runRoot(t, "config", "target", "add", "/tmp/claude")
	if r.err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(r.stderr, "[exit 1: CONFIG_DUPLICATE_TARGET]") {
		t.Errorf("stderr missing CONFIG_DUPLICATE_TARGET footer; got:\n%s", r.stderr)
	}
	if got := readConfig(t, path); got != before {
		t.Fatalf("config mutated despite duplicate rejection:\nbefore:\n%s\nafter:\n%s", before, got)
	}
}

// ─── TC-CONF-017 / 018 — `tai config target list` ──────────────────────

func TestConfigTargetList_TCCONF017_empty(t *testing.T) {
	configEnv(t)
	r := runRoot(t, "config", "target", "list")
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if !strings.Contains(r.stdout, "(no targets configured)") {
		t.Errorf("stdout missing empty marker, got:\n%s", r.stdout)
	}
	if r.stderr != "" {
		t.Errorf("stderr should be empty on success, got %q", r.stderr)
	}
}

// TestConfigTargetList_TCCLI003_channel_discipline exercises TC-CLI-003
// — the first concrete channel-discipline anchor in core. The targets
// table is data → stdout; nothing useful belongs on stderr on the
// success path. A regression that routes the table to stderr (or
// leaks an unrelated warning to stdout) would surface here.
func TestConfigTargetList_TCCLI003_channel_discipline(t *testing.T) {
	path := configEnv(t)
	writeConfig(t, path, "targets:\n  - root: /tmp/claude\n")
	r := runRoot(t, "config", "target", "list")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stdout, "/tmp/claude") {
		t.Errorf("stdout missing target row, got:\n%s", r.stdout)
	}
	if r.stderr != "" {
		t.Errorf("stderr should be empty on success, got %q", r.stderr)
	}
}

func TestConfigTargetList_TCCONF018_renders_table(t *testing.T) {
	path := configEnv(t)
	writeConfig(t, path, `targets:
  - root: /tmp/claude
  - root: /tmp/opencode
    commands: ""
`)
	r := runRoot(t, "config", "target", "list")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	for _, want := range []string{"root", "skills", "commands", "agents", "/tmp/claude", "/tmp/opencode", "(skip)"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout missing %q, got:\n%s", want, r.stdout)
		}
	}
	if r.stderr != "" {
		t.Errorf("stderr should be empty on success, got %q", r.stderr)
	}
}

// ─── TC-CONF-019 / 020 — `tai config target remove` ────────────────────

func TestConfigTargetRemove_TCCONF019_removes_existing(t *testing.T) {
	path := configEnv(t)
	writeConfig(t, path, `targets:
  - root: /tmp/claude
  - root: /tmp/opencode
`)
	r := runRoot(t, "config", "target", "remove", "/tmp/claude")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	body := readConfig(t, path)
	if strings.Contains(body, "/tmp/claude") {
		t.Errorf("config should no longer contain /tmp/claude, got:\n%s", body)
	}
	if !strings.Contains(body, "/tmp/opencode") {
		t.Errorf("config lost the other target, got:\n%s", body)
	}
	if r.stderr != "" {
		t.Errorf("stderr should be empty on success, got %q", r.stderr)
	}
}

func TestConfigTargetRemove_TCCONF020_missing_errors(t *testing.T) {
	path := configEnv(t)
	writeConfig(t, path, "targets:\n  - root: /tmp/keep\n")
	before := readConfig(t, path)

	r := runRoot(t, "config", "target", "remove", "/tmp/nope")
	if r.err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(r.stderr, "[exit 1: CONFIG_TARGET_NOT_FOUND]") {
		t.Errorf("stderr missing CONFIG_TARGET_NOT_FOUND footer; got:\n%s", r.stderr)
	}
	if got := readConfig(t, path); got != before {
		t.Fatalf("config mutated despite missing-target rejection:\nbefore:\n%s\nafter:\n%s", before, got)
	}
}
