package cmd_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/plugins"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// bundleFetcher copies a staged directory into the install staging
// area, standing in for the tarball download.
type bundleFetcher struct {
	root    string
	version string
	err     error
}

func (f *bundleFetcher) Fetch(_ context.Context, _ string, _ plugins.Source, destDir string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.version, copyTreeInto(f.root, destDir)
}

func copyTreeInto(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode())
	})
}

// stageBundle writes a minimal installable plugin: an executable that
// answers --help-summary, plus the assets/ directory the contract
// requires.
func stageBundle(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("plugin bundle fixtures use a POSIX shell stub")
	}
	root := t.TempDir()
	bin := filepath.Join(root, name)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 'Does a thing.'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// TC-PLG-029 — a successful install ends by telling the user how to
// find out what the plugin does.
func TestPluginsInstall_TCPLG029_prints_the_onboarding_hint(t *testing.T) {
	pluginsEnv(t)
	plugins.FetcherForTesting(t, &bundleFetcher{root: stageBundle(t, "triage"), version: "v1.0.0"})

	r := runRoot(t, "plugins", "install", "triage")
	if r.err != nil {
		t.Fatalf("unexpected error: %v (stderr %q)", r.err, r.stderr)
	}
	want := "→ Run `tai triage help` to learn how to use triage.\n"
	if !strings.Contains(r.stderr, want) {
		t.Errorf("stderr must carry %q, got %q", want, r.stderr)
	}
	// The hint names no AI tool — where the plugin fits in the user's
	// tool is the plugin's own help to explain.
	for _, tool := range []string{"Claude", "Cursor", "Cody", "Copilot"} {
		if strings.Contains(r.stderr, tool) {
			t.Errorf("hint must stay AI-tool-agnostic, names %q", tool)
		}
	}
	if strings.Contains(r.stdout, "Run `tai triage help`") {
		t.Errorf("the hint belongs on stderr, found it on stdout: %q", r.stdout)
	}
}

// TC-PLG-030 — nothing was installed, so there is nothing to learn
// how to use.
func TestPluginsInstall_TCPLG030_failed_install_prints_no_hint(t *testing.T) {
	pluginsEnv(t)
	plugins.FetcherForTesting(t, &bundleFetcher{
		err: errcode.New(errcode.PluginFetchFailed, "release not found"),
	})

	r := runRoot(t, "plugins", "install", "triage")
	if r.err == nil {
		t.Fatal("expected the install to fail")
	}
	if strings.Contains(r.stderr, "to learn how to use") {
		t.Errorf("a failed install must print no onboarding hint, got %q", r.stderr)
	}
}

// TC-PLG-031 — update lands the same hint: the plugin's verbs may
// have changed, and its help is where that shows up.
func TestPluginsUpdate_TCPLG031_prints_the_onboarding_hint(t *testing.T) {
	pluginsEnv(t)
	bundle := stageBundle(t, "triage")
	plugins.FetcherForTesting(t, &bundleFetcher{root: bundle, version: "v1.0.0"})
	if r := runRoot(t, "plugins", "install", "triage"); r.err != nil {
		t.Fatalf("setup install: %v (stderr %q)", r.err, r.stderr)
	}

	plugins.FetcherForTesting(t, &bundleFetcher{root: bundle, version: "v1.1.0"})
	r := runRoot(t, "plugins", "update", "triage")
	if r.err != nil {
		t.Fatalf("unexpected error: %v (stderr %q)", r.err, r.stderr)
	}
	want := "→ Run `tai triage help` to learn how to use triage.\n"
	if !strings.Contains(r.stderr, want) {
		t.Errorf("stderr must carry %q, got %q", want, r.stderr)
	}
}

// A fetch failure during update must not produce the hint either —
// the installed plugin is untouched, so there is no new news.
func TestPluginsUpdate_TCPLG030_failed_update_prints_no_hint(t *testing.T) {
	pluginsEnv(t)
	plugins.FetcherForTesting(t, &bundleFetcher{root: stageBundle(t, "triage"), version: "v1.0.0"})
	if r := runRoot(t, "plugins", "install", "triage"); r.err != nil {
		t.Fatalf("setup install: %v (stderr %q)", r.err, r.stderr)
	}

	plugins.FetcherForTesting(t, &bundleFetcher{err: errors.New("network down")})
	r := runRoot(t, "plugins", "update", "triage")
	if r.err == nil {
		t.Fatal("expected the update to fail")
	}
	if strings.Contains(r.stderr, "to learn how to use") {
		t.Errorf("a failed update must print no onboarding hint, got %q", r.stderr)
	}
}
