// Package cmdtest is the test harness for the tai CLI.
//
// It provides three layers:
//
//   - Run / RunWithStdin — invoke a *cli.Command in-process with captured
//     stdin/stdout/stderr. No subprocess, no built binary required.
//   - Assert* — vocabulary for asserting on the captured bytes. Helpers
//     fail with t.Fatal on mismatch and include the captured streams in
//     their failure messages, so debugging a red test does not require
//     re-running with extra logging.
//   - Isolate — file-system + env isolation for tests that touch the data
//     directory, slash-command target directory, or git context.
//
// Conventions:
//
//   - Every test that maps to a BDD case in test-cases.md names the
//     TC-ID in its function name AND its t.Run subtest descriptions, so a
//     failure surfaces the trace back to the spec. Example:
//
//     func TestVersion_TCCMD001_prints_version_string(t *testing.T) { ... }
//
//     The pattern is Test<Cmd>_<TCID>_<short_description>, where <TCID>
//     is the TC-ID with hyphens removed (TC-CMD-001 → TCCMD001).
//
//   - Helpers t.Helper() themselves so failure line numbers point at the
//     caller, not at this file.
//
//   - The assert vocabulary is intentionally narrow. If you reach for a
//     helper that does not exist, prefer adding it here over inlining the
//     check — the harness is the contract every test obeys.
package cmdtest

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

// Result captures everything tests need to assert against after running
// the tai CLI.
type Result struct {
	// Stdout is the bytes written to the command's Writer.
	Stdout string
	// Stderr is the bytes written to the command's ErrWriter.
	Stderr string
	// ExitCode is the OS exit code the binary would have produced. It is
	// derived from the error returned by cli.Command.Run: nil → 0; any
	// non-nil error that implements cli.ExitCoder → that code; any other
	// non-nil error → 1 (the default cmd/tai/main.go produces).
	ExitCode int
	// Err is the raw error returned by cli.Command.Run. Tests that care
	// about the user-observable error contract should prefer
	// AssertErrorFooter and AssertExitCode (which encode the spec); Err
	// is exposed for assertions that need the underlying Go value.
	Err error
}

// Run invokes cmd with the given argv (NOT including the executable name —
// the harness prepends "tai" for you). Stdin is empty; stdout and stderr
// are captured into the returned Result.
//
// The cmd's Writer / ErrWriter / Reader fields are overwritten by this
// call; callers should pass a freshly-built command (typically
// cmd.NewRoot()).
func Run(t *testing.T, cmd *cli.Command, argv ...string) Result {
	t.Helper()
	return RunWithStdin(t, cmd, "", argv...)
}

// RunWithStdin is like Run but pipes stdin as the command's standard input.
func RunWithStdin(t *testing.T, cmd *cli.Command, stdin string, argv ...string) Result {
	t.Helper()

	var stdout, stderr bytes.Buffer
	cmd.Writer = &stdout
	cmd.ErrWriter = &stderr
	cmd.Reader = strings.NewReader(stdin)

	fullArgs := append([]string{"tai"}, argv...)
	err := cmd.Run(context.Background(), fullArgs)

	return Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCodeFor(err),
		Err:      err,
	}
}

// exitCodeFor mirrors the mapping cmd/tai/main.go applies to translate a
// cli.Command.Run error into an OS exit code. Keeping the logic here lets
// tests assert on ExitCode without spawning a subprocess.
func exitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	var ec cli.ExitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return 1
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
// when the test is specifically about the foundation's error contract;
// this helper only verifies that *some* error occurred.
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

// AssertStdoutContains fails the test when substr does not appear in stdout.
func AssertStdoutContains(t *testing.T, r Result, substr string) {
	t.Helper()
	if !strings.Contains(r.Stdout, substr) {
		t.Fatalf("expected stdout to contain %q\nstdout: %q", substr, r.Stdout)
	}
}

// AssertStdoutMatches fails the test when stdout does not match the regex.
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

// AssertStdoutEquals fails when stdout differs from want byte-for-byte.
// Use sparingly — substring/regex matching is more forgiving and friendlier
// to revising output formatting.
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

// AssertStderrContains fails the test when substr does not appear in stderr.
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
// stderr (trailing newline allowed); this helper enforces that — content
// after the matched footer fails the assertion.
//
// This is the primary error-path assertion every TC-ERR test will use
// once add-tai-foundation lands.
func AssertErrorFooter(t *testing.T, r Result, wantCode string, wantExitCode int) {
	t.Helper()
	assertErrorFooter(t, r, wantCode, wantExitCode)
}

// assertErrorFooter is the internal seam exercised by AssertErrorFooter's
// own unit tests. Public callers use AssertErrorFooter.
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

	// Spec: "the footer MUST be the last line written to stderr". Allow
	// any number of trailing newlines but no other content.
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
