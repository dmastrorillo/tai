package cmd_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/core/internal/config"
)

// bareRemote creates a bare git repo at a temp path and returns its
// file:// URL — the cheapest fixture that lets tai's git invocations
// (clone / fetch / ls-remote) talk to a "remote" without leaving
// the test machine. Skips on machines with no git, or on git older
// than 2.28 (the version that introduced --initial-branch).
//
// Not tied to a TC-ID — test fixture helper.
func bareRemote(t *testing.T) string {
	t.Helper()
	requireGit(t)
	requireModernGit(t)
	dir := t.TempDir()
	mustRun(t, "", "git", "init", "--bare", "--initial-branch=main", dir)
	return "file://" + dir
}

// requireModernGit skips a test on git versions older than 2.28
// (released 2020-07), which is when --initial-branch landed. Some
// enterprise distros ship older git; the test fixtures need the flag.
//
// Production code (`repoinit.gitInitAndCommit`) has a runtime fallback
// for older git, so this guard is test-only.
//
// Not tied to a TC-ID — test fixture helper.
func requireModernGit(t *testing.T) {
	t.Helper()
	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		t.Skipf("git --version failed: %v", err)
	}
	// Output is "git version X.Y.Z" or "git version X.Y.Z (...)".
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) < 3 {
		t.Skipf("git --version produced unexpected output: %q", out)
	}
	ver := parts[2]
	verParts := strings.SplitN(ver, ".", 3)
	if len(verParts) < 2 {
		t.Skipf("git version %q does not parse as MAJOR.MINOR.*", ver)
	}
	major, minor := atoi(verParts[0]), atoi(verParts[1])
	if major < 2 || (major == 2 && minor < 28) {
		t.Skipf("git %s is older than 2.28; --initial-branch unavailable", ver)
	}
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// seedRemote clones url, writes the given files (map of relative path
// → content) into the working tree, commits, pushes. Used to populate
// the bare remote with content tai will sync.
//
// Behaviour note: subsequent calls add files on top of whatever the
// remote currently has — the resulting remote tree is the union of
// all prior seeds plus these files. To replace content, use
// removeFromRemote first, or call this helper with the canonical
// final state.
//
// Not tied to a TC-ID — test fixture helper.
func seedRemote(t *testing.T, url string, files map[string]string) {
	t.Helper()
	work := t.TempDir()
	repoPath := strings.TrimPrefix(url, "file://")
	mustRun(t, "", "git", "clone", repoPath, work)
	mustRun(t, work, "git", "config", "user.email", "test@local")
	mustRun(t, work, "git", "config", "user.name", "test")
	for rel, body := range files {
		full := filepath.Join(work, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for seed file %q: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write seed file %q: %v", rel, err)
		}
	}
	mustRun(t, work, "git", "add", "-A")
	mustRun(t, work, "git", "commit", "-m", "seed")
	mustRun(t, work, "git", "push", "-u", "origin", "main")
}

// removeFromRemote clones url, deletes the named relative paths,
// commits + pushes. Used to simulate "the source repo dropped a
// file" for orphan/prune tests.
//
// Not tied to a TC-ID — test fixture helper.
func removeFromRemote(t *testing.T, url string, paths ...string) {
	t.Helper()
	work := t.TempDir()
	repoPath := strings.TrimPrefix(url, "file://")
	mustRun(t, "", "git", "clone", repoPath, work)
	mustRun(t, work, "git", "config", "user.email", "test@local")
	mustRun(t, work, "git", "config", "user.name", "test")
	for _, rel := range paths {
		full := filepath.Join(work, rel)
		if err := os.Remove(full); err != nil {
			t.Fatalf("remove %q from seed: %v", rel, err)
		}
	}
	mustRun(t, work, "git", "add", "-A")
	mustRun(t, work, "git", "commit", "-m", "remove")
	mustRun(t, work, "git", "push", "origin", "main")
}

