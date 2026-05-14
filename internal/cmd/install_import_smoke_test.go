package cmd_test

import (
	"path/filepath"
	"testing"

	"github.com/danielmastrorillo/tai/internal/cmd"
	"github.com/danielmastrorillo/tai/internal/cmdframework"
	"github.com/danielmastrorillo/tai/internal/cmdtest"
	"github.com/danielmastrorillo/tai/internal/installer"
)

// TestInstall_TCINST043_import_command_bundled exercises TC-INST-043:
// the bundled `import.md` flows through `tai install` cleanly — the
// file lands in the target directory and the classifier reports it as
// `up-to-date` on the next pass. The production-bundle counterpart of
// the fake-bundle install tests in install_test.go.
func TestInstall_TCINST043_import_command_bundled(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "commands")

	r := cmdtest.Run(t, cmd.NewRoot(), "install", "--commands-dir", dir)
	cmdtest.AssertNoError(t, r)
	cmdtest.AssertExitCode(t, r, 0)
	cmdtest.AssertStdoutContains(t, r, "import")

	ledger, err := cmdframework.LedgerStrict("import")
	if err != nil {
		t.Fatalf("LedgerStrict: %v", err)
	}
	class, err := installer.Classify(filepath.Join(dir, "import.md"), ledger)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if class != installer.ClassUpToDate {
		t.Fatalf("expected up-to-date after install, got %s", class)
	}
}
