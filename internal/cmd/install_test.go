package cmd_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielmastrorillo/tai/internal/cmd"
	"github.com/danielmastrorillo/tai/internal/cmdframework"
	"github.com/danielmastrorillo/tai/internal/cmdtest"
	"github.com/danielmastrorillo/tai/internal/installer/installtest"
)

// probeSrc is the canonical bundled-command markdown used by these
// tests, re-exported from installtest so the assertions can compare
// installed bytes against the same constant they fed the fake bundle.
const probeSrc = installtest.ProbeSrc

// singleVerbBundle returns a fake bundle with one verb whose current
// body matches probeSrc.
func singleVerbBundle(t *testing.T) *installtest.FakeBundle {
	t.Helper()
	return installtest.NewSingleVerb()
}

// targetDir returns a per-test target directory that does not yet exist.
// Use this for tests that exercise the "fresh install" path.
func targetDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".claude", "commands", "tai")
}

// TestInstall_TCINST020_fresh_install: a fresh target directory ends up
// with every bundled command written, and the summary reports them
// under `Installed`.
func TestInstall_TCINST020_fresh_install(t *testing.T) {
	dir := targetDir(t)
	bundle := singleVerbBundle(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"install", "--commands-dir", dir)

	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)
	cmdtest.AssertStdoutContains(t, r, "Installed: 1 command (probe)")
	cmdtest.AssertStdoutContains(t, r, "[exit 0]")

	got, err := os.ReadFile(filepath.Join(dir, "probe.md"))
	if err != nil {
		t.Fatalf("ReadFile probe.md: %v", err)
	}
	if string(got) != probeSrc {
		t.Fatalf("installed file mismatch\nwant: %q\ngot:  %q", probeSrc, got)
	}
}

// TestInstall_TCINST021_idempotent_rerun: a second `tai install` after
// the first is a no-op — the summary reports the command under Skipped
// and the file is unchanged.
func TestInstall_TCINST021_idempotent_rerun(t *testing.T) {
	dir := targetDir(t)
	bundle := singleVerbBundle(t)

	r1 := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"install", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r1)

	info1, err := os.Stat(filepath.Join(dir, "probe.md"))
	if err != nil {
		t.Fatalf("Stat after first install: %v", err)
	}

	r2 := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"install", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r2)
	cmdtest.AssertStdoutContains(t, r2, "Skipped: 1 command (up to date)")

	info2, err := os.Stat(filepath.Join(dir, "probe.md"))
	if err != nil {
		t.Fatalf("Stat after second install: %v", err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Errorf("mtime changed on idempotent rerun: %v → %v", info1.ModTime(), info2.ModTime())
	}
}

// TestInstall_TCINST022_stale_overwritten: when the on-disk hash is an
// older ledger entry, the file is silently overwritten with the
// current version.
func TestInstall_TCINST022_stale_overwritten(t *testing.T) {
	dir := targetDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Old source whose body hash lives in the ledger but isn't current.
	oldSrc := strings.Replace(probeSrc, "body of probe\n", "old body\n", 1)
	oldBody, _ := cmdframework.Body([]byte(oldSrc))
	oldHash := cmdframework.HashBody(oldBody)

	bundle := singleVerbBundle(t)
	bundle.Ledgers["probe"] = []string{oldHash, bundle.Ledgers["probe"][0]}

	// Write the OLD source to disk — simulates "stale but untouched".
	if err := os.WriteFile(filepath.Join(dir, "probe.md"), []byte(oldSrc), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"install", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Updated: 1 command (probe)")

	got, _ := os.ReadFile(filepath.Join(dir, "probe.md"))
	if string(got) != probeSrc {
		t.Fatalf("file not overwritten\nwant: %q\ngot:  %q", probeSrc, got)
	}
}

// TestInstall_TCINST023_force_overwrites_modified: --force overwrites
// a user-modified file without prompting.
func TestInstall_TCINST023_force_overwrites_modified(t *testing.T) {
	dir := targetDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "probe.md"),
		[]byte("---\nname: x\ndescription: x\ncategory: Workflow\ntags: [x]\nversion: 1\ncontent_hash: \"sha256:0000000000000000000000000000000000000000000000000000000000000000\"\n---\nuser custom body\n"),
		0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	bundle := singleVerbBundle(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"install", "--force", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Updated: 1 command (probe)")

	got, _ := os.ReadFile(filepath.Join(dir, "probe.md"))
	if string(got) != probeSrc {
		t.Fatalf("file not overwritten under --force\nwant: %q\ngot:  %q", probeSrc, got)
	}
}

// TestInstall_TCINST024_env_overwrites_modified:
// TAI_ACCEPT_COMMAND_UPDATES=1 overwrites a user-modified file.
func TestInstall_TCINST024_env_overwrites_modified(t *testing.T) {
	t.Setenv("TAI_ACCEPT_COMMAND_UPDATES", "1")

	dir := targetDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "probe.md"), []byte("user content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	bundle := singleVerbBundle(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"install", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Updated: 1 command (probe)")
}

