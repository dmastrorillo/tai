// Package board serves the bulk-decision surface for triage.
//
// The board renders what it is given and derives nothing. The AI reads
// each stored comment, opens the flagged file, and works out the seven
// fields the triage loop puts in front of a decision; the result is
// piped here as a briefing. Two of those fields are not stored at all:
// `cause` is always derived from the code, and `why_fix` and
// `suggested_fix` are worked out when the record lacks them. A board
// reading the database could show neither, so it holds no database
// handle.
package board

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Briefing is the JSON the AI pipes to `tai triage board -`.
type Briefing struct {
	Repo     string    `json:"repo"`
	Scope    Scope     `json:"scope"`
	Batches  []Batch   `json:"batches"`
	Comments []Comment `json:"comments"`
}

// Scope names the target the briefing covers. It also names the intents
// artifact the board writes, which is why the board needs no --pr or
// --branch flag.
type Scope struct {
	Kind   string `json:"kind"`
	PR     *int   `json:"pr,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// Batch is a group of comments sharing a corrective action.
type Batch struct {
	BatchKey string `json:"batch_key"`
	Title    string `json:"title"`
}

// Comment is one investigated finding. The seven presentation fields
// are RaisedBy through ConcernsIfSkipped; ID, BatchKey and Severity
// drive identity, grouping and order rather than display of a stored
// column.
type Comment struct {
	ID                 int    `json:"id"`
	BatchKey           string `json:"batch_key,omitempty"`
	Severity           string `json:"severity"`
	RaisedBy           string `json:"raised_by"`
	Location           string `json:"location"`
	Description        string `json:"description"`
	Cause              string `json:"cause"`
	WhyFix             string `json:"why_fix"`
	SuggestedFix       string `json:"suggested_fix"`
	SuggestedFixOrigin string `json:"suggested_fix_origin"`
	ConcernsIfSkipped  string `json:"concerns_if_skipped"`
}

// severityRank orders the four severities. Lower sorts first.
var severityRank = map[string]int{
	"critical": 0,
	"major":    1,
	"minor":    2,
	"nitpick":  3,
}

var validScopeKind = map[string]struct{}{"pr": {}, "branch": {}}

var validFixOrigin = map[string]struct{}{"reviewer": {}, "investigation": {}}

// ValidationError is one schema violation, carrying the JSON-Pointer-style
// path where it occurred.
type ValidationError struct {
	Path    string
	Message string
}

// Decode reads a briefing with unknown fields rejected at every level,
// so a typo in a field name fails loudly rather than being dropped.
func Decode(r io.Reader) (Briefing, error) {
	var b Briefing
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return Briefing{}, err
	}
	if err := trailingContent(dec); err != nil {
		return Briefing{}, err
	}
	return b, nil
}

// DecodeBytes is Decode for callers already holding the whole briefing.
func DecodeBytes(p []byte) (Briefing, error) { return Decode(bytes.NewReader(p)) }

// trailingContent rejects a second JSON value after the briefing, which
// otherwise parses silently and discards whatever followed.
func trailingContent(dec *json.Decoder) error {
	var extra json.RawMessage
	switch err := dec.Decode(&extra); {
	case err == io.EOF:
		return nil
	case err != nil:
		return err
	default:
		return fmt.Errorf("unexpected trailing content after the briefing object")
	}
}

// Validate collects EVERY violation before returning. The consumer is an
// AI regenerating the briefing, so reporting one at a time would cost a
// round trip for each.
func Validate(b Briefing) []ValidationError {
	v := &validator{}
	v.checkRepo(b.Repo)
	v.checkScope(b.Scope)
	known := v.checkBatches(b.Batches)
	v.checkComments(b.Comments, known)
	return v.errs
}

type validator struct{ errs []ValidationError }

func (v *validator) add(path, msg string) {
	v.errs = append(v.errs, ValidationError{Path: path, Message: msg})
}

func (v *validator) require(path, value string) {
	if strings.TrimSpace(value) == "" {
		v.add(path, "required field is empty")
	}
}

func (v *validator) checkRepo(repo string) {
	if strings.TrimSpace(repo) == "" {
		v.add("repo", "required field is empty")
		return
	}
	if strings.Count(repo, "/") != 1 || strings.HasPrefix(repo, "/") || strings.HasSuffix(repo, "/") {
		v.add("repo", fmt.Sprintf("%q does not match <owner>/<name>", repo))
	}
}

func (v *validator) checkScope(s Scope) {
	if s.Kind == "" {
		v.add("scope.kind", "required field is empty")
		return
	}
	if _, ok := validScopeKind[s.Kind]; !ok {
		v.add("scope.kind", fmt.Sprintf("%q is not one of (pr, branch)", s.Kind))
		return
	}
	switch s.Kind {
	case "pr":
		if s.Branch != "" {
			v.add("scope.branch", `must be absent when scope.kind is "pr"`)
		}
		if s.PR == nil {
			v.add("scope.pr", `required when scope.kind is "pr"`)
		} else if *s.PR <= 0 {
			v.add("scope.pr", "must be a positive integer")
		}
	case "branch":
		if s.PR != nil {
			v.add("scope.pr", `must be absent when scope.kind is "branch"`)
		}
		v.require("scope.branch", s.Branch)
	}
}

// checkBatches returns the set of declared batch keys so comments can be
// checked against it.
func (v *validator) checkBatches(batches []Batch) map[string]struct{} {
	known := make(map[string]struct{}, len(batches))
	for i, b := range batches {
		base := fmt.Sprintf("batches[%d]", i)
		if strings.TrimSpace(b.BatchKey) == "" {
			v.add(base+".batch_key", "required field is empty")
		} else if _, dup := known[b.BatchKey]; dup {
			v.add(base+".batch_key", fmt.Sprintf("%q is declared more than once", b.BatchKey))
		} else {
			known[b.BatchKey] = struct{}{}
		}
		v.require(base+".title", b.Title)
	}
	return known
}

func (v *validator) checkComments(comments []Comment, known map[string]struct{}) {
	seen := make(map[int]struct{}, len(comments))
	for i, c := range comments {
		base := fmt.Sprintf("comments[%d]", i)

		if c.ID <= 0 {
			v.add(base+".id", "must be a positive integer")
		} else if _, dup := seen[c.ID]; dup {
			v.add(base+".id", fmt.Sprintf("%d appears more than once", c.ID))
		} else {
			seen[c.ID] = struct{}{}
		}

		if c.Severity == "" {
			v.add(base+".severity", "required field is empty")
		} else if _, ok := severityRank[c.Severity]; !ok {
			v.add(base+".severity",
				fmt.Sprintf("%q is not one of (critical, major, minor, nitpick)", c.Severity))
		}

		v.require(base+".raised_by", c.RaisedBy)
		v.require(base+".location", c.Location)
		v.require(base+".description", c.Description)
		v.require(base+".cause", c.Cause)
		v.require(base+".why_fix", c.WhyFix)
		v.require(base+".suggested_fix", c.SuggestedFix)
		v.require(base+".concerns_if_skipped", c.ConcernsIfSkipped)

		if c.SuggestedFixOrigin == "" {
			v.add(base+".suggested_fix_origin", "required field is empty")
		} else if _, ok := validFixOrigin[c.SuggestedFixOrigin]; !ok {
			v.add(base+".suggested_fix_origin",
				fmt.Sprintf("%q is not one of (reviewer, investigation)", c.SuggestedFixOrigin))
		}

		if c.BatchKey != "" {
			if _, ok := known[c.BatchKey]; !ok {
				v.add(base+".batch_key",
					fmt.Sprintf("%q is not declared in batches", c.BatchKey))
			}
		}
	}
}

// SortErrors orders violations by path so the message is stable across
// runs and diffable.
func SortErrors(errs []ValidationError) []ValidationError {
	out := append([]ValidationError(nil), errs...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
