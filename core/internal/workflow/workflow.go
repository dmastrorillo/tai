// Package workflow loads YAML workflow files from a tai source-repo
// clone and exposes them to the `tai workflow list / run` verbs.
//
// A workflow at `<clone>/workflows/<path>.yml` is addressed by its
// colon-namespaced lowercased name (path segments joined with `:`).
// See openspec/changes/pivot-to-ai-as-code/specs/workflows/spec.md for
// the normative spec.
//
// Workflows are read-only state: this package never writes to the
// clone, and the `tai sync` flow deliberately does not copy them into
// configured targets — they live in the clone and AI agents pull the
// markdown plan on demand via `tai workflow run`.
package workflow

import (
	"bytes"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/dmastrorillo/tai/core/internal/sourcetree"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// Workflow is a parsed `<clone>/workflows/<path>.yml` file.
type Workflow struct {
	// Name is the colon-namespaced lowercased addressable name (e.g.
	// "release:cut-rc"). Two files whose names collide after
	// lowercasing are reported as a load-time warning; the
	// alphabetically-earlier source path wins.
	Name string

	// Description is the workflow's one-line summary. Empty when the
	// file did not declare a description; CLI surfaces emit
	// `(missing description)` in that case.
	Description string

	// Steps is the ordered list declared in the file. Always non-empty
	// for successfully-loaded workflows (the loader rejects files with
	// no steps via WORKFLOW_INVALID).
	Steps []Step

	// SourcePath is the absolute path to the YAML file on disk. Used
	// in warnings and error messages so the operator can find the
	// offending file quickly.
	SourcePath string
}

// Step is one entry in a workflow's `steps:` list. `Kind` is always
// `skill` or `command` — `agent` is rejected at load time.
type Step struct {
	Kind string
	Name string
}

// reservedWorkflowNames is the set of colon-namespaced names that
// collide with `tai workflow` sub-verbs. Only the exact bare name is
// reserved; a nested name like `foo:list` does not collide with
// `tai workflow list` and is allowed.
//
// This is distinct from the authoritative top-level reserved verb
// list in core/internal/verbs — those are top-level `tai` verbs;
// these are sub-verbs scoped under `tai workflow`.
var reservedWorkflowNames = map[string]bool{
	"list": true,
	"run":  true,
}

// rawWorkflowFile is the strict YAML shape Load expects. yaml.v3's
// KnownFields(true) decoder rejects any top-level key other than
// `description` and `steps`, satisfying the spec's "unknown top-level
// keys MUST be rejected" rule.
type rawWorkflowFile struct {
	Description string    `yaml:"description"`
	Steps       []rawStep `yaml:"steps"`
}

type rawStep struct {
	Kind string `yaml:"kind"`
	Name string `yaml:"name"`
}

// Load walks `<cloneDir>/workflows/**/*.yml` and returns the loaded
// workflows in alphabetical order by name. The first error
// encountered is returned as `*errcode.Error{Code: WORKFLOW_INVALID}`;
// non-fatal diagnostics (case-insensitive name collisions) are
// written to `warnings`.
//
// An absent or empty `workflows/` directory is not an error — Load
// returns an empty slice. Callers driving `tai workflow list` use
// the empty result to emit the `(no workflows)` line.
//
// The function takes a `warnings` writer rather than logging to
// stderr directly so tests can capture the diagnostic stream. The
// walk/name/dedupe algorithm lives in core/internal/sourcetree,
// shared with the standards loader; only the leaf parse step is
// workflow-specific.
func Load(cloneDir string, warnings io.Writer) ([]Workflow, error) {
	return sourcetree.Load(cloneDir, warnings, sourcetree.Options[Workflow]{
		Subdir:   "workflows",
		Ext:      ".yml",
		Kind:     "workflow",
		Verb:     "workflow",
		Code:     errcode.WorkflowInvalid,
		Reserved: reservedWorkflowNames,
		Parse:    parseWorkflowFile,
	})
}

// Find returns the workflow whose Name equals name, or (Workflow{},
// false) when no such workflow is loaded. Used by `tai workflow run`
// after Load returns.
func Find(workflows []Workflow, name string) (Workflow, bool) {
	for _, wf := range workflows {
		if wf.Name == name {
			return wf, true
		}
	}
	return Workflow{}, false
}

// parseWorkflowFile reads + validates one YAML file and returns the
// populated Workflow. The decoder is configured with KnownFields(true)
// so the spec's "unknown top-level keys MUST be rejected" rule is
// enforced by yaml.v3 itself.
func parseWorkflowFile(path, name string) (Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Workflow{}, errcode.Wrapf(errcode.InternalError, err,
			"read workflow %s", path)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var raw rawWorkflowFile
	if err := dec.Decode(&raw); err != nil {
		if err == io.EOF {
			return Workflow{}, errcode.Newf(errcode.WorkflowInvalid,
				"workflow %s is empty", path).
				WithHelp("a workflow must declare `description:` and `steps:`")
		}
		return Workflow{}, errcode.Wrapf(errcode.WorkflowInvalid, err,
			"parse workflow %s: %s", path, err).
			WithHelp(
				"check the file's YAML syntax",
				"top-level keys must be `description` and `steps`",
			)
	}

	if strings.TrimSpace(raw.Description) == "" {
		return Workflow{}, errcode.Newf(errcode.WorkflowInvalid,
			"workflow %s is missing `description`", path).
			WithHelp("add a one-line `description:` field at the top level")
	}
	if len(raw.Steps) == 0 {
		return Workflow{}, errcode.Newf(errcode.WorkflowInvalid,
			"workflow %s has no `steps`", path).
			WithHelp("a workflow must declare at least one step under `steps:`")
	}

	steps := make([]Step, 0, len(raw.Steps))
	for i, s := range raw.Steps {
		switch s.Kind {
		case "skill", "command":
			// ok
		case "agent":
			return Workflow{}, errcode.Newf(errcode.WorkflowInvalid,
				"workflow %s step %d has `kind: agent` — only `skill` and `command` are allowed",
				path, i+1).
				WithHelp("agents are reached transitively from skills/commands, not directly")
		case "":
			return Workflow{}, errcode.Newf(errcode.WorkflowInvalid,
				"workflow %s step %d is missing `kind`", path, i+1).
				WithHelp("set `kind:` to `skill` or `command`")
		default:
			return Workflow{}, errcode.Newf(errcode.WorkflowInvalid,
				"workflow %s step %d has unknown `kind: %s` — only `skill` and `command` are allowed",
				path, i+1, s.Kind)
		}
		if strings.TrimSpace(s.Name) == "" {
			return Workflow{}, errcode.Newf(errcode.WorkflowInvalid,
				"workflow %s step %d is missing `name`", path, i+1).
				WithHelp("set `name:` to the bare identifier (no leading slash)")
		}
		steps = append(steps, Step(s))
	}

	return Workflow{
		Name:        name,
		Description: strings.TrimSpace(raw.Description),
		Steps:       steps,
		SourcePath:  path,
	}, nil
}
