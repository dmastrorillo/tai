// Command tai is the core CLI entry point.
//
// In Phase 0 of the AI-as-code pivot this binary is a thin scaffold:
// the root *cli.Command (assembled by core/internal/cmd.NewRoot) carries
// --version, --help, and the UnknownSubcommand routing for any other
// invocation. Phase 1 grafts the new top-level verbs (tai config,
// tai sync, tai repo init, ...) onto that same root. The triage CLI
// surface that used to live here moves to plugins/triage/cmd/triage in
// a later phase; today it is exercised in-process by its own cmdtest
// harness.
//
// main is the single place that calls os.Exit. Subcommands and library
// code under core/internal/ and plugins/<name>/internal/ MUST return
// errors; main maps them via errcode.Code.ExitCode().
package main

import (
	"context"
	"os"

	"github.com/dmastrorillo/tai/core/internal/cmd"
	"github.com/dmastrorillo/tai/pkg/cliexec"
	"github.com/dmastrorillo/tai/pkg/cliout"
	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/pkg/exitcode"
)

func main() {
	err := cliexec.Run(context.Background(), cmd.NewRoot(), os.Args)
	if err == nil {
		os.Exit(exitcode.Success)
	}

	cliout.WriteError(os.Stderr, err)

	if e, ok := errcode.As(err); ok {
		os.Exit(e.Code.ExitCode())
	}
	// Non-errcode errors are unexpected; cliout has surfaced them as
	// INTERNAL_ERROR. Match the exit code so the footer and the OS
	// exit code agree.
	os.Exit(exitcode.Internal)
}
