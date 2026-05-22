package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// resetConfigEnv unsets every env var ResolvePath consults so each test
// starts from a known baseline.
func resetConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TAI_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AppData", "")
	// Override HOME to a fake directory so we never touch the developer's
	// real ~/.config/tai.
	t.Setenv("HOME", t.TempDir())
}

// TestResolve_TCCONF001_default_linux_path exercises TC-CONF-001 from
// core/test-cases.md: with no env overrides, the resolved path is
// $HOME/.config/tai/config.yml. The test is skipped on Windows because
// the Windows-default path is a separate scenario.
func TestResolve_TCCONF001_default_linux_path(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses %AppData% by default; covered separately")
	}
	home := "/tmp/fake-home"
	t.Setenv("TAI_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	got, err := config.ResolvePath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(home, ".config", "tai", "config.yml")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestResolve_TCCONF002_tai_config_overrides exercises TC-CONF-002:
// $TAI_CONFIG wins over $XDG_CONFIG_HOME and is used verbatim (no
// tai/ suffix appended).
func TestResolve_TCCONF002_tai_config_overrides(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	t.Setenv("TAI_CONFIG", "/tmp/explicit/config.yml")

	got, err := config.ResolvePath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/explicit/config.yml" {
		t.Fatalf("got %q, want /tmp/explicit/config.yml", got)
	}
}

// TestResolve_xdg_config_home covers the precedence between
// XDG_CONFIG_HOME and the platform default — XDG wins when set. Not
// directly tied to a TC-ID because both XDG-set and default-HOME are
// folded into TC-CONF-001 / TC-CONF-002 in the spec, but the parser
// has a distinct branch worth pinning.
func TestResolve_xdg_config_home(t *testing.T) {
	resetConfigEnv(t)
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")

	got, err := config.ResolvePath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/tmp/xdg", "tai", "config.yml")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestLoad_returns_nil_when_file_missing locks the contract that an
// absent config file is a valid state, not an error.
//
// Not tied to a TC-ID because the absent-file shape is the engine-level
// contract that TC-CONF-003 / 009 depend on at the CLI boundary; this
// test guards the helper that those CLI tests rely on.
func TestLoad_returns_nil_when_file_missing(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "no-such-config.yml")

	got, err := config.Load(missing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil config when file missing, got %+v", got)
	}
}

