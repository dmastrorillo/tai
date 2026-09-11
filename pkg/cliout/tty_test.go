package cliout_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creack/pty"

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

// TC-CLI-006 — the reader-side check treats a non-*os.File as
// non-interactive. Every test in this codebase passes a strings.Reader
// or bytes.Buffer as stdin, so this is the branch they all take.
func TestIsTTYReader_TCCLI006_non_file_is_non_tty(t *testing.T) {
	var buf bytes.Buffer
	if cliout.IsTTYReader(&buf) {
		t.Fatal("bytes.Buffer must be reported as non-TTY")
	}
	if cliout.IsTTYReader(strings.NewReader("y\n")) {
		t.Fatal("strings.Reader must be reported as non-TTY")
	}
}

// TC-CLI-007 — an *os.File on a regular file is non-interactive. This
// is the shape stdin takes under `< answers.txt`, where a prompt would
// read bytes nobody is there to type.
func TestIsTTYReader_TCCLI007_regular_file_is_non_tty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "in.txt")
	if err := os.WriteFile(path, []byte("y\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if cliout.IsTTYReader(f) {
		t.Fatal("regular file must be reported as non-TTY")
	}
}

// A nil reader is non-interactive and must not panic.
func TestIsTTYReader_nil_is_non_tty(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("IsTTYReader(nil) panicked: %v", r)
		}
	}()
	if cliout.IsTTYReader(nil) {
		t.Fatal("nil reader must be reported as non-TTY")
	}
}

// TC-CLI-008 — a real terminal reads as a terminal.
//
// This is the only case that can catch a reader-side check wired to
// the wrong stream: under `go test` os.Stdin is not a terminal either,
// so a bug like `isTerminalFile(os.Stdin)` returns false for every
// non-terminal input and matches what the false-branch cases above
// expect. Only a genuine terminal separates them.
func TestIsTTYReader_TCCLI008_a_real_terminal_is_a_tty(t *testing.T) {
	primary, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pty available on this platform: %v", err)
	}
	t.Cleanup(func() {
		_ = tty.Close()
		_ = primary.Close()
	})

	if !cliout.IsTTYReader(tty) {
		t.Error("a pty must be reported as a TTY on the reader side")
	}
	if !cliout.IsTTY(tty) {
		t.Error("a pty must be reported as a TTY on the writer side")
	}
}
