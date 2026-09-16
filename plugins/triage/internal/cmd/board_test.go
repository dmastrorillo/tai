package cmd_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/plugins/triage/internal/board"
	"github.com/dmastrorillo/tai/plugins/triage/internal/cmd"
	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdtest"
)

// validBriefing is the canonical happy-path briefing. Mutations are
// constructed inline per test.
const validBriefing = `{
  "repo": "acme/app",
  "scope": { "kind": "pr", "pr": 142 },
  "batches": [{ "batch_key": "B1", "title": "Replace execSync" }],
  "comments": [
    {
      "id": 1,
      "batch_key": "B1",
      "severity": "critical",
      "raised_by": "coderabbit",
      "location": "src/api/auth.ts:15-29",
      "description": "execSync interpolates user input",
      "cause": "config passes both values into a shell string",
      "why_fix": "shell metacharacters execute",
      "suggested_fix": "use execFileSync with an argv slice",
      "suggested_fix_origin": "reviewer",
      "concerns_if_skipped": "arbitrary command execution on the build host"
    }
  ]
}`

func intentsDirEntries(t *testing.T, env *cmdtest.Isolated) []string {
	t.Helper()
	dir := filepath.Join(env.DataDir, "plugins", "triage", "state", "intents")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("reading the intents directory: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestBoard_TCBRD002_missing_positional_is_a_usage_error(t *testing.T) {
	cmdtest.Isolate(t)
	r := cmdtest.Run(t, cmd.NewRoot(), "board")

	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "UNKNOWN_SUBCOMMAND", 1)
	cmdtest.AssertStderrContains(t, r, "read the briefing from stdin")
}

func TestBoard_TCBRD003_a_positional_other_than_dash_is_a_usage_error(t *testing.T) {
	cmdtest.Isolate(t)
	r := cmdtest.Run(t, cmd.NewRoot(), "board", "142")

	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "UNKNOWN_SUBCOMMAND", 1)
}

func TestBoard_TCBRD004_scope_flags_are_not_accepted(t *testing.T) {
	cmdtest.Isolate(t)
	board.NoBrowserForTesting(t)
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), validBriefing, "board", "-", "--pr", "142")

	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "UNKNOWN_SUBCOMMAND", 1)
	cmdtest.AssertStderrContains(t, r, "scope from the briefing")
}

func TestBoard_TCBRD005_malformed_json_is_rejected(t *testing.T) {
	env := cmdtest.Isolate(t)
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), "this is not json", "board", "-")

	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_BOARD_INVALID_JSON", 1)
	if got := intentsDirEntries(t, env); len(got) != 0 {
		t.Errorf("a rejected briefing must write nothing, found %v", got)
	}
}

func TestBoard_TCBRD006_every_violation_is_reported_at_once(t *testing.T) {
	env := cmdtest.Isolate(t)
	// One comment missing `cause`, one missing `raised_by`, one naming a
	// batch that is not declared.
	briefing := `{
	  "repo": "acme/app",
	  "scope": { "kind": "pr", "pr": 142 },
	  "batches": [],
	  "comments": [
	    { "id": 1, "severity": "critical", "raised_by": "x", "location": "a:1",
	      "description": "d", "cause": "", "why_fix": "w", "suggested_fix": "s",
	      "suggested_fix_origin": "reviewer", "concerns_if_skipped": "k" },
	    { "id": 2, "severity": "major", "raised_by": "", "location": "a:2",
	      "description": "d", "cause": "c", "why_fix": "w", "suggested_fix": "s",
	      "suggested_fix_origin": "reviewer", "concerns_if_skipped": "k" },
	    { "id": 3, "batch_key": "B9", "severity": "minor", "raised_by": "x",
	      "location": "a:3", "description": "d", "cause": "c", "why_fix": "w",
	      "suggested_fix": "s", "suggested_fix_origin": "reviewer",
	      "concerns_if_skipped": "k" }
	  ]
	}`
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), briefing, "board", "-")

	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_BOARD_SCHEMA_INVALID", 3)
	for _, path := range []string{
		"comments[0].cause",
		"comments[1].raised_by",
		"comments[2].batch_key",
	} {
		cmdtest.AssertStderrContains(t, r, path)
	}
	if got := intentsDirEntries(t, env); len(got) != 0 {
		t.Errorf("a rejected briefing must write nothing, found %v", got)
	}
}

func TestBoard_TCBRD007_an_unknown_field_is_rejected(t *testing.T) {
	cmdtest.Isolate(t)
	briefing := strings.Replace(validBriefing,
		`"id": 1,`, `"id": 1, "not_a_field": true,`, 1)
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), briefing, "board", "-")

	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_BOARD_INVALID_JSON", 1)
}

func TestBoardIntents_TCBRD026_emits_the_artifact_as_markdown(t *testing.T) {
	env := cmdtest.Isolate(t)
	seedPRScope(t, env)

	n := 142
	scope := board.Scope{Kind: "pr", PR: &n}
	if _, err := board.WriteIntents(board.NewIntents("acme/app", scope, []board.IntentEntry{
		{ID: 1, Intent: board.IntentAccept, Note: "use an argv slice"},
		{ID: 2, Intent: board.IntentUnanswered, Note: "explain this one to me"},
	})); err != nil {
		t.Fatalf("seeding intents: %v", err)
	}

	r := cmdtest.Run(t, cmd.NewRoot(), "board", "intents", "--repo", "acme/app", "--pr", "142")

	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Submitted at")
	cmdtest.AssertStdoutContains(t, r, "1: accept — use an argv slice")
	cmdtest.AssertStdoutContains(t, r, "2: unanswered — explain this one to me")
}

