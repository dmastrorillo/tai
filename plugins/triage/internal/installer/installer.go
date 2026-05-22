// Package installer contains the file-state classifier and supporting
// types that `tai install` and `tai uninstall` use to decide what to do
// with each target file.
//
// The classifier is the heart of the install command's safety contract:
// it compares a target file's body hash against the cumulative
// embedded ledger and emits one of four classifications:
//
//   - missing             → write current version (install)
//   - up-to-date          → skip silently
//   - stale-but-untouched → overwrite silently (install) / remove (uninstall)
//   - user-modified       → prompt the user (install) / leave alone (uninstall)
//
// The behavioural contract is in `openspec/specs/install/spec.md` (the
// long-lived capability spec); the proposal that introduced it lives in
// `openspec/changes/archive/*-add-install-command/`.
//
// This package depends on cmdframework for body parsing and hashing;
// it does NOT import the urfave/cli surface, so the classifier stays
// trivially testable in isolation. The ledger is passed in as a slice
// of hashes — the caller (install command) owns the lookup.
package installer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdframework"
)

// Classification is the four-state file label produced by Classify.
type Classification string

const (
	// ClassMissing — no file at the target path.
	ClassMissing Classification = "missing"

	// ClassUpToDate — file body hash equals the last entry in the ledger
	// (the current build's hash).
	ClassUpToDate Classification = "up-to-date"

	// ClassStaleButUntouched — file body hash appears in the ledger but
	// is not the last entry (so it's a version tai once shipped).
	ClassStaleButUntouched Classification = "stale-but-untouched"

	// ClassUserModified — file body hash does not appear in the ledger,
	// OR the file cannot be parsed as frontmatter+body (the latter is
	// the most conservative classification — anything we can't recognise
	// is treated as the user's, not ours).
	ClassUserModified Classification = "user-modified"
)

// Classify reads the file at targetPath and classifies it against
// ledger (a list of every body hash tai has ever shipped for the
// corresponding verb, oldest-first).
//
// An empty ledger always yields ClassUserModified for any file that
// exists — without a recorded history we have no evidence the file is
// ours. ClassMissing still applies when the file does not exist.
//
// Errors are reserved for I/O failures other than "file not exists" —
// e.g. permission denied on the target path. The caller decides whether
// to halt or continue.
func Classify(targetPath string, ledger []string) (Classification, error) {
	data, err := os.ReadFile(targetPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ClassMissing, nil
		}
		return "", fmt.Errorf("read %s: %w", targetPath, err)
	}

	// Parse the on-disk file. A parse failure means the file is not a
	// recognisable bundled-command shape — the most conservative label
	// is "user-modified". The body-parsing helper preserves the
	// trailing newline so hashes match exactly.
	body, err := cmdframework.Body(data)
	if err != nil {
		return ClassUserModified, nil
	}
	gotHash := cmdframework.HashBody(body)

	if len(ledger) == 0 {
		return ClassUserModified, nil
	}

	if ledger[len(ledger)-1] == gotHash {
		return ClassUpToDate, nil
	}
	for _, h := range ledger {
		if h == gotHash {
			return ClassStaleButUntouched, nil
		}
	}
	return ClassUserModified, nil
}