// mustRun executes name in dir and fails the test if it errors.
//
// Not tied to a TC-ID — test fixture helper.
func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s in %s: %v\n%s", name, strings.Join(args, " "), dir, err, out)
	}
}

// syncEnv stages a fresh TAI_DATA_DIR + TAI_CONFIG + HOME and writes a
// config that points at the given repo url with one target at the
// returned path. Returns (dataDir, target, cfgPath). Sets
// update-check-interval to 0 so the background poll stays out of
// these tests; poll behaviour has its own tests.
//
// Not tied to a TC-ID — test fixture helper.
func syncEnv(t *testing.T, url string) (string, string, string) {
	t.Helper()
	// Test-only bypass so the file:// fixture URL survives config
	// validation. The helper auto-restores via t.Cleanup so subsequent
	// tests start from the strict validator.
	config.AllowFileURLsForTesting(t)

	dataDir := t.TempDir()
	t.Setenv("TAI_DATA_DIR", dataDir)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	t.Setenv("TAI_CONFIG", cfgPath)

	target := filepath.Join(t.TempDir(), "claude")
	body := fmt.Sprintf("repo-url: %s\nupdate-check-interval: 0\ntargets:\n  - root: %s\n", url, target)
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	return dataDir, target, cfgPath
}

// TestSync_TCSYNC001_first_sync_creates_clone exercises TC-SYNC-001.
func TestSync_TCSYNC001_first_sync_creates_clone(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{"skills/foo.md": "hello\n"})

	dataDir, _, _ := syncEnv(t, url)

	r := runRoot(t, "sync", "-y")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "source", ".git")); err != nil {
		t.Fatalf("expected clone at <DATA>/source/.git, got: %v", err)
	}
}

// TestSync_TCSYNC002_subsequent_sync_reuses_clone exercises TC-SYNC-002:
// the .git directory's inode (Stat info) survives across two syncs.
func TestSync_TCSYNC002_subsequent_sync_reuses_clone(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{"skills/foo.md": "v1\n"})
	dataDir, _, _ := syncEnv(t, url)

	if r := runRoot(t, "sync", "-y"); r.err != nil {
		t.Fatalf("first sync failed: %v\nstderr:\n%s", r.err, r.stderr)
	}
	gitPath := filepath.Join(dataDir, "source", ".git")
	infoBefore, err := os.Stat(gitPath)
	if err != nil {
		t.Fatalf("stat .git: %v", err)
	}

	if r := runRoot(t, "sync", "-y"); r.err != nil {
		t.Fatalf("second sync failed: %v\nstderr:\n%s", r.err, r.stderr)
	}
	infoAfter, err := os.Stat(gitPath)
	if err != nil {
		t.Fatalf("stat .git after second sync: %v", err)
	}
	if !os.SameFile(infoBefore, infoAfter) {
		t.Errorf(".git directory was re-created across syncs (inode differs)")
	}
}

// TestSync_TCSYNC003_fetch_failure_warning exercises TC-SYNC-003.
//
// We seed the remote, sync once (clone + first fetch succeed), then
// delete the remote and sync again. The second fetch fails; tai must
// surface a one-line stderr warning and continue with the cached
// clone (no error returned).
func TestSync_TCSYNC003_fetch_failure_warning(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{"skills/foo.md": "v1\n"})
	dataDir, target, _ := syncEnv(t, url)

	if r := runRoot(t, "sync", "-y"); r.err != nil {
		t.Fatalf("first sync failed: %v", r.err)
	}
	// Sabotage the remote: remove the bare repo dir from under tai's
	// feet so `git fetch` fails.
	if err := os.RemoveAll(strings.TrimPrefix(url, "file://")); err != nil {
		t.Fatalf("rm remote: %v", err)
	}

	r := runRoot(t, "sync", "-y")
	if r.err != nil {
		t.Fatalf("sync should not error on fetch failure; got: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stderr, "fetch failed") {
		t.Errorf("stderr missing fetch-failed warning, got:\n%s", r.stderr)
	}
	// The cached file is still on disk from sync #1.
	if _, err := os.Stat(filepath.Join(target, "skills", "foo.md")); err != nil {
		t.Errorf("sync did not fall back to cache: %v", err)
	}
	_ = dataDir
}

