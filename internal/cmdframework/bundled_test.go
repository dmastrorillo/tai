package cmdframework_test

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/internal/cmdframework"
)

// TestBundle_TCINST003_current_hash_is_last_entry exercises the
// build-time invariant: for every bundled verb in BundleFS, the body
// hash of `commands/<verb>.md` equals the last element of
// `commands/<verb>.ledger.json`. If a body is edited without re-running
// the tai-ledger helper, this test fails before merge.
//
// Walks the embedded FS rather than the on-disk repo so the test sees
// exactly what the production binary sees.
func TestBundle_TCINST003_current_hash_is_last_entry(t *testing.T) {
	verbs := cmdframework.Verbs()
	if len(verbs) == 0 {
		t.Log("no bundled verbs in this build — nothing to verify")
		return
	}

	for _, verb := range verbs {
		want, err := cmdframework.BundleHash(verb)
		if err != nil {
			t.Errorf("BundleHash(%q): %v", verb, err)
			continue
		}
		hist, err := cmdframework.LedgerStrict(verb)
		if err != nil {
			t.Errorf("LedgerStrict(%q): %v", verb, err)
			continue
		}
		if len(hist) == 0 {
			t.Errorf("ledger for %q is empty — run `make ledger-update`", verb)
			continue
		}
		if got := hist[len(hist)-1]; got != want {
			t.Errorf("%s: current body hash %s != last ledger entry %s\n"+
				"run `make ledger-update` and re-commit", verb, want, got)
		}
	}
}

// TestBundle_frontmatter_matches_body_hash exercises the parallel
// add-tai-foundation invariant: every bundled `<verb>.md`'s frontmatter
// content_hash equals HashBody(body). Same invariant the foundation
// guarded against before bundling existed, retained here so an edit to
// a body without a frontmatter update is caught alongside the ledger
// invariant.
func TestBundle_frontmatter_matches_body_hash(t *testing.T) {
	verbs := cmdframework.Verbs()
	if len(verbs) == 0 {
		t.Log("no bundled verbs in this build — nothing to verify")
		return
	}

	for _, verb := range verbs {
		src, err := cmdframework.BundleSource(verb)
		if err != nil {
			t.Errorf("BundleSource(%q): %v", verb, err)
			continue
		}
		fm, body, err := cmdframework.Parse(src)
		if err != nil {
			t.Errorf("parse %s.md: %v", verb, err)
			continue
		}
		if want := cmdframework.HashBody(body); fm.ContentHash != want {
			t.Errorf("%s: content_hash %s != recomputed %s",
				verb, fm.ContentHash, want)
		}
	}
}

// TestBundleFS_only_known_extensions_present is a hygiene check: the
// commands/ embed should contain ONLY the README, *.md, and
// *.ledger.json files. A stray file likely indicates an editor backup
// or a misnamed ledger.
func TestBundleFS_only_known_extensions_present(t *testing.T) {
	err := fs.WalkDir(cmdframework.BundleFS, "commands", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		base := path.Base(p)
		switch {
		case base == "README.md":
		case strings.HasSuffix(base, ".ledger.json"):
		case strings.HasSuffix(base, ".md"):
		default:
			t.Errorf("unexpected bundled file: %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk commands/: %v", err)
	}
}
