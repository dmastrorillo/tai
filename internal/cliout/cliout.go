// Package cliout renders user-facing CLI output.
//
// Today it owns one function — WriteError — which formats any error into
// the foundation's standard stderr template:
//
//	Error: <one-line summary>
//
//	What to do:
//	  • <remediation step>
//	  • <additional remediation step if applicable>
//
//	[exit <exit_code>: <ERROR_CODE>]
//
// The "What to do" block is omitted when the error has no remediation
// bullets (typically *errcode.Error with empty Help, e.g. recovered
// panics). The footer line is always present and is always the last
// non-empty line written.
//
// Errors that are not *errcode.Error are surfaced as INTERNAL_ERROR with
// the underlying error message as the summary — this catches
// unanticipated failures and routes them through the same contract.
package cliout

import (
	"fmt"
	"io"
	"strings"

	"github.com/danielmastrorillo/tai/internal/errcode"
)

// WriteError writes the standard error template to w. If err is nil,
// WriteError is a no-op.
func WriteError(w io.Writer, err error) {
	if err == nil {
		return
	}

	taiErr, ok := errcode.As(err)
	if !ok {
		taiErr = errcode.New(errcode.InternalError, err.Error())
	}

	// The spec requires "Error:" to be a single summary line. Wrapped
	// errors and recovered panics can contain embedded newlines that
	// would break that contract; collapse them to spaces so the line
	// stays single but no information is lost.
	summary := strings.ReplaceAll(taiErr.Msg, "\n", " ")
	fmt.Fprintf(w, "Error: %s\n", summary)

	if len(taiErr.Help) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "What to do:")
		for _, h := range taiErr.Help {
			fmt.Fprintf(w, "  • %s\n", h)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "[exit %d: %s]\n", taiErr.Code.ExitCode(), taiErr.Code)
}
