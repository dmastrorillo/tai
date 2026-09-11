package cliout_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/pkg/cliout"
)

// errReader fails on read, standing in for a broken stdin (a closed
// pipe, an I/O error on the terminal device).
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// TC-CLI-004 — only an explicit yes is a yes.
func TestConfirmYesNo_TCCLI004_accepts_yes(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "yes\n", "YES\n", "  yes  \n", "y"} {
		got, err := cliout.ConfirmYesNo(strings.NewReader(answer))
		if err != nil {
			t.Errorf("answer %q: unexpected error %v", answer, err)
		}
		if !got {
			t.Errorf("answer %q must be a yes", answer)
		}
	}
}

// TC-CLI-004 — anything else is a no, including the empty answer a
// bare Enter produces and the EOF a closed stdin produces.
func TestConfirmYesNo_TCCLI004_everything_else_is_no(t *testing.T) {
	for _, answer := range []string{"n\n", "N\n", "no\n", "\n", "", "sure\n", "yep\n", "yes please\n"} {
		got, err := cliout.ConfirmYesNo(strings.NewReader(answer))
		if err != nil {
			t.Errorf("answer %q: unexpected error %v", answer, err)
		}
		if got {
			t.Errorf("answer %q must not be a yes", answer)
		}
	}
}

// TC-CLI-005 — a stdin that cannot be read is not a yes, and the cause
// is not swallowed.
//
// The two are separate obligations: a consent gate must deny on a read
// failure, and a caller must still be able to say why it denied.
func TestConfirmYesNo_TCCLI005_read_failure_denies_and_reports(t *testing.T) {
	sentinel := errors.New("stdin exploded")

	got, err := cliout.ConfirmYesNo(errReader{err: sentinel})
	if got {
		t.Error("a read failure must never be reported as consent")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("the cause must survive, got %v", err)
	}
}

// A nil reader is not a yes and must not panic.
func TestConfirmYesNo_nil_reader_is_no(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ConfirmYesNo(nil) panicked: %v", r)
		}
	}()
	got, err := cliout.ConfirmYesNo(nil)
	if got || err != nil {
		t.Errorf("nil reader must be a silent no, got (%v, %v)", got, err)
	}
}
