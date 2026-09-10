package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/plugins"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

const acmeSource = "github.com/acme/tai-plugin-acme"

// TC-PLG-033 — a third-party plugin will run arbitrary code on the
// machine, so nothing is fetched until the user has said yes. Outside
// a terminal there is nobody to ask.
func TestPluginsInstall_TCPLG033_thirdparty_needs_confirmation(t *testing.T) {
	dataDir := pluginsEnv(t)
	plugins.FetcherForTesting(t, &bundleFetcher{root: stageBundle(t, "acme"), version: "v1.0.0"})

	r := runRoot(t, "plugins", "install", "acme", "--source", acmeSource)
	if r.err == nil {
		t.Fatal("expected the install to be refused")
	}
	assertCode(t, r.err, errcode.PluginThirdpartyUnconfirmed)
	if strings.Contains(r.stderr, "[y/N]") {
		t.Errorf("no prompt may be shown when nobody can answer it, got %q", r.stderr)
	}
	if !strings.Contains(r.stderr, "--yes") {
		t.Errorf("the error must name the flag that unblocks it, got %q", r.stderr)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "plugins", "acme")); !os.IsNotExist(err) {
		t.Errorf("nothing may be written before consent: %v", err)
	}
}

// TC-PLG-034 — `--yes` is the non-interactive consent, and it is
// enough on its own.
func TestPluginsInstall_TCPLG034_yes_flag_installs_thirdparty(t *testing.T) {
	dataDir := pluginsEnv(t)
	plugins.FetcherForTesting(t, &bundleFetcher{root: stageBundle(t, "acme"), version: "v1.0.0"})

	r := runRoot(t, "plugins", "install", "acme", "--source", acmeSource, "--yes")
	if r.err != nil {
		t.Fatalf("unexpected error: %v (stderr %q)", r.err, r.stderr)
	}
	if strings.Contains(r.stderr, "[y/N]") {
		t.Errorf("--yes must skip the prompt entirely, got %q", r.stderr)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "plugins", "acme")); err != nil {
		t.Errorf("the plugin must be installed after consent: %v", err)
	}
}

// TC-PLG-035 — a plugin from the built-in registry is tai's own code
// by another name. Gating it would train the user to type y without
// reading.
func TestPluginsInstall_TCPLG035_firstparty_never_prompts(t *testing.T) {
	pluginsEnv(t)
	plugins.FetcherForTesting(t, &bundleFetcher{root: stageBundle(t, "triage"), version: "v1.0.0"})

	r := runRoot(t, "plugins", "install", "triage")
	if r.err != nil {
		t.Fatalf("unexpected error: %v (stderr %q)", r.err, r.stderr)
	}
	if strings.Contains(r.stderr, "third-party") {
		t.Errorf("a built-in plugin must not be called third-party, got %q", r.stderr)
	}
}

// TC-PLG-036 — update re-fetches from the same third-party source, so
// it needs the same consent as the original install.
func TestPluginsUpdate_TCPLG036_thirdparty_needs_confirmation(t *testing.T) {
	pluginsEnv(t)
	bundle := stageBundle(t, "acme")
	plugins.FetcherForTesting(t, &bundleFetcher{root: bundle, version: "v1.0.0"})
	if r := runRoot(t, "plugins", "install", "acme", "--source", acmeSource, "--yes"); r.err != nil {
		t.Fatalf("setup install: %v (stderr %q)", r.err, r.stderr)
	}

	plugins.FetcherForTesting(t, &bundleFetcher{root: bundle, version: "v1.1.0"})
	refused := runRoot(t, "plugins", "update", "acme")
	if refused.err == nil {
		t.Fatal("expected the update to be refused")
	}
	assertCode(t, refused.err, errcode.PluginThirdpartyUnconfirmed)

	accepted := runRoot(t, "plugins", "update", "acme", "--yes")
	if accepted.err != nil {
		t.Fatalf("unexpected error: %v (stderr %q)", accepted.err, accepted.stderr)
	}
	if !strings.Contains(accepted.stdout, "v1.1.0") {
		t.Errorf("the update must land after consent, got %q", accepted.stdout)
	}
}
