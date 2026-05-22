package cmd_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmd"
	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdtest"
)

// seedInstalled writes <dir>/probe.md with content from probeSrc — the
// "this file is a tai-shipped current version" baseline used by most
// uninstall tests.
func seedInstalled(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "probe.md"), []byte(probeSrc), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestUninstall_TCINST030_clean_uninstall: every bundled file is
// removed, the directory is removed (it is empty afterwards), and the
// summary reports the verb under `Removed`.
func TestUninstall_TCINST030_clean_uninstall(t *testing.T) {
	dir := targetDir(t)
	seedInstalled(t, dir)
	bundle := singleVerbBundle(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"uninstall", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)
	cmdtest.AssertStdoutContains(t, r, "Removed: 1 command (probe)")

	if _, err := os.Stat(filepath.Join(dir, "probe.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("file should not exist after uninstall, got err=%v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("empty dir should be removed after uninstall, got err=%v", err)
	}
}

// TestUninstall_TCINST031_leaves_modified_in_place: a user-modified
// file is preserved when neither --force nor the env override is set.
func TestUninstall_TCINST031_leaves_modified_in_place(t *testing.T) {
	dir := targetDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "probe.md"), []byte("user content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bundle := singleVerbBundle(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"uninstall", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Prompted-skipped: 1 command (probe)")

	got, err := os.ReadFile(filepath.Join(dir, "probe.md"))
	if err != nil {
		t.Fatalf("file should still exist: %v", err)
	}
	if string(got) != "user content\n" {
		t.Errorf("file was modified, got %q", got)
	}
}

// TestUninstall_TCINST032_force_removes_modified: --force removes a
// user-modified file without prompting.
func TestUninstall_TCINST032_force_removes_modified(t *testing.T) {
	dir := targetDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "probe.md"), []byte("user content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bundle := singleVerbBundle(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"uninstall", "--force", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Removed: 1 command (probe)")

	if _, err := os.Stat(filepath.Join(dir, "probe.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("file should be removed under --force, got err=%v", err)
	}
}

// TestUninstall_TCINST033_env_removes_modified:
// TAI_ACCEPT_COMMAND_UPDATES=1 removes user-modified files.
func TestUninstall_TCINST033_env_removes_modified(t *testing.T) {
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
		"uninstall", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Removed: 1 command (probe)")
}

// TestUninstall_TCINST034_unrelated_preserved: a file in the target
// directory whose filename does not match any known verb is preserved.
func TestUninstall_TCINST034_unrelated_preserved(t *testing.T) {
	dir := targetDir(t)
	seedInstalled(t, dir)
	unrelated := filepath.Join(dir, "other-tool-command.md")
	if err := os.WriteFile(unrelated, []byte("other tool content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bundle := singleVerbBundle(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"uninstall", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)

	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated file should be preserved: %v", err)
	}
}

// TestUninstall_TCINST035_dir_preserved_when_non_empty: when unrelated
// files remain after processing, the directory is preserved.
func TestUninstall_TCINST035_dir_preserved_when_non_empty(t *testing.T) {
	dir := targetDir(t)
	seedInstalled(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "other.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bundle := singleVerbBundle(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"uninstall", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory should be preserved: %v", err)
	}
}

// TestUninstall_TCINST036_empty_dir_removed: an empty directory (no
// unrelated files) is removed after processing.
func TestUninstall_TCINST036_empty_dir_removed(t *testing.T) {
	dir := targetDir(t)
	seedInstalled(t, dir)
	bundle := singleVerbBundle(t)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"uninstall", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)

	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("empty directory should be removed, got err=%v", err)
	}
}

// TestUninstall_TCINST042_outside_git_repo: uninstall runs outside any
// git repository with no REPO_NOT_FOUND footer (mirror of TC-INST-040
// for the install side).
func TestUninstall_TCINST042_outside_git_repo(t *testing.T) {
	cmdtest.Chdir(t, t.TempDir())

	bundle := singleVerbBundle(t)
	dir := targetDir(t)
	seedInstalled(t, dir)

	r := cmdtest.Run(t, cmd.NewRoot(cmd.WithBundle(bundle)),
		"uninstall", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)
}
