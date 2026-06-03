package cmd_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	"github.com/dmastrorillo/tai/core/internal/cmd"
	"github.com/dmastrorillo/tai/core/internal/version"
	"github.com/dmastrorillo/tai/pkg/cliexec"
	"github.com/dmastrorillo/tai/pkg/cliout"
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
func runRoot(t *testing.T, argv ...string) runResult {
	t.Helper()

	var stdout, stderr bytes.Buffer
	root := cmd.NewRoot()
	wireStreams(root, &stdout, &stderr)

	fullArgs := append([]string{"tai"}, argv...)
	err := cliexec.Run(context.Background(), root, fullArgs)
	writeStructuredError(&stderr, err)

	return runResult{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: exitCodeFor(err),
		err:      err,
	}
}

// writeStructuredError mirrors core/cmd/tai/main.go's error rendering
// rule: render the foundation template ONLY when err is a structured
// *errcode.Error or a truly unstructured error. A cli.ExitCoder that
// is NOT an *errcode.Error (today: pluginExitError carrying a child
// subprocess's exit code) MUST NOT have an INTERNAL_ERROR template
// rendered over it — the plugin has already written its own stderr.
func writeStructuredError(stderr *bytes.Buffer, err error) {
	if err == nil {
		return
	}
	if _, ok := errcode.As(err); ok {
		cliout.WriteError(stderr, err)
		return
	}
	if _, ok := err.(cli.ExitCoder); ok {
		return
	}
	cliout.WriteError(stderr, err)
}

// wireStreams sets Writer/ErrWriter on the root and every descendant so
// captured buffers receive subcommand output too. urfave/cli does not
// propagate these fields automatically.
func wireStreams(c *cli.Command, out, errOut *bytes.Buffer) {
	c.Writer = out
	c.ErrWriter = errOut
	c.Reader = strings.NewReader("")
	for _, child := range c.Commands {
		wireStreams(child, out, errOut)
	}
}

// exitCodeFor mirrors the mapping core/cmd/tai/main.go applies:
// nil → 0; cli.ExitCoder → its code; any other error → 1.
func exitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	if e, ok := err.(cli.ExitCoder); ok {
		return e.ExitCode()
	}
	return 1
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
