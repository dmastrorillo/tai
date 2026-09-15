package board

import (
	"path/filepath"
	"strings"
	"testing"
)

// isolateDataDir points datadir.Resolve at a per-test directory.
func isolateDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TAI_DATA_DIR", dir)
	t.Setenv("XDG_DATA_HOME", "")
	return dir
}

func TestIntents_TCBRD020_records_all_three_intents(t *testing.T) {
	isolateDataDir(t)
	scope := prScope(142)

	if _, err := WriteIntents(NewIntents("acme/app", scope, []IntentEntry{
		{ID: 1, Intent: IntentAccept},
		{ID: 2, Intent: IntentDismiss, Note: "covered by the sandbox"},
		{ID: 3, Intent: IntentUnanswered},
	})); err != nil {
		t.Fatalf("WriteIntents: %v", err)
	}

	got, found, err := ReadIntents("acme/app", scope)
	if err != nil || !found {
		t.Fatalf("ReadIntents: found=%v err=%v", found, err)
	}
	if len(got.Entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(got.Entries))
	}
	if got.Entries[0].Intent != IntentAccept ||
		got.Entries[1].Intent != IntentDismiss ||
		got.Entries[2].Intent != IntentUnanswered {
		t.Errorf("intents round-tripped wrong: %+v", got.Entries)
	}
	if got.Entries[1].Note != "covered by the sandbox" {
		t.Errorf("dismiss note lost: %q", got.Entries[1].Note)
	}
	if got.SubmittedAt == "" {
		t.Error("submitted_at must be stamped")
	}
}

func TestIntents_TCBRD021_keeps_a_note_on_an_unanswered_comment(t *testing.T) {
	isolateDataDir(t)
	scope := prScope(7)

	if _, err := WriteIntents(NewIntents("acme/app", scope, []IntentEntry{
		{ID: 4, Intent: IntentUnanswered, Note: "explain this one to me"},
	})); err != nil {
		t.Fatalf("WriteIntents: %v", err)
	}

	got, _, err := ReadIntents("acme/app", scope)
	if err != nil {
		t.Fatalf("ReadIntents: %v", err)
	}
	if got.Entries[0].Note != "explain this one to me" {
		t.Errorf("an unanswered note is what the developer wants from the "+
			"conversation about that comment; it was lost: %+v", got.Entries[0])
	}
}

func TestIntents_TCBRD023_resubmitting_replaces_the_artifact(t *testing.T) {
	isolateDataDir(t)
	scope := prScope(142)

	if _, err := WriteIntents(NewIntents("acme/app", scope, []IntentEntry{
		{ID: 1, Intent: IntentAccept},
		{ID: 2, Intent: IntentAccept},
	})); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := WriteIntents(NewIntents("acme/app", scope, []IntentEntry{
		{ID: 1, Intent: IntentDismiss, Note: "changed my mind"},
	})); err != nil {
		t.Fatalf("second write: %v", err)
	}

	got, _, err := ReadIntents("acme/app", scope)
	if err != nil {
		t.Fatalf("ReadIntents: %v", err)
	}
	if len(got.Entries) != 1 || got.Entries[0].Intent != IntentDismiss {
		t.Errorf("the second submission must replace the first in full, got %+v", got.Entries)
	}
}

func TestIntents_TCBRD024_artifacts_are_scope_keyed(t *testing.T) {
	isolateDataDir(t)

	if _, err := WriteIntents(NewIntents("acme/app", prScope(142),
		[]IntentEntry{{ID: 1, Intent: IntentAccept}})); err != nil {
		t.Fatalf("write 142: %v", err)
	}
	if _, err := WriteIntents(NewIntents("acme/app", prScope(200),
		[]IntentEntry{{ID: 1, Intent: IntentDismiss}})); err != nil {
		t.Fatalf("write 200: %v", err)
	}

	a, _, _ := ReadIntents("acme/app", prScope(142))
	b, _, _ := ReadIntents("acme/app", prScope(200))
	if a.Entries[0].Intent != IntentAccept || b.Entries[0].Intent != IntentDismiss {
		t.Error("one scope's artifact was read for another")
	}
}

func TestIntents_TCBRD027_absence_is_reported_not_errored(t *testing.T) {
	isolateDataDir(t)

	_, found, err := ReadIntents("acme/app", prScope(999))
	if err != nil {
		t.Fatalf("absence must not be an error: %v", err)
	}
	if found {
		t.Error("want found=false for a scope with no artifact")
	}
}

func TestIntents_TCBRD025_a_branch_with_a_slash_stays_inside_the_directory(t *testing.T) {
	isolateDataDir(t)

	dir, err := IntentsDir()
	if err != nil {
		t.Fatalf("IntentsDir: %v", err)
	}
	path, err := ArtifactPath("acme/app", Scope{Kind: "branch", Branch: "feat/oauth"})
	if err != nil {
		t.Fatalf("ArtifactPath: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("an unslugged slash would place the artifact outside %s; got %s", dir, path)
	}
	if strings.Contains(filepath.Base(path), "/") {
		t.Errorf("the filename still carries a separator: %s", filepath.Base(path))
	}
}

func TestIntents_TCBRD024_pr_and_branch_paths_differ(t *testing.T) {
	isolateDataDir(t)

	pr, err := ArtifactPath("acme/app", prScope(142))
	if err != nil {
		t.Fatalf("ArtifactPath(pr): %v", err)
	}
	br, err := ArtifactPath("acme/app", Scope{Kind: "branch", Branch: "main"})
	if err != nil {
		t.Fatalf("ArtifactPath(branch): %v", err)
	}
	if pr == br {
		t.Fatal("a PR scope and a branch scope must not share an artifact path")
	}
	if !strings.Contains(filepath.Base(pr), "pr-142") {
		t.Errorf("PR path should name the number, got %s", filepath.Base(pr))
	}
	if !strings.Contains(filepath.Base(br), "branch-main") {
		t.Errorf("branch path should name the branch, got %s", filepath.Base(br))
	}
}
