// Package importer persists a validated import payload into tai's
// SQLite database under a single transaction. It is the second half of
// the import flow — the first (parse + schema validation) lives in
// internal/import/payload; this package assumes its input is already
// schema-valid and focuses on the upsert state machine.
//
// The behavioural contract is `openspec/changes/add-import-command/
// specs/import/spec.md` (requirements "Upsert by external_refs", "Batch
// upsert", "Repo and target rows are upserted", "Empty payload is a
// successful no-op"). This file is the executable mirror of those
// requirements — any divergence is a bug here.
//
// The caller (internal/cmd/import.go) owns:
//
//   - Validating the payload via payload.Validate before calling Import.
//   - Opening the *storage.DB.
//   - Mapping our returned errors onto the foundation's error contract.
//
// Import opens a transaction, performs every upsert inside it, and
// commits on success. Any failure rolls the whole import back — see the
// spec's "Transaction rolls back on failure" scenario.
package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/dmastrorillo/tai/pkg/errcode"
	pkgpayload "github.com/dmastrorillo/tai/plugins/triage/internal/import/payload"
	"github.com/dmastrorillo/tai/plugins/triage/internal/storage"
)

// Summary captures the per-counter totals emitted on stdout after a
// successful import. The CLI surface (internal/cmd/import.go) reads
// these to format the spec's success-output block.
type Summary struct {
	Repo          string
	TargetLabel   string
	CommentCount  int
	BatchCount    int
	Inserted      int
	Updated       int
	Frozen        int
	RefsAdded     int
	BatchInserted int
	BatchUpdated  int
}

// AmbiguousRefsError carries the conflicting comment IDs surfaced when
// a comment's external_refs resolve to more than one existing row. The
// CLI maps this to IMPORT_AMBIGUOUS_REFS.
type AmbiguousRefsError struct {
	// CommentIndex is the index of the offending comment in payload.Comments.
	CommentIndex int
	// CommentIDs is the sorted slice of conflicting existing comment IDs.
	CommentIDs []int64
}

func (e *AmbiguousRefsError) Error() string {
	return fmt.Sprintf(
		"comments[%d].external_refs resolve to multiple existing comments: %v",
		e.CommentIndex, e.CommentIDs)
}

// Import is the package entry point. It opens a transaction on db,
// runs every upsert (repo → target → batches → comments + refs), and
// commits. On any failure the transaction is rolled back and a
// *errcode.Error (or an *AmbiguousRefsError) is returned.
//
// External refs are disambiguated first (see DisambiguateRefs), so a
// review body holding several findings yields one ref per finding.
//
// The payload is assumed to have already passed payload.Validate. We
// re-check a couple of structural invariants (e.g. target.kind matches
// its body) defensively, but enrichment-field presence is the spec
// validator's job.
func Import(ctx context.Context, db *storage.DB, p pkgpayload.Payload) (Summary, error) {
	// Several findings extracted from one review body all arrive
	// carrying that body's single GitHub id. Give each its own ref
	// before anything is written, or they resolve to one another and
	// overwrite in turn. Runs outside the transaction because it
	// touches only the payload and must fail before any DB change.
	if err := DisambiguateRefs(p.Comments); err != nil {
		return Summary{}, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Summary{}, errcode.Wrap(errcode.InternalError, err,
			"begin import transaction")
	}
	// Rollback is a no-op after Commit; safe in the happy path.
	defer func() { _ = tx.Rollback() }()

	s, err := importTx(ctx, tx, p)
	if err != nil {
		return Summary{}, err
	}

	if err := tx.Commit(); err != nil {
		return Summary{}, errcode.Wrap(errcode.InternalError, err,
			"commit import transaction")
	}
	return s, nil
}

// importTx runs every upsert inside the open transaction. Split from
// Import so the rollback/commit lifecycle stays a thin wrapper.
func importTx(ctx context.Context, tx *sql.Tx, p pkgpayload.Payload) (Summary, error) {
	now := time.Now().Unix()

	repoID, err := upsertRepo(ctx, tx, p.Repo, now)
	if err != nil {
		return Summary{}, err
	}

	prID, branchID, label, err := upsertTarget(ctx, tx, repoID, p.Target, now)
	if err != nil {
		return Summary{}, err
	}

	batchIDs, bIns, bUpd, err := upsertBatches(ctx, tx, prID, branchID, p.Batches, now)
	if err != nil {
		return Summary{}, err
	}

	ins, upd, frozen, refsAdded, err := upsertComments(ctx, tx, prID, branchID, p.Comments, batchIDs, now)
	if err != nil {
		return Summary{}, err
	}

	return Summary{
		Repo:          p.Repo,
		TargetLabel:   label,
		CommentCount:  len(p.Comments),
		BatchCount:    len(p.Batches),
		Inserted:      ins,
		Updated:       upd,
		Frozen:        frozen,
		RefsAdded:     refsAdded,
		BatchInserted: bIns,
		BatchUpdated:  bUpd,
	}, nil
}

