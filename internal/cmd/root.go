// Package cmd assembles the tai CLI's urfave/cli command tree.
//
// Subcommands land here as their OpenSpec changes are applied. The package
// is the single seam every test and the production binary uses to build
// the command — there is no global state, and the function is cheap to
// call repeatedly.
package cmd

import (
	"context"

	"github.com/danielmastrorillo/tai/internal/version"
	"github.com/urfave/cli/v3"
)

// NewRoot returns a freshly-assembled tai root command.
//
// Writer / ErrWriter / Reader default to os.Stdout/Stderr/Stdin on the
// returned value; tests in internal/cmdtest swap them for buffers.
func NewRoot() *cli.Command {
	return &cli.Command{
		Name:    "tai",
		Usage:   "Triage AI — store, walk, and verify code-review comments",
		Version: version.String,

		// Suppress urfave/cli's built-in "Incorrect Usage: …" print so
		// stderr has exactly one source of truth: cmd/tai/main.go (and,
		// after add-tai-foundation lands, the foundation's structured
		// error printer). Without this, usage errors would be printed
		// once by urfave/cli and again by main.go.
		OnUsageError: func(_ context.Context, _ *cli.Command, err error, _ bool) error {
			return err
		},
	}
}
