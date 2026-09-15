// Package assets_test pins the content contract of the markdown the
// triage plugin ships for installation into target directories.
//
// The host copies `assets/commands/*.md` into
// `<target>/commands/tai-triage/`, which is what makes them reachable
// as `/tai-triage:<verb>`. A file that tells the reader to invoke
// itself under any other name sends them to a command that does not
// exist, and nothing else in the pipeline would catch it — the host
// copies bytes without reading them.
package assets_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// commandsDir is the tree the plugin tarball ships and the host
// copies from.
const commandsDir = "commands"

// staleRef matches a slash-command reference in the pre-plugin-host
// namespace: `/tai:<verb>`. The host now routes these files into a
// `tai-triage/` subdirectory, so `/tai-triage:<verb>` is the only
// name that resolves.
var staleRef = regexp.MustCompile(`/tai:[a-z-]+`)

// staleInvocation matches a CLI invocation in the pre-plugin-host
// form: `tai <triage-verb>`. These verbs moved out of the core binary
// when triage became a plugin, so the only form that runs today is
// `tai triage <verb>`. The negative lookahead Go's regexp lacks is
// unnecessary here — the corrected form puts the plugin name between
// `tai` and the verb, so it never matches this pattern.
//
// Checked separately from staleRef because the two drifted
// independently: an earlier pass fixed every `/tai:` reference and
// left all 58 CLI invocations behind, which this test could not see.
var staleInvocation = regexp.MustCompile(`\btai (status|list|show|accept|dismiss|complete|forget|import)\b`)

// TC-AST-001 — every bundled command addresses itself by its
// installed slash-command name.
func TestBundledCommands_TCAST001_use_the_plugin_namespace(t *testing.T) {
	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		t.Fatal(err)
	}

	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		checked++
		t.Run(e.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(commandsDir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			body := string(data)
			if found := staleRef.FindAllString(body, -1); len(found) > 0 {
				t.Errorf("%d stale slash-command reference(s): %v\n"+
					"the host installs these under `tai-triage/`, so they must read `/tai-triage:<verb>`",
					len(found), unique(found))
			}
			if found := staleInvocation.FindAllString(body, -1); len(found) > 0 {
				t.Errorf("%d stale CLI invocation(s): %v\n"+
					"these verbs live in the triage plugin, so they must read `tai triage <verb>`",
					len(found), unique(found))
			}
		})
	}

	// A rename or a move that empties the tree would otherwise let
	// this test pass by checking nothing.
	if checked == 0 {
		t.Fatalf("no command markdown found in %s/ — the tarball ships this tree", commandsDir)
	}
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// TC-AST-003 — the triage loop investigates before it asks.
//
// A decision prompt is only as good as what precedes it. Echoing the
// stored record puts the reviewer's original wording in front of the
// user unchecked, including a cause nobody confirmed and a fix that
// may not fit the code as it stands today. The loop has to open the
// file first and present what it found.
func TestTriageCommand_TCAST003_presents_an_investigated_review(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(commandsDir, "triage.md"))
	if err != nil {
		t.Fatalf("read triage.md: %v", err)
	}
	text := string(body)

	// Every field the presentation owes the reader.
	for _, want := range []string{
		"**who raised it**",
		"**file:line**",
		"**description**",
		"**cause**",
		"**why fix it**",
		"**suggested fix**",
		"**concerns if skipped**",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("triage.md must name the %s field in its presentation contract", want)
		}
	}

	// The instruction that made the loop a passthrough.
	if strings.Contains(text, "Surface the markdown verbatim") {
		t.Error("triage.md must not tell the loop to echo `tai triage show` verbatim")
	}

	// Cause is derived, never repeated from the record.
	if !strings.Contains(text, "never taken from the record") {
		t.Error("triage.md must state that cause is investigated, not read from the stored comment")
	}

	// References were deliberately dropped; a reader should not be
	// told to produce one.
	if strings.Contains(text, "**references**") {
		t.Error("triage.md must not ask for a references field")
	}
}

