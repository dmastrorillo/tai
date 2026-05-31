package workflow_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/testutil"
	"github.com/dmastrorillo/tai/core/internal/workflow"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// seedWorkflows writes the given relative-path → YAML-body map under a
// fresh `<clone>/workflows/` tree and returns the cloneDir.
//
// Not tied to a TC-ID — test fixture helper.
func seedWorkflows(t *testing.T, files map[string]string) string {
	t.Helper()
	clone := t.TempDir()
	for rel, body := range files {
		full := filepath.Join(clone, "workflows", rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return clone
}

// TestLoad_TCWF001_valid_workflow_accepted exercises TC-WF-001.
func TestLoad_TCWF001_valid_workflow_accepted(t *testing.T) {
	clone := seedWorkflows(t, map[string]string{
		"propose.yml": `description: propose a change
steps:
  - kind: skill
    name: spar
  - kind: command
    name: openspec:propose
`,
	})

	var warn bytes.Buffer
	got, err := workflow.Load(clone, &warn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 workflow, got %d", len(got))
	}
	wf := got[0]
	if wf.Name != "propose" {
		t.Errorf("Name: want %q, got %q", "propose", wf.Name)
	}
	if wf.Description != "propose a change" {
		t.Errorf("Description: want %q, got %q", "propose a change", wf.Description)
	}
	if len(wf.Steps) != 2 {
		t.Fatalf("want 2 steps, got %d", len(wf.Steps))
	}
	if wf.Steps[0].Kind != "skill" || wf.Steps[0].Name != "spar" {
		t.Errorf("step[0]: want skill/spar, got %+v", wf.Steps[0])
	}
	if wf.Steps[1].Kind != "command" || wf.Steps[1].Name != "openspec:propose" {
		t.Errorf("step[1]: want command/openspec:propose, got %+v", wf.Steps[1])
	}
	if warn.Len() != 0 {
		t.Errorf("unexpected warning: %s", warn.String())
	}
}

// TestLoad_TCWF002_kind_agent_rejected exercises TC-WF-002.
func TestLoad_TCWF002_kind_agent_rejected(t *testing.T) {
	clone := seedWorkflows(t, map[string]string{
		"bad.yml": `description: tries to use agent
steps:
  - kind: agent
    name: code-reviewer
`,
	})

	_, err := workflow.Load(clone, io.Discard)
	testutil.AssertErrCode(t, err, errcode.WorkflowInvalid)
	if !strings.Contains(err.Error(), "kind: agent") {
		t.Errorf("error should name the offending kind, got: %v", err)
	}
}

// TestLoad_TCWF003_unknown_top_level_key_rejected exercises TC-WF-003.
func TestLoad_TCWF003_unknown_top_level_key_rejected(t *testing.T) {
	clone := seedWorkflows(t, map[string]string{
		"bad.yml": `description: has an unknown key
notes: this shouldn't be here
steps:
  - kind: skill
    name: x
`,
	})

	_, err := workflow.Load(clone, io.Discard)
	testutil.AssertErrCode(t, err, errcode.WorkflowInvalid)
	if !strings.Contains(err.Error(), "notes") {
		t.Errorf("error should name the unknown key, got: %v", err)
	}
}

// TestLoad_TCWF004_missing_required_fields_rejected exercises TC-WF-004.
func TestLoad_TCWF004_missing_required_fields_rejected(t *testing.T) {
	t.Run("missing description", func(t *testing.T) {
		clone := seedWorkflows(t, map[string]string{
			"a.yml": `steps:
  - kind: skill
    name: x
`,
		})
		_, err := workflow.Load(clone, io.Discard)
		testutil.AssertErrCode(t, err, errcode.WorkflowInvalid)
		if !strings.Contains(err.Error(), "description") {
			t.Errorf("error should name the missing field, got: %v", err)
		}
	})

	t.Run("missing step kind", func(t *testing.T) {
		clone := seedWorkflows(t, map[string]string{
			"a.yml": `description: x
steps:
  - name: y
`,
		})
		_, err := workflow.Load(clone, io.Discard)
		testutil.AssertErrCode(t, err, errcode.WorkflowInvalid)
		if !strings.Contains(err.Error(), "kind") {
			t.Errorf("error should name the missing field, got: %v", err)
		}
	})

	t.Run("missing step name", func(t *testing.T) {
		clone := seedWorkflows(t, map[string]string{
			"a.yml": `description: x
steps:
  - kind: skill
`,
		})
		_, err := workflow.Load(clone, io.Discard)
		testutil.AssertErrCode(t, err, errcode.WorkflowInvalid)
		if !strings.Contains(err.Error(), "name") {
			t.Errorf("error should name the missing field, got: %v", err)
		}
	})
}

// TestLoad_TCWF005_nested_colon_namespaced_name exercises TC-WF-005.
func TestLoad_TCWF005_nested_colon_namespaced_name(t *testing.T) {
	clone := seedWorkflows(t, map[string]string{
		"release/cut-rc.yml": `description: cut a release candidate
steps:
  - kind: command
    name: tag
`,
	})

	got, err := workflow.Load(clone, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "release:cut-rc" {
		t.Fatalf("want one workflow named release:cut-rc, got %+v", got)
	}
}

// TestLoad_TCWF006_reserved_name_list_rejected exercises TC-WF-006.
func TestLoad_TCWF006_reserved_name_list_rejected(t *testing.T) {
	clone := seedWorkflows(t, map[string]string{
		"list.yml": `description: bad
steps:
  - kind: skill
    name: x
`,
	})

	_, err := workflow.Load(clone, io.Discard)
	testutil.AssertErrCode(t, err, errcode.WorkflowInvalid)
	if !strings.Contains(err.Error(), "list") || !strings.Contains(err.Error(), "reserved") {
		t.Errorf("error should name `list` as reserved, got: %v", err)
	}
}

// TestLoad_TCWF007_reserved_name_run_rejected exercises TC-WF-007.
func TestLoad_TCWF007_reserved_name_run_rejected(t *testing.T) {
	clone := seedWorkflows(t, map[string]string{
		"run.yml": `description: bad
steps:
  - kind: skill
    name: x
`,
	})

	_, err := workflow.Load(clone, io.Discard)
	testutil.AssertErrCode(t, err, errcode.WorkflowInvalid)
	if !strings.Contains(err.Error(), "run") || !strings.Contains(err.Error(), "reserved") {
		t.Errorf("error should name `run` as reserved, got: %v", err)
	}
}

// TestLoad_TCWF008_duplicate_warning_first_wins exercises TC-WF-008.
//
// The case-insensitive collision the spec describes requires two paths
// that lowercase to the same name. On case-insensitive filesystems
// (default APFS on macOS, NTFS on Windows) the two files would be the
// same on disk, so the test cannot stage the scenario. We detect that
// up front and skip — the production logic still runs on case-
// sensitive filesystems in CI.
func TestLoad_TCWF008_duplicate_warning_first_wins(t *testing.T) {
	testutil.SkipIfCaseInsensitiveFS(t)
	clone := seedWorkflows(t, map[string]string{
		"Build.yml": `description: capital B
steps:
  - kind: skill
    name: x
`,
		"build.yml": `description: lowercase b
steps:
  - kind: skill
    name: y
`,
	})

	var warn bytes.Buffer
	got, err := workflow.Load(clone, &warn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("collision should reduce to one workflow, got %d", len(got))
	}
	if got[0].Description != "capital B" {
		t.Errorf("first-wins should select Build.yml, got description %q", got[0].Description)
	}
	if !strings.Contains(warn.String(), "build") {
		t.Errorf("warning should name the colliding workflow, got: %q", warn.String())
	}
	// Both file paths should appear in the warning so the operator
	// can find them.
	if !strings.Contains(warn.String(), "Build.yml") || !strings.Contains(warn.String(), "build.yml") {
		t.Errorf("warning should name both source paths, got: %q", warn.String())
	}
}

// TestLoad_empty_workflows_dir_returns_empty exercises the spec's
// "absent workflows/ is not an error" rule. Not tied to a TC-ID — it's
// the loader-level anchor for TC-WF-010's `(no workflows)` line, which
// has its own e2e test at the CLI boundary.
func TestLoad_empty_workflows_dir_returns_empty(t *testing.T) {
	clone := t.TempDir() // no workflows/ subdir at all
	got, err := workflow.Load(clone, io.Discard)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty slice, got %+v", got)
	}
}
