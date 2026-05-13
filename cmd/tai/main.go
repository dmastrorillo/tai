// Command tai is the entry point binary. It is intentionally thin: it
// constructs the root urfave/cli command and runs it. All business logic
// lives under internal/.
//
// Exit-code mapping is performed here — main is the only place that
// translates errors into exit codes (per the foundation contract). Until
// add-tai-foundation lands (see openspec/changes/add-tai-foundation/
// proposal.md), any non-nil error from cli.Run produces exit code 1 and
// the error's string form is written to stderr.
//
// urfave/cli's default "Incorrect Usage: …" printer is suppressed in
// NewRoot via OnUsageError so this file is the sole source of stderr
// content on error.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/danielmastrorillo/tai/internal/cmd"
)

func main() {
	if err := cmd.NewRoot().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
