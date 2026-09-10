package cmd_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/plugins"
	syncpkg "github.com/dmastrorillo/tai/core/internal/sync"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// stubAutoInstall records every auto-install and pretends it worked,
// so a sync test needs neither a network nor a real release.
func stubAutoInstall(t *testing.T) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	syncpkg.AutoInstallForTesting(t, func(_ context.Context, name, dataDir string, _ *config.File, _ plugins.InstallOptions) (*plugins.Entry, error) {
		calls.Add(1)
		state, _ := plugins.LoadState(dataDir)
		entry := plugins.Entry{
			Name:        name,
			Source:      plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
			Version:     "v0.0.0-test",
			InstalledAt: time.Now().UTC(),
		}
		state.Upsert(entry)
		_ = plugins.SaveState(dataDir, state)
		return &entry, nil
	})
	return &calls
}

func trustPath(dataDir string) string {
	return filepath.Join(dataDir, "state", "trust.json")
}

const thirdPartyPluginsYML = `plugins:
  - name: triage
  - name: acme
    source: github.com/acme/tai-plugin-acme
`

const builtinOnlyPluginsYML = `plugins:
  - name: triage
`

// TC-PLG-038 — a plugins.yml naming only built-in plugins asks
// nothing and remembers nothing.
func TestSync_TCPLG038_builtin_only_yml_never_prompts(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{
		"skills/foo.md": "x",
		"plugins.yml":   builtinOnlyPluginsYML,
	})
	dataDir, _, _ := syncEnv(t, url)
	calls := stubAutoInstall(t)

	r := runRoot(t, "sync", "-y")
	if r.err != nil {
		t.Fatalf("sync error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if calls.Load() != 1 {
		t.Errorf("expected the built-in plugin to install, got %d calls", calls.Load())
	}
	if strings.Contains(r.stderr, "third-party") {
		t.Errorf("no third-party notice may appear, got %q", r.stderr)
	}
	if _, err := os.Stat(trustPath(dataDir)); !os.IsNotExist(err) {
		t.Errorf("no trust record may be written when nothing is third-party: %v", err)
	}
}

// TC-PLG-039 — a third-party entry stops an unattended sync before it
// installs anything or touches a target.
func TestSync_TCPLG039_thirdparty_yml_aborts_unattended(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{
		"skills/foo.md": "x",
		"plugins.yml":   thirdPartyPluginsYML,
	})
	dataDir, target, _ := syncEnv(t, url)
	calls := stubAutoInstall(t)

	r := runRoot(t, "sync", "-y")
	if r.err == nil {
		t.Fatal("expected the sync to be refused")
	}
	assertCode(t, r.err, errcode.PluginThirdpartyUnconfirmed)
	if !strings.Contains(r.stderr, "--trust-third-party") {
		t.Errorf("the error must name the flag that unblocks it, got %q", r.stderr)
	}
	if calls.Load() != 0 {
		t.Errorf("no plugin may be installed before consent, got %d calls", calls.Load())
	}
	if _, err := os.Stat(filepath.Join(target, "skills", "foo.md")); !os.IsNotExist(err) {
		t.Error("the asset-sync phase must not run when the plugin phase aborts")
	}
	if _, err := os.Stat(trustPath(dataDir)); !os.IsNotExist(err) {
		t.Error("a refused sync must remember nothing")
	}
}

// TC-PLG-040 — `--trust-third-party` is the unattended consent, and
// what the user agreed to is recorded against the repo it came from.
func TestSync_TCPLG040_flag_confirms_and_records_the_hash(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{
		"skills/foo.md": "x",
		"plugins.yml":   thirdPartyPluginsYML,
	})
	dataDir, _, _ := syncEnv(t, url)
	calls := stubAutoInstall(t)

	r := runRoot(t, "sync", "-y", "--trust-third-party")
	if r.err != nil {
		t.Fatalf("sync error: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if calls.Load() != 2 {
		t.Errorf("expected both plugins to install, got %d calls", calls.Load())
	}

	store, err := plugins.LoadTrust(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := store.Get(url)
	if !ok {
		t.Fatalf("consent must be recorded against the repo url %q; store: %+v", url, store)
	}
	sum := sha256.Sum256([]byte(thirdPartyPluginsYML))
	if want := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("recorded hash: want %q, got %q", want, got)
	}
}

