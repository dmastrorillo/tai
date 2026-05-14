package importer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/danielmastrorillo/tai/internal/errcode"
	importer "github.com/danielmastrorillo/tai/internal/import"
	pkgpayload "github.com/danielmastrorillo/tai/internal/import/payload"
	"github.com/danielmastrorillo/tai/internal/storage/storagetest"
)

// basePRPayload returns a well-formed PR payload with one batch and one
// comment. Helpers below mutate the result for specific scenarios.
func basePRPayload() pkgpayload.Payload {
	return pkgpayload.Payload{
		Repo: "acme/app",
		Target: pkgpayload.Target{
			Kind: "pr",
			PR: &pkgpayload.PR{
				Number:     142,
				Title:      "feat: oauth",
				URL:        "https://github.com/acme/app/pull/142",
				HeadBranch: "feat/oauth",
			},
		},
		Batches: []pkgpayload.Batch{
			{BatchKey: "B1", Title: "Replace execSync"},
		},
		Comments: []pkgpayload.Comment{
			{
				ExternalRefs: []pkgpayload.ExternalRef{
					{Kind: "github-pr-comment", ID: "12345", Reviewer: "coderabbit"},
				},
				Severity:     "critical",
				Category:     "security",
				File:         "src/api/auth.ts",
				Lines:        "15-29",
				Source:       "coderabbit",
				Title:        "shell injection",
				Description:  "execSync interpolates user input",
				WhyFix:       "shell metachars get executed",
				SuggestedFix: "use execFileSync",
				Consequences: "RCE",
				BatchKey:     "B1",
			},
		},
	}
}

// TestImport_TCIMP040_creates_repo_and_pr exercises the "first import
// creates repo and target" case.
func TestImport_TCIMP040_creates_repo_and_pr(t *testing.T) {
	db := storagetest.NewMemDB(t)
	p := basePRPayload()

	s, err := importer.Import(context.Background(), db, p)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if s.Inserted != 1 || s.BatchInserted != 1 {
		t.Fatalf("counters: %+v", s)
	}

	var ownerName string
	if err := db.QueryRow(`SELECT owner_name FROM repos`).Scan(&ownerName); err != nil {
		t.Fatalf("repos: %v", err)
	}
	if ownerName != "acme/app" {
		t.Fatalf("repos.owner_name = %q", ownerName)
	}
	var prNum int
	if err := db.QueryRow(`SELECT number FROM prs`).Scan(&prNum); err != nil {
		t.Fatalf("prs: %v", err)
	}
	if prNum != 142 {
		t.Fatalf("prs.number = %d", prNum)
	}
}

// TestImport_TCIMP041_reimport_preserves_repo_created_at exercises
// the "repo unchanged on re-import" scenario.
func TestImport_TCIMP041_reimport_preserves_repo_created_at(t *testing.T) {
	db := storagetest.NewMemDB(t)
	if _, err := importer.Import(context.Background(), db, basePRPayload()); err != nil {
		t.Fatalf("first Import: %v", err)
	}
	var created1 int64
	_ = db.QueryRow(`SELECT created_at FROM repos`).Scan(&created1)

	if _, err := importer.Import(context.Background(), db, basePRPayload()); err != nil {
		t.Fatalf("second Import: %v", err)
	}
	var created2 int64
	_ = db.QueryRow(`SELECT created_at FROM repos`).Scan(&created2)
	if created1 != created2 {
		t.Fatalf("repo.created_at changed on re-import: %d → %d", created1, created2)
	}
}

// TestImport_TCIMP042_pr_title_not_overwritten exercises "PR title not
// updated on re-import".
func TestImport_TCIMP042_pr_title_not_overwritten(t *testing.T) {
	db := storagetest.NewMemDB(t)
	p := basePRPayload()
	p.Target.PR.Title = "feat: original"
	if _, err := importer.Import(context.Background(), db, p); err != nil {
		t.Fatalf("first Import: %v", err)
	}

	p2 := basePRPayload()
	p2.Target.PR.Title = "feat: changed upstream"
	if _, err := importer.Import(context.Background(), db, p2); err != nil {
		t.Fatalf("second Import: %v", err)
	}

	var title string
	_ = db.QueryRow(`SELECT title FROM prs`).Scan(&title)
	if title != "feat: original" {
		t.Fatalf("PR title changed on re-import: got %q", title)
	}
}

// TestImport_TCIMP043_branch_row_created exercises "branch row created
// on first branch-scope import".
func TestImport_TCIMP043_branch_row_created(t *testing.T) {
	db := storagetest.NewMemDB(t)
	p := pkgpayload.Payload{
		Repo: "acme/app",
		Target: pkgpayload.Target{
			Kind:   "branch",
			Branch: &pkgpayload.Branch{Name: "feat/x"},
		},
	}
	s, err := importer.Import(context.Background(), db, p)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if s.TargetLabel != "branch feat/x" {
		t.Fatalf("label: %q", s.TargetLabel)
	}
	var name string
	_ = db.QueryRow(`SELECT name FROM branches`).Scan(&name)
	if name != "feat/x" {
		t.Fatalf("branches.name = %q", name)
	}
}

