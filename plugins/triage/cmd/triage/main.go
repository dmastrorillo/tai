// Command triage is the Triage AI plugin's subprocess entrypoint.
//
// After Phase 6 of pivot-to-ai-as-code the Triage codebase ships as
// a tai plugin: the `triage` binary lives under
// `<TAI_DATA_DIR>/plugins/triage/` and is invoked by the host as
// `triage <verb> <args>` per the plugin-host spec
// (openspec/changes/pivot-to-ai-as-code/specs/plugin-host/spec.md).
//
// At startup main calls `taiplugin.Load()` to parse the wire-contract
// environment variables (`TAI_DATA_DIR`, `TAI_CLONE_DIR`,
// `TAI_TARGETS`). The values feed downstream packages — most
// importantly `plugins/triage/internal/storage` resolves its SQLite
// path relative to TAI_DATA_DIR, and the install/uninstall verbs
// read TAI_TARGETS to know which target directories receive bundled
// command files.
//
// Error rendering and exit-code mapping go through cliexec.Exit —
// the exact translation core/cmd/tai/main.go uses, shared via
// pkg/cliexec so the rules cannot drift between binaries.
//
// Like the core binary, this is the single place in the triage
// plugin that calls os.Exit.
package main

import (
	"context"
	"os"

	"github.com/dmastrorillo/tai/pkg/cliexec"
	"github.com/dmastrorillo/tai/pkg/taiplugin"
	"github.com/dmastrorillo/tai/plugins/triage/internal/cmd"
)

// helpSummary is what `triage --help-summary` prints and what the
// host stores as the plugin's description in plugins.json — the line
// `tai --help` shows beside the plugin's name. One line, 80 characters
// or fewer, per the wire contract.
const helpSummary = "Walk through pending PR review comments interactively."

func main() {
	ctx := context.Background()

	// The host asks `triage --help-summary` at install and update time
	// and refuses to install a plugin that cannot answer. Handled
	// before the command tree so no urfave/cli output can reach the
	// stdout the host is parsing.
	if handled, err := taiplugin.HelpSummary(os.Stdout, os.Args, helpSummary); handled {
		os.Exit(cliexec.Exit(os.Stderr, err))
	}

	// taiplugin.Load() parses the wire-contract env vars. The values
	// are not used directly by this main — downstream packages
	// (storage) read TAI_DATA_DIR via pkg/datadir which
	// also honours the contract — but we call Load() up front so a
	// malformed TAI_TARGETS surfaces with the spec'd INTERNAL_ERROR
	// rather than as a cryptic crash later. Errors are non-fatal at
	// this layer: an empty or missing contract means the plugin is
	// being invoked outside `tai plugins triage`-style dispatch
	// (e.g. directly by a developer), and downstream commands either
	// work standalone or fail with their own structured errors.
	if _, err := taiplugin.Load(); err != nil {
		os.Exit(cliexec.Exit(os.Stderr, err))
	}

	// cliexec.Exit owns the error → exit-code translation, shared
	// with core/cmd/tai/main.go so the rules can't drift between
	// binary entry points.
	err := cliexec.Run(ctx, cmd.NewRoot(), os.Args)
	os.Exit(cliexec.Exit(os.Stderr, err))
}
