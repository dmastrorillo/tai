// Package exitcode is the single home for tai's OS exit codes.
//
// Five buckets, narrowed from sysexits.h tradition (see add-tai-foundation
// design.md §D4). Every code in pkg/errcode maps to exactly one of these.
// The CLI MUST NOT use exit codes outside this set without first
// extending it through an OpenSpec proposal that updates the foundation
// spec's "Exit-code conventions" requirement.
package exitcode

const (
	// Success: command completed as intended.
	Success = 0

	// Usage: unknown subcommand, malformed flag, conflicting options.
	Usage = 1

	// Precondition: required context missing (e.g. repo, PR identifier).
	Precondition = 2

	// Data: invalid input payload, schema mismatch, state conflict.
	Data = 3

	// Internal: recovered panic or unanticipated I/O failure.
	Internal = 70
)