// upsertRepo inserts the repo row if it doesn't exist, then returns
// its id. `INSERT OR IGNORE` preserves an existing row's created_at —
// the spec requires re-import not touch first-import timestamps.
func upsertRepo(ctx context.Context, tx *sql.Tx, ownerName string, now int64) (int64, error) {
	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO repos (owner_name, created_at) VALUES (?, ?)`,
		ownerName, now); err != nil {
		return 0, storage.MapDBError(err, "upsert repo")
	}
	var id int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM repos WHERE owner_name = ?`, ownerName).Scan(&id); err != nil {
		return 0, storage.MapDBError(err, "look up repo id")
	}
	return id, nil
}

// upsertTarget upserts the PR or branch row depending on the target's
// kind. Returns the resolved (prID, branchID) pair — exactly one is
// non-zero — plus the human-readable label used in the success summary
// header (`PR #142` or `branch feat/x`).
func upsertTarget(ctx context.Context, tx *sql.Tx, repoID int64, t pkgpayload.Target, now int64) (prID, branchID int64, label string, err error) {
	switch t.Kind {
	case "pr":
		if t.PR == nil {
			return 0, 0, "", errcode.New(errcode.ImportSchemaInvalid,
				"target.kind=pr but target.pr is absent")
		}
		// INSERT OR IGNORE: title/url/head_branch are written only on
		// first insert. Re-import doesn't overwrite them — see the spec's
		// "PR title not updated on re-import" scenario.
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO prs
			   (repo_id, number, title, url, head_branch, created_at)
			   VALUES (?, ?, ?, ?, ?, ?)`,
			repoID, t.PR.Number, t.PR.Title, t.PR.URL, t.PR.HeadBranch, now); err != nil {
			return 0, 0, "", storage.MapDBError(err, "upsert pr")
		}
		var id int64
		if err := tx.QueryRowContext(ctx,
			`SELECT id FROM prs WHERE repo_id = ? AND number = ?`,
			repoID, t.PR.Number).Scan(&id); err != nil {
			return 0, 0, "", storage.MapDBError(err, "look up pr id")
		}
		return id, 0, fmt.Sprintf("PR #%d", t.PR.Number), nil

	case "branch":
		if t.Branch == nil {
			return 0, 0, "", errcode.New(errcode.ImportSchemaInvalid,
				"target.kind=branch but target.branch is absent")
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO branches (repo_id, name, created_at) VALUES (?, ?, ?)`,
			repoID, t.Branch.Name, now); err != nil {
			return 0, 0, "", storage.MapDBError(err, "upsert branch")
		}
		var id int64
		if err := tx.QueryRowContext(ctx,
			`SELECT id FROM branches WHERE repo_id = ? AND name = ?`,
			repoID, t.Branch.Name).Scan(&id); err != nil {
			return 0, 0, "", storage.MapDBError(err, "look up branch id")
		}
		return 0, id, fmt.Sprintf("branch %s", t.Branch.Name), nil

	default:
		return 0, 0, "", errcode.Newf(errcode.ImportSchemaInvalid,
			"unsupported target.kind %q", t.Kind)
	}
}

// upsertBatches walks the payload's batches[] and returns a map from
// batch_key → batch_id. New batches are inserted with status='pending';
// existing batches have their title updated.
func upsertBatches(ctx context.Context, tx *sql.Tx, prID, branchID int64, batches []pkgpayload.Batch, now int64) (map[string]int64, int, int, error) {
	ids := map[string]int64{}
	inserted, updated := 0, 0
	for _, b := range batches {
		id, isNew, err := upsertOneBatch(ctx, tx, prID, branchID, b, now)
		if err != nil {
			return nil, 0, 0, err
		}
		ids[b.BatchKey] = id
		if isNew {
			inserted++
		} else {
			updated++
		}
	}
	return ids, inserted, updated, nil
}

