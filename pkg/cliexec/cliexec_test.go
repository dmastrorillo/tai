package cliexec_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/pkg/cliexec"
	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/urfave/cli/v3"
)

// TestRun_TCERR001_panic_becomes_internal_error exercises TC-ERR-001
// from test-cases.md: a panic during command execution is recovered by
// cliexec and surfaced as a *errcode.Error{Code: INTERNAL_ERROR}.
//
// The exit code mapped by errcode.Code.ExitCode() is 70 — the
// foundation's reserved code for unrecovered programmer errors.
func TestRun_TCERR001_panic_becomes_internal_error(t *testing.T) {
	cmd := &cli.Command{
		Name: "tai",
		Action: func(_ context.Context, _ *cli.Command) error {
			panic("boom")
		},
	}

	err := cliexec.Run(context.Background(), cmd, []string{"tai"})
	if err == nil {
		t.Fatal("expected an error from a panicking action, got nil")
	}

	taiErr, ok := errcode.As(err)
	if !ok {
		t.Fatalf("expected *errcode.Error, got %T: %v", err, err)
	}
	if taiErr.Code != errcode.InternalError {
		t.Fatalf("expected code INTERNAL_ERROR, got %s", taiErr.Code)
	}
	if taiErr.Code.ExitCode() != 70 {
		t.Fatalf("expected exit code 70, got %d", taiErr.Code.ExitCode())
	}
	// The recovered value should appear in the message so diagnostics
	// reach the user.
	if !strings.Contains(taiErr.Msg, "boom") {
		t.Fatalf("expected error message to contain panic value %q, got %q",
			"boom", taiErr.Msg)
	}
}

// TestRun_TCERR005_panic_with_error_preserves_cause_chain exercises
// TC-ERR-005: when an action panics with a value that is itself an
// error, cliexec wraps it preserving the cause chain so errors.Is /
// errors.As / errors.Unwrap continue to work.
func TestRun_TCERR005_panic_with_error_preserves_cause_chain(t *testing.T) {
	sentinel := errors.New("upstream failure")
	cmd := &cli.Command{
		Name: "tai",
		Action: func(_ context.Context, _ *cli.Command) error {
			panic(sentinel)
		},
	}

	err := cliexec.Run(context.Background(), cmd, []string{"tai"})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	taiErr, ok := errcode.As(err)
	if !ok {
		t.Fatalf("expected *errcode.Error, got %T: %v", err, err)
	}
	if taiErr.Code != errcode.InternalError {
		t.Fatalf("expected INTERNAL_ERROR, got %s", taiErr.Code)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected errors.Is(err, sentinel) to be true; cause chain not preserved")
	}
}

// TestRun_TCERR006_passes_through_non_panic_errors exercises TC-ERR-006:
// cliexec does not double-wrap or interfere with errors returned
// normally from actions.
func TestRun_TCERR006_passes_through_non_panic_errors(t *testing.T) {
	want := errors.New("normal failure")
	cmd := &cli.Command{
		Name: "tai",
		Action: func(_ context.Context, _ *cli.Command) error {
			return want
		},
	}

	err := cliexec.Run(context.Background(), cmd, []string{"tai"})
	if !errors.Is(err, want) {
		t.Fatalf("expected errors.Is(err, want), got %v", err)
	}
}

// Exit is the single implementation of the error → exit-code
// translation both binary entry points perform after Run returns.
// Before it existed the branch ladder was copy-pasted verbatim into
// core/cmd/tai/main.go and plugins/triage/cmd/triage/main.go; every
// future plugin main would have copied it again, and an error-shape
// fix would need to touch every copy.
func TestExit_maps_error_shapes_to_exit_codes(t *testing.T) {
	structured := errcode.New(errcode.InternalError, "structured failure")

	cases := []struct {
		name       string
		err        error
		wantCode   int
		wantFooter bool // error template rendered to w
	}{
		{
			name:       "nil error is success, nothing written",
			err:        nil,
			wantCode:   0,
			wantFooter: false,
		},
		{
			name:       "structured errcode error renders template and maps its exit code",
			err:        structured,
			wantCode:   structured.Code.ExitCode(),
			wantFooter: true,
		},
		{
			name: "plain cli.ExitCoder propagates the child's code without a template",
			// A plugin subprocess exit: the child already wrote its
			// own stderr template — rendering another would bury it.
			err:        cli.Exit("", 7),
			wantCode:   7,
			wantFooter: false,
		},
		{
			name:       "unstructured error surfaces as INTERNAL with template",
			err:        errors.New("third-party leak"),
			wantCode:   70,
			wantFooter: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf strings.Builder
			got := cliexec.Exit(&buf, tc.err)
			if got != tc.wantCode {
				t.Errorf("Exit = %d, want %d", got, tc.wantCode)
			}
			if tc.wantFooter && !strings.Contains(buf.String(), "[exit") {
				t.Errorf("want rendered error template, got %q", buf.String())
			}
			if !tc.wantFooter && buf.Len() != 0 {
				t.Errorf("want nothing written, got %q", buf.String())
			}
		})
	}
}
