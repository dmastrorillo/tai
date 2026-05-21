// Command tai is the entry point binary. It is intentionally thin: it
// constructs the root urfave/cli command, runs it through internal/cliexec
// (which adds panic recovery), and translates any error into the
// foundation's stderr template plus the matching OS exit code.
//
// main is the single place that calls os.Exit. Subcommands and library
// code under internal/ MUST return errors; main maps them via
// errcode.Code.ExitCode().
package main

import (
	"context"
	"os"

	"github.com/dmastrorillo/tai/internal/cliexec"
	"github.com/dmastrorillo/tai/internal/cliout"
	"github.com/dmastrorillo/tai/internal/cmd"
	"github.com/dmastrorillo/tai/internal/errcode"
	"github.com/dmastrorillo/tai/internal/exitcode"
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
