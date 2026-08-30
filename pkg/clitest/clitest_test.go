package clitest_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/pkg/clitest"
	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/urfave/cli/v3"
)

// The harness must translate errors into exit codes and stderr
// rendering through cliexec.Exit — the exact function both binaries'
// main uses — so a harness-driven test can never observe behaviour
// the shipped binary doesn't produce. (The two per-tree harnesses
// this package replaced had already drifted: one rendered an
// INTERNAL_ERROR footer over plugin-subprocess exits that main
// deliberately passes through.)
func TestRun_error_translation_matches_production(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantExit   int
		wantFooter bool
	}{
		{"nil error is exit 0, stderr empty", nil, 0, false},
		{"errcode error renders footer and maps its code", errcode.New(errcode.InternalError, "boom"), 70, true},
		{"plain ExitCoder passes through without footer", cli.Exit("", 7), 7, false},
		{"unstructured error is INTERNAL with footer", fmt.Errorf("leak"), 70, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cli.Command{
				Name: "fake",
				// Every tai-family root disables urfave/cli's default
				// HandleExitCoder (which would os.Exit mid-test); the
				// fixtures mirror that.
				ExitErrHandler: func(_ context.Context, _ *cli.Command, _ error) {},
				Action: func(_ context.Context, _ *cli.Command) error {
					return tc.err
				},
			}
			r := clitest.Run(t, cmd)
			if r.ExitCode != tc.wantExit {
				t.Errorf("ExitCode = %d, want %d", r.ExitCode, tc.wantExit)
			}
			hasFooter := strings.Contains(r.Stderr, "[exit ")
			if hasFooter != tc.wantFooter {
				t.Errorf("footer rendered = %v, want %v\nstderr: %q", hasFooter, tc.wantFooter, r.Stderr)
			}
		})
	}
}

// argv[0] comes from the command's own Name so the same harness
// serves any binary ("tai", "triage", a third-party plugin).
func TestRun_wires_streams_and_stdin_across_subcommands(t *testing.T) {
	cmd := &cli.Command{
		Name:           "fake",
		ExitErrHandler: func(_ context.Context, _ *cli.Command, _ error) {},
		Commands: []*cli.Command{{
			Name: "echo",
			Action: func(_ context.Context, c *cli.Command) error {
				in, _ := io.ReadAll(c.Reader)
				_, _ = fmt.Fprintf(c.Writer, "stdin was %q via %s", in, c.Root().Name)
				_, _ = fmt.Fprint(c.ErrWriter, "warn")
				return nil
			},
		}},
	}
	r := clitest.RunWithStdin(t, cmd, "piped", "echo")
	if want := `stdin was "piped" via fake`; r.Stdout != want {
		t.Errorf("stdout = %q, want %q", r.Stdout, want)
	}
	if r.Stderr != "warn" {
		t.Errorf("stderr = %q, want %q", r.Stderr, "warn")
	}
}

// PreRun gives per-binary harness wrappers a hook to mirror their
// main's pre-foreground writes (core's update banner) into the same
// captured stderr, in order.
func TestRunWith_prerun_writes_stderr_before_command(t *testing.T) {
	cmd := &cli.Command{
		Name:           "fake",
		ExitErrHandler: func(_ context.Context, _ *cli.Command, _ error) {},
		Action: func(_ context.Context, c *cli.Command) error {
			_, _ = fmt.Fprint(c.ErrWriter, "command")
			return nil
		},
	}
	r := clitest.RunWith(t, cmd, clitest.Options{
		PreRun: func(stderr io.Writer) { _, _ = fmt.Fprint(stderr, "banner|") },
	})
	if r.Stderr != "banner|command" {
		t.Errorf("stderr = %q, want %q", r.Stderr, "banner|command")
	}
}
