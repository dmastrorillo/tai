package cmd_test

import (
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/core/internal/version"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// runResult captures everything tests need from a single root invocation.
type runResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

// runRoot drives cmd.NewRoot() through pkg/cliexec.Run with captured
// stdio, mirroring how core/cmd/tai/main.go assembles the binary. It is
// the core-tree equivalent of plugins/triage/internal/cmdtest.Run, kept
// minimal here because the core tree currently has only the meta-verb
// tests below — when more land, promote the harness to a shared package.
//
// Error rendering and exit-code mapping go through cliexec.Exit — the
// exact translation main.go performs — so the harness can never drift
// from what the shipped binary does.
func runRoot(t *testing.T, argv ...string) runResult {
	t.Helper()
	return runRootStdin(t, "", argv...)
}

// TestVersion_TCCMD001_prints_version_string exercises TC-CMD-001 from
// core/test-cases.md: invoking `tai --version` writes a line of the form
// "tai version <version>" to stdout and exits with code 0, with no stderr.
//
// The assertion includes the literal "tai version " prefix so the test
// fails on accidentally-matching output (a substring search on the
// version alone would pass for free-form text that happened to contain
// "dev").
func TestVersion_TCCMD001_prints_version_string(t *testing.T) {
	r := runRoot(t, "--version")

	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if r.exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d", r.exitCode)
	}
	if r.stderr != "" {
		t.Fatalf("stderr: want empty, got %q", r.stderr)
	}
	want := "tai version " + version.String
	if !strings.Contains(r.stdout, want) {
		t.Fatalf("stdout: want substring %q, got %q", want, r.stdout)
	}
}

// TestRoot_TCCMD002_unknown_flag exercises TC-CMD-002 from
// core/test-cases.md (flag form): an unrecognised flag like
// `--bogus-flag` flows through OnUsageError, gets wrapped as
// UnknownSubcommand, and surfaces with exit 1 and the foundation footer.
func TestRoot_TCCMD002_unknown_flag(t *testing.T) {
	r := runRoot(t, "--bogus-flag")

	if r.err == nil {
		t.Fatal("expected error, got nil")
	}
	if r.exitCode != 1 {
		t.Fatalf("exit code: want 1, got %d", r.exitCode)
	}
	assertCode(t, r.err, errcode.UnknownSubcommand)
	for _, want := range []string{"Error:", "What to do:", "[exit 1: UNKNOWN_SUBCOMMAND]"} {
		if !strings.Contains(r.stderr, want) {
			t.Fatalf("stderr missing %q\nfull stderr:\n%s", want, r.stderr)
		}
	}
}

// TestRoot_TCCMD002_unknown_positional exercises TC-CMD-002's positional
// form: `tai bogus` (no subcommand named "bogus") flows through the root
// Action's catch-all and surfaces with exit 1 and the foundation footer.
func TestRoot_TCCMD002_unknown_positional(t *testing.T) {
	r := runRoot(t, "bogus")

	if r.err == nil {
		t.Fatal("expected error, got nil")
	}
	if r.exitCode != 1 {
		t.Fatalf("exit code: want 1, got %d", r.exitCode)
	}
	assertCode(t, r.err, errcode.UnknownSubcommand)
	if !strings.Contains(r.stderr, "bogus") {
		t.Fatalf("stderr should name the offending arg, got:\n%s", r.stderr)
	}
	if !strings.Contains(r.stderr, "[exit 1: UNKNOWN_SUBCOMMAND]") {
		t.Fatalf("stderr missing UNKNOWN_SUBCOMMAND footer, got:\n%s", r.stderr)
	}
}

// TestHelp_TCCMD008_outside_git_repo exercises TC-CMD-008: `tai --help`
// runs without invoking any repo resolver (the core binary has none in
// Phase 0), exits 0, and surfaces the app name on stdout.
//
// The "outside a git repository" clause is satisfied implicitly here:
// the core root does not call any repo-resolution helper, so cwd is
// irrelevant. A regression that wires repo resolution into the core
// root's --help path would surface as REPO_NOT_FOUND on stderr.
func TestHelp_TCCMD008_outside_git_repo(t *testing.T) {
	r := runRoot(t, "--help")

	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if r.exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d", r.exitCode)
	}
	if !strings.Contains(r.stdout, "tai") {
		t.Fatalf("stdout: want 'tai' in help banner, got %q", r.stdout)
	}
	if strings.Contains(r.stderr, "REPO_NOT_FOUND") {
		t.Fatalf("stderr leaked repo-resolution failure:\n%s", r.stderr)
	}
}

// assertCode fails the test if err is not a *errcode.Error with the
// expected Code.
func assertCode(t *testing.T, err error, want errcode.Code) {
	t.Helper()
	e, ok := errcode.As(err)
	if !ok {
		t.Fatalf("error is not *errcode.Error: %T %v", err, err)
	}
	if e.Code != want {
		t.Fatalf("error code: want %s, got %s", want, e.Code)
	}
}
