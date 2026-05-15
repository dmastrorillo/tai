package cmd_test

import (
	"testing"
)

// TestInstall_TCINST043_import_command_bundled exercises TC-INST-043:
// the bundled `import.md` flows through `tai install` cleanly — the
// file starts missing, lands in the target directory, and the
// classifier reports it as `up-to-date` on the next pass. The
// production-bundle counterpart of the fake-bundle install tests in
// install_test.go.
func TestInstall_TCINST043_import_command_bundled(t *testing.T) {
	runBundledInstallSmoke(t, "import")
}
