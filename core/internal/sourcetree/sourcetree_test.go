package sourcetree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/testutil"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

type entry struct {
	name string
	path string
}

func testOptions() Options[entry] {
	return Options[entry]{
		Subdir:   "things",
		Ext:      ".md",
		Kind:     "thing",
		Verb:     "things",
		Code:     errcode.InternalError,
		Reserved: map[string]bool{"list": true},
		Parse: func(path, name string) (entry, error) {
			return entry{name: name, path: path}, nil
		},
	}
}

func seedTree(t *testing.T, files ...string) string {
	t.Helper()
	clone := t.TempDir()
	for _, rel := range files {
		p := filepath.Join(clone, "things", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return clone
}

// The load algorithm's contract: colon-namespaced lowercased names,
// alphabetical output, case-insensitive collisions warn and keep the
// alphabetically-earlier path, reserved bare names reject. These are
// also pinned end-to-end by the standards and workflow suites; this
// test pins them once at the shared implementation so a third
// consumer inherits verified semantics.
func TestLoad_names_sorts_and_dedupes(t *testing.T) {
	testutil.SkipIfCaseInsensitiveFS(t)
	clone := seedTree(t,
		"Zeta.md",
		"nested/Alpha.md",
		"NESTED/alpha.md", // case-insensitive collision with the line above
	)

	var warnings strings.Builder
	got, err := Load(clone, &warnings, testOptions())
	if err != nil {
		t.Fatal(err)
	}

	names := make([]string, len(got))
	for i, e := range got {
		names[i] = e.name
	}
	want := []string{"nested:alpha", "zeta"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}

	// Tie-break: "NESTED/alpha.md" sorts before "nested/Alpha.md", so
	// the uppercase-N path wins and the warning names both.
	if !strings.Contains(warnings.String(), `thing name "nested:alpha" is claimed by both`) {
		t.Errorf("missing collision warning, got: %q", warnings.String())
	}
	winner := got[0].path
	if filepath.Base(filepath.Dir(winner)) != "NESTED" {
		t.Errorf("alphabetically-earlier path should win, got %s", winner)
	}
}

// Runs on every filesystem (no case collision staged): pins the
// name-derivation and alphabetical-output contract locally even
// where the collision test above must skip.
func TestLoad_derives_colon_names_in_alphabetical_order(t *testing.T) {
	clone := seedTree(t,
		"Zeta.md",
		"nested/deep/Alpha.md",
		"beta.md",
		"notes.txt", // wrong extension — skipped
	)

	got, err := Load(clone, &strings.Builder{}, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"beta", "nested:deep:alpha", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].name != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, got[i].name, want[i])
		}
	}
}

func TestLoad_reserved_name_rejected(t *testing.T) {
	clone := seedTree(t, "list.md")

	_, err := Load(clone, &strings.Builder{}, testOptions())
	if err == nil {
		t.Fatal("want reserved-name error, got nil")
	}
	if !strings.Contains(err.Error(), `reserved name "list"`) {
		t.Errorf("unexpected message: %v", err)
	}
}

func TestLoad_missing_tree_is_empty_not_error(t *testing.T) {
	got, err := Load(t.TempDir(), &strings.Builder{}, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}