// TestSync_TCSYNC004_fresh_writes_all_files exercises TC-SYNC-004.
func TestSync_TCSYNC004_fresh_writes_all_files(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{
		"skills/a.md": "1", "skills/b.md": "2", "skills/c.md": "3",
	})
	_, target, _ := syncEnv(t, url)

	r := runRoot(t, "sync")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	for _, rel := range []string{"a.md", "b.md", "c.md"} {
		p := filepath.Join(target, "skills", rel)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing synced file %s: %v", p, err)
		}
	}
	// No overwrite prompt should have been needed → no prompt text on stderr.
	if strings.Contains(r.stderr, "[y/N]") {
		t.Errorf("unexpected overwrite prompt on fresh sync, got stderr:\n%s", r.stderr)
	}
}

// TestSync_TCSYNC005_single_overwrite_prompt exercises TC-SYNC-005.
func TestSync_TCSYNC005_single_overwrite_prompt(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{"skills/foo.md": "from-source\n"})
	_, target, _ := syncEnv(t, url)

	// Pre-seed an existing file at the destination so sync hits the
	// overwrite path.
	if err := os.MkdirAll(filepath.Join(target, "skills"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "skills", "foo.md"), []byte("user-version\n"), 0o644); err != nil {
		t.Fatalf("seed pre-existing: %v", err)
	}

	// User answers `y` to accept the overwrite.
	r := runRootStdin(t, "y\n", "sync")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if !strings.Contains(r.stderr, "Overwrite (skills)") {
		t.Errorf("stderr missing skills overwrite group, got:\n%s", r.stderr)
	}
	if !strings.Contains(r.stderr, "[y/N]") {
		t.Errorf("stderr missing y/N prompt, got:\n%s", r.stderr)
	}
	body, _ := os.ReadFile(filepath.Join(target, "skills", "foo.md"))
	if string(body) != "from-source\n" {
		t.Errorf("file not overwritten; got %q", body)
	}
}

// TestSync_TCSYNC006_batched_overwrite_prompt exercises TC-SYNC-006.
func TestSync_TCSYNC006_batched_overwrite_prompt(t *testing.T) {
	url := bareRemote(t)
	files := map[string]string{}
	for i := 0; i < 5; i++ {
		files[fmt.Sprintf("skills/s%d.md", i)] = "x"
	}
	for i := 0; i < 2; i++ {
		files[fmt.Sprintf("commands/c%d.md", i)] = "x"
	}
	files["agents/a0.md"] = "x"
	seedRemote(t, url, files)

	_, target, _ := syncEnv(t, url)
	for rel := range files {
		full := filepath.Join(target, rel)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte("old"), 0o644)
	}

	r := runRootStdin(t, "y\n", "sync")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	prompts := strings.Count(r.stderr, "[y/N]")
	if prompts != 1 {
		t.Errorf("expected exactly 1 batched prompt, got %d. stderr:\n%s", prompts, r.stderr)
	}
	for _, want := range []string{"Overwrite (skills)", "Overwrite (commands)", "Overwrite (agents)"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr missing %q, got:\n%s", want, r.stderr)
		}
	}
}

// TestSync_TCSYNC007_dash_y_bypasses_prompt exercises TC-SYNC-007.
func TestSync_TCSYNC007_dash_y_bypasses_prompt(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{"skills/foo.md": "new"})
	_, target, _ := syncEnv(t, url)
	_ = os.MkdirAll(filepath.Join(target, "skills"), 0o755)
	_ = os.WriteFile(filepath.Join(target, "skills", "foo.md"), []byte("old"), 0o644)

	r := runRoot(t, "sync", "-y")
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if strings.Contains(r.stderr, "[y/N]") {
		t.Errorf("-y should suppress the prompt; got stderr:\n%s", r.stderr)
	}
	body, _ := os.ReadFile(filepath.Join(target, "skills", "foo.md"))
	if string(body) != "new" {
		t.Errorf("-y should have proceeded with the overwrite; got body %q", body)
	}
	if !strings.Contains(r.stderr, "overwrote") {
		t.Errorf("stderr missing post-write summary, got:\n%s", r.stderr)
	}
}

