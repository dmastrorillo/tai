package plugins

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/config"
)

// writePluginAssets lays out `<dir>/assets/<sub>/<name>` files for a
// fake plugin install directory.
func writePluginAssets(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(dir, "assets", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TC-PLG-017 — a failed asset copy must not destroy the assets a
// prior install already placed in the target. The old wipe-then-copy
// shape removed the plugin's whole namespace before writing anything,
// so a copy failure partway left the target with the old files gone
// and the new set incomplete. The contract is now copy-then-prune: on
// failure the previously-installed files are still on disk.
func TestSyncAssets_TCPLG017_failed_copy_preserves_previous_install(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-000 unreadable-file setup is not portable to windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root ignores file permissions")
	}

	targetRoot := t.TempDir()
	target := config.Target{Root: targetRoot}

	// v1 install: one skill lands in the target.
	v1 := t.TempDir()
	writePluginAssets(t, v1, map[string]string{
		"skills/tai-demo-keeper.md": "v1 content",
	})
	if err := SyncAssetsToTargets(v1, "demo", []config.Target{target}, io.Discard); err != nil {
		t.Fatalf("v1 sync: %v", err)
	}
	keeper := filepath.Join(targetRoot, "skills", "tai-demo-keeper.md")
	if _, err := os.Stat(keeper); err != nil {
		t.Fatalf("v1 skill not installed: %v", err)
	}

	// v2 install: an unreadable asset that sorts BEFORE the keeper
	// forces the copy loop to fail on its first entry.
	v2 := t.TempDir()
	writePluginAssets(t, v2, map[string]string{
		"skills/tai-demo-broken.md": "unreadable",
		"skills/tai-demo-keeper.md": "v2 content",
	})
	if err := os.Chmod(filepath.Join(v2, "assets", "skills", "tai-demo-broken.md"), 0o000); err != nil {
		t.Fatal(err)
	}

	if err := SyncAssetsToTargets(v2, "demo", []config.Target{target}, io.Discard); err == nil {
		t.Fatal("want error from unreadable asset, got nil")
	}

	if _, err := os.Stat(keeper); err != nil {
		t.Errorf("failed sync destroyed previously-installed asset: %v", err)
	}
}

// A successful re-sync prunes namespace entries the new version no
// longer ships (the namespace IS the manifest), while entries outside
// the plugin's namespace are untouched.
func TestSyncAssets_prunes_stale_namespace_entries(t *testing.T) {
	targetRoot := t.TempDir()
	target := config.Target{Root: targetRoot}

	v1 := t.TempDir()
	writePluginAssets(t, v1, map[string]string{
		"skills/tai-demo-old.md":    "goes away in v2",
		"skills/tai-demo-stays.md":  "v1",
		"commands/old-command.md":   "goes away in v2",
		"commands/stays-command.md": "v1",
	})
	if err := SyncAssetsToTargets(v1, "demo", []config.Target{target}, io.Discard); err != nil {
		t.Fatalf("v1 sync: %v", err)
	}
	// A user-owned file sharing the skills dir must survive syncs.
	userFile := filepath.Join(targetRoot, "skills", "user-owned.md")
	if err := os.WriteFile(userFile, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	v2 := t.TempDir()
	writePluginAssets(t, v2, map[string]string{
		"skills/tai-demo-stays.md":  "v2",
		"commands/stays-command.md": "v2",
	})
	if err := SyncAssetsToTargets(v2, "demo", []config.Target{target}, io.Discard); err != nil {
		t.Fatalf("v2 sync: %v", err)
	}

	assertGone := func(rel string) {
		t.Helper()
		p := filepath.Join(targetRoot, filepath.FromSlash(rel))
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("stale entry %s should be pruned, stat: %v", rel, err)
		}
	}
	assertContent := func(rel, want string) {
		t.Helper()
		p := filepath.Join(targetRoot, filepath.FromSlash(rel))
		got, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("read %s: %v", rel, err)
			return
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}

	assertGone("skills/tai-demo-old.md")
	assertGone("commands/tai-demo/old-command.md")
	assertContent("skills/tai-demo-stays.md", "v2")
	assertContent("commands/tai-demo/stays-command.md", "v2")
	assertContent("skills/user-owned.md", "mine")
}