// TC-PLG-041 — an unchanged plugins.yml is already agreed to, so a
// later sync needs neither the flag nor a prompt.
func TestSync_TCPLG041_recorded_consent_is_reused(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{
		"skills/foo.md": "x",
		"plugins.yml":   thirdPartyPluginsYML,
	})
	dataDir, _, _ := syncEnv(t, url)
	stubAutoInstall(t)

	if r := runRoot(t, "sync", "-y", "--trust-third-party"); r.err != nil {
		t.Fatalf("setup sync: %v\nstderr:\n%s", r.err, r.stderr)
	}
	// Forget the installs so the second sync has work to do again.
	if err := plugins.SaveState(dataDir, &plugins.State{}); err != nil {
		t.Fatal(err)
	}
	calls := stubAutoInstall(t)

	r := runRoot(t, "sync", "-y")
	if r.err != nil {
		t.Fatalf("a recorded consent must carry over: %v\nstderr:\n%s", r.err, r.stderr)
	}
	if calls.Load() != 2 {
		t.Errorf("expected both plugins to install, got %d calls", calls.Load())
	}
	if strings.Contains(r.stderr, "[y/N]") {
		t.Errorf("no prompt may appear for an already-agreed file, got %q", r.stderr)
	}
}

// TC-PLG-042 — consent covers the exact file that was agreed to. A
// new third-party entry is a new question.
func TestSync_TCPLG042_changed_yml_needs_fresh_consent(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{
		"skills/foo.md": "x",
		"plugins.yml":   thirdPartyPluginsYML,
	})
	dataDir, _, _ := syncEnv(t, url)
	stubAutoInstall(t)

	if r := runRoot(t, "sync", "-y", "--trust-third-party"); r.err != nil {
		t.Fatalf("setup sync: %v\nstderr:\n%s", r.err, r.stderr)
	}

	seedRemote(t, url, map[string]string{
		"skills/foo.md": "x",
		"plugins.yml": thirdPartyPluginsYML + `  - name: other
    source: github.com/somebody/tai-plugin-other
`,
	})
	if err := plugins.SaveState(dataDir, &plugins.State{}); err != nil {
		t.Fatal(err)
	}

	r := runRoot(t, "sync", "-y")
	if r.err == nil {
		t.Fatal("expected the changed file to be refused")
	}
	assertCode(t, r.err, errcode.PluginThirdpartyUnconfirmed)
}

// TC-PLG-044 — consenting to a source repo's third-party plugins has
// to carry through to the installs that consent authorises.
//
// This test deliberately does NOT stub the installer via
// AutoInstallForTesting: the bug it guards lives in what the loop
// hands to the real plugins.Install, so a stubbed installer cannot
// see it. Only the network is faked, via the default fetcher.
func TestSync_TCPLG044_consent_reaches_the_real_installer(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{
		"skills/foo.md": "x",
		"plugins.yml": `plugins:
  - name: acme
    source: github.com/acme/tai-plugin-acme
`,
	})
	dataDir, _, _ := syncEnv(t, url)
	plugins.FetcherForTesting(t, &bundleFetcher{root: stageBundle(t, "acme"), version: "v1.0.0"})

	r := runRoot(t, "sync", "-y", "--trust-third-party")
	if r.err != nil {
		t.Fatalf("consent given, so the install must proceed: %v\nstderr:\n%s", r.err, r.stderr)
	}

	state, err := plugins.LoadState(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, idx := state.Find("acme"); idx < 0 {
		t.Errorf("the consented plugin must be installed; state: %+v", state)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "plugins", "acme", "acme")); err != nil {
		t.Errorf("the plugin binary must be on disk: %v", err)
	}
}

// Consent is per file, so a sync whose consent was never given must
// still refuse — the fix for TC-PLG-044 must not become a blanket
// AssumeYes that defeats the gate.
func TestSync_TCPLG044_no_consent_still_refuses_the_real_installer(t *testing.T) {
	url := bareRemote(t)
	seedRemote(t, url, map[string]string{
		"skills/foo.md": "x",
		"plugins.yml": `plugins:
  - name: acme
    source: github.com/acme/tai-plugin-acme
`,
	})
	dataDir, _, _ := syncEnv(t, url)
	plugins.FetcherForTesting(t, &bundleFetcher{root: stageBundle(t, "acme"), version: "v1.0.0"})

	r := runRoot(t, "sync", "-y")
	if r.err == nil {
		t.Fatal("no consent was given, so the sync must be refused")
	}
	assertCode(t, r.err, errcode.PluginThirdpartyUnconfirmed)
	if _, err := os.Stat(filepath.Join(dataDir, "plugins", "acme")); !os.IsNotExist(err) {
		t.Errorf("nothing may be installed without consent: %v", err)
	}
}
