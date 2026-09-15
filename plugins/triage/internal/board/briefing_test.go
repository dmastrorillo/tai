package board

import (
	"strings"
	"testing"
)

func comment(id int, sev, batch string) Comment {
	return Comment{
		ID: id, Severity: sev, BatchKey: batch,
		RaisedBy: "coderabbit", Location: "a.ts:1-2",
		Description: "d", Cause: "c", WhyFix: "w",
		SuggestedFix: "s", SuggestedFixOrigin: "reviewer",
		ConcernsIfSkipped: "k",
	}
}

func prScope(n int) Scope { return Scope{Kind: "pr", PR: &n} }

func TestValidate_TCBRD006_reports_every_violation_at_once(t *testing.T) {
	missingCause := comment(1, "critical", "")
	missingCause.Cause = ""
	missingRaisedBy := comment(2, "major", "")
	missingRaisedBy.RaisedBy = ""
	undeclaredBatch := comment(3, "minor", "B9")

	errs := Validate(Briefing{
		Repo:     "acme/app",
		Scope:    prScope(142),
		Comments: []Comment{missingCause, missingRaisedBy, undeclaredBatch},
	})

	if len(errs) != 3 {
		t.Fatalf("want 3 violations reported together, got %d: %+v", len(errs), errs)
	}
	wantPaths := []string{
		"comments[0].cause",
		"comments[1].raised_by",
		"comments[2].batch_key",
	}
	for _, want := range wantPaths {
		var found bool
		for _, e := range errs {
			if e.Path == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no violation reported at path %q; got %+v", want, errs)
		}
	}
}

func TestValidate_TCBRD007_rejects_an_unknown_field(t *testing.T) {
	_, err := DecodeBytes([]byte(`{
	  "repo": "acme/app",
	  "scope": {"kind":"pr","pr":1},
	  "batches": [],
	  "comments": [{"id":1,"severity":"minor","raised_by":"x","location":"a:1",
	    "description":"d","cause":"c","why_fix":"w","suggested_fix":"s",
	    "suggested_fix_origin":"reviewer","concerns_if_skipped":"k",
	    "not_a_field":"boom"}]
	}`))
	if err == nil {
		t.Fatal("want the decoder to reject an unknown comment field, got nil")
	}
	if !strings.Contains(err.Error(), "not_a_field") {
		t.Errorf("error should name the offending field, got %q", err)
	}
}

func TestValidate_TCBRD007_accepts_a_minimal_briefing(t *testing.T) {
	errs := Validate(Briefing{
		Repo:     "acme/app",
		Scope:    prScope(142),
		Comments: []Comment{comment(1, "critical", "")},
	})
	if len(errs) != 0 {
		t.Fatalf("want no violations, got %+v", errs)
	}
}

func TestValidate_TCBRD006_rejects_bad_enums_and_scope(t *testing.T) {
	bad := comment(1, "blocker", "")
	bad.SuggestedFixOrigin = "somewhere"

	errs := Validate(Briefing{
		Repo:     "acme-app",
		Scope:    Scope{Kind: "tag"},
		Comments: []Comment{bad},
	})

	want := map[string]bool{
		"repo":                             false,
		"scope.kind":                       false,
		"comments[0].severity":             false,
		"comments[0].suggested_fix_origin": false,
	}
	for _, e := range errs {
		if _, ok := want[e.Path]; ok {
			want[e.Path] = true
		}
	}
	for path, seen := range want {
		if !seen {
			t.Errorf("expected a violation at %q; got %+v", path, errs)
		}
	}
}

func TestOrder_TCBRD030_batches_first_then_severity_then_id(t *testing.T) {
	b := Briefing{
		Repo:  "acme/app",
		Scope: prScope(1),
		Batches: []Batch{
			{BatchKey: "B2", Title: "second"},
			{BatchKey: "B1", Title: "first"},
			{BatchKey: "B3", Title: "third"},
		},
		Comments: []Comment{
			comment(10, "minor", ""),
			comment(9, "minor", ""),
			comment(1, "major", "B2"),
			comment(2, "major", "B1"),
			comment(3, "critical", "B3"),
		},
	}

	groups := Order(b)

	if len(groups) != 5 {
		t.Fatalf("want 3 batches + 2 loose comments = 5 groups, got %d", len(groups))
	}
	if groups[0].Batch == nil || groups[0].Batch.BatchKey != "B3" {
		t.Errorf("the critical batch sorts first, got %+v", groups[0].Batch)
	}
	// B1 and B2 are both major: the tie breaks on batch key ascending.
	if groups[1].Batch.BatchKey != "B1" || groups[2].Batch.BatchKey != "B2" {
		t.Errorf("equal-severity batches order by key ascending, got %s then %s",
			groups[1].Batch.BatchKey, groups[2].Batch.BatchKey)
	}
	if groups[3].Batch != nil || groups[4].Batch != nil {
		t.Fatal("non-batched comments must follow every batch")
	}
	// Equal severity: the tie breaks on id ascending.
	if groups[3].Comments[0].ID != 9 || groups[4].Comments[0].ID != 10 {
		t.Errorf("equal-severity loose comments order by id ascending, got %d then %d",
			groups[3].Comments[0].ID, groups[4].Comments[0].ID)
	}
}