// TestImport_TCIMP050_new_batch_inserted exercises "new batch inserted".
func TestImport_TCIMP050_new_batch_inserted(t *testing.T) {
	db := storagetest.NewMemDB(t)
	s, err := importer.Import(context.Background(), db, basePRPayload())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if s.BatchInserted != 1 || s.BatchUpdated != 0 {
		t.Fatalf("batch counters: %+v", s)
	}
}

// TestImport_TCIMP051_existing_batch_title_updated exercises "existing
// batch title updated; status preserved".
func TestImport_TCIMP051_existing_batch_title_updated(t *testing.T) {
	db := storagetest.NewMemDB(t)
	if _, err := importer.Import(context.Background(), db, basePRPayload()); err != nil {
		t.Fatalf("first Import: %v", err)
	}
	// Manually mark the batch as accepted to verify status preservation.
	if _, err := db.Exec(`UPDATE batches SET status='accepted' WHERE batch_key='B1'`); err != nil {
		t.Fatalf("mark accepted: %v", err)
	}

	p := basePRPayload()
	p.Batches[0].Title = "Replace execSync (clarified)"
	s, err := importer.Import(context.Background(), db, p)
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if s.BatchUpdated != 1 || s.BatchInserted != 0 {
		t.Fatalf("batch counters: %+v", s)
	}

	var title, status string
	_ = db.QueryRow(`SELECT title, status FROM batches WHERE batch_key='B1'`).Scan(&title, &status)
	if title != "Replace execSync (clarified)" {
		t.Fatalf("title not updated: %q", title)
	}
	if status != "accepted" {
		t.Fatalf("status changed: %q (want accepted)", status)
	}
}

// TestImport_TCIMP060_new_comment_inserted exercises "new comment
// inserted with status=pending; all refs inserted".
func TestImport_TCIMP060_new_comment_inserted(t *testing.T) {
	db := storagetest.NewMemDB(t)
	s, err := importer.Import(context.Background(), db, basePRPayload())
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if s.Inserted != 1 {
		t.Fatalf("Inserted=%d", s.Inserted)
	}
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM comments WHERE status='pending'`).Scan(&count)
	if count != 1 {
		t.Fatalf("pending comments = %d", count)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM comment_external_refs`).Scan(&count)
	if count != 1 {
		t.Fatalf("refs = %d", count)
	}
}

// TestImport_TCIMP061_pending_refresh exercises "pending comment is
// refreshed: enrichment fields updated, status stays pending".
func TestImport_TCIMP061_pending_refresh(t *testing.T) {
	db := storagetest.NewMemDB(t)
	if _, err := importer.Import(context.Background(), db, basePRPayload()); err != nil {
		t.Fatalf("first Import: %v", err)
	}

	p := basePRPayload()
	p.Comments[0].Description = "rewritten description"
	p.Comments[0].WhyFix = "rewritten reason"
	s, err := importer.Import(context.Background(), db, p)
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if s.Updated != 1 || s.Inserted != 0 || s.Frozen != 0 {
		t.Fatalf("counters: %+v", s)
	}

	var description, whyFix, status string
	_ = db.QueryRow(`SELECT description, why_fix, status FROM comments`).Scan(&description, &whyFix, &status)
	if description != "rewritten description" || whyFix != "rewritten reason" {
		t.Fatalf("not refreshed: %q / %q", description, whyFix)
	}
	if status != "pending" {
		t.Fatalf("status changed: %q", status)
	}
}

// TestImport_TCIMP062_accepted_frozen exercises "accepted comment is
// frozen: enrichment NOT updated; refs still attached".
func TestImport_TCIMP062_accepted_frozen(t *testing.T) {
	testFrozen(t, "accepted")
}

// TestImport_TCIMP063_dismissed_frozen exercises "dismissed comment is
// frozen".
func TestImport_TCIMP063_dismissed_frozen(t *testing.T) {
	testFrozen(t, "dismissed")
}

// TestImport_TCIMP064_completed_frozen exercises "completed comment is
// frozen".
func TestImport_TCIMP064_completed_frozen(t *testing.T) {
	testFrozen(t, "completed")
}

func testFrozen(t *testing.T, finalStatus string) {
	t.Helper()
	db := storagetest.NewMemDB(t)
	if _, err := importer.Import(context.Background(), db, basePRPayload()); err != nil {
		t.Fatalf("first Import: %v", err)
	}
	if _, err := db.Exec(`UPDATE comments SET status = ?`, finalStatus); err != nil {
		t.Fatalf("mark %s: %v", finalStatus, err)
	}

	p := basePRPayload()
	p.Comments[0].Description = "should not stick"
	s, err := importer.Import(context.Background(), db, p)
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if s.Frozen != 1 || s.Updated != 0 || s.Inserted != 0 {
		t.Fatalf("counters: %+v", s)
	}

	var description string
	_ = db.QueryRow(`SELECT description FROM comments`).Scan(&description)
	if description == "should not stick" {
		t.Fatalf("enrichment was overwritten on frozen comment")
	}
}

