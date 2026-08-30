package taiplugin_test

import (
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/pkg/taiplugin"
)

const desc = "Walk through pending PR review comments interactively."

// The host invokes `<plugin> --help-summary` during install and
// update and stores the captured line as the plugin's description.
// HelpSummary gives every SDK-built plugin that wire verb without
// each author re-implementing the argument sniffing and the exact
// single-line-plus-newline output shape the host parses.
func TestHelpSummary_TCSDK001_answers_the_wire_verb(t *testing.T) {
	var out strings.Builder

	handled, err := taiplugin.HelpSummary(&out, []string{"triage", "--help-summary"}, desc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handled {
		t.Fatal("want handled=true for --help-summary")
	}
	if got := out.String(); got != desc+"\n" {
		t.Errorf("stdout = %q, want %q", got, desc+"\n")
	}
}

// Any other invocation must fall through untouched so the plugin's
// own command tree runs — the helper is called before cli.Run, so a
// false positive would swallow every real verb.
func TestHelpSummary_TCSDK002_ignores_other_invocations(t *testing.T) {
	cases := [][]string{
		{"triage"},
		{"triage", "list"},
		{"triage", "--help"},
		{"triage", "list", "--status", "pending"},
		// Only the exact flag counts; a verb that merely contains the
		// word must not be hijacked.
		{"triage", "help-summary"},
	}
	for _, args := range cases {
		var out strings.Builder
		handled, err := taiplugin.HelpSummary(&out, args, desc)
		if err != nil {
			t.Fatalf("%v: unexpected error: %v", args, err)
		}
		if handled {
			t.Errorf("%v: want handled=false", args)
		}
		if out.Len() != 0 {
			t.Errorf("%v: wrote %q, want nothing", args, out.String())
		}
	}
}

// The host rejects an empty or multi-line summary, so the SDK
// normalises what it can (surrounding whitespace, a trailing
// newline) and refuses what it cannot, surfacing an author error at
// the plugin's own boundary rather than shipping a broken plugin
// that only fails at install time on someone else's machine.
func TestHelpSummary_TCSDK003_normalises_and_rejects_bad_descriptions(t *testing.T) {
	t.Run("trims surrounding whitespace and a trailing newline", func(t *testing.T) {
		var out strings.Builder
		if _, err := taiplugin.HelpSummary(&out, []string{"p", "--help-summary"}, "  "+desc+"\n"); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != desc+"\n" {
			t.Errorf("stdout = %q, want %q", got, desc+"\n")
		}
	})

	t.Run("keeps only the first line", func(t *testing.T) {
		var out strings.Builder
		if _, err := taiplugin.HelpSummary(&out, []string{"p", "--help-summary"}, desc+"\nsecond line"); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != desc+"\n" {
			t.Errorf("stdout = %q, want the first line only", got)
		}
	})

	t.Run("empty description is an author error", func(t *testing.T) {
		var out strings.Builder
		handled, err := taiplugin.HelpSummary(&out, []string{"p", "--help-summary"}, "   ")
		if !handled {
			t.Error("want handled=true — the verb was requested even though the answer is broken")
		}
		if err == nil {
			t.Fatal("want an error for an empty description")
		}
		if e, ok := errcode.As(err); !ok || e.Code != errcode.PluginHelpSummaryFailed {
			t.Errorf("want PLUGIN_HELP_SUMMARY_FAILED, got %v", err)
		}
		if out.Len() != 0 {
			t.Errorf("wrote %q, want nothing on the error path", out.String())
		}
	})
}
