package cliexec_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/danielmastrorillo/tai/internal/cliexec"
	"github.com/danielmastrorillo/tai/internal/errcode"
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
