package cmd_test

import (
	"os"
	"path/filepath"
	"testing"
)

// sourceTreeEnv stages a clone at <dataDir>/source/ pre-seeded with
// the given files under one source-tree subdirectory ("workflows",
// "standards", ...), plus a config file that survives strict
// validation. update-check-interval is 0 so the background poll
// stays out of these tests. Returns the data dir.
//
// Single fixture behind workflowEnv and standardsEnv — the two were
// copy-pasted ~30-line twins that could drift independently.
//
// Not tied to a TC-ID — test fixture helper.
func sourceTreeEnv(t *testing.T, subdir string, files map[string]string) string {
	t.Helper()

	dataDir := t.TempDir()
	t.Setenv("TAI_DATA_DIR", dataDir)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	t.Setenv("TAI_CONFIG", cfgPath)
	body := "repo-url: git@github.com:acme/repo.git\nupdate-check-interval: 0\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	// Stage the tree under <dataDir>/source/<subdir>/. Loaders resolve
	// this path via sync.CloneDir(dataDir), so the fixture mirrors a
	// post-sync state.
	for rel, content := range files {
		full := filepath.Join(dataDir, "source", subdir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dataDir
}
