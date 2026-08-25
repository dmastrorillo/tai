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
	"io"

	"github.com/dmastrorillo/tai/pkg/cliout"
	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/pkg/exitcode"
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

// Exit maps the error returned by Run to the process exit code,
// rendering the foundation error template to w where one is owed.
// It is the single implementation of the post-Run translation every
// binary's main performs — `os.Exit(cliexec.Exit(os.Stderr, err))` —
// so the branch ladder cannot drift between entry points. The rules:
//
//   - nil → exitcode.Success, nothing written.
//   - *errcode.Error → template via cliout.WriteError, the code's
//     mapped exit.
//   - other cli.ExitCoder (a plugin subprocess exit) → the child's
//     exit code, no template: the child already wrote its own stderr
//     output and rendering another would bury it.
//   - anything else → template, exitcode.Internal, so the OS exit
//     and the rendered footer agree.
//
// Exit only computes the code; calling os.Exit stays in main, the
// single place per binary allowed to terminate the process.
func Exit(w io.Writer, err error) int {
	if err == nil {
		return exitcode.Success
	}
	if e, ok := errcode.As(err); ok {
		cliout.WriteError(w, err)
		return e.Code.ExitCode()
	}
	if ec, ok := err.(cli.ExitCoder); ok {
		return ec.ExitCode()
	}
	cliout.WriteError(w, err)
	return exitcode.Internal
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
