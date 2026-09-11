package cmd_test

import (
	"strings"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/core/internal/plugins"
)

// describeFakePlugin records a description on an already-installed
// fake plugin, standing in for the one the host captures from
// `<plugin> --help-summary` at install time.
func describeFakePlugin(t *testing.T, dataDir, name, description string) {
	t.Helper()
	state, err := plugins.LoadState(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	state.Upsert(plugins.Entry{
		Name:        name,
		Source:      plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
		Version:     "v0.0.0-test",
		InstalledAt: time.Now().UTC(),
		Description: description,
	})
	if err := plugins.SaveState(dataDir, state); err != nil {
		t.Fatal(err)
	}
}

// TC-PLG-026 — `tai --help` lists installed plugins under their own
// heading, with the description captured at install time.
func TestHelp_TCPLG026_lists_installed_plugins(t *testing.T) {
	dataDir := pluginsEnv(t)
	installFakePlugin(t, dataDir, "triage", "exit 0\n")
	const desc = "Walk through pending PR review comments interactively."
	describeFakePlugin(t, dataDir, "triage", desc)

	r := runRoot(t, "--help")
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if !strings.Contains(r.stdout, "PLUGINS:") {
		t.Errorf("help must carry a PLUGINS: heading, got:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, desc) {
		t.Errorf("help must show the stored description, got:\n%s", r.stdout)
	}

	// The heading is what separates a plugin from a built-in verb, so
	// the plugin must appear under it rather than loose among them.
	_, after, found := strings.Cut(r.stdout, "PLUGINS:")
	if !found || !strings.Contains(after, "triage") {
		t.Errorf("triage must be listed under PLUGINS:, got:\n%s", r.stdout)
	}
}

// TC-PLG-027 — with nothing installed the heading is absent entirely,
// rather than printed empty.
func TestHelp_TCPLG027_no_plugins_no_heading(t *testing.T) {
	pluginsEnv(t)

	r := runRoot(t, "--help")
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if strings.Contains(r.stdout, "PLUGINS:") {
		t.Errorf("help must not carry a PLUGINS: heading when none are installed, got:\n%s", r.stdout)
	}
}

// A plugin installed before descriptions were captured has an empty
// one — the field is append-only, so older entries simply lack it.
// It must still be listed, with something in the usage column.
func TestHelp_TCPLG026_plugin_without_description_still_listed(t *testing.T) {
	dataDir := pluginsEnv(t)
	installFakePlugin(t, dataDir, "legacy", "exit 0\n")

	r := runRoot(t, "--help")
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	_, after, found := strings.Cut(r.stdout, "PLUGINS:")
	if !found || !strings.Contains(after, "legacy") {
		t.Errorf("a plugin with no stored description must still be listed, got:\n%s", r.stdout)
	}
}

// TC-PLG-028 — `tai <plugin> help` forwards to the plugin, while
// `tai help <plugin>` is the host's own help verb and must not exec
// the plugin at all.
func TestHelp_TCPLG028_help_routing(t *testing.T) {
	dataDir := pluginsEnv(t)
	// Echoes its argv so the test can see what the plugin received.
	installFakePlugin(t, dataDir, "triage",
		`for a in "$@"; do printf '%s\n' "$a"; done
`)

	forwarded := runRoot(t, "triage", "help")
	if forwarded.err != nil {
		t.Fatalf("unexpected error: %v", forwarded.err)
	}
	if strings.TrimSpace(forwarded.stdout) != "help" {
		t.Errorf("`tai triage help` must reach the plugin with argv[1]=help, got %q", forwarded.stdout)
	}

	hostHelp := runRoot(t, "help", "triage")
	if hostHelp.err != nil {
		t.Fatalf("unexpected error: %v", hostHelp.err)
	}
	if strings.TrimSpace(hostHelp.stdout) == "triage" {
		t.Error("`tai help triage` must render host help, not exec the plugin")
	}
	if !strings.Contains(hostHelp.stdout, "USAGE") {
		t.Errorf("`tai help triage` must render help output, got:\n%s", hostHelp.stdout)
	}
}
