// Package testutil holds tiny shared helpers used across the
// `core/internal/...` test packages. Each helper is exported only
// because Go requires that for cross-package use; nothing here is
// part of any production code path.
//
// The helpers in this package MUST NOT grow into a test framework —
// keep them small and obvious, or move them back next to their
// callers when only one test package uses them.
package testutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// AssertErrCode fails t when err is not (or does not wrap) a
// *errcode.Error whose Code equals want. Used by every test package
// that needs to anchor on the error-code taxonomy contract.
func AssertErrCode(t testing.TB, err error, want errcode.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %s, got nil", want)
	}
	var e *errcode.Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *errcode.Error, got %T %v", err, err)
	}
	if e.Code != want {
		t.Fatalf("error code: want %s, got %s", want, e.Code)
	}
}

// SkipIfCaseInsensitiveFS skips t when the temp filesystem treats
// `CASEPROBE` and `caseprobe` as the same file — the colliding-name
// BDD cases (TC-WF-008, TC-STD-006) cannot be staged on case-
// insensitive volumes (default APFS on macOS, NTFS on Windows). The
// production logic still runs on case-sensitive filesystems in CI.
func SkipIfCaseInsensitiveFS(t testing.TB) {
	t.Helper()
	dir := t.TempDir()
	upper := filepath.Join(dir, "CASEPROBE")
	lower := filepath.Join(dir, "caseprobe")
	if err := os.WriteFile(upper, []byte("u"), 0o644); err != nil {
		t.Fatalf("probe write: %v", err)
	}
	if err := os.WriteFile(lower, []byte("l"), 0o644); err != nil {
		t.Fatalf("probe write: %v", err)
	}
	probe, err := os.ReadFile(upper)
	if err != nil {
		t.Fatalf("probe read: %v", err)
	}
	if string(probe) != "u" {
		t.Skip("filesystem is case-insensitive; cannot stage colliding-name fixture")
	}
}
