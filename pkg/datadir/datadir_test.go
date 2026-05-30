package datadir_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/pkg/datadir"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// TestResolve_TCCFG001_default_linux_no_overrides exercises TC-CFG-001:
// when both TAI_DATA_DIR and XDG_DATA_HOME are unset, the default is
// $HOME/.local/share/tai on Linux/macOS, or the LOCALAPPDATA equivalent
// on Windows.
func TestResolve_TCCFG001_default_linux_no_overrides(t *testing.T) {
	clearEnvForTest(t, "TAI_DATA_DIR")
	clearEnvForTest(t, "XDG_DATA_HOME")

	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", `C:\Users\test\AppData\Local`)
		got, err := datadir.Resolve()
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		want := filepath.Join(`C:\Users\test\AppData\Local`, "tai")
		if got != want {
			t.Fatalf("want %q, got %q", want, got)
		}
		return
	}

	t.Setenv("HOME", "/tmp/fake-home")
	got, err := datadir.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join("/tmp/fake-home", ".local", "share", "tai")
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestResolve_TCCFG002_xdg_overrides_default exercises TC-CFG-002:
// when XDG_DATA_HOME is set and TAI_DATA_DIR is not, the resolved
// directory is "$XDG_DATA_HOME/tai" (the "tai" suffix appended).
func TestResolve_TCCFG002_xdg_overrides_default(t *testing.T) {
	clearEnvForTest(t, "TAI_DATA_DIR")
	t.Setenv("XDG_DATA_HOME", "/custom/xdg")

	got, err := datadir.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join("/custom/xdg", "tai")
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestResolve_TCCFG003_tai_data_dir_wins exercises TC-CFG-003:
// when TAI_DATA_DIR is set, it takes precedence over XDG_DATA_HOME and
// is used verbatim (no "tai" suffix appended).
func TestResolve_TCCFG003_tai_data_dir_wins(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/xdg")
	t.Setenv("TAI_DATA_DIR", "/explicit/tai-data")

	got, err := datadir.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "/explicit/tai-data" {
		t.Fatalf("want /explicit/tai-data, got %q", got)
	}
}

// TestEnsureWritable_TCCFG004_unwritable_dir exercises TC-CFG-004:
// EnsureWritable surfaces DATA_DIR_UNWRITABLE when the resolved path
// cannot be created or is not writable.
//
// We point TAI_DATA_DIR at /dev/null/cannot-mkdir — a path that cannot
// possibly be created on any POSIX system because /dev/null is a file.
func TestEnsureWritable_TCCFG004_unwritable_dir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not have /dev/null; equivalent test is integration-only")
	}
	t.Setenv("TAI_DATA_DIR", "/dev/null/cannot-mkdir")

	_, err := datadir.EnsureWritable()
	if err == nil {
		t.Fatal("expected DATA_DIR_UNWRITABLE error, got nil")
	}
	taiErr, ok := errcode.As(err)
	if !ok {
		t.Fatalf("expected *errcode.Error, got %T: %v", err, err)
	}
	if taiErr.Code != errcode.DataDirUnwritable {
		t.Fatalf("expected DATA_DIR_UNWRITABLE, got %s", taiErr.Code)
	}
	if len(taiErr.Help) == 0 {
		t.Fatal("expected remediation help bullets on DATA_DIR_UNWRITABLE")
	}
	// Sanity: the message should at least mention the failing path so
	// users can locate the problem.
	if !strings.Contains(taiErr.Msg, "/dev/null/cannot-mkdir") {
		t.Fatalf("expected error message to name the failing path, got %q", taiErr.Msg)
	}
}

// TestResolve_does_not_touch_filesystem locks in that Resolve is pure —
// no MkdirAll, no file creation. The harness inspects the absence of
// the target directory after the call.
func TestResolve_does_not_touch_filesystem(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "should-not-exist")
	t.Setenv("TAI_DATA_DIR", target)

	got, err := datadir.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != target {
		t.Fatalf("want %q, got %q", target, got)
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatalf("Resolve unexpectedly created %s", target)
	}
}

// clearEnvForTest unsets a variable for the test's lifetime, restoring
// it on cleanup. Mirrors plugins/triage/internal/cmdtest's
// unsetEnvForTest helper so this package stays self-contained (no
// test-only dependency on cmdtest).
func clearEnvForTest(t *testing.T, name string) {
	t.Helper()
	prev, had := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("clearEnvForTest %s: %v", name, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(name, prev)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}