func TestBoardIntents_TCBRD027_no_artifact_surfaces_TRIAGE_NO_INTENTS(t *testing.T) {
	env := cmdtest.Isolate(t)
	seedPRScope(t, env)

	r := cmdtest.Run(t, cmd.NewRoot(), "board", "intents", "--repo", "acme/app", "--pr", "142")

	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_NO_INTENTS", 2)
	if got := intentsDirEntries(t, env); len(got) != 0 {
		t.Errorf("reading intents must not create one, found %v", got)
	}
}

func TestBoardIntents_TCBRD028_reading_is_repeatable_and_non_destructive(t *testing.T) {
	env := cmdtest.Isolate(t)
	seedPRScope(t, env)

	n := 142
	scope := board.Scope{Kind: "pr", PR: &n}
	if _, err := board.WriteIntents(board.NewIntents("acme/app", scope,
		[]board.IntentEntry{{ID: 1, Intent: board.IntentAccept}})); err != nil {
		t.Fatalf("seeding intents: %v", err)
	}

	first := cmdtest.Run(t, cmd.NewRoot(), "board", "intents", "--repo", "acme/app", "--pr", "142")
	second := cmdtest.Run(t, cmd.NewRoot(), "board", "intents", "--repo", "acme/app", "--pr", "142")

	cmdtest.AssertNoError(t, first)
	cmdtest.AssertNoError(t, second)
	if first.Stdout != second.Stdout {
		t.Errorf("repeated reads must be identical:\nfirst:\n%s\nsecond:\n%s",
			first.Stdout, second.Stdout)
	}
	if got := intentsDirEntries(t, env); len(got) != 1 {
		t.Errorf("the artifact must survive both reads, found %v", got)
	}
}

// seedPRScope imports a minimal payload so a PR scope exists for the
// board-intents verb to resolve. The board itself never needs this —
// it holds no database handle — but `board intents` resolves scope the
// way every other triage verb does.
func seedPRScope(t *testing.T, _ *cmdtest.Isolated) {
	t.Helper()
	payload := `{
	  "repo": "acme/app",
	  "target": { "kind": "pr", "pr": { "number": 142, "title": "feat: oauth",
	    "url": "https://x", "head_branch": "feat/oauth" } },
	  "batches": [],
	  "comments": [
	    { "external_refs": [{ "kind": "github-pr-comment", "id": "1" }],
	      "severity": "critical", "category": "security", "file": "a.ts",
	      "lines": "1", "source": "coderabbit", "title": "t", "description": "d",
	      "why_fix": "w", "suggested_fix": "s", "consequences": "c" }
	  ]
	}`
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)
}

// TestBoard_TCBRD032_repo_flag_is_not_accepted pins the second of the two
// guards keeping the board's scope sourced from the briefing rather than
// from flags. The other one — --pr / --branch — is covered by TC-BRD-004.
func TestBoard_TCBRD032_repo_flag_is_not_accepted(t *testing.T) {
	cmdtest.Isolate(t)
	board.NoBrowserForTesting(t)

	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), validBriefing, "--repo", "acme/app", "board", "-")

	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "UNKNOWN_SUBCOMMAND", 1)
	cmdtest.AssertStderrContains(t, r, "repo identity is read from the briefing")
}

// captureBriefing swaps the spawn seam for one that records the briefing
// it was handed and reports a board at url, so a test can drive the
// launching command without a real process being spawned.
func captureBriefing(t *testing.T, url string) <-chan []byte {
	t.Helper()
	got := make(chan []byte, 1)
	board.HandOffForTesting(t, func(briefing []byte) (string, error) {
		got <- append([]byte(nil), briefing...)
		return url, nil
	})
	return got
}

func TestBoard_TCBRD033_returns_without_waiting_for_a_submit(t *testing.T) {
	cmdtest.Isolate(t)
	const url = "http://127.0.0.1:45671/b/0123456789abcdef0123456789abcdef/"
	got := captureBriefing(t, url)

	done := make(chan cmdtest.Result, 1)
	go func() {
		done <- cmdtest.RunWithStdin(t, cmd.NewRoot(), validBriefing, "board", "-")
	}()

	var r cmdtest.Result
	select {
	case r = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("`board -` never returned: it must exit once the board is serving, " +
			"not block until the developer submits")
	}

	cmdtest.AssertNoError(t, r)
	cmdtest.AssertStdoutContains(t, r, "Board ready at "+url)
	// The command no longer reports the outcome, so it has to say where
	// the outcome will be readable.
	cmdtest.AssertStdoutContains(t, r, "tai triage board intents --pr 142")

	select {
	case briefing := <-got:
		if !bytes.Contains(briefing, []byte(`"repo": "acme/app"`)) {
			t.Errorf("the briefing did not reach the server process, got %q", briefing)
		}
	default:
		t.Fatal("no server process was ever handed the briefing")
	}
}

func TestBoard_TCBRD034_a_server_that_cannot_be_spawned_surfaces_the_code(t *testing.T) {
	env := cmdtest.Isolate(t)
	board.HandOffForTesting(t, func([]byte) (string, error) {
		return "", errors.New("bind: permission denied")
	})

	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), validBriefing, "board", "-")

	cmdtest.AssertError(t, r)
	cmdtest.AssertErrorFooter(t, r, "TRIAGE_BOARD_UNAVAILABLE", 3)
	cmdtest.AssertStderrContains(t, r, "bind: permission denied")
	if strings.Contains(r.Stdout, "Board ready") {
		t.Errorf("a URL was announced for a board nothing is serving: %q", r.Stdout)
	}
	if names := intentsDirEntries(t, env); len(names) != 0 {
		t.Errorf("a launch that never served wrote %v", names)
	}
}
