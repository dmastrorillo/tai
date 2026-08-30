// Package standards loads markdown standards from a tai source-repo
// clone and exposes them to the `tai standards list / load` verbs.
//
// A standard at `<clone>/standards/<path>.md` is addressed by its
// colon-namespaced lowercased name (path segments joined with `:`).
// See openspec/changes/pivot-to-ai-as-code/specs/standards/spec.md for
// the normative spec.
//
// Standards are content TAI does not interpret: the body is opaque
// markdown that AI sessions consume via `tai standards load`. The
// only structural surface TAI parses is an optional YAML frontmatter
// `description:` field used by `tai standards list`.
package standards

import (
	"bytes"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/dmastrorillo/tai/core/internal/sourcetree"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// MissingDescription is the literal string used in place of a real
// description when the standard file has no frontmatter (or no
// `description:` field in it). Pinned by TC-STD-002 — tests assert on
// this exact value.
const MissingDescription = "(missing description in frontmatter)"

// Standard is a parsed `<clone>/standards/<path>.md` file.
type Standard struct {
	// Name is the colon-namespaced lowercased addressable name (e.g.
	// "devops:security:best-practices"). Case-insensitive collisions
	// between two source files are reported as a load-time warning;
	// the alphabetically-earlier source path wins.
	Name string

	// Description is the value of the frontmatter `description:`
	// field, or MissingDescription when the file has no frontmatter
	// or no description in its frontmatter.
	Description string

	// Body is the markdown content of the file with any YAML
	// frontmatter block stripped. Emitted byte-for-byte by
	// `tai standards load <name>`.
	Body []byte

	// SourcePath is the absolute path to the markdown file on disk.
	SourcePath string
}

// reservedStandardNames is the set of colon-namespaced names that
// collide with `tai standards` sub-verbs. Only the exact bare name is
// reserved; a nested name like `foo:list` does not collide and is
// allowed.
var reservedStandardNames = map[string]bool{
	"list": true,
	"load": true,
}

// Load walks `<cloneDir>/standards/**/*.md` and returns the loaded
// standards in alphabetical order by name. Validation failures
// (reserved name) surface as `*errcode.Error{Code: STANDARD_INVALID}`;
// case-insensitive name collisions are emitted as warnings to
// `warnings` and the alphabetically-earlier source path wins.
//
// An absent or empty `standards/` directory is not an error — Load
// returns an empty slice. Callers driving `tai standards list` use
// the empty result to emit the `(no standards)` line.
//
// The walk/name/dedupe algorithm lives in core/internal/sourcetree,
// shared with the workflow loader; only the leaf parse step is
// standards-specific.
func Load(cloneDir string, warnings io.Writer) ([]Standard, error) {
	return sourcetree.Load(cloneDir, warnings, sourcetree.Options[Standard]{
		Subdir:   "standards",
		Ext:      ".md",
		Kind:     "standard",
		Verb:     "standards",
		Code:     errcode.StandardInvalid,
		Reserved: reservedStandardNames,
		Parse:    parseStandardFile,
	})
}

// Find returns the standard whose Name equals name, or (Standard{},
// false) when no such standard is loaded.
func Find(standards []Standard, name string) (Standard, bool) {
	for _, s := range standards {
		if s.Name == name {
			return s, true
		}
	}
	return Standard{}, false
}

// parseStandardFile reads `path`, separates the optional frontmatter
// block from the body, and extracts the `description:` field if
// present. The body is preserved byte-for-byte after frontmatter
// removal — TAI does NOT transform markdown.
func parseStandardFile(path, name string) (Standard, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Standard{}, errcode.Wrapf(errcode.InternalError, err,
			"read standard %s", path)
	}

	desc, body := splitFrontmatter(data)
	return Standard{
		Name:        name,
		Description: desc,
		Body:        body,
		SourcePath:  path,
	}, nil
}

// splitFrontmatter peels off a leading `---\n<yaml>\n---\n` block
// from data and returns (description, body). When data has no
// frontmatter — or the frontmatter has no `description:` field — the
// description is MissingDescription and body is data unchanged.
//
// CRLF-terminated opening fences (`---\r\n`) are tolerated for files
// authored on Windows; the offset into `data` tracks the actual
// terminator length so the YAML block is not corrupted by a stray
// leading `\r`.
//
// Frontmatter parse failures are non-fatal: the description falls
// back to MissingDescription and the body is data unchanged. This
// mirrors the spec's "content is opaque to TAI" stance — a broken
// frontmatter shouldn't make a standard unloadable.
func splitFrontmatter(data []byte) (string, []byte) {
	var openLen int
	switch {
	case bytes.HasPrefix(data, []byte("---\r\n")):
		openLen = len("---\r\n")
	case bytes.HasPrefix(data, []byte("---\n")):
		openLen = len("---\n")
	default:
		return MissingDescription, data
	}

	// Find the closing fence. Scan for "\n---" after the opening
	// fence, then validate the terminator that follows. Fall back to
	// MissingDescription when no valid closing fence exists.
	rest := data[openLen:]
	idx := bytes.Index(rest, []byte("\n---"))
	if idx < 0 {
		return MissingDescription, data
	}
	yamlBlock := rest[:idx]
	after := rest[idx+len("\n---"):]
	switch {
	case bytes.HasPrefix(after, []byte("\r\n")):
		after = after[2:]
	case bytes.HasPrefix(after, []byte("\n")):
		after = after[1:]
	default:
		// "---" not followed by a newline — not a valid fence.
		return MissingDescription, data
	}

	var fm struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal(yamlBlock, &fm); err != nil {
		return MissingDescription, data
	}
	desc := strings.TrimSpace(fm.Description)
	if desc == "" {
		desc = MissingDescription
	}
	return desc, after
}
