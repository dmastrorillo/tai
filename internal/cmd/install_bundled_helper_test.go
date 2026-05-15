package cmd_test

import (
	"path/filepath"
	"testing"

	"github.com/danielmastrorillo/tai/internal/cmd"
	"github.com/danielmastrorillo/tai/internal/cmdframework"
	"github.com/danielmastrorillo/tai/internal/cmdtest"
	"github.com/danielmastrorillo/tai/internal/installer"
)

// runBundledInstallSmoke is the shared core of the per-verb install
// smoke tests (TC-INST-043, TC-INST-044, …). For a given verb it:
//
//  1. Creates a fresh `<tmp>/commands` target.
//  2. Asserts the target is `ClassMissing` BEFORE install (the Given
//     clause every install smoke TC names — clean target, no prior
//     file).
//  3. Runs `tai install --commands-dir <dir>` and asserts a clean exit
//     plus that the verb is mentioned in stdout.
//  4. Reads the production ledger via cmdframework.LedgerStrict.
//  5. Asserts post-install Classify is `ClassUpToDate`.
//
// Returns the installed target path and the ledger so callers can layer
// additional assertions (e.g. mutate the file and re-classify).
func runBundledInstallSmoke(t *testing.T, verb string) (target string, ledger []string) {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "commands")
	target = filepath.Join(dir, verb+".md")

	ledger, err := cmdframework.LedgerStrict(verb)
	if err != nil {
		t.Fatalf("LedgerStrict(%q): %v", verb, err)
	}

	preClass, err := installer.Classify(target, ledger)
	if err != nil {
		t.Fatalf("Classify before install: %v", err)
	}
	if preClass != installer.ClassMissing {
		t.Fatalf("expected missing on clean target, got %s", preClass)
	}

	r := cmdtest.Run(t, cmd.NewRoot(), "install", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)
	cmdtest.AssertStdoutContains(t, r, verb)

	postClass, err := installer.Classify(target, ledger)
	if err != nil {
		t.Fatalf("Classify after install: %v", err)
	}
	if postClass != installer.ClassUpToDate {
		t.Fatalf("expected up-to-date after install, got %s", postClass)
	}
	return target, ledger
}
