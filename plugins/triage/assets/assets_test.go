// Package assets_test owns the divergence guard between the
// transitional dual bundle trees. Phase 6 of pivot-to-ai-as-code
// COPIED the triage plugin's bundled slash-command markdowns into
// `plugins/triage/assets/commands/` so the new plugin-host install
// path could pick them up. The original tree at
// `plugins/triage/internal/cmdframework/commands/` still serves the
// pre-pivot in-process `tai triage install` path until that flow
// retires (tracked for Phase 7+).
//
// While both trees ship the same payload, this test enforces
// byte-identical parity for every `<verb>.md` file. A developer
// editing one copy and not the other surfaces a hard CI failure
// rather than silently diverging install paths.
package assets_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAssetsMirrorCmdframework byte-compares every `<verb>.md` file
// under plugins/triage/assets/commands/ against its counterpart under
// plugins/triage/internal/cmdframework/commands/. Files missing on
// either side fail the test.
//
// The .ledger.json files live only in cmdframework/commands/ — they
// are pre-pivot bookkeeping irrelevant to the plugin-host flow — and
// are deliberately NOT mirrored.
func TestAssetsMirrorCmdframework(t *testing.T) {
	assetsDir := "commands"
	cmdframeworkDir := filepath.Join("..", "internal", "cmdframework", "commands")

	assetEntries, err := os.ReadDir(assetsDir)
	if err != nil {
		t.Fatalf("read assets dir: %v", err)
	}
	cmdEntries, err := os.ReadDir(cmdframeworkDir)
	if err != nil {
		t.Fatalf("read cmdframework dir: %v", err)
	}

	assetMD := mdFiles(assetEntries)
	cmdMD := mdFiles(cmdEntries)

	for name := range assetMD {
		if !cmdMD[name] {
			t.Errorf("%s exists in assets/commands/ but not in cmdframework/commands/", name)
		}
	}
	for name := range cmdMD {
		if !assetMD[name] {
			t.Errorf("%s exists in cmdframework/commands/ but not in assets/commands/", name)
		}
	}

	for name := range assetMD {
		if !cmdMD[name] {
			continue
		}
		assetBody, err := os.ReadFile(filepath.Join(assetsDir, name))
		if err != nil {
			t.Errorf("read %s in assets: %v", name, err)
			continue
		}
		cmdBody, err := os.ReadFile(filepath.Join(cmdframeworkDir, name))
		if err != nil {
			t.Errorf("read %s in cmdframework: %v", name, err)
			continue
		}
		if !bytes.Equal(assetBody, cmdBody) {
			t.Errorf("%s bytes differ between assets/ and cmdframework/ — both trees must ship identical payload until the cmdframework duplicate retires (Phase 7+)",
				name)
		}
	}
}

// mdFiles returns the set of `.md` file basenames in entries,
// excluding README.md (which is documentation, not a bundled
// command).
func mdFiles(entries []os.DirEntry) map[string]bool {
	out := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "README.md" {
			continue
		}
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		out[name] = true
	}
	return out
}