// TestLoad_TCCONF007_all_falsy_subpaths_rejected exercises TC-CONF-007:
// a target whose three sub-paths are every set to "" must be rejected
// with CONFIG_INVALID.
func TestLoad_TCCONF007_all_falsy_subpaths_rejected(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yml")
	body := []byte(`targets:
  - root: /tmp/example
    skills: ""
    commands: ""
    agents: ""
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	taiErr, ok := errcode.As(err)
	if !ok || taiErr.Code != errcode.ConfigInvalid {
		t.Fatalf("expected CONFIG_INVALID, got %v", err)
	}
	if !strings.Contains(taiErr.Msg, "/tmp/example") {
		t.Fatalf("error message should name the offending target, got %q", taiErr.Msg)
	}
}

// TestTarget_TCCONF005_effective_subpath_defaults exercises TC-CONF-005:
// a target with no explicit sub-paths resolves to the standard names
// under root.
func TestTarget_TCCONF005_effective_subpath_defaults(t *testing.T) {
	tgt := config.Target{Root: "/home/user/.claude"}
	skills, commands, agents := tgt.EffectiveSubpaths()

	if skills != "/home/user/.claude/skills" {
		t.Errorf("skills = %q, want /home/user/.claude/skills", skills)
	}
	if commands != "/home/user/.claude/commands" {
		t.Errorf("commands = %q, want /home/user/.claude/commands", commands)
	}
	if agents != "/home/user/.claude/agents" {
		t.Errorf("agents = %q, want /home/user/.claude/agents", agents)
	}
}

// TestTarget_falsy_skips_category locks the spec's falsy-skip rule at
// the loader/struct level: an explicit empty-string sub-path yields an
// empty EffectiveSubpath, signalling "skip this category".
//
// Not tied to a TC-ID because the user-visible falsy-skip-with-warning
// behaviour is part of Phase 2's tai sync work — this test guards the
// engine invariant that Phase 2 will hook into.
func TestTarget_falsy_skips_category(t *testing.T) {
	empty := ""
	tgt := config.Target{Root: "/r", Commands: &empty}
	skills, commands, agents := tgt.EffectiveSubpaths()

	if skills != "/r/skills" {
		t.Errorf("skills should default to /r/skills, got %q", skills)
	}
	if commands != "" {
		t.Errorf("commands should be empty (skip), got %q", commands)
	}
	if agents != "/r/agents" {
		t.Errorf("agents should default to /r/agents, got %q", agents)
	}
}

// TestValidate_rejects_file_url confirms the loader rejects a file://
// repo-url at validation time — covers the engine half of TC-CONF-006
// (the CLI-boundary half lands in core/internal/cmd/config_test.go).
func TestValidate_rejects_file_url(t *testing.T) {
	f := &config.File{RepoURL: "file:///tmp/repo"}
	err := config.Validate(f)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	taiErr, ok := errcode.As(err)
	if !ok || taiErr.Code != errcode.ConfigInvalidRepoURL {
		t.Fatalf("expected CONFIG_INVALID_REPO_URL, got %v", err)
	}
}

// TestValidate_rejects_malformed_git_at_url confirms that a `git@host`
// URL with no colon (the SCP separator) is rejected. The colon-check
// branch in validateRepoURL exists for this case; the test guards
// against a future refactor that collapses it.
//
// Not tied to a TC-ID because TC-CONF-006 carves the user-observable
// rejections for `file://` and absolute paths only; this is a
// parser-level corner case.
func TestValidate_rejects_malformed_git_at_url(t *testing.T) {
	err := config.Validate(&config.File{RepoURL: "git@github.com"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	taiErr, ok := errcode.As(err)
	if !ok || taiErr.Code != errcode.ConfigInvalidRepoURL {
		t.Fatalf("expected CONFIG_INVALID_REPO_URL, got %v", err)
	}
}

// TestValidate_rejects_local_path confirms an absolute filesystem path
// is rejected as a repo-url. Covers the second half of TC-CONF-006.
func TestValidate_rejects_local_path(t *testing.T) {
	f := &config.File{RepoURL: "/tmp/repo"}
	err := config.Validate(f)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	taiErr, ok := errcode.As(err)
	if !ok || taiErr.Code != errcode.ConfigInvalidRepoURL {
		t.Fatalf("expected CONFIG_INVALID_REPO_URL, got %v", err)
	}
}

// TestValidate_accepts_remote_urls locks the accepted-form table for
// repo-url. Each entry must validate without error.
//
// Not tied to a TC-ID because the spec only carves a scenario for the
// rejection side (TC-CONF-006); this is the parser's positive table,
// which guards regressions in the accepted-prefix set.
func TestValidate_accepts_remote_urls(t *testing.T) {
	urls := []string{
		"git@github.com:acme/repo.git",
		"git@example.com:org/repo",
		"ssh://git@github.com/acme/repo.git",
		"https://github.com/acme/repo.git",
		"https://gitlab.example.com/acme/repo",
	}
	for _, u := range urls {
		t.Run(u, func(t *testing.T) {
			if err := config.Validate(&config.File{RepoURL: u}); err != nil {
				t.Errorf("Validate(%q) failed: %v", u, err)
			}
		})
	}
}

// TestSave_creates_parent_dir_and_writes locks the lazy-creation
// behaviour at the engine level. The CLI-boundary half lands in
// core/internal/cmd/config_test.go (TC-CONF-004).
//
// Not tied to a TC-ID because TC-CONF-004 already covers the
// user-observable path; this is the engine seam it relies on.
func TestSave_creates_parent_dir_and_writes(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nested", "deeper", "config.yml")

	skills := "custom"
	f := &config.File{
		Targets: []config.Target{{Root: "/r", Skills: &skills}},
	}
	if err := config.Save(path, f); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if !strings.Contains(string(got), "/r") || !strings.Contains(string(got), "custom") {
		t.Fatalf("written config missing target fields, got:\n%s", got)
	}
}

// TestSave_unwritable_dir_surfaces_CONFIG_UNWRITABLE confirms an
// unwritable parent directory triggers the CONFIG_UNWRITABLE code.
// The test skips on platforms where mode 0o000 doesn't enforce
// permissions (Windows, root-as-test-runner).
//
// Not tied to a TC-ID because the spec scenario "If the resolved
// config file cannot be created or its parent directory is not
// writable, the system MUST exit with error code CONFIG_UNWRITABLE"
// describes a CLI-boundary behaviour; this engine-level test pins the
// underlying error path so the CLI boundary doesn't need to reproduce
// the unwritable-directory setup.
func TestSave_unwritable_dir_surfaces_CONFIG_UNWRITABLE(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses mode 0o000")
	}
	tmp := t.TempDir()
	readonly := filepath.Join(tmp, "readonly")
	if err := os.Mkdir(readonly, 0o555); err != nil {
		t.Fatalf("mkdir readonly: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readonly, 0o755) })

	path := filepath.Join(readonly, "sub", "config.yml")
	err := config.Save(path, &config.File{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	taiErr, ok := errcode.As(err)
	if !ok || taiErr.Code != errcode.ConfigUnwritable {
		t.Fatalf("expected CONFIG_UNWRITABLE, got %v", err)
	}
}

// TestEffectiveUpdateCheckInterval_defaults_and_parses pins the
// 6h-default and 0-disables behaviour spec'd for update-check-interval.
//
// Not tied to a TC-ID because the user-visible banner cadence lands in
// Phase 5 (TC-UB-*); this engine test guards the parser the banner
// goroutine will read.
func TestEffectiveUpdateCheckInterval_defaults_and_parses(t *testing.T) {
	cases := []struct {
		in      string
		wantSec int64
		wantErr bool
	}{
		{"", 6 * 3600, false},
		{"30m", 30 * 60, false},
		{"6h", 6 * 3600, false},
		{"0", 0, false},
		{"not-a-duration", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			f := &config.File{UpdateCheckInterval: tc.in}
			got, err := f.EffectiveUpdateCheckInterval()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err: got %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && int64(got.Seconds()) != tc.wantSec {
				t.Fatalf("seconds: got %d, want %d", int64(got.Seconds()), tc.wantSec)
			}
		})
	}
}

// TestLoad_preserves_unset_subpath_as_nil pins the YAML-pointer
// behaviour the loader depends on to distinguish "omitted" from
// "explicit empty".
//
// Not tied to a TC-ID because the user-observable consequence (sync
// defaults vs sync skips) surfaces in Phase 2; this test guards the
// engine seam (nil pointer vs pointer-to-empty-string) that the
// falsy-skip rule reads.
func TestLoad_preserves_unset_subpath_as_nil(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yml")
	body := []byte(`targets:
  - root: /tmp/a
    skills: custom-skills
  - root: /tmp/b
    commands: ""
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	f, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(f.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(f.Targets))
	}
	if f.Targets[0].Skills == nil || *f.Targets[0].Skills != "custom-skills" {
		t.Fatalf("target[0].Skills: want ptr to 'custom-skills', got %v", f.Targets[0].Skills)
	}
	if f.Targets[0].Commands != nil {
		t.Fatalf("target[0].Commands: want nil (omitted), got %q", *f.Targets[0].Commands)
	}
	if f.Targets[1].Commands == nil || *f.Targets[1].Commands != "" {
		t.Fatalf("target[1].Commands: want ptr to '' (explicit skip), got %v", f.Targets[1].Commands)
	}
}

// TestCommentedTemplate_parses_as_empty_config confirms that stripping
// the leading "# " from every commented example line produces a YAML
// document that parses successfully into *config.File. This locks the
// template against drift when a new field lands: the substring test
// catches a missing field name, but only this round-trip catches
// example values that no longer parse.
//
// Not tied to a TC-ID because the template's parseability is an engine
// invariant; the user-observable behaviour (template appears on disk)
// is covered by TC-CONF-010.
func TestCommentedTemplate_parses_as_empty_config(t *testing.T) {
	// The template's contract (documented in CommentedTemplate's doc):
	// "##"-prefixed lines are prose comments (skipped here); "# "
	// (hash-space) lines are commented YAML examples. Stripping "# "
	// from those gives a valid YAML document. A broken example value
	// surfaces immediately rather than waiting for a user to uncomment
	// and hit it.
	raw := string(config.CommentedTemplate())
	var stripped strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		switch {
		case strings.HasPrefix(trimmed, "## "), trimmed == "##", strings.HasPrefix(trimmed, "##"):
			// Prose comment — emit a blank to preserve line numbers.
			stripped.WriteByte('\n')
		case strings.HasPrefix(trimmed, "# "):
			// Drop the "# " prefix; preserve indentation so YAML
			// nesting survives.
			indent := line[:len(line)-len(trimmed)]
			stripped.WriteString(indent)
			stripped.WriteString(trimmed[2:])
			stripped.WriteByte('\n')
		default:
			stripped.WriteString(line)
			stripped.WriteByte('\n')
		}
	}

	var f config.File
	if err := yaml.Unmarshal([]byte(stripped.String()), &f); err != nil {
		t.Fatalf("template does not parse as YAML after stripping `# ` example markers: %v\nstripped:\n%s", err, stripped.String())
	}
}

// TestCommentedTemplate_contains_every_field locks the contract that
// the template documents every supported top-level key.
//
// Not tied to a TC-ID because TC-CONF-010 covers the user-observable
// "template appears on disk" behaviour; this guards the template's
// content so a missing field would surface at the engine level rather
// than waiting for the CLI test to catch it.
func TestCommentedTemplate_contains_every_field(t *testing.T) {
	got := string(config.CommentedTemplate())
	for _, want := range []string{"repo-url", "targets", "update-check-interval"} {
		if !strings.Contains(got, want) {
			t.Errorf("template missing %q field, got:\n%s", want, got)
		}
	}
}

// TestErrcode_unwrap_chain confirms Validate's wrapped errors preserve
// the underlying cause through errors.Unwrap.
//
// Not tied to a TC-ID because the cause-preservation invariant is a
// foundation-wide contract (covered for the writer side by TC-ERR-005
// in pkg/test-cases.md); this test pins it for the validator.
func TestErrcode_unwrap_chain(t *testing.T) {
	f := &config.File{UpdateCheckInterval: "garbage"}
	err := config.Validate(f)
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Unwrap(err) == nil {
		t.Fatal("expected wrapped cause, got nil chain")
	}
}
