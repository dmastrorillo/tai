package sync

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// TestTargetSubpath_three_state_semantics locks the nil/empty/value
// override semantics. The three branches map directly onto the spec's
// "default vs skip vs override" sub-path rule.
//
// Not tied to a TC-ID because the user-observable surfaces (sync
// behaviour, target list table) are covered by TC-CONF-005 and
// TC-SYNC-004/005 at the CLI boundary; this is the engine seam those
// rely on, pinned independently so a refactor of the override logic
// can't silently break them.
func TestTargetSubpath_three_state_semantics(t *testing.T) {
	empty := ""
	custom := "my-custom"
	cases := []struct {
		name        string
		override    *string
		defaultName string
		want        string
	}{
		{"nil applies default", nil, "skills", filepath.Join("/root", "skills")},
		{"empty signals skip", &empty, "skills", ""},
		{"value overrides", &custom, "skills", filepath.Join("/root", "my-custom")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TargetSubpath("/root", tc.override, tc.defaultName)
			if got != tc.want {
				t.Errorf("TargetSubpath = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestJoinRel_normalises_forward_slashes confirms manifest entries
// (which are forward-slash-only on disk for cross-platform stability)
// are joined with the platform separator at filesystem-touch time.
//
// Not tied to a TC-ID because cross-platform path correctness is an
// engine invariant; no end-to-end TC exercises it on Windows in this
// repo today.
func TestJoinRel_normalises_forward_slashes(t *testing.T) {
	got := JoinRel("/base", "skills/triage/import.md")
	want := filepath.Join("/base", "skills", "triage", "import.md")
	if got != want {
		t.Errorf("JoinRel = %q, want %q", got, want)
	}
}

// TestSourceFiles_returns_empty_when_category_missing pins the
// graceful-skip rule for source repos that don't carry every
// category. The function returns ([]string{}, nil) — not nil and not
// an error — so callers can iterate over an empty slice without
// special-casing.
//
// Not tied to a TC-ID; engine invariant guarding the per-category
// walk used by buildPlan.
func TestSourceFiles_returns_empty_when_category_missing(t *testing.T) {
	tmp := t.TempDir()
	// No category subdirectories at all.
	got, err := SourceFiles(tmp, CategorySkills)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatalf("SourceFiles should return non-nil empty slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("SourceFiles should return empty slice, got %v", got)
	}
}

// TestSourceFiles_returns_sorted_relative_paths confirms the walker
// produces deterministic output suitable for batched-prompt rendering.
func TestSourceFiles_returns_sorted_relative_paths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test layout uses unix separators; the helper is exercised on Windows by sync e2e tests")
	}
	tmp := t.TempDir()
	skills := filepath.Join(tmp, "skills")
	for _, rel := range []string{"zebra.md", "alpha.md", "nested/inside.md"} {
		full := filepath.Join(skills, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	got, err := SourceFiles(tmp, CategorySkills)
	if err != nil {
		t.Fatalf("SourceFiles: %v", err)
	}
	want := []string{"alpha.md", "nested/inside.md", "zebra.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SourceFiles = %v, want %v", got, want)
	}
}
