package importer

import (
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/pkg/errcode"
	payload "github.com/dmastrorillo/tai/plugins/triage/internal/import/payload"
)

func cmt(title, file, lines string, refs ...payload.ExternalRef) payload.Comment {
	return payload.Comment{
		ExternalRefs: refs, Title: title, File: file, Lines: lines,
		Severity: "minor", Category: "code-quality", Source: "review",
		Description: "d", WhyFix: "w", SuggestedFix: "s", Consequences: "c",
	}
}

func ref(kind, id string) payload.ExternalRef { return payload.ExternalRef{Kind: kind, ID: id} }

func ids(cs []payload.Comment) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		for _, r := range c.ExternalRefs {
			out = append(out, r.ID)
		}
	}
	return out
}

// A ref that identifies exactly one comment is already unique — an
// inline GitHub comment carries its own id and its own permalink — so
// it must pass through untouched. Suffixing it would break re-import
// against every row already in the database.
func TestDisambiguateRefs_leaves_unique_refs_untouched(t *testing.T) {
	cs := []payload.Comment{
		cmt("A", "a.go", "1", ref("github-pr-comment", "3386158470")),
		cmt("B", "b.go", "2", ref("github-pr-comment", "3386158594")),
	}
	if err := DisambiguateRefs(cs); err != nil {
		t.Fatal(err)
	}
	want := []string{"3386158470", "3386158594"}
	for i, g := range ids(cs) {
		if g != want[i] {
			t.Errorf("id[%d] = %q, want %q", i, g, want[i])
		}
	}
}

// One review body holding several findings is a single GitHub object,
// so every finding arrives carrying the same review id. The importer
// derives the distinguishing suffix itself — the model that produced
// the payload never invents one.
func TestDisambiguateRefs_suffixes_shared_refs(t *testing.T) {
	cs := []payload.Comment{
		cmt("redactedKeys is discarded at startup", "app/utils/bugsnag.js", "67", ref("github-review-body", "5015957736")),
		cmt("Report still ships queued attendee PII", "app/record-view.js", "363", ref("github-review-body", "5015957736")),
		cmt("Tests assert the constant against itself", "app/utils/bugsnag.test.js", "124", ref("github-review-body", "5015957736")),
	}
	if err := DisambiguateRefs(cs); err != nil {
		t.Fatal(err)
	}

	got := ids(cs)
	seen := map[string]bool{}
	for i, g := range got {
		if !strings.HasPrefix(g, "5015957736#") {
			t.Errorf("id[%d] = %q, want the review id plus a suffix", i, g)
		}
		if seen[g] {
			t.Errorf("id[%d] = %q collides with an earlier comment", i, g)
		}
		seen[g] = true
	}
	if len(seen) != 3 {
		t.Fatalf("want 3 distinct ids, got %d: %v", len(seen), got)
	}
}

// Re-importing the same JSON must produce byte-identical refs, or the
// second import duplicates every row instead of updating it.
func TestDisambiguateRefs_is_deterministic_and_order_independent(t *testing.T) {
	build := func() []payload.Comment {
		return []payload.Comment{
			cmt("finding one", "a.go", "1", ref("github-review-body", "500")),
			cmt("finding two", "b.go", "2", ref("github-review-body", "500")),
		}
	}
	first, second := build(), build()
	if err := DisambiguateRefs(first); err != nil {
		t.Fatal(err)
	}
	if err := DisambiguateRefs(second); err != nil {
		t.Fatal(err)
	}
	for i := range ids(first) {
		if ids(first)[i] != ids(second)[i] {
			t.Fatalf("run 2 differs at %d: %q vs %q", i, ids(first)[i], ids(second)[i])
		}
	}

	// The suffix must follow the finding, not its position, so a
	// re-extraction that reorders findings still updates in place.
	shuffled := []payload.Comment{
		cmt("finding two", "b.go", "2", ref("github-review-body", "500")),
		cmt("finding one", "a.go", "1", ref("github-review-body", "500")),
	}
	if err := DisambiguateRefs(shuffled); err != nil {
		t.Fatal(err)
	}
	if got, want := ids(shuffled)[1], ids(first)[0]; got != want {
		t.Errorf("reordering changed the suffix for \"finding one\": %q vs %q", got, want)
	}
}

// Two comments identical in both source ref and identity cannot be
// told apart by any derived key — that is a real duplicate in the
// payload, and the fix belongs to whoever produced it.
func TestDisambiguateRefs_rejects_true_duplicates_with_instructions(t *testing.T) {
	cs := []payload.Comment{
		cmt("same finding", "a.go", "1", ref("github-review-body", "500")),
		cmt("other finding", "b.go", "2", ref("github-review-body", "500")),
		cmt("same finding", "a.go", "1", ref("github-review-body", "500")),
	}

	err := DisambiguateRefs(cs)
	if err == nil {
		t.Fatal("want an error for two identical comments sharing a ref")
	}
	e, ok := errcode.As(err)
	if !ok || e.Code != errcode.ImportDuplicateRefs {
		t.Fatalf("want IMPORT_DUPLICATE_REFS, got %v", err)
	}
	// The message must name the colliding comments so the author can
	// act without re-deriving the analysis.
	for _, want := range []string{"comments[0]", "comments[2]", "same finding"} {
		if !strings.Contains(e.Msg, want) {
			t.Errorf("message %q does not mention %q", e.Msg, want)
		}
	}
	if len(e.Help) == 0 {
		t.Error("want remediation help bullets")
	}
}
