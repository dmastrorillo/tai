package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// Manifest is the per-target record of every relative path tai has
// installed and not yet pruned. The file lives at
// <TAI_DATA_DIR>/manifests/<sha256-of-target-root>.json and MUST NOT
// be inside any target directory (the user might wipe their target;
// the manifest survives so orphan detection keeps working).
//
// The entries map's keys are relative paths in the form
// "<category>/<rel>" — e.g. "skills/triage-comments.md". This carries
// enough context for orphan deletion across all three categories
// without needing a separate manifest per category.
type Manifest struct {
	// Paths is the unsorted set of installed entries. The exported
	// form is a map so contains/add/delete operations stay O(1); on
	// disk we serialise as a sorted slice for diff-friendly output.
	Paths map[string]bool `json:"-"`
}

// ManifestPath returns the absolute path to the manifest file for the
// target rooted at root. The filename is a hex sha256 of the root so
// "~/.claude" and "/Users/dan/.claude" can't accidentally share a
// manifest after path expansion.
func ManifestPath(dataDir, root string) string {
	h := sha256.Sum256([]byte(root))
	return filepath.Join(dataDir, "manifests", hex.EncodeToString(h[:])+".json")
}

// LoadManifest reads the manifest file for root. Returns an empty
// (non-nil) Manifest when the file does not exist — "no manifest yet"
// is the first-sync state.
func LoadManifest(dataDir, root string) (*Manifest, error) {
	path := ManifestPath(dataDir, root)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Manifest{Paths: map[string]bool{}}, nil
		}
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"read manifest %s", path)
	}
	var disk struct {
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal(data, &disk); err != nil {
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"parse manifest %s", path)
	}
	m := &Manifest{Paths: make(map[string]bool, len(disk.Paths))}
	for _, p := range disk.Paths {
		m.Paths[p] = true
	}
	return m, nil
}

// SaveManifest serialises m to its canonical path atomically. The
// on-disk representation sorts entries alphabetically and lives in
// JSON-with-pretty-print so PR diffs stay readable when the manifest
// is checked into git (not the default, but supported for review
// workflows).
//
// Write is tempfile-then-rename to match SaveState's pattern: two
// concurrent `tai sync` runs writing the same target's manifest can
// no longer leave it half-written.
func SaveManifest(dataDir, root string, m *Manifest) error {
	path := ManifestPath(dataDir, root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errcode.Wrapf(errcode.InternalError, err,
			"create manifests directory")
	}
	keys := make([]string, 0, len(m.Paths))
	for k := range m.Paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := struct {
		Paths []string `json:"paths"`
	}{Paths: keys}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return errcode.Wrapf(errcode.InternalError, err, "marshal manifest")
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return errcode.Wrapf(errcode.InternalError, err,
			"write manifest %s", tmp)
	}
	if err := os.Rename(tmp, path); err != nil {
		return errcode.Wrapf(errcode.InternalError, err,
			"rename manifest %s -> %s", tmp, path)
	}
	return nil
}

// Add inserts entry (e.g. "skills/foo.md") into the manifest. Idempotent.
func (m *Manifest) Add(entry string) { m.Paths[entry] = true }

// Remove deletes entry from the manifest. Idempotent.
func (m *Manifest) Remove(entry string) { delete(m.Paths, entry) }

// Has reports whether entry is currently in the manifest.
func (m *Manifest) Has(entry string) bool { return m.Paths[entry] }

// Orphans returns the entries in the manifest that are NOT present in
// the current set of source-side entries. Returned slice is sorted
// for deterministic prompt output.
func (m *Manifest) Orphans(currentSource map[string]bool) []string {
	var out []string
	for entry := range m.Paths {
		if !currentSource[entry] {
			out = append(out, entry)
		}
	}
	sort.Strings(out)
	return out
}
