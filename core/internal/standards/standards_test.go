package standards_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/standards"
	"github.com/dmastrorillo/tai/core/internal/testutil"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// seedStandards writes the given relative-path → markdown-body map
// under a fresh `<clone>/standards/` tree and returns the cloneDir.
//
// Not tied to a TC-ID — test fixture helper.
func seedStandards(t *testing.T, files map[string]string) string {
	t.Helper()
	clone := t.TempDir()
	for rel, body := range files {
		full := filepath.Join(clone, "standards", rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return clone
}

// TestLoad_TCSTD001_description_from_frontmatter exercises TC-STD-001.
func TestLoad_TCSTD001_description_from_frontmatter(t *testing.T) {
	clone := seedStandards(t, map[string]string{
		"SDLC.md": "---\ndescription: Software development lifecycle\n---\nbody\n",
	})

	got, err := standards.Load(clone, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 standard, got %d", len(got))
	}
	if got[0].Name != "sdlc" {
		t.Errorf("Name: want %q, got %q", "sdlc", got[0].Name)
	}
	if got[0].Description != "Software development lifecycle" {
		t.Errorf("Description: want %q, got %q", "Software development lifecycle", got[0].Description)
	}
}

// TestLoad_crlf_frontmatter_parses_description is a regression test
// for a CRLF off-by-one bug in splitFrontmatter: when the opening
// fence is `---\r\n` (5 bytes) but the slice offset assumed `---\n`
// (4 bytes), a stray `\r` corrupted the YAML block and the
// description silently fell back to MissingDescription. Not tied to a
// TC-ID — it's a defence against a specific code-level regression.
func TestLoad_crlf_frontmatter_parses_description(t *testing.T) {
	clone := seedStandards(t, map[string]string{
		"crlf.md": "---\r\ndescription: Windows-authored\r\n---\r\nbody\r\n",
	})

	got, err := standards.Load(clone, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 standard, got %d", len(got))
	}
	if got[0].Description != "Windows-authored" {
		t.Errorf("CRLF frontmatter description: want %q, got %q",
			"Windows-authored", got[0].Description)
	}
}

// TestLoad_TCSTD002_missing_frontmatter_fallback exercises TC-STD-002.
func TestLoad_TCSTD002_missing_frontmatter_fallback(t *testing.T) {
	clone := seedStandards(t, map[string]string{
		"SDLC.md": "no frontmatter at all\n",
	})

	got, err := standards.Load(clone, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 standard, got %d", len(got))
	}
	if got[0].Description != standards.MissingDescription {
		t.Errorf("Description: want %q, got %q", standards.MissingDescription, got[0].Description)
	}
	if string(got[0].Body) != "no frontmatter at all\n" {
		t.Errorf("Body should be the file content unchanged, got: %q", got[0].Body)
	}
}

// TestLoad_TCSTD003_nested_colon_namespaced_name exercises TC-STD-003.
func TestLoad_TCSTD003_nested_colon_namespaced_name(t *testing.T) {
	clone := seedStandards(t, map[string]string{
		"devOps/security/best-practices.md": "body\n",
	})

	got, err := standards.Load(clone, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 standard, got %d", len(got))
	}
	if got[0].Name != "devops:security:best-practices" {
		t.Errorf("Name: want %q, got %q", "devops:security:best-practices", got[0].Name)
	}
}

// TestLoad_TCSTD004_reserved_name_list_rejected exercises TC-STD-004.
func TestLoad_TCSTD004_reserved_name_list_rejected(t *testing.T) {
	clone := seedStandards(t, map[string]string{
		"list.md": "body\n",
	})

	_, err := standards.Load(clone, io.Discard)
	testutil.AssertErrCode(t, err, errcode.StandardInvalid)
	if !strings.Contains(err.Error(), "list") || !strings.Contains(err.Error(), "reserved") {
		t.Errorf("error should name `list` as reserved, got: %v", err)
	}
}

// TestLoad_TCSTD005_reserved_name_load_rejected exercises TC-STD-005.
func TestLoad_TCSTD005_reserved_name_load_rejected(t *testing.T) {
	clone := seedStandards(t, map[string]string{
		"load.md": "body\n",
	})

	_, err := standards.Load(clone, io.Discard)
	testutil.AssertErrCode(t, err, errcode.StandardInvalid)
	if !strings.Contains(err.Error(), "load") || !strings.Contains(err.Error(), "reserved") {
		t.Errorf("error should name `load` as reserved, got: %v", err)
	}
}

// TestLoad_TCSTD006_duplicate_warning_first_wins exercises TC-STD-006.
//
// As with the workflows collision case, this requires a case-sensitive
// filesystem.
func TestLoad_TCSTD006_duplicate_warning_first_wins(t *testing.T) {
	testutil.SkipIfCaseInsensitiveFS(t)
	clone := seedStandards(t, map[string]string{
		"Foo.md": "---\ndescription: capital F\n---\nbody-F\n",
		"foo.md": "---\ndescription: lowercase f\n---\nbody-f\n",
	})

	var warn bytes.Buffer
	got, err := standards.Load(clone, &warn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("collision should reduce to one standard, got %d", len(got))
	}
	if got[0].Description != "capital F" {
		t.Errorf("first-wins should select Foo.md, got description %q", got[0].Description)
	}
	if !strings.Contains(warn.String(), "Foo.md") || !strings.Contains(warn.String(), "foo.md") {
		t.Errorf("warning should name both source paths, got: %q", warn.String())
	}
}
