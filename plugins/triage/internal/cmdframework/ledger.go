// This file owns the per-verb hash-history access contract and the
// embedded bundle of slash-command markdowns those hashes describe.
//
// The conceptual path used in the docs and OpenSpec proposals is
// `commands/<verb>.md`; physically the directory is rooted at
// `internal/cmdframework/commands/` because //go:embed paths must be
// relative to the source file and cannot traverse upward.

package cmdframework

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// BundleFS is the embedded view of internal/cmdframework/commands/. The
// install command, the build-time hash invariant test, and the
// `Ledger` reader all walk through this single seam so production
// behaviour and the test suite exercise the same byte sequences.
//
// The `all:` prefix is necessary so the directory is embedded even
// before any `<verb>.md` files exist — the README placeholder is
// always present and keeps the build green during bootstrap.
//
//go:embed all:commands
var BundleFS embed.FS

const bundleDir = "commands"

// ledgerHashRe is the lexical shape every ledger entry must match.
var ledgerHashRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Verbs returns the names of every bundled verb in this build, sorted
// alphabetically.
//
// A verb is recognised when BOTH `commands/<verb>.md` and
// `commands/<verb>.ledger.json` are present in the embedded FS. The
// `README.md` placeholder is filtered out because it has no matching
// ledger file.
func Verbs() []string {
	entries, err := BundleFS.ReadDir(bundleDir)
	if err != nil {
		return nil
	}
	var verbs []string
	for _, e := range entries {
		name := e.Name()
		const suffix = ".ledger.json"
		if !strings.HasSuffix(name, suffix) {
			continue
		}
		verb := strings.TrimSuffix(name, suffix)
		if _, err := BundleFS.Open(path.Join(bundleDir, verb+".md")); err != nil {
			continue
		}
		verbs = append(verbs, verb)
	}
	sort.Strings(verbs)
	return verbs
}

// BundleSource returns the raw bytes of `commands/<verb>.md`
// (frontmatter + body, exactly as installed). Returns fs.ErrNotExist
// when verb is not a recognised bundled command.
func BundleSource(verb string) ([]byte, error) {
	return BundleFS.ReadFile(path.Join(bundleDir, verb+".md"))
}

// BundleBody returns the body bytes of `commands/<verb>.md`
// (post-frontmatter, trailing newline preserved). Returns
// fs.ErrNotExist when verb is not a recognised bundled command.
func BundleBody(verb string) ([]byte, error) {
	src, err := BundleSource(verb)
	if err != nil {
		return nil, err
	}
	body, err := Body(src)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", verb, err)
	}
	return body, nil
}

// BundleHash returns the sha256:<hex> body hash for verb's bundled
// markdown.
func BundleHash(verb string) (string, error) {
	body, err := BundleBody(verb)
	if err != nil {
		return "", err
	}
	return HashBody(body), nil
}

// LedgerStrict returns the cumulative hash history for verb,
// oldest-first, with full corruption diagnostics.
//
// When the ledger file is absent, the returned slice is empty (this
// signals "no history" — every disk file is treated as user-modified).
// When the ledger IS present but malformed, the returned error is a
// *errcode.Error{Code: InstallLedgerCorrupt} so callers route the
// failure through the standard error contract.
func LedgerStrict(verb string) ([]string, error) {
	return ledgerFromFS(BundleFS, verb)
}

// ledgerFromFS is the unexported core of LedgerStrict, parameterised on
// the source filesystem so tests can exercise the corruption branches
// with a testing/fstest.MapFS without mutating the production embed.
// LedgerStrict pins this to BundleFS.
func ledgerFromFS(fsys fs.FS, verb string) ([]string, error) {
	data, err := fs.ReadFile(fsys, path.Join(bundleDir, verb+".ledger.json"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, errcode.Wrapf(errcode.InstallLedgerCorrupt, err,
			"cannot read embedded ledger for %q", verb).
			WithHelp("re-run `make ledger-update` and rebuild")
	}
	var hashes []string
	if err := json.Unmarshal(data, &hashes); err != nil {
		return nil, errcode.Wrapf(errcode.InstallLedgerCorrupt, err,
			"ledger for %q is not valid JSON", verb).
			WithHelp("re-run `make ledger-update` and rebuild")
	}
	for i, h := range hashes {
		if !ledgerHashRe.MatchString(h) {
			return nil, errcode.Newf(errcode.InstallLedgerCorrupt,
				"ledger for %q has malformed entry at index %d: %q", verb, i, h).
				WithHelp("re-run `make ledger-update` and rebuild")
		}
	}
	return hashes, nil
}

// Ledger returns the cumulative history of content_hash values ever
// shipped for the named verb's slash-command markdown. Order is
// oldest-first; the LAST element is the hash of the current build's
// commands/<verb>.md body (a property the build-time test enforces).
//
// Unknown verbs and corrupt ledgers return an empty slice — this
// preserves the foundation's pre-bundle API for callers that don't
// need diagnostic detail. Callers that need to surface a corrupt
// ledger to the user MUST call LedgerStrict.
func Ledger(verb string) []string {
	hashes, err := LedgerStrict(verb)
	if err != nil {
		return nil
	}
	return hashes
}
