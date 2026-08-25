package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scaffoldEnv stages an isolated $HOME/$XDG/$TAI_CONFIG layout for repo
// init tests so they never touch the developer's real config. Returns
// the resolved config path so tests that assert "config unchanged" can
// snapshot it.
//
// Not tied to a TC-ID — test fixture helper.
func scaffoldEnv(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AppData", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	t.Setenv("TAI_CONFIG", path)
	return path
}

// requireGit skips a test when git isn't on PATH. The repo init flow
// shells out to git; the no-git path is its own dedicated test.
//
// Not tied to a TC-ID — test fixture helper.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH; covered separately by TC-INIT-006")
	}
}

// TestRepoInit_TCINIT001_fresh_directory exercises TC-INIT-001: every
// expected file lands with non-empty content (or, for plugins.yml, at
// least exists).
func TestRepoInit_TCINIT001_fresh_directory(t *testing.T) {
	requireGit(t)
	scaffoldEnv(t)

	root := filepath.Join(t.TempDir(), "fresh")
	r := runRoot(t, "repo", "init", root)
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	must := []string{
		"README.md",
		"skills/README.md",
		"commands/README.md",
		"agents/README.md",
		"workflows/README.md",
		"standards/README.md",
		".gitignore",
		"plugins.yml",
	}
	for _, rel := range must {
		p := filepath.Join(root, rel)
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("missing scaffold file %q: %v", rel, err)
			continue
		}
		if rel != "plugins.yml" && info.Size() == 0 {
			t.Errorf("scaffold file %q is empty; expected non-empty content", rel)
		}
	}
}

// TestRepoInit_TCINIT002_existing_empty_dir exercises TC-INIT-002:
// scaffolding into a pre-existing empty directory succeeds.
func TestRepoInit_TCINIT002_existing_empty_dir(t *testing.T) {
	requireGit(t)
	scaffoldEnv(t)

	root := t.TempDir() // already exists, empty
	r := runRoot(t, "repo", "init", root)
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if r.exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d", r.exitCode)
	}
	if _, err := os.Stat(filepath.Join(root, "README.md")); err != nil {
		t.Fatalf("README.md not written into existing empty dir: %v", err)
	}
}

// TestRepoInit_TCINIT003_non_empty_target_rejected exercises TC-INIT-003.
func TestRepoInit_TCINIT003_non_empty_target_rejected(t *testing.T) {
	scaffoldEnv(t)

	root := t.TempDir()
	preexisting := filepath.Join(root, "user-file.txt")
	if err := os.WriteFile(preexisting, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("seed pre-existing file: %v", err)
	}

	r := runRoot(t, "repo", "init", root)
	if r.err == nil {
		t.Fatal("expected error, got nil")
	}
	if r.exitCode != 1 {
		t.Errorf("exit code: want 1, got %d", r.exitCode)
	}
	if !strings.Contains(r.stderr, "[exit 1: REPO_INIT_TARGET_NOT_EMPTY]") {
		t.Errorf("stderr missing REPO_INIT_TARGET_NOT_EMPTY footer; got:\n%s", r.stderr)
	}
	// The pre-existing file MUST survive untouched; no scaffold writes
	// should have leaked through.
	body, err := os.ReadFile(preexisting)
	if err != nil {
		t.Fatalf("pre-existing file disappeared: %v", err)
	}
	if string(body) != "hello\n" {
		t.Errorf("pre-existing file mutated; got %q", string(body))
	}
	if _, err := os.Stat(filepath.Join(root, "skills")); !os.IsNotExist(err) {
		t.Errorf("scaffold leaked content into rejected target; stat skills/: %v", err)
	}
}

// TestRepoInit_TCINIT004_readme_content exercises TC-INIT-004: the
// READMEs and plugins.yml carry the substrings the spec promises.
func TestRepoInit_TCINIT004_readme_content(t *testing.T) {
	requireGit(t)
	scaffoldEnv(t)

	root := filepath.Join(t.TempDir(), "content")
	r := runRoot(t, "repo", "init", root)
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	cases := []struct {
		path string
		want string
	}{
		{"skills/README.md", "tai-<plugin>-"},
		{"workflows/README.md", "description:"},
		{"standards/README.md", ":"},
	}
	for _, c := range cases {
		body, err := os.ReadFile(filepath.Join(root, c.path))
		if err != nil {
			t.Errorf("read %s: %v", c.path, err)
			continue
		}
		if !strings.Contains(string(body), c.want) {
			t.Errorf("%s should contain %q, got:\n%s", c.path, c.want, body)
		}
	}

	plugins, err := os.ReadFile(filepath.Join(root, "plugins.yml"))
	if err != nil {
		t.Fatalf("read plugins.yml: %v", err)
	}
	// At least one line starting with `#` somewhere in the file.
	hasComment := false
	for _, line := range strings.Split(string(plugins), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			hasComment = true
			break
		}
	}
	if !hasComment {
		t.Errorf("plugins.yml should contain at least one `#` comment line, got:\n%s", plugins)
	}
}

