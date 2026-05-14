package installer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielmastrorillo/tai/internal/cmdframework"
	"github.com/danielmastrorillo/tai/internal/installer"
)

// goldenSrc is a parseable bundled-command markdown. The frontmatter's
// content_hash field is zero-padded; Classify only inspects the body.
const goldenSrc = `---
name: "TAI: Probe"
description: "Probe."
category: "Workflow"
tags: [probe]
version: 1
content_hash: "sha256:0000000000000000000000000000000000000000000000000000000000000000"
---
body of probe
`

// writeSrc writes src at <dir>/probe.md and returns the path.
func writeSrc(t *testing.T, dir, src string) string {
	t.Helper()
	path := filepath.Join(dir, "probe.md")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// bodyHash returns the body hash of the given source.
func bodyHash(t *testing.T, src string) string {
	t.Helper()
	body, err := cmdframework.Body([]byte(src))
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	return cmdframework.HashBody(body)
}

// TestClassify_TCINST010_missing: when the target file does not exist,
// Classify returns ClassMissing.
func TestClassify_TCINST010_missing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.md")
	// File deliberately not created.

	got, err := installer.Classify(path, []string{
		"sha256:0000000000000000000000000000000000000000000000000000000000000001",
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got != installer.ClassMissing {
		t.Fatalf("Classify on missing file = %q, want %q", got, installer.ClassMissing)
	}
}

// TestClassify_TCINST011_up_to_date: when the on-disk body hash equals
// the LAST entry of the ledger, Classify returns ClassUpToDate.
func TestClassify_TCINST011_up_to_date(t *testing.T) {
	dir := t.TempDir()
	path := writeSrc(t, dir, goldenSrc)

	ledger := []string{
		"sha256:0000000000000000000000000000000000000000000000000000000000000001",
		bodyHash(t, goldenSrc),
	}
	got, err := installer.Classify(path, ledger)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got != installer.ClassUpToDate {
		t.Fatalf("Classify with current body = %q, want %q", got, installer.ClassUpToDate)
	}
}

// TestClassify_TCINST012_stale_but_untouched: when the body hash
// matches an EARLIER entry but not the last, Classify returns
// ClassStaleButUntouched.
func TestClassify_TCINST012_stale_but_untouched(t *testing.T) {
	dir := t.TempDir()
	path := writeSrc(t, dir, goldenSrc)

	// The on-disk hash is at position 0 (an older shipped version);
	// the current build's hash is at position 1.
	ledger := []string{
		bodyHash(t, goldenSrc),
		"sha256:0000000000000000000000000000000000000000000000000000000000000001",
	}
	got, err := installer.Classify(path, ledger)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got != installer.ClassStaleButUntouched {
		t.Fatalf("Classify with stale body = %q, want %q", got, installer.ClassStaleButUntouched)
	}
}

// TestClassify_TCINST013_user_modified: when the body hash does NOT
// appear in the ledger at all, Classify returns ClassUserModified.
func TestClassify_TCINST013_user_modified(t *testing.T) {
	dir := t.TempDir()
	path := writeSrc(t, dir, goldenSrc)

	ledger := []string{
		"sha256:0000000000000000000000000000000000000000000000000000000000000001",
		"sha256:0000000000000000000000000000000000000000000000000000000000000002",
	}
	got, err := installer.Classify(path, ledger)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got != installer.ClassUserModified {
		t.Fatalf("Classify with unknown body = %q, want %q", got, installer.ClassUserModified)
	}
}

// TestClassify_TCINST014_unparseable_file_is_user_modified: when the
// target file cannot be parsed as frontmatter+body, Classify returns
// ClassUserModified (most conservative).
func TestClassify_TCINST014_unparseable_file_is_user_modified(t *testing.T) {
	dir := t.TempDir()
	path := writeSrc(t, dir, "this is not a frontmatter document at all\n")

	ledger := []string{
		"sha256:0000000000000000000000000000000000000000000000000000000000000001",
	}
	got, err := installer.Classify(path, ledger)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got != installer.ClassUserModified {
		t.Fatalf("Classify on unparseable file = %q, want %q", got, installer.ClassUserModified)
	}
}

// TestClassify_empty_ledger_is_user_modified: when the ledger is empty
// (verb unknown or no history yet), an existing file is classified as
// user-modified — without a recorded history we cannot prove ownership.
func TestClassify_empty_ledger_is_user_modified(t *testing.T) {
	dir := t.TempDir()
	path := writeSrc(t, dir, goldenSrc)

	got, err := installer.Classify(path, nil)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got != installer.ClassUserModified {
		t.Fatalf("Classify with empty ledger = %q, want %q", got, installer.ClassUserModified)
	}
}

// TestClassify_permission_error_surfaces: an I/O error that is NOT
// fs.ErrNotExist must be returned to the caller (not silently mapped
// to a classification). We provoke a permission error by pointing at a
// directory and asking ReadFile to slurp it.
func TestClassify_permission_error_surfaces(t *testing.T) {
	// Reading a directory as a file fails on every supported platform.
	dir := t.TempDir()
	_, err := installer.Classify(dir, []string{
		"sha256:0000000000000000000000000000000000000000000000000000000000000001",
	})
	if err == nil {
		t.Fatal("expected error when reading a directory as a file, got nil")
	}
	if !strings.Contains(err.Error(), dir) {
		t.Fatalf("error should mention the path %q, got %v", dir, err)
	}
}
