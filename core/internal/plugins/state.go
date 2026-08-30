package plugins

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// State is the on-disk record of currently-installed plugins. The
// file lives at `<TAI_DATA_DIR>/state/plugins.json`. It is read by
// `tai plugins list` and rewritten by install/update/remove. The
// file is the authoritative record of what's installed; the
// directory layout under `<TAI_DATA_DIR>/plugins/` is treated as a
// derived artefact and is not consulted by `list`.
type State struct {
	Plugins []Entry `json:"plugins"`
}

// Entry is one installed plugin's record. SourceVersion is the
// version actually installed (resolved from `--version` or "latest"
// at install time). Source captures where the install came from so
// update can re-fetch from the same place.
type Entry struct {
	Name        string    `json:"name"`
	Source      Source    `json:"source"`
	Version     string    `json:"version"`
	InstalledAt time.Time `json:"installed-at"`

	// Description is the single line the plugin printed for the
	// `--help-summary` wire verb at install or update time, and the
	// text `tai --help` shows beside the plugin's name. Captured once
	// rather than exec'd at help-render time, so rendering help never
	// spawns a subprocess per installed plugin.
	//
	// Append-only, like every field here: entries written before this
	// field existed omit it and read back as empty, which is why the
	// help renderer must tolerate a blank description.
	Description string `json:"description,omitempty"`
}

// statePath returns the canonical state-file path for a given data
// directory. The directory layout `state/plugins.json` is fixed by
// the spec and shared with the update-banner cache, which uses
// `state/update-check.json` — both live as siblings.
func statePath(dataDir string) string {
	return filepath.Join(dataDir, "state", "plugins.json")
}

// LoadState reads the plugins state file from dataDir. Returns an
// empty State (not nil) when the file does not exist — absence is a
// valid initial condition, not an error.
//
// On parse failure, returns `*errcode.Error{Code: INTERNAL_ERROR}`
// preserving the cause. A corrupted state file is a host bug; the
// only safe action is to surface it loudly so the user can remove
// the file.
func LoadState(dataDir string) (*State, error) {
	p := statePath(dataDir)
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &State{}, nil
		}
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"read plugin state %s", p)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, errcode.Wrapf(errcode.InternalError, err,
			"parse plugin state %s: %s", p, err).
			WithHelp(
				"the state file appears corrupted",
				"remove it with `rm "+p+"` and re-run install for each plugin",
			)
	}
	return &s, nil
}

// SaveState writes s to the plugins state file under dataDir,
// creating the `state/` directory if needed. Marshalled with
// indentation so the file is grep-able and human-diffable; the file
// is small (one row per plugin).
func SaveState(dataDir string, s *State) error {
	p := statePath(dataDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return errcode.Wrapf(errcode.InternalError, err,
			"create %s", filepath.Dir(p))
	}
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return errcode.Wrapf(errcode.InternalError, err, "marshal plugin state")
	}
	body = append(body, '\n')
	if err := os.WriteFile(p, body, 0o644); err != nil {
		return errcode.Wrapf(errcode.InternalError, err, "write %s", p)
	}
	return nil
}

// Find returns the entry for name and its index, or (Entry{}, -1)
// when no such entry exists.
func (s *State) Find(name string) (Entry, int) {
	for i, e := range s.Plugins {
		if e.Name == name {
			return e, i
		}
	}
	return Entry{}, -1
}

// Upsert inserts or replaces the entry for e.Name. Returns whether
// the entry already existed (true) or was newly inserted (false).
func (s *State) Upsert(e Entry) bool {
	if _, i := s.Find(e.Name); i >= 0 {
		s.Plugins[i] = e
		return true
	}
	s.Plugins = append(s.Plugins, e)
	return false
}

// Remove deletes the entry for name. Returns whether an entry was
// actually removed (true) or absent (false).
func (s *State) Remove(name string) bool {
	_, i := s.Find(name)
	if i < 0 {
		return false
	}
	s.Plugins = append(s.Plugins[:i], s.Plugins[i+1:]...)
	return true
}
