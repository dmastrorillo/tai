package cliout_test

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/internal/cliout"
	"github.com/dmastrorillo/tai/internal/errcode"
)

var footerRe = regexp.MustCompile(`(?m)^\[exit \d+: [A-Z][A-Z0-9_]*\]$`)

// TestWriteError_TCERR002_template_with_help exercises TC-ERR-002 from
// test-cases.md: an *errcode.Error with Help bullets renders the full
// template — Error line, "What to do" block, footer.
func TestWriteError_TCERR002_template_with_help(t *testing.T) {
	var buf bytes.Buffer
	cliout.WriteError(&buf, errcode.New(errcode.RepoNotFound,
		"not in a git repository with an origin remote").
		WithHelp(
			"cd into a git repository, or",
			"pass --repo <owner/name> to specify explicitly",
		))

	got := buf.String()

	assertContains(t, got, "Error: not in a git repository with an origin remote\n")
	assertContains(t, got, "What to do:\n")
	assertContains(t, got, "  • cd into a git repository, or\n")
	assertContains(t, got, "  • pass --repo <owner/name> to specify explicitly\n")
	assertContains(t, got, "[exit 2: REPO_NOT_FOUND]\n")

	// Footer is the last non-empty line.
	lastLine := lastNonEmptyLine(got)
	if !footerRe.MatchString(lastLine) {
		t.Fatalf("last line %q does not match footer regex", lastLine)
	}
}

// TestWriteError_TCERR003_internal_error_omits_help exercises TC-ERR-003:
// an *errcode.Error with no Help bullets omits the "What to do" block
// entirely. This is the path recovered panics take.
func TestWriteError_TCERR003_internal_error_omits_help(t *testing.T) {
	var buf bytes.Buffer
	cliout.WriteError(&buf, errcode.Newf(errcode.InternalError, "panic: boom"))

	got := buf.String()

	assertContains(t, got, "Error: panic: boom\n")
	assertContains(t, got, "[exit 70: INTERNAL_ERROR]\n")

	if strings.Contains(got, "What to do:") {
		t.Fatalf("INTERNAL_ERROR without Help bullets should omit the 'What to do:' block; got:\n%s", got)
	}
}

// TestWriteError_TCERR004_footer_regex_invariant exercises TC-ERR-004:
// every error path's stderr ends with a footer matching the contract
// regex, regardless of error type.
func TestWriteError_TCERR004_footer_regex_invariant(t *testing.T) {
	cases := []error{
		errcode.New(errcode.RepoNotFound, "msg"),
		errcode.New(errcode.RepoFlagInvalid, "bad"),
		errcode.Newf(errcode.InternalError, "x %d", 1),
		errors.New("a non-structured error"),
	}

	for _, err := range cases {
		var buf bytes.Buffer
		cliout.WriteError(&buf, err)
		last := lastNonEmptyLine(buf.String())
		if !footerRe.MatchString(last) {
			t.Fatalf("for err %v, last line %q does not match footer regex", err, last)
		}
	}
}

// TestWriteError_TCERR007_unstructured_error_becomes_internal_error
// exercises TC-ERR-007: any non-*errcode.Error is surfaced as
// INTERNAL_ERROR with the original message as the summary.
func TestWriteError_TCERR007_unstructured_error_becomes_internal_error(t *testing.T) {
	var buf bytes.Buffer
	cliout.WriteError(&buf, errors.New("something exploded"))

	got := buf.String()

	assertContains(t, got, "Error: something exploded\n")
	assertContains(t, got, "[exit 70: INTERNAL_ERROR]\n")
}

// TestWriteError_TCERR008_multiline_message_collapsed_to_single_line
// exercises TC-ERR-008: an error message containing embedded newlines
// is collapsed to a single "Error:" line so the template's invariant
// (summary is one line) holds.
func TestWriteError_TCERR008_multiline_message_collapsed_to_single_line(t *testing.T) {
	var buf bytes.Buffer
	cliout.WriteError(&buf, errors.New("first line\nsecond line\nthird line"))

	got := buf.String()

	// The "Error:" line must contain all three pieces on one line.
	lines := strings.Split(got, "\n")
	var errorLine string
	for _, l := range lines {
		if strings.HasPrefix(l, "Error:") {
			errorLine = l
			break
		}
	}
	if errorLine == "" {
		t.Fatalf("no Error: line in output:\n%s", got)
	}
	if strings.Contains(errorLine, "\n") {
		t.Fatalf("Error line still contains a newline: %q", errorLine)
	}
	for _, piece := range []string{"first line", "second line", "third line"} {
		if !strings.Contains(errorLine, piece) {
			t.Fatalf("Error line %q should contain %q", errorLine, piece)
		}
	}
}

// TestWriteError_nil_is_noop is an engine test: it verifies the
// defensive nil-error branch produces no output. Not tied to a BDD
// case because the user never invokes WriteError with nil; this is a
// caller-correctness guardrail.
func TestWriteError_nil_is_noop(t *testing.T) {
	var buf bytes.Buffer
	cliout.WriteError(&buf, nil)
	if buf.Len() != 0 {
		t.Fatalf("expected no output for nil error, got %q", buf.String())
	}
}

func assertContains(t *testing.T, got, substr string) {
	t.Helper()
	if !strings.Contains(got, substr) {
		t.Fatalf("expected output to contain %q\noutput:\n%s", substr, got)
	}
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i] != "" {
			return lines[i]
		}
	}
	return ""
}
