package cmd_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdtest"
)

// idForTitle returns the ID column of the row whose title matches.
func idForTitle(t *testing.T, out, title string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, title) {
			continue
		}
		if m := regexp.MustCompile(`^\s*(\d+)\s`).FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	t.Fatalf("no row titled %q in:\n%s", title, out)
	return ""
}

// TC-TRG-105 — a comment's position is a property of the comment, not
// of the query that listed it.
//
// The position comes from a ROW_NUMBER() window. Applied after a
// status filter it numbers only the matching rows, so a filtered list
// and `show` disagree — and the disagreement misdirects rather than
// erroring: an id read from `list --status accepted` resolves to a
// different comment entirely, including one that was just dismissed.
func TestList_TCTRG105_positions_are_stable_across_status_filters(t *testing.T) {
	cmdtest.Isolate(t)
	seedPR(t, 1,
		commentJSON("r1", "first", "critical", "pending")+","+
			commentJSON("r2", "second", "major", "pending")+","+
			commentJSON("r3", "third", "minor", "pending"))

	// Dismiss the middle one so a filtered listing has a gap in it.
	cmdtest.AssertNoError(t, triage(t, "dismiss", "2", "--pr", "1", "--reason", "not a real issue"))

	full := triage(t, "list", "--pr", "1")
	cmdtest.AssertNoError(t, full)
	filtered := triage(t, "list", "--status", "pending", "--pr", "1")
	cmdtest.AssertNoError(t, filtered)

	for _, title := range []string{"first", "third"} {
		want := idForTitle(t, full.Stdout, title)
		got := idForTitle(t, filtered.Stdout, title)
		if got != want {
			t.Errorf("%q is id %s unfiltered but id %s when filtered — an id read from a "+
				"filtered list resolves to the wrong comment", title, want, got)
		}
	}

	// The failure that matters: feed a filtered id straight to show.
	id := idForTitle(t, filtered.Stdout, "third")
	shown := triage(t, "show", id, "--pr", "1")
	cmdtest.AssertNoError(t, shown)
	cmdtest.AssertStdoutContains(t, shown, "third")
}