// TestRepoInit_TCINIT009_readme_backlinks_tai exercises TC-INIT-009.
// The scaffolded top-level README MUST point new readers at the
// upstream tai project, explain what tai is in one paragraph, and
// MUST NOT reference the hallucinated `docs.tai.sh` domain that
// shipped in v0.1.
func TestRepoInit_TCINIT009_readme_backlinks_tai(t *testing.T) {
	requireGit(t)
	scaffoldEnv(t)

	root := filepath.Join(t.TempDir(), "readme")
	r := runRoot(t, "repo", "init", root)
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	body, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	got := string(body)

	const backlink = "https://github.com/dmastrorillo/tai"
	if !strings.Contains(got, backlink) {
		t.Errorf("top-level README must contain the tai project backlink %q; got:\n%s", backlink, got)
	}
	// Intro paragraph: just assert the README explicitly names tai as
	// a CLI / distribution tool. The exact wording is owned by the
	// template; this check is intentionally loose so future template
	// edits don't bounce the test as long as the orientation paragraph
	// is still there.
	lower := strings.ToLower(got)
	if !strings.Contains(lower, "tai") || !strings.Contains(lower, "cli") {
		t.Errorf("top-level README must explain what tai is (named `tai` + describes it as a CLI); got:\n%s", got)
	}
	if strings.Contains(got, "docs.tai.sh") {
		t.Errorf("top-level README must NOT reference the hallucinated `docs.tai.sh` domain; got:\n%s", got)
	}
}

// TestRepoInit_TCINIT005_git_init_and_commit exercises TC-INIT-005.
func TestRepoInit_TCINIT005_git_init_and_commit(t *testing.T) {
	requireGit(t)
	scaffoldEnv(t)

	root := filepath.Join(t.TempDir(), "withgit")
	r := runRoot(t, "repo", "init", root)
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf(".git directory not created: %v", err)
	}

	out, err := exec.Command("git", "-C", root, "log", "-1", "--format=%s").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != "Initial TAI source-repo scaffold" {
		t.Errorf("commit message = %q, want %q", got, "Initial TAI source-repo scaffold")
	}
}

// TestRepoInit_TCINIT006_git_unavailable exercises TC-INIT-006 by
// overriding PATH so `git` can't be found, then asserting the scaffold
// files still landed and the error code surfaced.
//
// On Windows the PATH override mechanism differs; skip there since the
// command's git-detection is platform-uniform once exec.LookPath fails.
func TestRepoInit_TCINIT006_git_unavailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH manipulation differs on Windows; git-unavailable path is covered manually")
	}
	scaffoldEnv(t)
	// An empty PATH guarantees exec.LookPath("git") fails — even if
	// the developer has git in /usr/bin, we strip the whole search
	// path for this test.
	t.Setenv("PATH", "")

	root := filepath.Join(t.TempDir(), "nogit")
	r := runRoot(t, "repo", "init", root)
	if r.err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(r.stderr, "[exit 3: REPO_INIT_GIT_UNAVAILABLE]") {
		t.Errorf("stderr missing REPO_INIT_GIT_UNAVAILABLE footer; got:\n%s", r.stderr)
	}
	// The scaffold files MUST be on disk despite the git step failing.
	if _, err := os.Stat(filepath.Join(root, "README.md")); err != nil {
		t.Errorf("scaffold did not write files before the git step; stat README.md: %v", err)
	}
}

// TestRepoInit_TCINIT007_next_steps_block exercises TC-INIT-007.
//
// Also asserts `r.stderr == ""`: the next-steps block is data the
// user asked for, so it belongs on stdout. A regression that routes
// the block (or any of git's chatty init/add/commit output) to
// stderr would surface here.
func TestRepoInit_TCINIT007_next_steps_block(t *testing.T) {
	requireGit(t)
	scaffoldEnv(t)

	root := filepath.Join(t.TempDir(), "nextsteps")
	r := runRoot(t, "repo", "init", root)
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	for _, want := range []string{
		"Next steps:",
		"git remote add origin",
		"tai config set repo-url",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("stdout missing %q\nfull stdout:\n%s", want, r.stdout)
		}
	}
	if r.stderr != "" {
		t.Errorf("stderr should be empty on success, got %q", r.stderr)
	}
}

// TestRepoInit_TCINIT008_local_config_untouched exercises TC-INIT-008.
func TestRepoInit_TCINIT008_local_config_untouched(t *testing.T) {
	requireGit(t)
	configPath := scaffoldEnv(t)
	original := "repo-url: git@github.com:acme/existing.git\ntargets:\n  - root: /tmp/my-target\n"
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config parent: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	root := filepath.Join(t.TempDir(), "noconfig-touch")
	r := runRoot(t, "repo", "init", root)
	if r.err != nil {
		t.Fatalf("unexpected error: %v\nstderr:\n%s", r.err, r.stderr)
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != original {
		t.Fatalf("config mutated by repo init:\nbefore:\n%s\nafter:\n%s", original, got)
	}
}
