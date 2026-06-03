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
// The error-rendering rules mirror core/cmd/tai/main.go:
//
//   - *errcode.Error: render via cliout.WriteError, exit with the
//     code's mapped exit.
//   - cli.ExitCoder that ISN'T *errcode.Error: exit with the
//     reported code, no template (the child plugin — none today, but
//     reserved — has already written its own output).
//   - Anything else: surface as INTERNAL_ERROR.
//
// Like the core binary, this is the single place in the triage
// plugin that calls os.Exit.
package main

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/dmastrorillo/tai/pkg/cliexec"
	"github.com/dmastrorillo/tai/pkg/cliout"
	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/pkg/exitcode"
	"github.com/dmastrorillo/tai/pkg/taiplugin"
	"github.com/dmastrorillo/tai/plugins/triage/internal/cmd"
)

func main() {
	ctx := context.Background()

	// taiplugin.Load() parses the wire-contract env vars. The values
	// are not used directly by this main — downstream packages
	// (storage, installer) read TAI_DATA_DIR via pkg/datadir which
	// also honours the contract — but we call Load() up front so a
	// malformed TAI_TARGETS surfaces with the spec'd INTERNAL_ERROR
	// rather than as a cryptic crash later. Errors are non-fatal at
	// this layer: an empty or missing contract means the plugin is
	// being invoked outside `tai plugins triage`-style dispatch
	// (e.g. directly by a developer), and downstream commands either
	// work standalone or fail with their own structured errors.
	if _, err := taiplugin.Load(); err != nil {
		cliout.WriteError(os.Stderr, err)
		if e, ok := errcode.As(err); ok {
			os.Exit(e.Code.ExitCode())
		}
		os.Exit(exitcode.Internal)
	}

	err := cliexec.Run(ctx, cmd.NewRoot(), os.Args)
	if err == nil {
		os.Exit(exitcode.Success)
	}

	if e, ok := errcode.As(err); ok {
		cliout.WriteError(os.Stderr, err)
		os.Exit(e.Code.ExitCode())
	}
	if ec, ok := err.(cli.ExitCoder); ok {
		os.Exit(ec.ExitCode())
	}

	cliout.WriteError(os.Stderr, err)
	os.Exit(exitcode.Internal)
}
