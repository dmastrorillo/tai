package cmd_test

import (
	"path/filepath"
	"testing"

	"github.com/danielmastrorillo/tai/internal/cmd"
	"github.com/danielmastrorillo/tai/internal/cmdframework"
	"github.com/danielmastrorillo/tai/internal/cmdtest"
	"github.com/danielmastrorillo/tai/internal/installer"
)

// TestInstall_TCINST046_all_bundled_verbs_up_to_date exercises
// TC-INST-046: a single `tai install` run must land EVERY bundled
// verb such that the classifier reports `up-to-date` for each of
// them. The per-verb smoke tests (TC-INST-043/044/045) each run an
// install but classify only their own verb. A regression that wrote
// some verbs correctly while breaking others would slip past those
// tests; this one catches it by iterating `cmdframework.Verbs()`
// after a single install run.
func TestInstall_TCINST046_all_bundled_verbs_up_to_date(t *testing.T) {
	verbs := cmdframework.Verbs()
	if len(verbs) == 0 {
		t.Skip("no bundled verbs in this build — nothing to verify")
	}

	dir := filepath.Join(t.TempDir(), "commands")

	r := cmdtest.Run(t, cmd.NewRoot(), "install", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)

	for _, verb := range verbs {
		ledger, err := cmdframework.LedgerStrict(verb)
		if err != nil {
			t.Errorf("LedgerStrict(%q): %v", verb, err)
			continue
		}
		target := filepath.Join(dir, verb+".md")
		class, err := installer.Classify(target, ledger)
		if err != nil {
			t.Errorf("Classify(%q): %v", verb, err)
			continue
		}
		if class != installer.ClassUpToDate {
			t.Errorf("verb %q: expected up-to-date after install, got %s", verb, class)
		}
	}
}