// TestInstall_TCINST025_env_non_truthy_ignored: a non-truthy env value
// is treated as if the var were unset; user-modified files are skipped
// when stdin is not a TTY. Exercises representative values from each
// "off" category named in the spec — explicit falses (`0`, `false`,
// `no`, `off`), the unset shape (empty string), and an unrecognised
// string. The exhaustive truthy/falsy table is owned by
// TestIsTruthyEnv at the unit level.
func TestInstall_TCINST025_env_non_truthy_ignored(t *testing.T) {
	for _, value := range []string{"0", "false", "no", "off", "", "maybe"} {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv("TAI_ACCEPT_COMMAND_UPDATES", value)

			dir := targetDir(t)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "probe.md"), []byte("user content\n"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			bundle := singleVerbBundle(t)

			r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
				"install", "--commands-dir", dir)
			cmdtest.AssertNoError(t, r)
			cmdtest.AssertStdoutContains(t, r, "Prompted-skipped: 1 command (probe)")

			got, _ := os.ReadFile(filepath.Join(dir, "probe.md"))
			if string(got) != "user content\n" {
				t.Fatalf("file was modified despite non-truthy env %q, got %q", value, got)
			}
		})
	}
}

// TestInstall_TCINST026_commands_dir_override: --commands-dir writes to
// the provided path instead of the default.
func TestInstall_TCINST026_commands_dir_override(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom-target")
	bundle := singleVerbBundle(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"install", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)

	if _, err := os.Stat(filepath.Join(dir, "probe.md")); err != nil {
		t.Fatalf("file should exist under --commands-dir: %v", err)
	}
}

// TestInstall_TCINST027_unwritable_target: a target whose parent is a
// regular file (not a directory) is unwritable; install surfaces
// INSTALL_TARGET_UNWRITABLE.
func TestInstall_TCINST027_unwritable_target(t *testing.T) {
	root := t.TempDir()
	// Make `root/notadir` a regular file so trying to MkdirAll inside it fails.
	collision := filepath.Join(root, "notadir")
	if err := os.WriteFile(collision, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bundle := singleVerbBundle(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"install", "--commands-dir", filepath.Join(collision, "subdir"))

	cmdtest.AssertError(t, r)
	cmdtest.AssertExitCode(t, r, 3)
	cmdtest.AssertErrorFooter(t, r, "INSTALL_TARGET_UNWRITABLE", 3)
}

// TestInstall_TCINST028_invalid_commands_dir: an empty --commands-dir
// surfaces INSTALL_INVALID_TARGET.
func TestInstall_TCINST028_invalid_commands_dir(t *testing.T) {
	bundle := singleVerbBundle(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"install", "--commands-dir", "")

	cmdtest.AssertError(t, r)
	cmdtest.AssertExitCode(t, r, 1)
	cmdtest.AssertErrorFooter(t, r, "INSTALL_INVALID_TARGET", 1)
}

// TestInstall_TCINST029_non_interactive_skip: with non-TTY stdin and no
// override, user-modified files are reported as Prompted-skipped and
// the run exits 0.
func TestInstall_TCINST029_non_interactive_skip(t *testing.T) {
	dir := targetDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "probe.md"), []byte("user content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	bundle := singleVerbBundle(t)

	// cmdtest.Run uses a strings.Reader for stdin — that is non-TTY.
	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"install", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)
	cmdtest.AssertStdoutContains(t, r, "Prompted-skipped: 1 command (probe)")
}

// TestInstall_TCINST040_outside_git_repo: tai install runs outside any
// git repository with no REPO_NOT_FOUND footer.
func TestInstall_TCINST040_outside_git_repo(t *testing.T) {
	cmdtest.Chdir(t, t.TempDir())

	bundle := singleVerbBundle(t)
	dir := targetDir(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"install", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)
	if strings.Contains(r.Stderr, "REPO_NOT_FOUND") {
		t.Fatalf("stderr should not contain REPO_NOT_FOUND, got %q", r.Stderr)
	}
}

// TestInstall_TCINST041_does_not_touch_data_dir: tai install does not
// create or modify the configured data directory.
func TestInstall_TCINST041_does_not_touch_data_dir(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "foreign-data")
	t.Setenv("TAI_DATA_DIR", dataDir)

	bundle := singleVerbBundle(t)
	dir := targetDir(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"install", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)

	if _, err := os.Stat(dataDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("data dir should not exist after install, got err=%v", err)
	}
}
