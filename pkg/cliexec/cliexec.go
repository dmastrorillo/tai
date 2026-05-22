// Package cliexec runs a urfave/cli command with panic recovery.
//
// It is the single seam used by every binary entry point
// (core/cmd/tai/main.go, plugins/<name>/cmd/<binary>/main.go) and by every
// cmdtest harness (plugins/<name>/internal/cmdtest/) so panic-recovery
// behaviour cannot drift between production and tests. A panic during
// command execution surfaces as an errcode.InternalError, written to
// stderr by the caller via pkg/cliout.
package cliexec

import (
	"context"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/urfave/cli/v3"
)

// Run executes cmd with args, recovering any panic into a structured
// *errcode.Error with code INTERNAL_ERROR. The recovered panic's value
// is included in the error message for diagnostics; when the value is
// itself an error, that error is wrapped as the cause so callers can
// recover the chain via errors.Unwrap / errors.Is / errors.As.
//
// Callers MUST NOT recover panics themselves around Run; doing so would
// hide the message.
func Run(ctx context.Context, cmd *cli.Command, args []string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = panicToError(r)
		}
	}()
	return cmd.Run(ctx, args)
}

// panicToError converts a recovered value into a structured *errcode.Error.
// When the panic value implements error, its chain is preserved as
// errcode.Error.Cause.
func panicToError(r any) error {
	if rerr, ok := r.(error); ok {
		return errcode.Wrapf(errcode.InternalError, rerr, "panic: %v", rerr)
	}
	return errcode.Newf(errcode.InternalError, "panic: %v", r)
}
