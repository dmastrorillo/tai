// This file holds the structured *Error type and its constructors.
// The Code constants and their exit-code mapping live in errcode.go;
// the two files are siblings of the same package.

package errcode

import (
	"errors"
	"fmt"
)

// Error is tai's structured error. Every error that crosses a package
// boundary toward the CLI surface SHOULD be (or wrap) an *Error so the
// CLI's error printer can render the contract-shaped footer.
//
// Implements:
//
//   - error (standard interface)
//   - errors.Unwrap (preserves the cause chain)
//   - the urfave/cli ExitCoder interface via ExitCode(), so urfave/cli
//     uses the right exit code when an action returns an *Error.
type Error struct {
	// Code identifies the failure class. Drives the exit code and the
	// footer label.
	Code Code

	// Msg is the one-line human-readable summary written after "Error: ".
	// It should be a complete sentence in plain language.
	Msg string

	// Help is the ordered list of remediation bullets written under the
	// "What to do:" block. Leave empty when there is no meaningful
	// remediation (e.g. recovered panics); the printer will omit the
	// block.
	Help []string

	// Cause is the underlying error, if any.
	Cause error
}

// New returns a fresh *Error with the given code and message.
func New(code Code, msg string) *Error {
	return &Error{Code: code, Msg: msg}
}

// Newf is New with fmt.Sprintf semantics on the message.
func Newf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Wrap returns a new *Error with the given code and message wrapping
// cause. cause is preserved for errors.Unwrap.
func Wrap(code Code, cause error, msg string) *Error {
	return &Error{Code: code, Msg: msg, Cause: cause}
}

// Wrapf is Wrap with fmt.Sprintf semantics on the message.
func Wrapf(code Code, cause error, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...), Cause: cause}
}

// WithHelp appends remediation bullets and returns e for chaining.
func (e *Error) WithHelp(bullets ...string) *Error {
	e.Help = append(e.Help, bullets...)
	return e
}

// Error returns the one-line summary. The full error template (header,
// "What to do" block, footer) is the responsibility of pkg/cliout.
func (e *Error) Error() string { return e.Msg }

// Unwrap returns the underlying cause, supporting errors.Is and
// errors.As against the chain.
func (e *Error) Unwrap() error { return e.Cause }

// ExitCode satisfies the urfave/cli ExitCoder interface so urfave/cli's
// own machinery maps a returned *Error to the correct exit code.
func (e *Error) ExitCode() int { return e.Code.ExitCode() }

// As returns the *Error in err's chain, if any.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}