// TestSync_TCSYNC008_user_rejection_cancels exercises TC-SYNC-008.
func TestSync_TCSYNC008_user_rejection_cancels(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{"skills/foo.md": "new"})
	_, target, _ := syncEnv(t, url)
	_ = os.MkdirAll(filepath.Join(target, "skills"), 0o755)
	_ = os.WriteFile(filepath.Join(target, "skills", "foo.md"), []byte("old"), 0o644)

	r := runRootStdin(t, "N\n", "sync")
	if r.err != nil {
		t.Fatalf("cancellation should not produce an error: %v", r.err)
	}
	if r.exitCode != 0 {
		t.Errorf("exit code: want 0, got %d", r.exitCode)
	}
	body, _ := os.ReadFile(filepath.Join(target, "skills", "foo.md"))
	if string(body) != "old" {
		t.Errorf("cancellation must not overwrite; got %q", body)
	}
	if !strings.Contains(r.stderr, "cancelled") && !strings.Contains(r.stderr, "Cancelled") {
		t.Errorf("stderr missing cancellation notice, got:\n%s", r.stderr)
	}
}

// TestSync_TCSYNC009_first_sync_creates_manifest exercises TC-SYNC-009.
func TestSync_TCSYNC009_first_sync_creates_manifest(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{
		"skills/a.md":   "x",
		"skills/b.md":   "x",
		"skills/c.md":   "x",
		"commands/c.md": "x",
	})
	dataDir, target, _ := syncEnv(t, url)
	_ = target

	if r := runRoot(t, "sync", "-y"); r.err != nil {
		t.Fatalf("sync error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	// Manifest file exists (one per target).
	entries, err := os.ReadDir(filepath.Join(dataDir, "manifests"))
	if err != nil {
		t.Fatalf("read manifests/: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one manifest file, got %d", len(entries))
	}
	body, err := os.ReadFile(filepath.Join(dataDir, "manifests", entries[0].Name()))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	for _, want := range []string{"skills/a.md", "skills/b.md", "skills/c.md", "commands/c.md"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("manifest missing entry %q, got:\n%s", want, body)
		}
	}
}

// TestSync_TCSYNC010_subsequent_sync_extends_manifest exercises TC-SYNC-010.
func TestSync_TCSYNC010_subsequent_sync_extends_manifest(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{"skills/a.md": "x", "skills/b.md": "x"})
	dataDir, _, _ := syncEnv(t, url)
	if r := runRoot(t, "sync", "-y"); r.err != nil {
		t.Fatalf("first sync: %v", r.err)
	}

	// Add a new file to the remote, sync again, expect 3 entries.
	seedRemote(t, url, map[string]string{"skills/c.md": "x"})
	if r := runRoot(t, "sync", "-y"); r.err != nil {
		t.Fatalf("second sync: %v", r.err)
	}

	entries, _ := os.ReadDir(filepath.Join(dataDir, "manifests"))
	body, _ := os.ReadFile(filepath.Join(dataDir, "manifests", entries[0].Name()))
	for _, want := range []string{"skills/a.md", "skills/b.md", "skills/c.md"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("manifest missing %q, got:\n%s", want, body)
		}
	}
}

