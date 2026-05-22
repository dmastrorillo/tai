// Command tai-ledger keeps each bundled command's hash ledger in sync.
//
// For every `commands/<verb>.md` it finds, the helper recomputes the
// body's sha256 and appends it to `commands/<verb>.ledger.json` iff the
// hash is not already the last entry. The helper is idempotent: running
// it twice with no body changes is a no-op.
//
// The helper is invoked from the release pipeline (see the `make
// ledger-update` Makefile target) and from a developer's terminal after
// editing a command body. It is NOT shipped as part of the user-facing
// `tai` binary; it ships as a separate binary in this directory.
//
// Usage:
//
//	go run ./cmd/tai-ledger             (auto-resolves the commands/ dir)
//	go run ./cmd/tai-ledger -dir <path> (override the commands/ dir)
//
// Default --dir is `internal/cmdframework/commands/` relative to the
// repo root (located by walking up the tree until a go.mod is found).
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdframework"
)

func main() {
	dirFlag := flag.String("dir", "", "path to the bundled commands/ directory (default: <repo>/internal/cmdframework/commands)")
	flag.Parse()

	dir, err := resolveCommandsDir(*dirFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	updated, err := updateAll(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if len(updated) == 0 {
		fmt.Println("ledgers up to date — no changes")
		return
	}
	fmt.Printf("updated %d ledger(s):\n", len(updated))
	for _, verb := range updated {
		fmt.Printf("  %s\n", verb)
	}
}

// resolveCommandsDir picks the directory to scan. An explicit -dir flag
// wins; otherwise we walk up from the working directory until go.mod is
// found and append `internal/cmdframework/commands`.
func resolveCommandsDir(flagValue string) (string, error) {
	if flagValue != "" {
		info, err := os.Stat(flagValue)
		if err != nil {
			return "", fmt.Errorf("--dir: %w", err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("--dir %q is not a directory", flagValue)
		}
		return flagValue, nil
	}
	root, err := findRepoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "internal", "cmdframework", "commands"), nil
}

// findRepoRoot walks up from the current working directory until a
// directory containing go.mod is found.
func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod walking up from %s", wd)
		}
		dir = parent
	}
}

// updateAll iterates every `<verb>.md` in dir, appending the current
// body hash to `<verb>.ledger.json` when it isn't already the last
// entry. README.md (and any other documented non-verb markdown — see
// the skip set below) is excluded by name. Every remaining .md file
// MUST be a valid bundled-command frontmatter+body, otherwise the
// helper fails loudly with a parse error naming the file. This
// stricter heuristic is intentional: the runtime `cmdframework.Verbs()`
// recognises verbs by the presence of a paired ledger, but the helper
// itself is what CREATES that ledger, so it cannot use the runtime
// heuristic to bootstrap a new verb.
//
// Returns the sorted list of verbs whose ledger files were created or
// extended (used for the report on stdout).
func updateAll(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}

	var updated []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		if isNonVerbMarkdown(name) {
			continue
		}
		verb := strings.TrimSuffix(name, ".md")
		changed, err := updateOne(dir, verb)
		if err != nil {
			return nil, err
		}
		if changed {
			updated = append(updated, verb)
		}
	}
	sort.Strings(updated)
	return updated, nil
}

// isNonVerbMarkdown is the centralised list of markdown filenames in
// commands/ that are documentation, not bundled slash-commands. The set
// is kept tiny on purpose — adding to it without a paired comment is a
// signal of leaking documentation into the bundle directory.
func isNonVerbMarkdown(name string) bool {
	switch name {
	case "README.md":
		return true
	}
	return false
}

// updateOne computes the body hash for `<dir>/<verb>.md` and appends it
// to `<dir>/<verb>.ledger.json` iff the hash differs from the last
// entry. Returns true when the ledger was created or extended.
func updateOne(dir, verb string) (bool, error) {
	mdPath := filepath.Join(dir, verb+".md")
	src, err := os.ReadFile(mdPath)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", mdPath, err)
	}
	hash, err := cmdframework.HashSource(src)
	if err != nil {
		return false, fmt.Errorf("hash %s: %w", mdPath, err)
	}

	ledgerPath := filepath.Join(dir, verb+".ledger.json")
	existing, err := readLedger(ledgerPath)
	if err != nil {
		return false, err
	}
	if len(existing) > 0 && existing[len(existing)-1] == hash {
		return false, nil
	}
	existing = append(existing, hash)
	if err := writeLedger(ledgerPath, existing); err != nil {
		return false, err
	}
	return true, nil
}

func readLedger(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var hashes []string
	if err := json.Unmarshal(data, &hashes); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return hashes, nil
}

func writeLedger(path string, hashes []string) error {
	// Pretty-format so PR diffs are readable: one hash per line, inside
	// brackets, two-space indent — same default `json.MarshalIndent`
	// produces. A trailing newline keeps the file POSIX-friendly.
	data, err := json.MarshalIndent(hashes, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ledger: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
