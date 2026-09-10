package sync

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/plugins"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

const repoURL = "https://github.com/example/source-repo.git"

var thirdPartyEntries = []pluginsYAMLEntry{
	{Name: "triage"},
	{Name: "acme", Source: "github.com/acme/tai-plugin-acme"},
}

// TC-PLG-043 — in a terminal the user is asked, the question names
// every third-party entry, and only an explicit yes proceeds.
func TestConfirmThirdPartyPlugins_TCPLG043_interactive_yes(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "yes\n", " yes \n"} {
		dataDir := t.TempDir()
		var stderr bytes.Buffer
		opts := Options{Stdin: strings.NewReader(answer), Stderr: &stderr}

		if err := confirmThirdPartyPlugins(thirdPartyEntries, []byte("raw"), repoURL, dataDir, opts, true); err != nil {
			t.Fatalf("answer %q must proceed, got %v", answer, err)
		}
		prompt := stderr.String()
		for _, want := range []string{"acme", "github.com/acme/tai-plugin-acme", "arbitrary code", "[y/N]"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("prompt must contain %q, got %q", want, prompt)
			}
		}
		// A built-in entry in the same file is not what is being
		// agreed to, and listing it would blur what the yes covers.
		if strings.Contains(prompt, "triage") {
			t.Errorf("prompt must list only third-party entries, got %q", prompt)
		}

		store, err := plugins.LoadTrust(dataDir)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := store.Get(repoURL); !ok {
			t.Errorf("answer %q must be recorded against the repo url", answer)
		}
	}
}

// TC-PLG-043 — anything that is not a yes aborts the sync, and
// nothing is remembered.
func TestConfirmThirdPartyPlugins_TCPLG043_interactive_no(t *testing.T) {
	for _, answer := range []string{"n\n", "N\n", "\n", "", "no\n", "ok\n"} {
		dataDir := t.TempDir()
		var stderr bytes.Buffer
		opts := Options{Stdin: strings.NewReader(answer), Stderr: &stderr}

		err := confirmThirdPartyPlugins(thirdPartyEntries, []byte("raw"), repoURL, dataDir, opts, true)
		coded, ok := errcode.As(err)
		if !ok || coded.Code != errcode.PluginThirdpartyUnconfirmed {
			t.Errorf("answer %q: want PLUGIN_THIRDPARTY_UNCONFIRMED, got %v", answer, err)
		}
		store, loadErr := plugins.LoadTrust(dataDir)
		if loadErr != nil {
			t.Fatal(loadErr)
		}
		if _, recorded := store.Get(repoURL); recorded {
			t.Errorf("answer %q must record nothing", answer)
		}
	}
}

// TC-PLG-038 — a file with no third-party entries is not a question,
// so no hash is computed and no consent is stored.
func TestConfirmThirdPartyPlugins_TCPLG038_builtin_only_records_nothing(t *testing.T) {
	dataDir := t.TempDir()
	var stderr bytes.Buffer
	opts := Options{Stdin: strings.NewReader(""), Stderr: &stderr}

	entries := []pluginsYAMLEntry{{Name: "triage"}}
	if err := confirmThirdPartyPlugins(entries, []byte("raw"), repoURL, dataDir, opts, true); err != nil {
		t.Fatalf("a built-in-only file must pass straight through: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("no prompt may be printed, got %q", stderr.String())
	}
	store, err := plugins.LoadTrust(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Trust) != 0 {
		t.Errorf("no consent may be stored, got %+v", store.Trust)
	}
}

// An entry that resolves to nothing is broken, not untrusted. The
// install path surfaces PLUGIN_UNKNOWN, which says something useful;
// a consent prompt would not.
func TestConfirmThirdPartyPlugins_unresolvable_entry_is_not_thirdparty(t *testing.T) {
	dataDir := t.TempDir()
	var stderr bytes.Buffer
	opts := Options{Stdin: strings.NewReader(""), Stderr: &stderr}

	entries := []pluginsYAMLEntry{{Name: "not-a-real-plugin"}}
	if err := confirmThirdPartyPlugins(entries, []byte("raw"), repoURL, dataDir, opts, true); err != nil {
		t.Fatalf("an unresolvable entry must not be gated: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("no prompt may be printed, got %q", stderr.String())
	}
}
