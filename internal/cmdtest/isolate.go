package cmdtest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// Isolated is a fully-isolated test environment: a tmp directory rooted at
// t.TempDir() with a private HOME (and USERPROFILE on Windows), a private
// TAI_DATA_DIR, and any other env vars callers explicitly set.
//
// All environment changes are torn down via t.Cleanup, so tests do not
// leak state into each other when run with -parallel.
type Isolated struct {
	// Root is the test's private tmp directory. Use it to write fixtures
	// or inspect what tai produced.
	Root string
	// Home is the private HOME directory; everything under Root/home.
	Home string
	// DataDir is the resolved TAI_DATA_DIR; everything under Root/data.
	DataDir string
}

// Isolate creates an Isolated test environment and wires the relevant
// env vars for the duration of the test:
//
//   - HOME is set on every platform.
//   - USERPROFILE is set on Windows (where it is the conventional home
//     directory pointer).
//   - TAI_DATA_DIR is set to a per-test data directory.
//   - XDG_DATA_HOME is unset (not merely emptied) so the foundation's
//     data-directory resolver does not see a host-machine value.
//
// Tests that want to override additional env vars can call t.Setenv after
// Isolate returns; those overrides are also torn down on t.Cleanup.
//
// Note: this does NOT change the process's working directory. Tests that
// need a specific cwd (e.g. to exercise repo detection) should call Chdir.
func Isolate(t *testing.T) *Isolated {
	t.Helper()

	root := t.TempDir()
	home := filepath.Join(root, "home")
	dataDir := filepath.Join(root, "data")

	for _, d := range []string{home, dataDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("Isolate: mkdir %s: %v", d, err)
		}
	}

	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
	t.Setenv("TAI_DATA_DIR", dataDir)
	unsetEnvForTest(t, "XDG_DATA_HOME")

	return &Isolated{Root: root, Home: home, DataDir: dataDir}
}

// unsetEnvForTest unsets an environment variable for the duration of the
// test, restoring its prior value (or absence) via t.Cleanup. testing.T's
// Setenv only sets values; this is the missing-companion helper.
func unsetEnvForTest(t *testing.T, name string) {
	t.Helper()
	prev, had := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unsetEnvForTest %s: %v", name, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(name, prev)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

// chdirMu and chdirOwner together implement a panic-on-concurrent-use
// guard for Chdir. The process cwd is global state; two parallel tests
// calling Chdir would interleave non-deterministically. The guard makes
// the bug loud rather than silent.
var (
	chdirMu    sync.Mutex
	chdirOwner string // empty when no test currently holds the cwd
)

// Chdir changes the process's working directory to dir for the test's
// lifetime, restoring the original cwd via t.Cleanup.
//
// The process cwd is global state — this helper is UNSAFE under
// t.Parallel(). The guard above panics if a second test attempts to
// Chdir while another test's Chdir scope is still active.
func Chdir(t *testing.T, dir string) {
	t.Helper()

	chdirMu.Lock()
	if chdirOwner != "" {
		owner := chdirOwner
		chdirMu.Unlock()
		panic(fmt.Sprintf(
			"cmdtest.Chdir: concurrent call from %q while %q still owns the cwd; "+
				"do not call t.Parallel() in tests that use Chdir",
			t.Name(), owner))
	}
	chdirOwner = t.Name()
	chdirMu.Unlock()

	orig, err := os.Getwd()
	if err != nil {
		clearChdirOwner()
		t.Fatalf("Chdir: Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		clearChdirOwner()
		t.Fatalf("Chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Logf("Chdir restore failed: %v", err)
		}
		clearChdirOwner()
	})
}

func clearChdirOwner() {
	chdirMu.Lock()
	chdirOwner = ""
	chdirMu.Unlock()
}

// WriteFile is a convenience for fixture creation: writes content to
// path (relative to Root or absolute), creating parent directories as
// needed, and fails the test on error.
func (i *Isolated) WriteFile(t *testing.T, path, content string) {
	t.Helper()
	if !filepath.IsAbs(path) {
		path = filepath.Join(i.Root, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("WriteFile: mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}
