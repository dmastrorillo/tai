package cmdtest

import (
	"fmt"
	"testing"
)

// TestAssertErrorFooter_RegexAndLastLine exercises assertErrorFooter
// (the internal seam behind AssertErrorFooter) directly, since the
// helper is non-trivial and codifies a spec contract: the footer MUST
// be the last line of stderr, must match the prescribed shape, and the
// embedded code (and optionally numeric exit code) must equal the
// expected values.
//
// A recording fake stands in for *testing.T so we can table-drive the
// pass / fail outcomes without polluting the outer test's status.
func TestAssertErrorFooter_RegexAndLastLine(t *testing.T) {
	tests := []struct {
		name         string
		stderr       string
		wantCode     string
		wantExitCode int // -1 skips numeric check
		wantFail     bool
	}{
		{
			name:         "exact match passes",
			stderr:       "Error: nope\n\n[exit 2: REPO_NOT_FOUND]\n",
			wantCode:     "REPO_NOT_FOUND",
			wantExitCode: 2,
		},
		{
			name:         "exit code ignored when -1",
			stderr:       "[exit 99: ANY_CODE]\n",
			wantCode:     "ANY_CODE",
			wantExitCode: -1,
		},
		{
			name:         "wrong code fails",
			stderr:       "[exit 2: REPO_NOT_FOUND]\n",
			wantCode:     "INTERNAL_ERROR",
			wantExitCode: -1,
			wantFail:     true,
		},
		{
			name:         "wrong exit code fails",
			stderr:       "[exit 2: REPO_NOT_FOUND]\n",
			wantCode:     "REPO_NOT_FOUND",
			wantExitCode: 70,
			wantFail:     true,
		},
		{
			name:         "no footer fails",
			stderr:       "Error: nope\n\n",
			wantCode:     "ANY",
			wantExitCode: -1,
			wantFail:     true,
		},
		{
			name:         "content after footer fails (footer not last)",
			stderr:       "[exit 2: REPO_NOT_FOUND]\nstray trailing text\n",
			wantCode:     "REPO_NOT_FOUND",
			wantExitCode: 2,
			wantFail:     true,
		},
		{
			name:         "trailing newlines tolerated",
			stderr:       "[exit 2: REPO_NOT_FOUND]\n\n\n",
			wantCode:     "REPO_NOT_FOUND",
			wantExitCode: 2,
		},
		{
			name:         "multiple footer-shaped lines, last one matched",
			stderr:       "[exit 1: FIRST]\nmiddle line\n[exit 2: REPO_NOT_FOUND]\n",
			wantCode:     "REPO_NOT_FOUND",
			wantExitCode: 2,
		},
		{
			name:         "lowercase code rejected by regex",
			stderr:       "[exit 2: repo_not_found]\n",
			wantCode:     "repo_not_found",
			wantExitCode: -1,
			wantFail:     true,
		},
		{
			name:         "footer mid-stderr rejected by last-line rule",
			stderr:       "[exit 2: REPO_NOT_FOUND]\nError: another\n",
			wantCode:     "REPO_NOT_FOUND",
			wantExitCode: 2,
			wantFail:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingT{}
			assertErrorFooter(rec, Result{Stderr: tt.stderr}, tt.wantCode, tt.wantExitCode)
			if rec.failed != tt.wantFail {
				t.Fatalf("got fail=%v, want fail=%v\nlast Fatalf message: %s",
					rec.failed, tt.wantFail, rec.message)
			}
		})
	}
}

// recordingT records calls to Helper / Fatalf without stopping the goroutine,
// so a single test function can drive many positive and negative cases.
//
// It implements the asserterT interface (Helper, Fatalf) defined in
// cmdtest.go.
type recordingT struct {
	failed  bool
	message string
}

func (*recordingT) Helper() {}

func (r *recordingT) Fatalf(format string, args ...any) {
	r.failed = true
	r.message = fmt.Sprintf(format, args...)
}