// TestSync_TCSYNC011_prune_deletes_orphans exercises TC-SYNC-011.
func TestSync_TCSYNC011_prune_deletes_orphans(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{"skills/keep.md": "k", "skills/gone.md": "g"})
	_, target, _ := syncEnv(t, url)
	if r := runRoot(t, "sync", "-y"); r.err != nil {
		t.Fatalf("first sync: %v", r.err)
	}

	// Remove from remote, then prune.
	removeFromRemote(t, url, "skills/gone.md")
	r := runRoot(t, "sync", "--prune", "-y")
	if r.err != nil {
		t.Fatalf("prune sync: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(target, "skills", "gone.md")); !os.IsNotExist(err) {
		t.Errorf("orphan was not deleted; stat returned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "skills", "keep.md")); err != nil {
		t.Errorf("kept file was deleted: %v", err)
	}
}

// TestSync_TCSYNC012_orphan_count_summary exercises TC-SYNC-012.
func TestSync_TCSYNC012_orphan_count_summary(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{"skills/keep.md": "k", "skills/gone.md": "g"})
	_, target, _ := syncEnv(t, url)
	if r := runRoot(t, "sync", "-y"); r.err != nil {
		t.Fatalf("first sync: %v", r.err)
	}
	removeFromRemote(t, url, "skills/gone.md")

	r := runRoot(t, "sync", "-y") // no --prune
	if r.err != nil {
		t.Fatalf("sync: %v", r.err)
	}
	if !strings.Contains(r.stderr, "1 orphan pending") {
		t.Errorf("stderr missing orphan-count summary, got:\n%s", r.stderr)
	}
	// The orphan file still exists at the target.
	if _, err := os.Stat(filepath.Join(target, "skills", "gone.md")); err != nil {
		t.Errorf("orphan should still exist without --prune; got: %v", err)
	}
}

// TestSync_TCSYNC013_requires_both_config_fields exercises TC-SYNC-013.
func TestSync_TCSYNC013_requires_both_config_fields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("TAI_DATA_DIR", t.TempDir())
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	t.Setenv("TAI_CONFIG", cfgPath)

	// repo-url set, targets empty.
	_ = os.WriteFile(cfgPath, []byte("repo-url: git@example.com:acme/repo.git\n"), 0o644)

	r := runRoot(t, "sync")
	if r.err == nil {
		t.Fatal("expected error, got nil")
	}
	if r.exitCode != 2 {
		t.Errorf("exit code: want 2, got %d", r.exitCode)
	}
	if !strings.Contains(r.stderr, "[exit 2: TAI_NOT_CONFIGURED]") {
		t.Errorf("stderr missing TAI_NOT_CONFIGURED footer, got:\n%s", r.stderr)
	}
	if !strings.Contains(r.stderr, "tai config target add") {
		t.Errorf("'what to do' missing target-add suggestion, got:\n%s", r.stderr)
	}
}

// --- background-poll tests (TC-SYNC-014/015/016/017) ---
//
// These tests exercise sync.Poll directly because the background
// goroutine in main.go is a fire-and-forget shape; calling Poll
// synchronously gives deterministic results without needing
// runRoot's stdio gymnastics.

// TestUpdatePoll_TCSYNC014_stale_cache_refreshed exercises TC-SYNC-014.
func TestUpdatePoll_TCSYNC014_stale_cache_refreshed(t *testing.T) {
	requireGit(t)
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{"README.md": "x"})

	dataDir, _, _ := syncEnv(t, url)
	// syncEnv sets update-check-interval=0 (disabled). Override the
	// config to a short interval so IsStale flips true.
	cfgPath, _ := filepath.Abs(os.Getenv("TAI_CONFIG"))
	cfg := fmt.Sprintf("repo-url: %s\nupdate-check-interval: 1ns\n", url)
	_ = os.WriteFile(cfgPath, []byte(cfg), 0o644)

	// Seed a stale cache: a record with LastCheck deep in the past.
	statePath := filepath.Join(dataDir, "state", "update-check.json")
	_ = os.MkdirAll(filepath.Dir(statePath), 0o755)
	old := `{"last-check":"2000-01-01T00:00:00Z","has-updates":false}` + "\n"
	_ = os.WriteFile(statePath, []byte(old), 0o644)
	beforeInfo, _ := os.Stat(statePath)

	pollDirect(t, url, dataDir)

	afterInfo, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("state file gone: %v", err)
	}
	if !afterInfo.ModTime().After(beforeInfo.ModTime()) {
		t.Errorf("state file mtime did not advance; before=%v after=%v", beforeInfo.ModTime(), afterInfo.ModTime())
	}
}