func upsertOneBatch(ctx context.Context, tx *sql.Tx, prID, branchID int64, b pkgpayload.Batch, now int64) (int64, bool, error) {
	var existing int64
	var lookupErr error
	if prID != 0 {
		lookupErr = tx.QueryRowContext(ctx,
			`SELECT id FROM batches WHERE pr_id = ? AND batch_key = ?`,
			prID, b.BatchKey).Scan(&existing)
	} else {
		lookupErr = tx.QueryRowContext(ctx,
			`SELECT id FROM batches WHERE branch_id = ? AND batch_key = ?`,
			branchID, b.BatchKey).Scan(&existing)
	}
	switch {
	case lookupErr == nil:
		// Existing batch: update title, leave status alone.
		if _, err := tx.ExecContext(ctx,
			`UPDATE batches SET title = ? WHERE id = ?`, b.Title, existing); err != nil {
			return 0, false, storage.MapDBError(err, "update batch")
		}
		return existing, false, nil
	case errors.Is(lookupErr, sql.ErrNoRows):
		// New batch: insert with default status='pending'.
		var res sql.Result
		var err error
		if prID != 0 {
			res, err = tx.ExecContext(ctx,
				`INSERT INTO batches (pr_id, batch_key, title, created_at) VALUES (?, ?, ?, ?)`,
				prID, b.BatchKey, b.Title, now)
		} else {
			res, err = tx.ExecContext(ctx,
				`INSERT INTO batches (branch_id, batch_key, title, created_at) VALUES (?, ?, ?, ?)`,
				branchID, b.BatchKey, b.Title, now)
		}
		if err != nil {
			return 0, false, storage.MapDBError(err, "insert batch")
		}
		id, _ := res.LastInsertId()
		return id, true, nil
	default:
		return 0, false, storage.MapDBError(lookupErr, "look up batch")
	}
}

// upsertComments is the heart of the import: for each comment we
// resolve external_refs to existing comment_ids, then take one of four
// branches per the spec. Returns the four summary counters.
func upsertComments(
	ctx context.Context, tx *sql.Tx,
	prID, branchID int64,
	comments []pkgpayload.Comment,
	batchIDs map[string]int64,
	now int64,
) (inserted, updated, frozen, refsAdded int, err error) {
	for i, c := range comments {
		batchID := sql.NullInt64{}
		if c.BatchKey != "" {
			id, ok := batchIDs[c.BatchKey]
			if !ok {
				return 0, 0, 0, 0, errcode.Newf(errcode.ImportSchemaInvalid,
					"comments[%d].batch_key %q references unknown batch", i, c.BatchKey)
			}
			batchID = sql.NullInt64{Int64: id, Valid: true}
		}

		resolved, err := resolveRefs(ctx, tx, c.ExternalRefs)
		if err != nil {
			return 0, 0, 0, 0, err
		}

		switch len(resolved) {
		case 0:
			// Refs inserted alongside a brand-new comment are implicit in
			// "Inserted: N new comments"; they are NOT counted in
			// refsAdded, which the spec defines as refs attached to
			// EXISTING comments only.
			if err := insertNewComment(ctx, tx, prID, branchID, batchID, c, now); err != nil {
				return 0, 0, 0, 0, err
			}
			inserted++

		case 1:
			id := resolved[0]
			status, err := readCommentStatus(ctx, tx, id)
			if err != nil {
				return 0, 0, 0, 0, err
			}
			added, err := attachNewRefs(ctx, tx, id, c.ExternalRefs)
			if err != nil {
				return 0, 0, 0, 0, err
			}
			refsAdded += added
			if status == "pending" {
				if err := refreshEnrichment(ctx, tx, id, batchID, c, now); err != nil {
					return 0, 0, 0, 0, err
				}
				updated++
			} else {
				frozen++
			}

		default:
			return 0, 0, 0, 0, &AmbiguousRefsError{
				CommentIndex: i,
				CommentIDs:   resolved,
			}
		}
	}
	return inserted, updated, frozen, refsAdded, nil
}

