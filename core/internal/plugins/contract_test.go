package plugins_test

import (
	"context"
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

// stageContractBundle stages a plugin bundle whose binary is a shell
// stub the test controls, so the wire verb's answer (or refusal) can
// be scripted. assetsDir selects the tarball shape: "populated" ships
// assets/commands/, "empty" ships a bare assets/ directory, "absent"
// ships none at all.
//
// Not tied to a TC-ID — test fixture helper.
func stageContractBundle(t *testing.T, name, script, assetsDir string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script plugin stub is not portable to windows")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	switch assetsDir {
	case "populated":
		p := filepath.Join(root, "assets", "commands", "go.md")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# go\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	case "empty":
		if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
			t.Fatal(err)
		}
	case "absent":
	default:
		t.Fatalf("unknown assetsDir %q", assetsDir)
	}
	return root
}

// answersHelpSummary is a stub that honours the wire contract.
func answersHelpSummary(desc string) string {
	return "#!/bin/sh\nif [ \"$1\" = \"--help-summary\" ]; then echo '" + desc + "'; exit 0; fi\nexit 0\n"
}

// installContractBundle runs Install against a staged bundle and
// returns the data dir alongside the result.
func installContractBundle(t *testing.T, name, bundle string) (string, *plugins.Entry, error) {
	t.Helper()
	dataDir := t.TempDir()
	cfg := &config.File{Targets: []config.Target{{Root: t.TempDir()}}}
	entry, err := plugins.Install(context.Background(), name, dataDir, cfg, plugins.InstallOptions{
		Source:  plugins.Source{Host: "github.com", Repo: "acme/demo"},
		Fetcher: &fakeFetcher{source: bundle, version: "v1.0.0"},
	})
	return dataDir, entry, err
}

// assertNotPromoted fails when anything was left under the plugin's
// final install directory — a rejected bundle must leave no trace.
func assertNotPromoted(t *testing.T, dataDir, name string) {
	t.Helper()
	final := filepath.Join(dataDir, "plugins", name)
	if _, err := os.Stat(final); !os.IsNotExist(err) {
		t.Errorf("%s should not exist after a rejected install, stat: %v", final, err)
	}
}

// TC-PLG-018 — a tarball with no assets/ directory is rejected. The
// directory is the host's guaranteed input to SyncAssetsToTargets;
// its absence signals a plugin that expects to place its own assets,
// which the wire contract forbids.
func TestInstall_TCPLG018_missing_assets_dir_rejected(t *testing.T) {
	bundle := stageContractBundle(t, "demo", answersHelpSummary("Does the thing."), "absent")

	dataDir, _, err := installContractBundle(t, "demo", bundle)

	testutil.AssertErrCode(t, err, errcode.PluginAssetMissing)
	assertNotPromoted(t, dataDir, "demo")
}

// TC-PLG-019 — an empty assets/ directory is valid: a pure-binary
// plugin ships no skills, commands, or agents but still declares that
// the host owns placement.
func TestInstall_TCPLG019_empty_assets_dir_accepted(t *testing.T) {
	bundle := stageContractBundle(t, "demo", answersHelpSummary("Does the thing."), "empty")

	dataDir, entry, err := installContractBundle(t, "demo", bundle)
	if err != nil {
		t.Fatalf("empty assets/ must install cleanly, got %v", err)
	}
	if entry == nil {
		t.Fatal("want an entry")
	}
	if _, statErr := os.Stat(filepath.Join(dataDir, "plugins", "demo")); statErr != nil {
		t.Errorf("plugin should be installed: %v", statErr)
	}
}

// TC-PLG-020 — install captures the plugin's --help-summary answer
// and persists it as the entry's description.
func TestInstall_TCPLG020_captures_description(t *testing.T) {
	const desc = "Walk through pending PR review comments interactively."
	bundle := stageContractBundle(t, "demo", answersHelpSummary(desc), "populated")

	dataDir, entry, err := installContractBundle(t, "demo", bundle)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Description != desc {
		t.Errorf("entry.Description = %q, want %q", entry.Description, desc)
	}

	// The description must survive the round-trip to plugins.json —
	// `tai --help` reads it back from there, not from this return.
	state, err := plugins.LoadState(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range state.Plugins {
		if e.Name == "demo" {
			found = true
			if e.Description != desc {
				t.Errorf("persisted description = %q, want %q", e.Description, desc)
			}
		}
	}
	if !found {
		t.Error("demo missing from plugins.json")
	}
}

// TC-PLG-021 — a plugin that cannot answer the wire verb is not
// installed. Each failure shape aborts before promotion, so a prior
// install of the same plugin is left intact.
func TestInstall_TCPLG021_help_summary_failure_aborts(t *testing.T) {
	cases := []struct {
		name   string
		script string
	}{
		{
			name:   "exits non-zero",
			script: "#!/bin/sh\nif [ \"$1\" = \"--help-summary\" ]; then exit 1; fi\nexit 0\n",
		},
		{
			name:   "writes no stdout",
			script: "#!/bin/sh\nexit 0\n",
		},
		{
			name:   "writes only whitespace",
			script: "#!/bin/sh\nif [ \"$1\" = \"--help-summary\" ]; then echo '   '; fi\nexit 0\n",
		},
		{
			name: "exceeds the 1 KB cap",
			script: "#!/bin/sh\nif [ \"$1\" = \"--help-summary\" ]; then " +
				"printf '%1200s' '' | tr ' ' 'x'; echo; fi\nexit 0\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bundle := stageContractBundle(t, "demo", tc.script, "populated")

			dataDir, _, err := installContractBundle(t, "demo", bundle)

			testutil.AssertErrCode(t, err, errcode.PluginHelpSummaryFailed)
			assertNotPromoted(t, dataDir, "demo")
		})
	}
}

// TC-PLG-022 — a summary spanning several lines is reduced to its
// first line, trimmed. The host stores one line; a plugin that prints
// a paragraph gets its headline rather than a rejection.
func TestInstall_TCPLG022_multiline_summary_truncated_to_first_line(t *testing.T) {
	script := "#!/bin/sh\nif [ \"$1\" = \"--help-summary\" ]; then " +
		"printf '  Headline.  \\nbody line\\n'; fi\nexit 0\n"
	bundle := stageContractBundle(t, "demo", script, "populated")

	_, entry, err := installContractBundle(t, "demo", bundle)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Description != "Headline." {
		t.Errorf("entry.Description = %q, want %q", entry.Description, "Headline.")
	}
	if strings.Contains(entry.Description, "body line") {
		t.Error("only the first line may be stored")
	}
}
