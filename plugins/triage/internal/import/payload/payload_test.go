package payload_test

import (
	"strings"
	"testing"

	payload "github.com/dmastrorillo/tai/plugins/triage/internal/import/payload"
)

// validPR is a known-good payload exercised by happy-path tests; the
// negative tests start from a deep copy and remove or mutate one field
// so the error attribution is unambiguous.
const validPR = `{
  "repo": "acme/app",
  "target": {
    "kind": "pr",
    "pr": {
      "number": 142,
      "title": "feat: oauth",
      "url": "https://github.com/acme/app/pull/142",
      "head_branch": "feat/oauth"
    }
  },
  "batches": [
    { "batch_key": "B1", "title": "Replace execSync" }
  ],
  "comments": [
    {
      "external_refs": [
        { "kind": "github-pr-comment", "id": "12345", "reviewer": "coderabbit" }
      ],
      "severity": "critical",
      "category": "security",
      "file": "src/api/auth.ts",
      "lines": "15-29",
      "source": "coderabbit",
      "title": "shell injection",
      "description": "execSync interpolates user input",
      "why_fix": "shell metachars get executed",
      "suggested_fix": "use execFileSync",
      "consequences": "RCE in build env",
      "batch_key": "B1"
    }
  ]
}`

const validBranch = `{
  "repo": "acme/app",
  "target": {
    "kind": "branch",
    "branch": { "name": "feat/oauth" }
  },
  "comments": []
}`

// TestDecodeValidate_TCIMP001_well_formed_PR exercises TC-IMP-001: a
// well-formed PR payload decodes and validates cleanly.
func TestDecodeValidate_TCIMP001_well_formed_PR(t *testing.T) {
	p, err := payload.DecodeBytes([]byte(validPR))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if errs := payload.Validate(p); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
}

// TestDecodeValidate_TCIMP002_well_formed_branch exercises TC-IMP-002: a
// well-formed branch payload decodes and validates cleanly.
func TestDecodeValidate_TCIMP002_well_formed_branch(t *testing.T) {
	p, err := payload.DecodeBytes([]byte(validBranch))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if errs := payload.Validate(p); len(errs) != 0 {
		t.Fatalf("Validate: %v", errs)
	}
}

// TestValidate_TCIMP003_missing_why_fix_rejected exercises TC-IMP-003:
// a comment missing the why_fix enrichment field is rejected.
func TestValidate_TCIMP003_missing_why_fix_rejected(t *testing.T) {
	src := strings.Replace(validPR,
		`"why_fix": "shell metachars get executed",`, "", 1)
	p, err := payload.DecodeBytes([]byte(src))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	errs := payload.Validate(p)
	requirePathError(t, errs, "comments[0].why_fix")
}

// TestValidate_TCIMP004_invalid_severity_rejected exercises TC-IMP-004:
// severity outside the enum is rejected.
func TestValidate_TCIMP004_invalid_severity_rejected(t *testing.T) {
	src := strings.Replace(validPR,
		`"severity": "critical"`, `"severity": "urgent"`, 1)
	p, err := payload.DecodeBytes([]byte(src))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	errs := payload.Validate(p)
	requirePathError(t, errs, "comments[0].severity")
}

// TestValidate_TCIMP005_target_with_both_pr_and_branch exercises
// TC-IMP-005: a target carrying both pr and branch bodies is rejected.
func TestValidate_TCIMP005_both_pr_and_branch(t *testing.T) {
	src := `{
  "repo": "acme/app",
  "target": {
    "kind": "pr",
    "pr": { "number": 1, "title": "t", "url": "u", "head_branch": "b" },
    "branch": { "name": "feat/x" }
  }
}`
	p, err := payload.DecodeBytes([]byte(src))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	errs := payload.Validate(p)
	requirePathError(t, errs, "target.branch")
}

// TestValidate_TCIMP006_kind_pr_without_pr_body exercises TC-IMP-006:
// kind=pr without a pr body is rejected.
func TestValidate_TCIMP006_kind_pr_without_pr_body(t *testing.T) {
	src := `{
  "repo": "acme/app",
  "target": { "kind": "pr" }
}`
	p, err := payload.DecodeBytes([]byte(src))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	errs := payload.Validate(p)
	requirePathError(t, errs, "target.pr")
}

// TestValidate_TCIMP007_missing_target_pr_number exercises a sub-case
// of TC-IMP-006: kind=pr with a partial pr body (missing number).
func TestValidate_TCIMP007_missing_pr_number(t *testing.T) {
	src := `{
  "repo": "acme/app",
  "target": {
    "kind": "pr",
    "pr": { "number": 0, "title": "t", "url": "u", "head_branch": "b" }
  }
}`
	p, err := payload.DecodeBytes([]byte(src))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	errs := payload.Validate(p)
	requirePathError(t, errs, "target.pr.number")
}