// resolveRefs looks up every (kind, id) pair and returns the sorted,
// distinct set of comment IDs they point to.
func resolveRefs(ctx context.Context, tx *sql.Tx, refs []pkgpayload.ExternalRef) ([]int64, error) {
	seen := map[int64]struct{}{}
	for _, r := range refs {
		var cid int64
		err := tx.QueryRowContext(ctx,
			`SELECT comment_id FROM comment_external_refs WHERE source_kind = ? AND external_id = ?`,
			r.Kind, r.ID).Scan(&cid)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, storage.MapDBError(err, "resolve external_refs")
		}
		seen[cid] = struct{}{}
	}
	out := make([]int64, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// insertNewComment inserts a brand-new comments row plus every
// external_ref. The spec's "Refs added" counter is for refs attached
// to existing comments, so refs inserted here don't contribute to it.
func insertNewComment(
	ctx context.Context, tx *sql.Tx,
	prID, branchID int64, batchID sql.NullInt64,
	c pkgpayload.Comment, now int64,
) error {
	var prArg, branchArg any
	if prID != 0 {
		prArg = prID
	}
	if branchID != 0 {
		branchArg = branchID
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO comments
		   (pr_id, branch_id, batch_id, severity, category, file, lines, source,
		    title, description, why_fix, suggested_fix, consequences,
		    status, created_at, updated_at)
		   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
		prArg, branchArg, nullIfZero(batchID),
		c.Severity, c.Category, c.File, c.Lines, c.Source,
		c.Title, c.Description, c.WhyFix, c.SuggestedFix, c.Consequences,
		now, now)
	if err != nil {
		return storage.MapDBError(err, "insert comment")
	}
	commentID, _ := res.LastInsertId()
	for _, r := range c.ExternalRefs {
		var reviewer any
		if r.Reviewer != "" {
			reviewer = r.Reviewer
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO comment_external_refs (comment_id, source_kind, external_id, reviewer)
			   VALUES (?, ?, ?, ?)`,
			commentID, r.Kind, r.ID, reviewer); err != nil {
			return storage.MapDBError(err, "insert external_ref")
		}
	}
	return nil
}

// attachNewRefs inserts any refs from the payload that aren't already
// attached to commentID. Returns the count newly inserted.
func attachNewRefs(ctx context.Context, tx *sql.Tx, commentID int64, refs []pkgpayload.ExternalRef) (int, error) {
	added := 0
	for _, r := range refs {
		var existing int64
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM comment_external_refs WHERE source_kind = ? AND external_id = ?`,
			r.Kind, r.ID).Scan(&existing)
		if err == nil {
			// Ref already in DB — assumed to point at commentID because
			// the caller called us only when resolveRefs returned a
			// single distinct comment_id.
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, storage.MapDBError(err, "look up existing ref")
		}
		var reviewer any
		if r.Reviewer != "" {
			reviewer = r.Reviewer
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO comment_external_refs (comment_id, source_kind, external_id, reviewer)
			   VALUES (?, ?, ?, ?)`,
			commentID, r.Kind, r.ID, reviewer); err != nil {
			return 0, storage.MapDBError(err, "insert external_ref")
		}
		added++
	}
	return added, nil
}

// refreshEnrichment overwrites the enrichment fields (and batch_id) on
// the existing pending comment row to match the incoming payload.
// updated_at is bumped so the freshest content drives downstream
// surfacing.
func refreshEnrichment(
	ctx context.Context, tx *sql.Tx,
	commentID int64, batchID sql.NullInt64,
	c pkgpayload.Comment, now int64,
) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE comments
		   SET severity = ?, category = ?, file = ?, lines = ?, source = ?,
		       title = ?, description = ?, why_fix = ?, suggested_fix = ?,
		       consequences = ?, batch_id = ?, updated_at = ?
		   WHERE id = ?`,
		c.Severity, c.Category, c.File, c.Lines, c.Source,
		c.Title, c.Description, c.WhyFix, c.SuggestedFix, c.Consequences,
		nullIfZero(batchID), now, commentID)
	if err != nil {
		return storage.MapDBError(err, "refresh comment")
	}
	return nil
}

func readCommentStatus(ctx context.Context, tx *sql.Tx, commentID int64) (string, error) {
	var status string
	if err := tx.QueryRowContext(ctx,
		`SELECT status FROM comments WHERE id = ?`, commentID).Scan(&status); err != nil {
		return "", storage.MapDBError(err, "read comment status")
	}
	return status, nil
}

func nullIfZero(n sql.NullInt64) any {
	if !n.Valid {
		return nil
	}
	return n.Int64
}
