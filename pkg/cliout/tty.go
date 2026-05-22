package cliout

// TTY-detection helpers for the cliout package. Lives here because
// downstream callers that emit ANSI / CR-driven progress need to gate
// on the same check the error-template writer uses. The authoritative
// package doc lives in cliout.go.

import (
	"io"
	"os"

	"golang.org/x/term"
)

// IsTTY reports whether w writes to a terminal. Callers MUST use this
// before emitting ANSI colour codes or carriage-return-driven progress
// animations to honour the stdout/stderr-discipline contract in
// openspec/changes/pivot-to-ai-as-code/specs/cli-framework/spec.md.
//
// The detection only succeeds for *os.File writers; any other writer
// type (bytes.Buffer in tests, a strings.Builder, a pipe, etc.) is
// treated as non-TTY. This is conservative on purpose — when in doubt,
// emit plain bytes so AI consumers and shell pipelines see exactly
// what the spec promises.
func IsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
