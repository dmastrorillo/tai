// Package clitest is the in-process test harness for every tai-family
// CLI: the core `tai` binary, first-party plugins like `triage`, and
// third-party plugins built on pkg/taiplugin. It invokes a
// *cli.Command with captured stdin/stdout/stderr — no subprocess, no
// built binary — and translates the result exactly the way a
// production main does: execution through cliexec.Run (shared panic
// recovery) and error rendering + exit-code mapping through
// cliexec.Exit. Because both binaries' mains call those same two
// functions, a harness-driven test can never observe behaviour the
// shipped binary doesn't produce, and a fix to the error contract
// lands in production and every test suite at once.
//
// Each tree layers its own conveniences on top: the core cmd tests
// mirror main.go's pre-foreground update-banner via Options.PreRun;
// the triage plugin's cmdtest package re-exports Run/Assert* and adds
// its Isolate filesystem/env fixture.
//
// Assert* helpers fail with t.Fatal and include the captured streams
// in their messages, so debugging a red test does not require
// re-running with extra logging. The vocabulary is intentionally
// narrow: if you reach for a helper that does not exist, prefer
// adding it here over inlining the check.
package clitest

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/pkg/cliexec"
	"github.com/urfave/cli/v3"
)

// Result captures everything tests need to assert against after
// running the CLI.
type Result struct {
	// Stdout is the bytes written to the command's Writer.
	Stdout string
	// Stderr is the bytes written to the command's ErrWriter,
	// including any error-template footer cliexec.Exit rendered.
	Stderr string
	// ExitCode is the OS exit code the binary would have produced —
	// cliexec.Exit's translation of the run's error, byte-identical
	// to what main does.
	ExitCode int
	// Err is the raw error returned by the run. Tests that care about
	// the user-observable contract should prefer AssertErrorFooter
	// and AssertExitCode; Err is exposed for assertions that need the
	// underlying Go value.
	Err error
}

// Options parameterises RunWith for the callers the plain Run /
// RunWithStdin shapes don't cover.
type Options struct {
	// Stdin is piped as the command's standard input.
	Stdin string
	// PreRun, when set, runs after the streams are wired but before
	// the command, receiving the captured stderr. Per-binary harness
	// wrappers use it to mirror their main's pre-foreground writes
	// (e.g. core's update-banner emission) into the same buffer.
	PreRun func(stderr io.Writer)
}

// Run invokes cmd with the given argv (NOT including the executable
// name — the harness prepends cmd.Name, which every tai-family root
// command sets to its binary name). Stdin is empty; stdout and stderr
// are captured into the returned Result.
//
// The cmd's Writer / ErrWriter / Reader fields are overwritten by
// this call; callers should pass a freshly-built command (typically
// their tree's NewRoot()).
func Run(t *testing.T, cmd *cli.Command, argv ...string) Result {
	t.Helper()
	return RunWith(t, cmd, Options{}, argv...)
}

// RunWithStdin is like Run but pipes stdin as standard input.
func RunWithStdin(t *testing.T, cmd *cli.Command, stdin string, argv ...string) Result {
	t.Helper()
	return RunWith(t, cmd, Options{Stdin: stdin}, argv...)
}

// RunWith is the full-options harness body behind Run / RunWithStdin.
//
// urfave/cli does NOT propagate Writer/ErrWriter/Reader to
// subcommands; the harness walks the tree and sets all three on every
// descendant so a subcommand's c.Writer.Write reaches the captured
// buffer.
func RunWith(t *testing.T, cmd *cli.Command, opts Options, argv ...string) Result {
	t.Helper()

	var stdout, stderr bytes.Buffer
	wireStreams(cmd, &stdout, &stderr, strings.NewReader(opts.Stdin))

	if opts.PreRun != nil {
		opts.PreRun(&stderr)
	}

	fullArgs := append([]string{cmd.Name}, argv...)
	err := cliexec.Run(context.Background(), cmd, fullArgs)
	exitCode := cliexec.Exit(&stderr, err)

	return Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Err:      err,
	}
}

// wireStreams recursively assigns out, errOut, and in to cmd and
// every (transitive) subcommand. Mirrors how a real process inherits
// stdio across a command tree, since urfave/cli leaves descendant
// streams at their nil-defaults (which setupDefaults later swaps for
// os.Std*).
func wireStreams(cmd *cli.Command, out, errOut *bytes.Buffer, in *strings.Reader) {
	cmd.Writer = out
	cmd.ErrWriter = errOut
	cmd.Reader = in
	for _, child := range cmd.Commands {
		wireStreams(child, out, errOut, in)
	}
}

// AssertNoError fails the test when r.Err is non-nil.
func AssertNoError(t *testing.T, r Result) {
	t.Helper()
	if r.Err != nil {
		t.Fatalf("expected no error, got %v\nstdout: %q\nstderr: %q",
			r.Err, r.Stdout, r.Stderr)
	}
}

