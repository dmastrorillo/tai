package cmdframework_test

import (
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdframework"
)

// goldenSrc is the canonical bundled-command shape every TC-CMD case
// builds from. Body bytes are everything after the closing `---\n`.
const goldenSrc = `---
name: "TAI: Import"
description: "Import comments from a PR or branch into tai."
category: "Workflow"
tags: [tai, triage]
version: 1
content_hash: "sha256:0000000000000000000000000000000000000000000000000000000000000000"
---
This is the body.

Multiple paragraphs.
`

// TestParse_TCCMD003_golden_good_frontmatter exercises TC-CMD-003: a
// well-formed bundled command parses into a populated Frontmatter and
// the body is everything after the closing `---\n`.
func TestParse_TCCMD003_golden_good_frontmatter(t *testing.T) {
	fm, body, err := cmdframework.Parse([]byte(goldenSrc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if fm.Name != "TAI: Import" {
		t.Errorf("Name: want %q, got %q", "TAI: Import", fm.Name)
	}
	if fm.Category != "Workflow" {
		t.Errorf("Category: want Workflow, got %q", fm.Category)
	}
	if got := strings.Join(fm.Tags, ","); got != "tai,triage" {
		t.Errorf("Tags: want tai,triage, got %s", got)
	}
	if fm.Version != 1 {
		t.Errorf("Version: want 1, got %d", fm.Version)
	}
	if !strings.HasPrefix(fm.ContentHash, "sha256:") {
		t.Errorf("ContentHash missing sha256: prefix: %q", fm.ContentHash)
	}
	if !strings.HasPrefix(string(body), "This is the body.") {
		t.Errorf("body should start with 'This is the body.', got %q", string(body))
	}
	if !strings.HasSuffix(string(body), "\n") {
		t.Errorf("body should preserve trailing newline")
	}
}

// TestParse_TCCMD004_missing_content_hash exercises TC-CMD-004: a
// frontmatter missing content_hash is rejected.
func TestParse_TCCMD004_missing_content_hash(t *testing.T) {
	src := `---
name: "x"
description: "y"
category: "Workflow"
tags: [a]
version: 1
---
body
`
	_, _, err := cmdframework.Parse([]byte(src))
	if err == nil {
		t.Fatal("expected error for missing content_hash, got nil")
	}
	if !strings.Contains(err.Error(), "content_hash") {
		t.Fatalf("expected error to name content_hash, got %v", err)
	}
}

// TestParse_TCCMD005_unknown_field exercises TC-CMD-005: an unknown
// top-level key is rejected.
func TestParse_TCCMD005_unknown_field(t *testing.T) {
	src := `---
name: "x"
description: "y"
category: "Workflow"
tags: [a]
version: 1
content_hash: "sha256:0000000000000000000000000000000000000000000000000000000000000000"
priority: "p0"
---
body
`
	_, _, err := cmdframework.Parse([]byte(src))
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "priority") {
		t.Fatalf("expected error to name the unknown field, got %v", err)
	}
}

// TestParse_block_tags accepts the YAML block-array form.
func TestParse_block_tags(t *testing.T) {
	src := `---
name: "x"
description: "y"
category: "Workflow"
tags:
  - tai
  - triage
version: 1
content_hash: "sha256:0000000000000000000000000000000000000000000000000000000000000000"
---
body
`
	fm, _, err := cmdframework.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := strings.Join(fm.Tags, ","); got != "tai,triage" {
		t.Fatalf("want tai,triage, got %s", got)
	}
}

// TestParse_invalid_hash rejects content_hash values that don't match
// the sha256:<64-lower-hex> shape.
func TestParse_invalid_hash(t *testing.T) {
	src := strings.Replace(goldenSrc,
		"sha256:0000000000000000000000000000000000000000000000000000000000000000",
		"sha256:NOTHEX",
		1)
	_, _, err := cmdframework.Parse([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "content_hash") {
		t.Fatalf("expected content_hash shape error, got %v", err)
	}
}

// TestParse_invalid_version rejects non-integer version values.
func TestParse_invalid_version(t *testing.T) {
	src := strings.Replace(goldenSrc, "version: 1", "version: zero", 1)
	_, _, err := cmdframework.Parse([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("expected version error, got %v", err)
	}
}

// TestParse_missing_open_delimiter rejects input without leading `---`.
func TestParse_missing_open_delimiter(t *testing.T) {
	_, _, err := cmdframework.Parse([]byte("name: x\n---\nbody\n"))
	if err == nil {
		t.Fatal("expected leading-delimiter error, got nil")
	}
}

// TestParse_missing_close_delimiter rejects input without closing
// `---` after the open one.
func TestParse_missing_close_delimiter(t *testing.T) {
	_, _, err := cmdframework.Parse([]byte("---\nname: x\nbody without close\n"))
	if err == nil {
		t.Fatal("expected closing-delimiter error, got nil")
	}
}

// TestHashBody_TCCMD006_determinism: HashBody is deterministic and
// pure — same input bytes always produce the same hash.
func TestHashBody_TCCMD006_determinism(t *testing.T) {
	a := cmdframework.HashBody([]byte("hello\n"))
	b := cmdframework.HashBody([]byte("hello\n"))
	if a != b {
		t.Fatalf("HashBody not deterministic: %q vs %q", a, b)
	}
	c := cmdframework.HashBody([]byte("hello"))
	if a == c {
		t.Fatalf("HashBody should differ on trailing newline; both %q", a)
	}
}

// TestHashSource_matches_HashBody_on_extracted_body: the convenience
// wrapper agrees with the lower-level primitive.
func TestHashSource_matches_HashBody_on_extracted_body(t *testing.T) {
	body, err := cmdframework.Body([]byte(goldenSrc))
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	want := cmdframework.HashBody(body)
	got, err := cmdframework.HashSource([]byte(goldenSrc))
	if err != nil {
		t.Fatalf("HashSource: %v", err)
	}
	if got != want {
		t.Fatalf("HashSource %q != HashBody(body) %q", got, want)
	}
}

// TestLedger_unknown_verb_returns_empty: TC-CMD-007 — Ledger returns an
// empty slice for any verb not in the embedded bundle. The foundation
// shipped this as a hardcoded-empty stub; add-install-command replaced
// it with the real //go:embed-backed reader that still returns empty
// for unknown verbs (the contract).
func TestLedger_TCCMD007_unknown_verb_returns_empty(t *testing.T) {
	if got := cmdframework.Ledger("nonexistent-verb"); len(got) != 0 {
		t.Fatalf("expected empty ledger for unknown verb, got %v", got)
	}
}
