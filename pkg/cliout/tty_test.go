package cliout_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dmastrorillo/tai/pkg/cliout"
)

// TestIsTTY_TCCLI001_non_file_is_non_tty exercises TC-CLI-001 from
// core/test-cases.md: a writer that isn't an *os.File is treated as
// non-TTY. The bytes.Buffer used by every test in this codebase falls
// into this branch, so the assertion locks the contract at the type
// boundary.
func TestIsTTY_TCCLI001_non_file_is_non_tty(t *testing.T) {
	var buf bytes.Buffer
	if cliout.IsTTY(&buf) {
		t.Fatal("bytes.Buffer must be reported as non-TTY")
	}
}

// TestIsTTY_TCCLI002_regular_file_is_non_tty exercises TC-CLI-002: an
// *os.File pointing at a regular file on disk is non-TTY. This pins
// the file-vs-terminal distinction so the helper stays conservative
// when output is redirected to disk.
func TestIsTTY_TCCLI002_regular_file_is_non_tty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if cliout.IsTTY(f) {
		t.Fatal("regular file must be reported as non-TTY")
	}
}

// TestIsTTY_nil_is_non_tty pins the defensive behaviour for a nil
// writer — should be reported non-TTY without panicking. Not tied to a
// TC-ID; pure defensive engine invariant.
func TestIsTTY_nil_is_non_tty(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("IsTTY(nil) panicked: %v", r)
		}
	}()
	if cliout.IsTTY(nil) {
		t.Fatal("nil writer must be reported as non-TTY")
	}
}