// TestUpdatePoll_TCSYNC015_fresh_cache_untouched exercises TC-SYNC-015.
func TestUpdatePoll_TCSYNC015_fresh_cache_untouched(t *testing.T) {
	requireGit(t)
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{"README.md": "x"})

	dataDir, _, _ := syncEnv(t, url)
	cfgPath, _ := filepath.Abs(os.Getenv("TAI_CONFIG"))
	// 1h interval — a fresh cache should easily fall inside it.
	cfg := fmt.Sprintf("repo-url: %s\nupdate-check-interval: 1h\n", url)
	_ = os.WriteFile(cfgPath, []byte(cfg), 0o644)

	statePath := filepath.Join(dataDir, "state", "update-check.json")
	_ = os.MkdirAll(filepath.Dir(statePath), 0o755)
	fresh := fmt.Sprintf(`{"last-check":%q,"has-updates":false}`+"\n",
		time.Now().UTC().Format(time.RFC3339Nano))
	_ = os.WriteFile(statePath, []byte(fresh), 0o644)
	before, _ := os.ReadFile(statePath)

	pollDirect(t, url, dataDir)

	after, _ := os.ReadFile(statePath)
	if string(before) != string(after) {
		t.Errorf("fresh cache should not be touched\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestUpdatePoll_TCSYNC016_poll_error_silent exercises TC-SYNC-016.
func TestUpdatePoll_TCSYNC016_poll_error_silent(t *testing.T) {
	requireGit(t)
	config.AllowFileURLsForTesting(t)
	// Use a bogus repo URL — git ls-remote will fail. The poll must
	// swallow the error and leave the cache untouched.
	bogus := "file:///does/not/exist/here"
	dataDir := t.TempDir()
	t.Setenv("TAI_DATA_DIR", dataDir)
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yml")
	t.Setenv("TAI_CONFIG", cfgPath)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	cfg := fmt.Sprintf("repo-url: %s\nupdate-check-interval: 1ns\n", bogus)
	_ = os.WriteFile(cfgPath, []byte(cfg), 0o644)

	statePath := filepath.Join(dataDir, "state", "update-check.json")
	_ = os.MkdirAll(filepath.Dir(statePath), 0o755)
	original := `{"last-check":"2000-01-01T00:00:00Z","has-updates":false}` + "\n"
	_ = os.WriteFile(statePath, []byte(original), 0o644)

	pollDirect(t, bogus, dataDir)

	got, _ := os.ReadFile(statePath)
	if string(got) != original {
		t.Errorf("state file should be byte-identical after a poll error\nbefore:\n%s\nafter:\n%s", original, got)
	}
}

// TestUpdatePoll_TCSYNC017_disabled_skips_poll exercises TC-SYNC-017.
func TestUpdatePoll_TCSYNC017_disabled_skips_poll(t *testing.T) {
	requireGit(t)
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{"README.md": "x"})

	dataDir, _, _ := syncEnv(t, url) // syncEnv sets interval=0

	statePath := filepath.Join(dataDir, "state", "update-check.json")
	_ = os.MkdirAll(filepath.Dir(statePath), 0o755)
	original := `{"last-check":"2000-01-01T00:00:00Z","has-updates":false}` + "\n"
	_ = os.WriteFile(statePath, []byte(original), 0o644)

	pollDirect(t, url, dataDir)

	got, _ := os.ReadFile(statePath)
	if string(got) != original {
		t.Errorf("disabled poll should leave state file untouched\nbefore:\n%s\nafter:\n%s", original, got)
	}
}
