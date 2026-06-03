package plugins_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/plugins"
	"github.com/dmastrorillo/tai/core/internal/testutil"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// fakeFetcher implements plugins.Fetcher by copying a pre-staged
// local directory into destDir. It lets install/update tests avoid
// the tarball/HTTP roundtrip and focus on orchestration.
//
// Not tied to a TC-ID — test fixture helper.
type fakeFetcher struct {
	source  string // local directory to copy from
	version string // version string Fetch should return
}

func (f *fakeFetcher) Fetch(_ context.Context, _ string, _ plugins.Source, destDir string) (string, error) {
	if err := copyTreeForTest(f.source, destDir); err != nil {
		return "", err
	}
	return f.version, nil
}

// stagePluginBundle creates a fake plugin bundle on disk that
// fakeFetcher can copy from. Layout:
//
//	<root>/<binName>            (executable stub)
//	<root>/assets/skills/<...>
//	<root>/assets/commands/<...>
//	<root>/assets/agents/<...>
//
// `assets` is a map of relative path → content. The binary's name
// follows the platform convention so the install flow's chmod path
// is exercised.
//
// Not tied to a TC-ID — test fixture helper.
func stagePluginBundle(t *testing.T, name string, assets map[string]string) string {
	t.Helper()
	root := t.TempDir()
	binName := name
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	if err := os.WriteFile(filepath.Join(root, binName), []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	for rel, body := range assets {
		full := filepath.Join(root, "assets", rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

// stageTarget returns a Target rooted at a fresh temp directory.
//
// Not tied to a TC-ID — test fixture helper.
func stageTarget(t *testing.T) config.Target {
	t.Helper()
	return config.Target{Root: t.TempDir()}
}

// TestInstall_TCPLG001_plugin_layout_on_disk exercises TC-PLG-001.
func TestInstall_TCPLG001_plugin_layout_on_disk(t *testing.T) {
	dataDir := t.TempDir()
	bundle := stagePluginBundle(t, "triage", map[string]string{
		"skills/tai-triage-pulse.md":  "x",
		"commands/import.md":          "x",
		"agents/tai-triage-helper.md": "x",
	})
	cfg := &config.File{Targets: []config.Target{stageTarget(t)}}

	_, err := plugins.Install(context.Background(), "triage", dataDir, cfg, plugins.InstallOptions{
		Source:  plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
		Fetcher: &fakeFetcher{source: bundle, version: "v0.5.0"},
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	installDir := plugins.PluginInstallDir(dataDir, "triage")
	binName := "triage"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	if _, err := os.Stat(filepath.Join(installDir, binName)); err != nil {
		t.Errorf("missing binary: %v", err)
	}
	if info, err := os.Stat(filepath.Join(installDir, "assets")); err != nil || !info.IsDir() {
		t.Errorf("assets/ not a directory: %v", err)
	}
}

// TestInstall_TCPLG006_skill_namespace_enforced exercises TC-PLG-006.
func TestInstall_TCPLG006_skill_namespace_enforced(t *testing.T) {
	dataDir := t.TempDir()
	bundle := stagePluginBundle(t, "mytool", map[string]string{
		// Missing required prefix `tai-mytool-`.
		"skills/foo.md": "x",
	})
	cfg := &config.File{Targets: []config.Target{stageTarget(t)}}

	_, err := plugins.Install(context.Background(), "mytool", dataDir, cfg, plugins.InstallOptions{
		Source:  plugins.Source{Host: "github.com", Repo: "acme/tai-plugin-mytool"},
		Fetcher: &fakeFetcher{source: bundle, version: "v0.1.0"},
	})
	testutil.AssertErrCode(t, err, errcode.PluginAssetNaming)
	if !strings.Contains(err.Error(), "foo.md") {
		t.Errorf("error should name the offending file, got: %v", err)
	}
}

// TestInstall_TCPLG007_commands_routed_into_namespace exercises TC-PLG-007.
func TestInstall_TCPLG007_commands_routed_into_namespace(t *testing.T) {
	dataDir := t.TempDir()
	bundle := stagePluginBundle(t, "triage", map[string]string{
		"commands/import.md": "import body",
	})
	tgt := stageTarget(t)
	cfg := &config.File{Targets: []config.Target{tgt}}

	_, err := plugins.Install(context.Background(), "triage", dataDir, cfg, plugins.InstallOptions{
		Source:  plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
		Fetcher: &fakeFetcher{source: bundle, version: "v0.5.0"},
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	want := filepath.Join(tgt.Root, "commands", "tai-triage", "import.md")
	body, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("expected command at %s, got: %v", want, err)
	}
	if string(body) != "import body" {
		t.Errorf("command body: %q", body)
	}
}

// TestInstall_TCPLG009_401_surfaces_unauthorized exercises TC-PLG-009.
func TestInstall_TCPLG009_401_surfaces_unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv("GITHUB_TOKEN", "")

	dataDir := t.TempDir()
	cfg := &config.File{Targets: []config.Target{stageTarget(t)}}

	_, err := plugins.Install(context.Background(), "triage", dataDir, cfg, plugins.InstallOptions{
		Source: plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
		Fetcher: &plugins.HTTPFetcher{
			Client:        srv.Client(),
			GitHubBaseURL: srv.URL,
		},
	})
	testutil.AssertErrCode(t, err, errcode.PluginFetchUnauthorized)
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should name the HTTP status, got: %v", err)
	}
}

// TestInstall_TCPLG010_5xx_surfaces_failure exercises TC-PLG-010.
func TestInstall_TCPLG010_5xx_surfaces_failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	dataDir := t.TempDir()
	cfg := &config.File{Targets: []config.Target{stageTarget(t)}}

	_, err := plugins.Install(context.Background(), "triage", dataDir, cfg, plugins.InstallOptions{
		Source: plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
		Fetcher: &plugins.HTTPFetcher{
			Client:        srv.Client(),
			GitHubBaseURL: srv.URL,
		},
	})
	testutil.AssertErrCode(t, err, errcode.PluginFetchFailed)
}

// TestInstall_reserved_name_rejected anchors the engine-side
// PLUGIN_NAME_RESERVED contract used by TC-PLG-004 at the CLI
// boundary. Not tied to a TC-ID because the user-visible footer
// assertion lives in core/internal/cmd's plugins_test.go.
func TestInstall_reserved_name_rejected(t *testing.T) {
	dataDir := t.TempDir()
	_, err := plugins.Install(context.Background(), "config", dataDir, &config.File{}, plugins.InstallOptions{})
	testutil.AssertErrCode(t, err, errcode.PluginNameReserved)
	if !strings.Contains(err.Error(), "config") {
		t.Errorf("error should name the offending verb, got: %v", err)
	}

	// And no directory was created under `<dataDir>/plugins/`.
	if _, statErr := os.Stat(filepath.Join(dataDir, "plugins", "config")); !os.IsNotExist(statErr) {
		t.Errorf("expected no install dir, stat: %v", statErr)
	}
}

// TestInstall_preserves_executable_bit_on_asset locks the
// regression caught in Phase 4 review: copyFile (used by
// SyncAssetsToTargets) previously hard-coded 0o644 and silently
// stripped the executable bit on bundled scripts. Not tied to a
// TC-ID — the user-observable behaviour is "the asset runs", which
// no Phase-4 TC exercises in isolation. This locks the in-tree
// invariant.
func TestInstall_preserves_executable_bit_on_asset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits don't apply on Windows")
	}
	dataDir := t.TempDir()
	bundle := stagePluginBundle(t, "triage", nil)
	// Manually drop an executable script into the bundle's assets.
	scriptPath := filepath.Join(bundle, "assets", "commands", "run.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	tgt := stageTarget(t)
	cfg := &config.File{Targets: []config.Target{tgt}}
	if _, err := plugins.Install(context.Background(), "triage", dataDir, cfg, plugins.InstallOptions{
		Source:  plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
		Fetcher: &fakeFetcher{source: bundle, version: "v0.0.0-test"},
	}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	want := filepath.Join(tgt.Root, "commands", "tai-triage", "run.sh")
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat %s: %v", want, err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("executable bit was stripped — mode is %o", info.Mode().Perm())
	}
}

// TestInstall_unknown_name_no_source anchors PLUGIN_UNKNOWN used by
// TC-PLG-008 at the CLI boundary.
func TestInstall_unknown_name_no_source(t *testing.T) {
	dataDir := t.TempDir()
	_, err := plugins.Install(context.Background(), "acme-custom", dataDir, &config.File{}, plugins.InstallOptions{})
	testutil.AssertErrCode(t, err, errcode.PluginUnknown)
}

// copyTreeForTest is a tiny recursive copier used by the fake
// fetcher. Deliberately separate from the production `copyDir`
// (assets.go) so a bug in the production copier can't silently make
// the fixture path pass — the test fixture's job is to reach the
// production install flow with a real tree, not to share its
// internals. Not tied to a TC-ID.
func copyTreeForTest(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode().Perm())
	})
}
