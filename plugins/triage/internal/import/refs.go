package importer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/dmastrorillo/tai/pkg/errcode"
	payload "github.com/dmastrorillo/tai/plugins/triage/internal/import/payload"
)

// refSuffixLen is how much of the identity digest is kept. Eight hex
// characters is 32 bits — ample for the handful of findings one review
// body can hold, and short enough to stay readable in `tai triage
// list` output and in the database.
const refSuffixLen = 8

// DisambiguateRefs makes every external ref in the payload identify
// exactly one comment, rewriting them in place.
//
// GitHub gives an inline comment and a top-level comment their own id
// and their own permalink, so one ref already means one finding. A
// review body does not: it is a single object holding a single
// markdown string, and a reviewer routinely writes several findings
// inside it. Every finding extracted from that body therefore arrives
// carrying the same review id.
//
// Left alone, those findings collide. The importer resolves a ref to
// an existing row, so the second finding overwrites the first, the
// third overwrites the second, and an import of four findings reports
// "1 inserted, 3 updated" against an empty scope while three of them
// are silently destroyed.
//
// So where a (kind, id) is claimed by more than one comment, each gets
// a suffix derived from the finding itself: `<id>#<digest>`. The
// digest comes from title, file and lines — what identifies a finding
// rather than how it is worded — so re-importing the same payload
// yields byte-identical refs and updates in place, and a reworded
// description does not orphan the row. Nothing is derived from
// position, so a re-extraction that reorders findings still matches.
//
// A ref claimed by exactly one comment is never touched: those ids
// are already unique and are what existing rows are keyed by.
func DisambiguateRefs(comments []payload.Comment) error {
	claims := map[payload.ExternalRef][]int{}
	for i, c := range comments {
		for _, r := range c.ExternalRefs {
			claims[r] = append(claims[r], i)
		}
	}

	for r, owners := range claims {
		if len(owners) < 2 {
			continue
		}
		byDigest := map[string][]int{}
		for _, i := range owners {
			d := identityDigest(comments[i])
			byDigest[d] = append(byDigest[d], i)
		}
		for d, sharing := range byDigest {
			if len(sharing) > 1 {
				return duplicateCommentsError(comments, r, sharing)
			}
			i := sharing[0]
			setRefID(&comments[i], r, r.ID+"#"+d)
		}
	}
	return nil
}

// identityDigest hashes what makes a finding that finding. Title,
// file and lines are stable across re-wordings of the prose fields,
// which is what keeps a re-import updating rather than duplicating.
func identityDigest(c payload.Comment) string {
	// The unit separator cannot appear in these fields, so no value
	// can be split across two of them to forge a collision.
	sum := sha256.Sum256([]byte(strings.Join([]string{c.Title, c.File, c.Lines}, "\x1f")))
	return hex.EncodeToString(sum[:])[:refSuffixLen]
}

// setRefID rewrites the one ref matching want on the comment. Refs
// are compared by value, so a comment carrying several refs keeps the
// others untouched.
func setRefID(c *payload.Comment, want payload.ExternalRef, id string) {
	for i, r := range c.ExternalRefs {
		if r == want {
			c.ExternalRefs[i].ID = id
			return
		}
	}
}

// duplicateCommentsError reports comments that share both a ref and
// an identity, which no derived key can separate. The message names
// the offending indices and title so the author can act on it without
// re-deriving the analysis.
func duplicateCommentsError(comments []payload.Comment, r payload.ExternalRef, sharing []int) error {
	sort.Ints(sharing)
	paths := make([]string, 0, len(sharing))
	for _, i := range sharing {
		paths = append(paths, fmt.Sprintf("comments[%d]", i))
	}
	return errcode.Newf(errcode.ImportDuplicateRefs,
		"%s all carry external ref %s:%s and the same title, file and lines (%q at %s:%s) — they cannot be told apart",
		strings.Join(paths, ", "), r.Kind, r.ID,
		comments[sharing[0]].Title, comments[sharing[0]].File, comments[sharing[0]].Lines).
		WithHelp(
			"if these are the same finding reported twice, delete all but one of them from the payload and re-run",
			"if they are different findings, give each a title, file or lines of its own — those three fields are what tell findings apart",
			"nothing was written to the database; the import stopped before any change",
		)
}
