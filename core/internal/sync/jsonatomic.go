package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// writeJSONAtomic marshals v as indented JSON (with a trailing
// newline) and writes it to path via stage-then-rename, creating
// parent directories as needed. It is the single implementation of
// "atomically persist JSON" shared by SaveManifest and SaveState.
//
// The staging file is created with os.CreateTemp, whose unique name
// makes concurrent savers of the same path safe: the previous fixed
// "<path>.tmp" scheme let writer B's pre-clean delete writer A's
// in-flight staging file, failing A's rename with ENOENT. A staging
// file orphaned by a killed process is left behind as bounded litter
// (at most one per killed run) and never collides with a live
// writer's name.
//
// Returns plain errors; callers wrap with their own errcode context.
func writeJSONAtomic(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// CreateTemp opens 0600; match the 0644 the final file has always
	// shipped with before the rename makes it visible.
	if err := writeAndClose(f, data); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// writeAndClose writes data, applies the canonical 0644 mode, and
// closes f, returning the first error encountered.
func writeAndClose(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