// TestImport_TCIMP065_ambiguous_refs_rejected exercises "refs resolve
// to multiple distinct comments: rejected with AmbiguousRefsError".
func TestImport_TCIMP065_ambiguous_refs_rejected(t *testing.T) {
	db := storagetest.NewMemDB(t)

	// First import: comment A with ref id="a".
	pA := basePRPayload()
	pA.Comments[0].ExternalRefs = []pkgpayload.ExternalRef{
		{Kind: "github-pr-comment", ID: "ref-a"},
	}
	if _, err := importer.Import(context.Background(), db, pA); err != nil {
		t.Fatalf("Import A: %v", err)
	}

	// Second import: comment B with ref id="b" (different comment).
	pB := basePRPayload()
	pB.Comments[0].ExternalRefs = []pkgpayload.ExternalRef{
		{Kind: "github-pr-comment", ID: "ref-b"},
	}
	pB.Comments[0].File = "src/other.ts"
	if _, err := importer.Import(context.Background(), db, pB); err != nil {
		t.Fatalf("Import B: %v", err)
	}

	// Third import: a comment whose refs include BOTH ref-a and ref-b.
	pC := basePRPayload()
	pC.Comments[0].ExternalRefs = []pkgpayload.ExternalRef{
		{Kind: "github-pr-comment", ID: "ref-a"},
		{Kind: "github-pr-comment", ID: "ref-b"},
	}
	_, err := importer.Import(context.Background(), db, pC)
	if err == nil {
		t.Fatal("expected AmbiguousRefsError")
	}
	var ambig *importer.AmbiguousRefsError
	if !errors.As(err, &ambig) {
		t.Fatalf("expected *AmbiguousRefsError, got %T: %v", err, err)
	}
	if len(ambig.CommentIDs) != 2 {
		t.Fatalf("expected 2 conflicting IDs, got %v", ambig.CommentIDs)
	}
}

// TestImport_TCIMP066_refs_added exercises "Refs added: new ref
// attached to existing pending comment".
func TestImport_TCIMP066_refs_added(t *testing.T) {
	db := storagetest.NewMemDB(t)
	if _, err := importer.Import(context.Background(), db, basePRPayload()); err != nil {
		t.Fatalf("first Import: %v", err)
	}

	p := basePRPayload()
	p.Comments[0].ExternalRefs = append(p.Comments[0].ExternalRefs,
		pkgpayload.ExternalRef{Kind: "github-review-body", ID: "999", Reviewer: "greptile"})

	s, err := importer.Import(context.Background(), db, p)
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if s.RefsAdded != 1 || s.Updated != 1 {
		t.Fatalf("counters: %+v", s)
	}
	var refCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM comment_external_refs`).Scan(&refCount)
	if refCount != 2 {
		t.Fatalf("refs in DB = %d", refCount)
	}
}

// TestImport_TCIMP070_transaction_rolls_back exercises "multi-comment
// payload where the third comment violates a constraint: zero rows
// persist".
func TestImport_TCIMP070_transaction_rolls_back(t *testing.T) {
	db := storagetest.NewMemDB(t)
	p := basePRPayload()
	good := p.Comments[0]
	bad := good
	bad.Severity = "" // empty string bypasses validate (we skip Validate here), CHECK constraint fires
	bad.ExternalRefs = []pkgpayload.ExternalRef{{Kind: "github-pr-comment", ID: "broken"}}
	p.Comments = []pkgpayload.Comment{good, good, bad}
	p.Comments[1].ExternalRefs = []pkgpayload.ExternalRef{{Kind: "github-pr-comment", ID: "good-2"}}

	_, err := importer.Import(context.Background(), db, p)
	if err == nil {
		t.Fatal("expected import to fail on bad comment")
	}
	taiErr, ok := errcode.As(err)
	if !ok || taiErr.Code != errcode.DBConstraintViolation {
		t.Fatalf("expected DB_CONSTRAINT_VIOLATION, got %v", err)
	}

	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 comments after rollback, got %d", count)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM comment_external_refs`).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 refs after rollback, got %d", count)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM repos`).Scan(&count)
	if count != 0 {
		t.Fatalf("expected 0 repos after rollback, got %d", count)
	}
}

// TestImport_TCIMP083_empty_payload_succeeds exercises TC-IMP-083:
// {comments:[], batches:[]} succeeds and upserts repo+target. (The
// CLI-boundary version of this scenario is TC-IMP-082, which checks
// stdout shape; this engine test pins the persistence half.)
func TestImport_TCIMP083_empty_payload_succeeds(t *testing.T) {
	db := storagetest.NewMemDB(t)
	p := basePRPayload()
	p.Comments = nil
	p.Batches = nil
	s, err := importer.Import(context.Background(), db, p)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if s.CommentCount != 0 || s.BatchCount != 0 {
		t.Fatalf("counters: %+v", s)
	}
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM prs`).Scan(&count)
	if count != 1 {
		t.Fatalf("expected PR row, got %d", count)
	}
}