// TestTriageCommand_TCAST004_documents_the_board_briefing pins the board
// contract in the shipped command. The board renders what it is briefed
// with and derives nothing, so an AI that has to guess the schema, or
// that copies stored fields into it, produces a surface the loop's own
// presentation contract forbids.
func TestTriageCommand_TCAST004_documents_the_board_briefing(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(commandsDir, "triage.md"))
	if err != nil {
		t.Fatalf("read triage.md: %v", err)
	}
	text := string(body)
	// Prose assertions match against a whitespace-collapsed copy so a
	// reflow of the markdown does not fail a test about its meaning.
	flat := strings.Join(strings.Fields(text), " ")

	// Every field the briefing carries, so the AI never has to guess.
	for _, want := range []string{
		`"raised_by"`, `"location"`, `"description"`, `"cause"`,
		`"why_fix"`, `"suggested_fix"`, `"suggested_fix_origin"`,
		`"concerns_if_skipped"`, `"batch_key"`, `"severity"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("triage.md must document the %s briefing field", want)
		}
	}

	// The board is offered, never assumed, and only above the threshold.
	if !strings.Contains(flat, "more than five") {
		t.Error("triage.md must state the threshold below which the board is not mentioned")
	}
	if !strings.Contains(flat, "Do NOT launch the board unless the user accepts") {
		t.Error("triage.md must state that the board is offered, not assumed")
	}

	// The investigation is moved ahead of the presentation, not replaced.
	if !strings.Contains(flat, "moved ahead of the presentation, not replaced by it") {
		t.Error("triage.md must say the board reuses the loop's investigation rather than skipping it")
	}

	// An intent carries no special obligations.
	if !strings.Contains(flat, "exactly equivalent to the user having typed that answer") {
		t.Error("triage.md must state the intent-equivalence rule")
	}
	if !strings.Contains(flat, "MUST NOT introduce any obligation that applies to an intent") {
		t.Error("triage.md must forbid intent-only obligations and exemptions")
	}

	// The board is launched and then waited on, never polled: the user
	// is in the conversation and says when they have submitted.
	if !strings.Contains(flat, "Do NOT poll") {
		t.Error("triage.md must tell the loop not to poll for the submission")
	}
	if !strings.Contains(flat, "WAIT for them to say so") {
		t.Error("triage.md must tell the loop to wait for the user to say they submitted")
	}

	// Both rejection codes, and the code for a scope with no board.
	for _, want := range []string{
		"TRIAGE_BOARD_INVALID_JSON",
		"TRIAGE_BOARD_SCHEMA_INVALID",
		"TRIAGE_NO_INTENTS",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("triage.md must document %q", want)
		}
	}
}

// TestFixCommand_TCAST005_sends_the_user_to_commit_and_push_before_verify
// pins the handoff between the two commands. /tai-triage:verify reads a
// PR scope's evidence from `gh pr diff`, which sees only pushed commits,
// so a recap naming verify alone sends the user into a run that caps
// every fix they just made at MEDIUM confidence.
func TestFixCommand_TCAST005_sends_the_user_to_commit_and_push_before_verify(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(commandsDir, "fix.md"))
	if err != nil {
		t.Fatalf("read fix.md: %v", err)
	}
	text := string(body)
	flat := strings.Join(strings.Fields(text), " ")

	if !strings.Contains(flat, "Commit and push these, then run `/tai-triage:verify`") {
		t.Error("fix.md's recap must tell the user to commit and push before verifying")
	}
	if !strings.Contains(flat, "which sees only what has been pushed") {
		t.Error("fix.md must explain that verify's evidence comes from the pushed diff")
	}
	// The command still must not do it itself.
	if !strings.Contains(flat, "Do NOT commit, stage, push, or create a PR yourself") {
		t.Error("fix.md must still forbid the command from committing or pushing")
	}
}
