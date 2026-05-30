package sync

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Category is one of the three asset buckets tai recognises. The
// values are the canonical source-side subdirectory names; the
// effective target-side name is overridable per-target in config.
type Category string

const (
	CategorySkills   Category = "skills"
	CategoryCommands Category = "commands"
	CategoryAgents   Category = "agents"
)

// Categories returns the three buckets in fixed order. Tests rely on
// this order to be stable so batched prompts read top-down.
func Categories() []Category {
	return []Category{CategorySkills, CategoryCommands, CategoryAgents}
}

// SourceFiles walks <cloneDir>/<category> and returns every file
// relative to that subdirectory. Directories themselves are not
// returned. The slice is sorted lexically so two runs against the
// same tree produce byte-identical output (deterministic prompts).
//
// Returns an empty slice (not nil) when the category subdir does not
// exist in the clone — a source repo without skills is valid; tai
// just has nothing to copy under that bucket.
func SourceFiles(cloneDir string, cat Category) ([]string, error) {
	root := filepath.Join(cloneDir, string(cat))
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		// Normalise to forward slashes so manifest entries are
		// platform-stable.
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// TargetSubpath joins root + the effective sub-path for cat. Returns
// an empty string when the sub-path is falsy ("skip this category") —
// callers MUST treat an empty result as "do nothing for this
// category".
//
// Inputs:
//
//   - root: the target's root (already resolved to an absolute path
//     by the caller).
//   - override: the target's per-category override pointer from the
//     config (nil = default, "" = skip, value = literal override).
//   - defaultName: the canonical sub-path name (skills/commands/agents)
//     used when override is nil.
func TargetSubpath(root string, override *string, defaultName string) string {
	if override == nil {
		return filepath.Join(root, defaultName)
	}
	if *override == "" {
		return ""
	}
	return filepath.Join(root, *override)
}

// JoinRel joins a base directory with a forward-slash-normalised
// relative path. Mirrors filepath.Join but accepts the sorted-slash
// form returned by SourceFiles without leaking forward-slashes onto
// Windows.
func JoinRel(base, rel string) string {
	return filepath.Join(base, strings.ReplaceAll(rel, "/", string(filepath.Separator)))
}
