package cmd_test

import (
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// standardsEnv stages standard files (rel-under-standards/ → body)
// via the shared sourceTreeEnv fixture.
func standardsEnv(t *testing.T, stds map[string]string) string {
	t.Helper()
	return sourceTreeEnv(t, "standards", stds)
}

// TestStandardsList_TCSTD007_prints_alphabetical exercises TC-STD-007.
func TestStandardsList_TCSTD007_prints_alphabetical(t *testing.T) {
	standardsEnv(t, map[string]string{
		"SDLC.md":                           "---\ndescription: Software development lifecycle\n---\nbody\n",
		"devOps/security/best-practices.md": "---\ndescription: Sec best practices\n---\nbody\n",
	})

	r := runRoot(t, "standards", "list")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	lines := splitNonEmptyLines(r.stdout)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d:\n%s", len(lines), r.stdout)
	}
	if !strings.HasPrefix(lines[0], "devops:security:best-practices") {
		t.Errorf("line[0] should start with devops:security:best-practices, got: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "sdlc") {
		t.Errorf("line[1] should start with sdlc, got: %q", lines[1])
	}
}

// TestStandardsList_TCSTD008_no_standards exercises TC-STD-008.
func TestStandardsList_TCSTD008_no_standards(t *testing.T) {
	standardsEnv(t, nil)

	r := runRoot(t, "standards", "list")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stdout, "(no standards)") {
		t.Errorf("stdout should contain `(no standards)`, got: %q", r.stdout)
	}
}

// TestStandardsLoad_TCSTD009_prints_body exercises TC-STD-009.
func TestStandardsLoad_TCSTD009_prints_body(t *testing.T) {
	body := "# SDLC\n\nReview before merging.\n"
	standardsEnv(t, map[string]string{
		"SDLC.md": "---\ndescription: Software development lifecycle\n---\n" + body,
	})

	r := runRoot(t, "standards", "load", "sdlc")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if r.stdout != body {
		t.Errorf("stdout: want %q, got %q", body, r.stdout)
	}
	if strings.Contains(r.stdout, "description:") {
		t.Errorf("frontmatter should not appear in stdout, got: %q", r.stdout)
	}
}

// TestStandardsLoad_TCSTD010_missing_standard exercises TC-STD-010.
func TestStandardsLoad_TCSTD010_missing_standard(t *testing.T) {
	standardsEnv(t, map[string]string{
		"SDLC.md": "---\ndescription: x\n---\nbody\n",
	})

	r := runRoot(t, "standards", "load", "nonexistent")
	if r.err == nil {
		t.Fatal("expected error")
	}
	if r.exitCode != 2 {
		t.Errorf("exit code: want 2, got %d", r.exitCode)
	}
	assertCode(t, r.err, errcode.StandardNotFound)
	if !strings.Contains(r.stderr, "[exit 2: STANDARD_NOT_FOUND]") {
		t.Errorf("stderr missing STANDARD_NOT_FOUND footer, got:\n%s", r.stderr)
	}
}
