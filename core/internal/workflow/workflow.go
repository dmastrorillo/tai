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
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

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
// stderr directly so tests can capture the diagnostic stream.
func Load(cloneDir string, warnings io.Writer) ([]Workflow, error) {
	root := filepath.Join(cloneDir, "workflows")
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"stat workflows tree at %s", root)
	}
	if !info.IsDir() {
		return nil, errcode.Newf(errcode.WorkflowInvalid,
			"%s exists but is not a directory", root)
	}

	// Collect candidate files first so we can apply alphabetical
	// tie-breaking when two paths lowercase to the same name.
	var paths []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".yml") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if walkErr != nil {
		return nil, errcode.Wrapf(errcode.InternalError, walkErr,
			"walk workflows tree at %s", root)
	}
	sort.Strings(paths)

	// byName accumulates loaded workflows keyed by colon-name so the
	// duplicate-detection loop has O(1) lookup. The first path that
	// claims a name wins (paths are sorted, so this is the
	// alphabetically-earlier file).
	byName := map[string]Workflow{}
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil, errcode.Wrapf(errcode.InternalError, err,
				"rel workflows path %s", p)
		}
		name, err := workflowName(rel)
		if err != nil {
			return nil, err
		}
		if reservedWorkflowNames[name] {
			return nil, errcode.Newf(errcode.WorkflowInvalid,
				"workflow %s uses reserved name %q (collides with `tai workflow %s`)",
				p, name, name).
				WithHelp("rename the file to a non-reserved name")
		}
		if _, dupe := byName[name]; dupe {
			_, _ = fmt.Fprintf(warnings,
				"[tai] workflow name %q is claimed by both %s and %s — using the first; rename one to disambiguate\n",
				name, byName[name].SourcePath, p)
			continue
		}
		wf, err := parseWorkflowFile(p, name)
		if err != nil {
			return nil, err
		}
		byName[name] = wf
	}

	out := make([]Workflow, 0, len(byName))
	for _, wf := range byName {
		out = append(out, wf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
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

// workflowName converts a forward-slash relative path under
// `workflows/` into the colon-namespaced lowercased name. Returns
// WORKFLOW_INVALID when the path is not a `.yml` file or has an
// empty stem.
func workflowName(rel string) (string, error) {
	rel = filepath.ToSlash(rel)
	if !strings.HasSuffix(strings.ToLower(rel), ".yml") {
		return "", errcode.Newf(errcode.WorkflowInvalid,
			"workflow file %s does not end in .yml", rel)
	}
	stem := strings.TrimSuffix(rel, filepath.Ext(rel))
	if stem == "" {
		return "", errcode.Newf(errcode.WorkflowInvalid,
			"workflow file %s has an empty name stem", rel)
	}
	segments := strings.Split(stem, "/")
	for i, seg := range segments {
		if seg == "" {
			return "", errcode.Newf(errcode.WorkflowInvalid,
				"workflow file %s has an empty path segment", rel)
		}
		segments[i] = strings.ToLower(seg)
	}
	return strings.Join(segments, ":"), nil
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