// TestValidate_TCIMP008_empty_external_refs exercises TC-IMP-008: a
// comment with an empty external_refs array is rejected.
func TestValidate_TCIMP008_empty_external_refs(t *testing.T) {
	src := strings.Replace(validPR,
		`"external_refs": [
        { "kind": "github-pr-comment", "id": "12345", "reviewer": "coderabbit" }
      ],`,
		`"external_refs": [],`, 1)
	p, err := payload.DecodeBytes([]byte(src))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	errs := payload.Validate(p)
	requirePathError(t, errs, "comments[0].external_refs")
}

// TestDecode_TCIMP009_unknown_field exercises TC-IMP-009: an unknown
// JSON field is rejected at decode time (strict mode).
func TestDecode_TCIMP009_unknown_field(t *testing.T) {
	src := strings.Replace(validPR,
		`"batch_key": "B1"`,
		`"batch_key": "B1", "priority": "p0"`, 1)
	_, err := payload.DecodeBytes([]byte(src))
	if err == nil {
		t.Fatal("expected unknown-field error, got nil")
	}
	if !strings.Contains(err.Error(), "priority") {
		t.Fatalf("error should mention the unknown field, got %v", err)
	}
}

// TestValidate_TCIMP011_batch_key_unknown_rejected exercises TC-IMP-011:
// a comment referencing a batch_key not present in payload.batches is
// rejected with a violation at comments[N].batch_key.
func TestValidate_TCIMP011_batch_key_unknown_rejected(t *testing.T) {
	src := strings.Replace(validPR,
		`"batch_key": "B1"`,
		`"batch_key": "B-does-not-exist"`, 1)
	p, err := payload.DecodeBytes([]byte(src))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	errs := payload.Validate(p)
	requirePathError(t, errs, "comments[0].batch_key")
}

// TestValidate_TCIMP010_multiple_errors exercises TC-IMP-010: every
// violation in a payload is reported, not just the first.
func TestValidate_TCIMP010_multiple_errors(t *testing.T) {
	src := `{
  "repo": "acme/app",
  "target": { "kind": "pr", "pr": { "number": 1, "title": "t", "url": "u", "head_branch": "b" } },
  "comments": [
    {
      "external_refs": [ { "kind": "github-pr-comment", "id": "1" } ],
      "severity": "urgent",
      "category": "security",
      "file": "f",
      "lines": "1",
      "source": "s",
      "title": "t",
      "description": "d",
      "why_fix": "",
      "suggested_fix": "s",
      "consequences": "c"
    }
  ]
}`
	p, err := payload.DecodeBytes([]byte(src))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	errs := payload.Validate(p)
	if len(errs) < 2 {
		t.Fatalf("expected multiple errors, got %d: %v", len(errs), errs)
	}
	hasSeverity := false
	hasWhyFix := false
	for _, e := range errs {
		if e.Path == "comments[0].severity" {
			hasSeverity = true
		}
		if e.Path == "comments[0].why_fix" {
			hasWhyFix = true
		}
	}
	if !hasSeverity || !hasWhyFix {
		t.Fatalf("expected both severity and why_fix errors, got %v", errs)
	}
}

// TestFormatErrors_TCIMP012_renders_alphabetical exercises TC-IMP-012:
// FormatErrors renders the per-violation lines in alphabetical path
// order so the rendered error body is deterministic.
func TestFormatErrors_TCIMP012_renders_alphabetical(t *testing.T) {
	errs := []payload.ValidationError{
		{Path: "comments[1].severity", Message: "bad"},
		{Path: "comments[0].why_fix", Message: "empty"},
	}
	got := payload.FormatErrors(errs)
	if !strings.Contains(got, "2 problems with the JSON payload:") {
		t.Fatalf("header missing: %q", got)
	}
	// Alphabetical order: comments[0]... before comments[1]...
	idx0 := strings.Index(got, "comments[0].why_fix")
	idx1 := strings.Index(got, "comments[1].severity")
	if idx0 < 0 || idx1 < 0 || idx0 > idx1 {
		t.Fatalf("errors not alphabetised: %q", got)
	}
}

// TestDecode_TCIMP013_malformed_JSON exercises TC-IMP-013: the decoder
// surfaces a real error for non-JSON input. The CLI maps this to
// IMPORT_INVALID_JSON (covered at the CLI boundary by TC-IMP-030).
func TestDecode_TCIMP013_malformed_JSON(t *testing.T) {
	_, err := payload.DecodeBytes([]byte("{not json"))
	if err == nil {
		t.Fatal("expected decode error for malformed JSON, got nil")
	}
}

// requirePathError fails the test if no ValidationError with the given
// path appears in errs. Used so each TC test pins the exact path the
// violation should be attributed to.
func requirePathError(t *testing.T, errs []payload.ValidationError, path string) {
	t.Helper()
	for _, e := range errs {
		if e.Path == path {
			return
		}
	}
	t.Fatalf("expected error at path %q; got %v", path, errs)
}
