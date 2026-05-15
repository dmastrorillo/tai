package cmd_test

import (
	"os"
	"testing"

	"github.com/danielmastrorillo/tai/internal/installer"
)

// TestInstall_TCINST044_triage_command_bundled exercises TC-INST-044:
// the bundled `triage.md` flows through `tai install` cleanly — the
// file starts missing on a clean target, the install lands it, the
// classifier reports `up-to-date` immediately afterwards, and a
// single-byte edit flips it to `user-modified`. The triage counterpart
// to TC-INST-043; the byte-append step additionally exercises the
// transition that drives the on-rerun prompt.
func TestInstall_TCINST044_triage_command_bundled(t *testing.T) {
	target, ledger := runBundledInstallSmoke(t, "triage")

	targetFile, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open target for append: %v", err)
	}
	if _, err := targetFile.WriteString("x"); err != nil {
		t.Fatalf("append byte: %v", err)
	}
	if err := targetFile.Close(); err != nil {
		t.Fatalf("close target: %v", err)
	}

	class, err := installer.Classify(target, ledger)
	if err != nil {
		t.Fatalf("Classify after edit: %v", err)
	}
	if class != installer.ClassUserModified {
		t.Fatalf("expected user-modified after hand-edit, got %s", class)
	}
}
