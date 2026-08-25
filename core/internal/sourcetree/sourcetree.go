// Package sourcetree loads colon-namespace-addressed files from one
// subdirectory of a tai source-repo clone. It is the single
// implementation of the walk → derive-name → reject-reserved →
// dedupe-collisions → parse → sort algorithm shared by the standards
// (`<clone>/standards/**/*.md`) and workflow
// (`<clone>/workflows/**/*.yml`) loaders, so subtle rules like the
// case-insensitive collision tie-break cannot drift between them —
// and a third named-asset kind gets the algorithm for free.
package sourcetree

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// Options parameterises Load for one asset kind.
type Options[T any] struct {
	// Subdir is the tree under the clone root ("standards",
	// "workflows"). Also appears in internal-error messages.
	Subdir string

	// Ext is the required lowercase file extension including the dot
	// (".md", ".yml"). Files with any other extension are skipped.
	Ext string

	// Kind is the singular noun used in user-facing messages
	// ("standard", "workflow").
	Kind string

	// Verb is the `tai <verb>` the reserved names collide with
	// ("standards", "workflow").
	Verb string

	// Code is the validation error code for this kind
	// (STANDARD_INVALID, WORKFLOW_INVALID).
	Code errcode.Code

	// Reserved is the set of bare colon-names that collide with the
	// verb's own subcommands.
	Reserved map[string]bool

	// Parse reads and validates one file. name is the derived
	// colon-namespaced name; path is absolute.
	Parse func(path, name string) (T, error)
}

// Load walks `<cloneDir>/<Subdir>/**/*<Ext>` and returns the parsed
// values in alphabetical order by name. Names are the lowercased
// path segments joined with ":". Validation failures surface with
// Options.Code; case-insensitive name collisions are emitted as
// warnings to `warnings` and the alphabetically-earlier source path
// wins.
//
// An absent or empty tree is not an error — Load returns an empty
// slice so callers can emit their "(no <kind>s)" line.
func Load[T any](cloneDir string, warnings io.Writer, o Options[T]) ([]T, error) {
	root := filepath.Join(cloneDir, o.Subdir)
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"stat %s tree at %s", o.Subdir, root)
	}
	if !info.IsDir() {
		return nil, errcode.Newf(o.Code,
			"%s exists but is not a directory", root)
	}

	// Collect candidate files first so alphabetical tie-breaking
	// applies when two paths lowercase to the same name.
	var paths []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), o.Ext) {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if walkErr != nil {
		return nil, errcode.Wrapf(errcode.InternalError, walkErr,
			"walk %s tree at %s", o.Subdir, root)
	}
	sort.Strings(paths)

	// winner tracks which source path first claimed each name (paths
	// are sorted, so the alphabetically-earlier file wins); byName
	// holds the parsed values.
	winner := map[string]string{}
	byName := map[string]T{}
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil, errcode.Wrapf(errcode.InternalError, err,
				"rel %s path %s", o.Subdir, p)
		}
		name, err := colonName(rel, o.Kind, o.Ext, o.Code)
		if err != nil {
			return nil, err
		}
		if o.Reserved[name] {
			return nil, errcode.Newf(o.Code,
				"%s %s uses reserved name %q (collides with `tai %s %s`)",
				o.Kind, p, name, o.Verb, name).
				WithHelp("rename the file to a non-reserved name")
		}
		if prev, dupe := winner[name]; dupe {
			_, _ = fmt.Fprintf(warnings,
				"[tai] %s name %q is claimed by both %s and %s — using the first; rename one to disambiguate\n",
				o.Kind, name, prev, p)
			continue
		}
		v, err := o.Parse(p, name)
		if err != nil {
			return nil, err
		}
		winner[name] = p
		byName[name] = v
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]T, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	return out, nil
}

// colonName converts a forward-slash relative path under the tree
// into the colon-namespaced lowercased name. Returns code when the
// path does not end in ext or has an empty stem or segment.
func colonName(rel, kind, ext string, code errcode.Code) (string, error) {
	rel = filepath.ToSlash(rel)
	if !strings.HasSuffix(strings.ToLower(rel), ext) {
		return "", errcode.Newf(code,
			"%s file %s does not end in %s", kind, rel, ext)
	}
	stem := strings.TrimSuffix(rel, filepath.Ext(rel))
	if stem == "" {
		return "", errcode.Newf(code,
			"%s file %s has an empty name stem", kind, rel)
	}
	segments := strings.Split(stem, "/")
	for i, seg := range segments {
		if seg == "" {
			return "", errcode.Newf(code,
				"%s file %s has an empty path segment", kind, rel)
		}
		segments[i] = strings.ToLower(seg)
	}
	return strings.Join(segments, ":"), nil
}
