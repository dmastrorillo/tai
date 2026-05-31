package errcode_test

import (
	"errors"
	"testing"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/pkg/exitcode"
)

// TestCode_ExitCode_taxonomy locks the Code → exit-code mapping. Each
// known code maps to its specified exit value. An unknown Code falls
// back to exitcode.Internal so a programmer who adds a new Code but
// forgets the switch case lands in INTERNAL territory rather than
// silently mapping to 0.
//
// Not tied to a TC-ID — this is an engine invariant the foundation
// taxonomy spec asserts (one code, one exit). The user-observable
// version is verified by the various TC-ERR-NNN footer tests; this
// guards the in-code source of truth.
func TestCode_ExitCode_taxonomy(t *testing.T) {
	cases := []struct {
		code errcode.Code
		want int
	}{
		{errcode.RepoNotFound, exitcode.Precondition},
		{errcode.RepoFlagInvalid, exitcode.Usage},
		{errcode.DataDirUnwritable, exitcode.Data},
		{errcode.UnknownSubcommand, exitcode.Usage},
		{errcode.InternalError, exitcode.Internal},
		{errcode.ConfigUnwritable, exitcode.Data},
		{errcode.ConfigInvalid, exitcode.Usage},
		{errcode.ConfigInvalidRepoURL, exitcode.Usage},
		{errcode.ConfigKeyNotScriptable, exitcode.Usage},
		{errcode.ConfigDuplicateTarget, exitcode.Usage},
		{errcode.ConfigTargetNotFound, exitcode.Usage},
		{errcode.ConfigEditorUnset, exitcode.Usage},
		{errcode.TaiNotConfigured, exitcode.Precondition},
		{errcode.MissingArg, exitcode.Usage},
		{errcode.RepoFetchFailed, exitcode.Data},
		{errcode.RepoInitTargetNotEmpty, exitcode.Usage},
		{errcode.RepoInitGitUnavailable, exitcode.Data},
		{errcode.WorkflowInvalid, exitcode.Usage},
		{errcode.WorkflowNotFound, exitcode.Precondition},
		{errcode.StandardInvalid, exitcode.Usage},
		{errcode.StandardNotFound, exitcode.Precondition},
		{errcode.DBOpenFailed, exitcode.Data},
		{errcode.DBMigrationFailed, exitcode.Data},
		{errcode.DBConstraintViolation, exitcode.Data},
		{errcode.InstallTargetUnwritable, exitcode.Data},
		{errcode.InstallInvalidTarget, exitcode.Usage},
		{errcode.InstallLedgerCorrupt, exitcode.Internal},
		{errcode.ImportInvalidJSON, exitcode.Usage},
		{errcode.ImportSchemaInvalid, exitcode.Data},
		{errcode.ImportAmbiguousRefs, exitcode.Data},
		{errcode.TriageNoScope, exitcode.Precondition},
		{errcode.TriageAmbiguousScope, exitcode.Precondition},
		{errcode.TriageNotFound, exitcode.Precondition},
		{errcode.TriageInvalidFlags, exitcode.Usage},
		{errcode.TriageConfirmationRequired, exitcode.Usage},
		{errcode.Code("UNKNOWN_FUTURE_CODE"), exitcode.Internal},
	}
	for _, tc := range cases {
		got := tc.code.ExitCode()
		if got != tc.want {
			t.Errorf("Code(%s).ExitCode() = %d, want %d", tc.code, got, tc.want)
		}
	}
}

// TestExitCode_constants locks the OS-exit constants. These five
// numbers are part of tai's public CLI contract; changing one without
// an OpenSpec proposal that updates the foundation spec is a breaking
// change.
func TestExitCode_constants(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"Success", exitcode.Success, 0},
		{"Usage", exitcode.Usage, 1},
		{"Precondition", exitcode.Precondition, 2},
		{"Data", exitcode.Data, 3},
		{"Internal", exitcode.Internal, 70},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("exitcode.%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

// TestError_chain verifies *Error supports errors.Is / errors.As /
// errors.Unwrap correctly, so the cause chain reaches callers.
func TestError_chain(t *testing.T) {
	cause := errors.New("inner")
	e := errcode.Wrap(errcode.InternalError, cause, "outer")

	if !errors.Is(e, cause) {
		t.Fatal("errors.Is should reach the wrapped cause")
	}
	if got := errors.Unwrap(e); got != cause {
		t.Fatalf("errors.Unwrap = %v, want %v", got, cause)
	}
	asTai, ok := errcode.As(e)
	if !ok || asTai.Code != errcode.InternalError {
		t.Fatalf("errcode.As did not return the wrapped *Error; got ok=%v code=%s", ok, asTai.Code)
	}
}

// TestError_Wrapf_formats_message verifies the Wrapf constructor used
// by callers like plugins/triage/internal/repoctx.
func TestError_Wrapf_formats_message(t *testing.T) {
	cause := errors.New("inner")
	e := errcode.Wrapf(errcode.InternalError, cause, "wrap %d", 42)

	if e.Msg != "wrap 42" {
		t.Fatalf("Msg: want %q, got %q", "wrap 42", e.Msg)
	}
	if !errors.Is(e, cause) {
		t.Fatal("cause not preserved")
	}
}

// TestError_WithHelp_appends verifies the fluent help-bullet API.
func TestError_WithHelp_appends(t *testing.T) {
	e := errcode.New(errcode.RepoNotFound, "msg").
		WithHelp("first").
		WithHelp("second", "third")

	want := []string{"first", "second", "third"}
	if len(e.Help) != len(want) {
		t.Fatalf("Help len = %d, want %d", len(e.Help), len(want))
	}
	for i, w := range want {
		if e.Help[i] != w {
			t.Errorf("Help[%d] = %q, want %q", i, e.Help[i], w)
		}
	}
}
