package cmd_test

import (
	"testing"
)

// TestInstall_TCINST045_verify_command_bundled exercises TC-INST-045:
// the bundled `verify.md` flows through `tai install` cleanly via the
// shared `runBundledInstallSmoke` helper — file starts missing on a
// clean target, install lands it, classifier reports `up-to-date`. The
// post-install mutation path is covered exhaustively by TC-INST-044 and
// is not duplicated here.
func TestInstall_TCINST045_verify_command_bundled(t *testing.T) {
	runBundledInstallSmoke(t, "verify")
}
