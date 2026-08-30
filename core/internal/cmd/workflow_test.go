package cmd_test

import (
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// workflowEnv stages workflow files (rel-under-workflows/ → body) via
// the shared sourceTreeEnv fixture.
func workflowEnv(t *testing.T, workflows map[string]string) string {
	t.Helper()
	return sourceTreeEnv(t, "workflows", workflows)
}

// TestWorkflowList_TCWF009_prints_alphabetical exercises TC-WF-009.
func TestWorkflowList_TCWF009_prints_alphabetical(t *testing.T) {
	workflowEnv(t, map[string]string{
		"verify.yml": `description: verify a fix
steps:
  - kind: command
    name: verify
`,
		"propose.yml": `description: propose a change
steps:
  - kind: skill
    name: spar
`,
		"release/cut-rc.yml": `description: cut a release candidate
steps:
  - kind: command
    name: tag
`,
	})

	r := runRoot(t, "workflow", "list")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	lines := splitNonEmptyLines(r.stdout)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), r.stdout)
	}
	wantOrder := []string{"propose", "release:cut-rc", "verify"}
	for i, want := range wantOrder {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("line[%d] = %q, want prefix %q", i, lines[i], want)
		}
	}
	if !strings.Contains(lines[0], "propose a change") {
		t.Errorf("propose line should include its description, got: %q", lines[0])
	}
}

// TestWorkflowList_TCWF010_no_workflows exercises TC-WF-010.
func TestWorkflowList_TCWF010_no_workflows(t *testing.T) {
	workflowEnv(t, nil)

	r := runRoot(t, "workflow", "list")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stdout, "(no workflows)") {
		t.Errorf("stdout should contain `(no workflows)`, got: %q", r.stdout)
	}
}

// TestWorkflowRun_TCWF011_emits_markdown_plan exercises TC-WF-011.
//
// The two-step fixture pins kindWidth alignment between two different
// widths (`skill` 5 chars, `command` 7 chars). The dedicated bullet-
// format assertion locks the spec contract `<kind>:  /<name>` with the
// kind left-justified to the longest-kind width so a regression that
// changes spacing or strips the kind label can't slip past the
// substring checks below.
func TestWorkflowRun_TCWF011_emits_markdown_plan(t *testing.T) {
	workflowEnv(t, map[string]string{
		"propose.yml": `description: propose a change
steps:
  - kind: skill
    name: spar
  - kind: command
    name: openspec:propose
`,
	})

	r := runRoot(t, "workflow", "run", "propose")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	for _, want := range []string{
		"# Workflow: propose", // H1
		"propose a change",    // description paragraph
		"## Required tools",   // required-tools section
		"## Steps",            // numbered steps section
		"## Failure mode",     // failure-mode section
		"abort",               // failure-mode instructs abort
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout missing %q\nstdout:\n%s", want, r.stdout)
		}
	}

	// Bullet-format contract: `<kind>:  /<name>` with the kind label
	// left-justified to the widest kind across the workflow's steps
	// (`command` here, width 7 → "command:" is 8 chars, so "skill:"
	// is padded to 8 chars with a trailing space).
	for _, want := range []string{
		"- skill:    /spar",
		"- command:  /openspec:propose",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout missing exact bullet %q\nstdout:\n%s", want, r.stdout)
		}
	}

	// Steps section: numbered, declaration order, `/<name>` rendered.
	for _, want := range []string{
		"1. Invoke `/spar` (skill).",
		"2. Invoke `/openspec:propose` (command).",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout missing exact step %q\nstdout:\n%s", want, r.stdout)
		}
	}
}

// TestWorkflowRun_TCWF011_single_step_renders_without_padding exercises
// the single-step shape of the markdown plan emitter. A two-step
// fixture (above) only exercises the multi-width padding path; a
// one-step workflow keeps the kindWidth equal to the only kind's
// length so the bullet has no trailing alignment space beyond the
// mandatory two-space separator.
//
// Tagged under TC-WF-011 as a sub-scenario (no new TC-ID — the spec
// contract is the same shape, just with one row).
func TestWorkflowRun_TCWF011_single_step_renders_without_padding(t *testing.T) {
	workflowEnv(t, map[string]string{
		"solo.yml": `description: a one-step workflow
steps:
  - kind: skill
    name: spar
`,
	})

	r := runRoot(t, "workflow", "run", "solo")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	// With one step the kindWidth is len("skill") so "skill:" gets
	// padded to 6 chars (the label width) and the bullet reads
	// "- skill:  /spar" — exactly two spaces between the label and
	// `/<name>`, no extra alignment padding.
	if !strings.Contains(r.stdout, "- skill:  /spar") {
		t.Errorf("single-step bullet should render without extra padding\nstdout:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, "1. Invoke `/spar` (skill).") {
		t.Errorf("single-step Steps section missing or malformed\nstdout:\n%s", r.stdout)
	}
}

// TestWorkflowRun_TCWF012_missing_workflow exercises TC-WF-012.
func TestWorkflowRun_TCWF012_missing_workflow(t *testing.T) {
	workflowEnv(t, map[string]string{
		"propose.yml": `description: x
steps:
  - kind: skill
    name: y
`,
	})

	r := runRoot(t, "workflow", "run", "nope")
	if r.err == nil {
		t.Fatal("expected error")
	}
	if r.exitCode != 2 {
		t.Errorf("exit code: want 2, got %d", r.exitCode)
	}
	assertCode(t, r.err, errcode.WorkflowNotFound)
	if !strings.Contains(r.stderr, "[exit 2: WORKFLOW_NOT_FOUND]") {
		t.Errorf("stderr missing WORKFLOW_NOT_FOUND footer, got:\n%s", r.stderr)
	}
}

// splitNonEmptyLines is a tiny helper that strips empty trailing
// lines and skips blank rows so tests can assert exact line counts
// against text output. Not tied to a TC-ID.
func splitNonEmptyLines(s string) []string {
	out := []string{}
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
