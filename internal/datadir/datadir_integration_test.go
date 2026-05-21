//go:build integration

package datadir_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dmastrorillo/tai/internal/datadir"
	"github.com/dmastrorillo/tai/internal/errcode"
)

// TestEnsureWritable_creates_missing_tree_when_writable verifies the
// happy path against the real filesystem: a fresh tmp directory is
// resolved, EnsureWritable creates the leaf, and a subsequent call is
// a no-op (idempotent).
func TestEnsureWritable_creates_missing_tree_when_writable(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "tai")
	t.Setenv("TAI_DATA_DIR", target)

	got, err := datadir.EnsureWritable()
	if err != nil {
		t.Fatalf("EnsureWritable: %v", err)
	}
	if got != target {
		t.Fatalf("want %q, got %q", target, got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("expected target to exist after EnsureWritable: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected target to be a directory")
	}

	// Idempotent second call.
	if _, err := datadir.EnsureWritable(); err != nil {
		t.Fatalf("second EnsureWritable: %v", err)
	}
}

// TestEnsureWritable_read_only_dir_surfaces_DATA_DIR_UNWRITABLE exercises
// the unwritable-dir path via a real chmod. POSIX only — Windows uses a
// different permissions model.
func TestEnsureWritable_read_only_dir_surfaces_DATA_DIR_UNWRITABLE(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX chmod semantics not portable to Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses POSIX permission checks")
	}

	tmp := t.TempDir()
	target := filepath.Join(tmp, "tai")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(target, 0o555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(target, 0o755) })

	t.Setenv("TAI_DATA_DIR", target)

	_, err := datadir.EnsureWritable()
	if err == nil {
		t.Fatal("expected DATA_DIR_UNWRITABLE on read-only directory, got nil")
	}
	taiErr, ok := errcode.As(err)
	if !ok {
		t.Fatalf("expected *errcode.Error, got %T: %v", err, err)
	}
	if taiErr.Code != errcode.DataDirUnwritable {
		t.Fatalf("expected DATA_DIR_UNWRITABLE, got %s", taiErr.Code)
	}
}
