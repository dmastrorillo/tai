package cmdframework_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/danielmastrorillo/tai/internal/cmdframework"
)

// TestBundledCommands_hash_matches_frontmatter walks commands/*.md from
// the repo root and verifies, for each bundled slash command, that the
// frontmatter's content_hash equals HashSource(body). This is the
// build-time invariant from add-tai-foundation tasks §5.6: if a body
// is edited without updating the frontmatter's hash, this test fails
// before merge.
//
// When the commands/ directory does not yet exist (the case during
// foundation application), the test is a no-op. Once add-install-command
// authors the directory, every file there is checked automatically.
func TestBundledCommands_hash_matches_frontmatter(t *testing.T) {
	root := repoRoot(t)
	commandsDir := filepath.Join(root, "commands")
	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("commands/ directory does not yet exist — install-time bundle hasn't shipped")
		}
		t.Fatalf("ReadDir %s: %v", commandsDir, err)
	}

	checked := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		path := filepath.Join(commandsDir, e.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		fm, body, err := cmdframework.Parse(src)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		want := cmdframework.HashBody(body)
		if fm.ContentHash != want {
			t.Errorf("%s: content_hash mismatch\n  frontmatter: %s\n  recomputed:  %s\n"+
				"run the tai-ledger helper or recompute by hand before committing",
				path, fm.ContentHash, want)
		}
		checked++
	}

	if checked == 0 {
		t.Log("no commands/*.md files found — skipping bundled-command hash check")
	}
}

// repoRoot walks up from the test's working directory until it finds a
// directory containing go.mod, returning that directory. Used so this
// test works regardless of where `go test` is invoked from.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod walking up from %s", wd)
		}
		dir = parent
	}
}