// AssertError fails the test when r.Err is nil. Use AssertErrorFooter
// when the test is specifically about the foundation's error
// contract; this helper only verifies that *some* error occurred.
func AssertError(t *testing.T, r Result) {
	t.Helper()
	if r.Err == nil {
		t.Fatalf("expected an error, got nil\nstdout: %q\nstderr: %q",
			r.Stdout, r.Stderr)
	}
}

// AssertExitCode fails the test when r.ExitCode != want.
func AssertExitCode(t *testing.T, r Result, want int) {
	t.Helper()
	if r.ExitCode != want {
		t.Fatalf("expected exit code %d, got %d\nstdout: %q\nstderr: %q",
			want, r.ExitCode, r.Stdout, r.Stderr)
	}
}

// AssertStdoutContains fails the test when substr does not appear in
// stdout.
func AssertStdoutContains(t *testing.T, r Result, substr string) {
	t.Helper()
	if !strings.Contains(r.Stdout, substr) {
		t.Fatalf("expected stdout to contain %q\nstdout: %q", substr, r.Stdout)
	}
}

// AssertStdoutMatches fails the test when stdout does not match the
// regex.
func AssertStdoutMatches(t *testing.T, r Result, pattern string) {
	t.Helper()
	matched, err := regexp.MatchString(pattern, r.Stdout)
	if err != nil {
		t.Fatalf("invalid regex %q: %v", pattern, err)
	}
	if !matched {
		t.Fatalf("expected stdout to match %q\nstdout: %q", pattern, r.Stdout)
	}
}

// AssertStdoutEquals fails when stdout differs from want
// byte-for-byte. Use sparingly — substring/regex matching is more
// forgiving and friendlier to revising output formatting.
func AssertStdoutEquals(t *testing.T, r Result, want string) {
	t.Helper()
	if r.Stdout != want {
		t.Fatalf("stdout mismatch\nwant: %q\ngot:  %q", want, r.Stdout)
	}
}

// AssertStderrEmpty fails the test when stderr contains any bytes.
func AssertStderrEmpty(t *testing.T, r Result) {
	t.Helper()
	if r.Stderr != "" {
		t.Fatalf("expected stderr empty, got %q", r.Stderr)
	}
}

// AssertStderrContains fails the test when substr does not appear in
// stderr.
func AssertStderrContains(t *testing.T, r Result, substr string) {
	t.Helper()
	if !strings.Contains(r.Stderr, substr) {
		t.Fatalf("expected stderr to contain %q\nstderr: %q", substr, r.Stderr)
	}
}

// errorFooterRe matches the foundation's error-contract footer line:
//
//	[exit <N>: <ERROR_CODE>]
//
// where N is an integer and ERROR_CODE is upper snake case.
var errorFooterRe = regexp.MustCompile(`(?m)^\[exit (\d+): ([A-Z][A-Z0-9_]*)\]$`)

// asserterT is the minimal subset of testing.T this package's helpers
// need. It exists so the internal seams can be exercised directly by
// the harness's own unit tests with a recording fake.
type asserterT interface {
	Helper()
	Fatalf(format string, args ...any)
}

// AssertErrorFooter asserts that the LAST line of stderr is the
// foundation's error-contract footer and that:
//
//   - the embedded code equals wantCode, and
//   - (when wantExitCode >= 0) the embedded numeric exit code equals
//     wantExitCode. Pass -1 to skip the numeric check.
//
// The spec requires the footer to be the literal last line written to
// stderr (trailing newline allowed); this helper enforces that —
// content after the matched footer fails the assertion.
func AssertErrorFooter(t *testing.T, r Result, wantCode string, wantExitCode int) {
	t.Helper()
	assertErrorFooter(t, r, wantCode, wantExitCode)
}

// assertErrorFooter is the internal seam exercised by
// AssertErrorFooter's own unit tests. Public callers use
// AssertErrorFooter.
func assertErrorFooter(t asserterT, r Result, wantCode string, wantExitCode int) {
	t.Helper()

	matches := errorFooterRe.FindAllStringSubmatchIndex(r.Stderr, -1)
	if len(matches) == 0 {
		t.Fatalf("expected stderr to contain error footer [exit N: %s]\nstderr: %q",
			wantCode, r.Stderr)
		return
	}
	last := matches[len(matches)-1]
	matchEnd := last[1]

	// Spec: "the footer MUST be the last line written to stderr".
	// Allow any number of trailing newlines but no other content.
	trailing := r.Stderr[matchEnd:]
	if strings.TrimLeft(trailing, "\n") != "" {
		t.Fatalf("error footer is not the last line of stderr\ntrailing content: %q\nstderr: %q",
			trailing, r.Stderr)
		return
	}

	gotCode := r.Stderr[last[4]:last[5]]
	if gotCode != wantCode {
		t.Fatalf("expected error code %q, got %q\nstderr: %q",
			wantCode, gotCode, r.Stderr)
		return
	}

	if wantExitCode >= 0 {
		gotExit := r.Stderr[last[2]:last[3]]
		wantExitStr := strconv.Itoa(wantExitCode)
		if gotExit != wantExitStr {
			t.Fatalf("expected footer exit code %s, got %s\nstderr: %q",
				wantExitStr, gotExit, r.Stderr)
			return
		}
	}
}
