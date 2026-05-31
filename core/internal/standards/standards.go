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
func Load(cloneDir string, warnings io.Writer) ([]Standard, error) {
	root := filepath.Join(cloneDir, "standards")
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"stat standards tree at %s", root)
	}
	if !info.IsDir() {
		return nil, errcode.Newf(errcode.StandardInvalid,
			"%s exists but is not a directory", root)
	}

	var paths []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if walkErr != nil {
		return nil, errcode.Wrapf(errcode.InternalError, walkErr,
			"walk standards tree at %s", root)
	}
	sort.Strings(paths)

	byName := map[string]Standard{}
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil, errcode.Wrapf(errcode.InternalError, err,
				"rel standards path %s", p)
		}
		name, err := standardName(rel)
		if err != nil {
			return nil, err
		}
		if reservedStandardNames[name] {
			return nil, errcode.Newf(errcode.StandardInvalid,
				"standard %s uses reserved name %q (collides with `tai standards %s`)",
				p, name, name).
				WithHelp("rename the file to a non-reserved name")
		}
		if _, dupe := byName[name]; dupe {
			_, _ = fmt.Fprintf(warnings,
				"[tai] standard name %q is claimed by both %s and %s — using the first; rename one to disambiguate\n",
				name, byName[name].SourcePath, p)
			continue
		}
		s, err := parseStandardFile(p, name)
		if err != nil {
			return nil, err
		}
		byName[name] = s
	}

	out := make([]Standard, 0, len(byName))
	for _, s := range byName {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
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

// standardName converts a forward-slash relative path under
// `standards/` into the colon-namespaced lowercased name. Returns
// STANDARD_INVALID when the path is not a `.md` file or has an empty
// stem.
func standardName(rel string) (string, error) {
	rel = filepath.ToSlash(rel)
	if !strings.HasSuffix(strings.ToLower(rel), ".md") {
		return "", errcode.Newf(errcode.StandardInvalid,
			"standard file %s does not end in .md", rel)
	}
	stem := strings.TrimSuffix(rel, filepath.Ext(rel))
	if stem == "" {
		return "", errcode.Newf(errcode.StandardInvalid,
			"standard file %s has an empty name stem", rel)
	}
	segments := strings.Split(stem, "/")
	for i, seg := range segments {
		if seg == "" {
			return "", errcode.Newf(errcode.StandardInvalid,
				"standard file %s has an empty path segment", rel)
		}
		segments[i] = strings.ToLower(seg)
	}
	return strings.Join(segments, ":"), nil
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
