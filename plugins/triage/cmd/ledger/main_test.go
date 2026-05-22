package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdframework"
)

// goldenSrc is a syntactically valid bundled-command markdown. The
// content_hash field is filled with zeros; the helper only computes the
// BODY hash, so the frontmatter value is irrelevant to ledger updates.
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

// writeFile writes content at path, failing the test on error. Parent
// directories must exist.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// readLedgerFile parses a ledger JSON file from disk.
func readLedgerFile(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	var hashes []string
	if err := json.Unmarshal(data, &hashes); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return hashes
}

// TestUpdateAll_new_command_creates_ledger covers the happy path: a
// fresh <verb>.md with no sibling ledger gets a ledger file containing
// exactly the current body hash.
func TestUpdateAll_new_command_creates_ledger(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "probe.md"), goldenSrc)

	updated, err := updateAll(dir)
	if err != nil {
		t.Fatalf("updateAll: %v", err)
	}
	if got := strings.Join(updated, ","); got != "probe" {
		t.Fatalf("updated = %q, want \"probe\"", got)
	}

	body, err := cmdframework.Body([]byte(goldenSrc))
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	wantHash := cmdframework.HashBody(body)

	hashes := readLedgerFile(t, filepath.Join(dir, "probe.ledger.json"))
	if len(hashes) != 1 {
		t.Fatalf("ledger len = %d, want 1", len(hashes))
	}
	if hashes[0] != wantHash {
		t.Fatalf("hashes[0] = %q, want %q", hashes[0], wantHash)
	}
}

// TestUpdateAll_idempotent: running updateAll twice with no body
// changes between runs is a no-op. The second run reports no updates
// and the ledger file is byte-identical.
func TestUpdateAll_idempotent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "probe.md"), goldenSrc)

	if _, err := updateAll(dir); err != nil {
		t.Fatalf("first updateAll: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(dir, "probe.ledger.json"))
	if err != nil {
		t.Fatalf("ReadFile after first run: %v", err)
	}

	updated, err := updateAll(dir)
	if err != nil {
		t.Fatalf("second updateAll: %v", err)
	}
	if len(updated) != 0 {
		t.Fatalf("second run updated %v, want empty", updated)
	}
	second, err := os.ReadFile(filepath.Join(dir, "probe.ledger.json"))
	if err != nil {
		t.Fatalf("ReadFile after second run: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("ledger file mutated on idempotent run\nfirst:  %q\nsecond: %q",
			first, second)
	}
}

// TestUpdateAll_changed_body_appends_entry: when a body has changed
// since the last ledger update, exactly ONE new entry is appended (the
// new current hash); existing entries are preserved in order.
func TestUpdateAll_changed_body_appends_entry(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "probe.md"), goldenSrc)
	if _, err := updateAll(dir); err != nil {
		t.Fatalf("first updateAll: %v", err)
	}

	// Replace the body with a different one (the frontmatter is the same).
	v2 := strings.Replace(goldenSrc, "body of probe\n", "different body of probe\n", 1)
	writeFile(t, filepath.Join(dir, "probe.md"), v2)

	updated, err := updateAll(dir)
	if err != nil {
		t.Fatalf("second updateAll: %v", err)
	}
	if got := strings.Join(updated, ","); got != "probe" {
		t.Fatalf("updated = %q, want \"probe\"", got)
	}

	hashes := readLedgerFile(t, filepath.Join(dir, "probe.ledger.json"))
	if len(hashes) != 2 {
		t.Fatalf("ledger len = %d, want 2 (v1 + v2)", len(hashes))
	}
	body, _ := cmdframework.Body([]byte(v2))
	wantHash := cmdframework.HashBody(body)
	if hashes[1] != wantHash {
		t.Fatalf("hashes[1] = %q, want %q", hashes[1], wantHash)
	}
}

// TestUpdateAll_malformed_ledger_surfaces_clear_error: an existing
// ledger file with invalid JSON produces an error that names the file,
// so the developer can locate and fix it.
func TestUpdateAll_malformed_ledger_surfaces_clear_error(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "probe.md"), goldenSrc)
	writeFile(t, filepath.Join(dir, "probe.ledger.json"), "not valid json")

	_, err := updateAll(dir)
	if err == nil {
		t.Fatal("expected error for malformed ledger, got nil")
	}
	if !strings.Contains(err.Error(), "probe.ledger.json") {
		t.Fatalf("error should name the malformed file; got %v", err)
	}
}

// TestUpdateAll_skips_readme: a `README.md` in the commands/ directory
// is not treated as a verb and gets no ledger.
func TestUpdateAll_skips_readme(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "README.md"), "# docs\n")

	updated, err := updateAll(dir)
	if err != nil {
		t.Fatalf("updateAll: %v", err)
	}
	if len(updated) != 0 {
		t.Fatalf("README.md should be skipped, but got updated=%v", updated)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.ledger.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("README.ledger.json should not exist, got err=%v", err)
	}
}
