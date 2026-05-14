// Package payload owns tai's import-payload JSON schema: the Go shape
// of `tai import -`'s stdin, the strict decoder that rejects unknown
// fields, and the validator that walks a decoded payload and collects
// every schema violation in one pass.
//
// The schema is normatively documented in
// `openspec/changes/add-import-command/specs/import/spec.md` ("Requirement:
// JSON payload schema"). This package is the executable mirror — any
// behavioural difference between the spec and the validator is a bug in
// the validator.
//
// Design notes:
//
//   - DisallowUnknownFields is used at every nesting level so a typo
//     (e.g. `priority` instead of `severity`) is surfaced loudly instead
//     of silently dropped.
//   - Validate collects every error before returning, so the slash
//     command surfaces all violations in a single round rather than
//     iterating.
//   - Enum values (severity, category, kind) are listed here so the
//     validator and the spec stay in sync; the storage layer's CHECK
//     constraints enforce the same set at write time.
package payload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// ownerNameRe matches the canonical `<owner>/<name>` shape: each
// segment contains at least one character and no slash or whitespace.
// Kept in sync with repoctx.identityRe so payload-imported repos pass
// the same shape check that --repo enforces.
var ownerNameRe = regexp.MustCompile(`^[^/\s]+/[^/\s]+$`)

// Payload is the top-level JSON shape `tai import -` reads from stdin.
type Payload struct {
	Repo     string    `json:"repo"`
	Target   Target    `json:"target"`
	Batches  []Batch   `json:"batches"`
	Comments []Comment `json:"comments"`
}

// Target is the PR-or-branch discriminated shape on the payload.
// `Kind` selects which of `PR` / `Branch` is populated; exactly one
// must be present.
type Target struct {
	Kind   string  `json:"kind"`
	PR     *PR     `json:"pr,omitempty"`
	Branch *Branch `json:"branch,omitempty"`
}

// PR is the per-PR target detail.
type PR struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	HeadBranch string `json:"head_branch"`
}

// Branch is the per-branch target detail.
type Branch struct {
	Name string `json:"name"`
}

// Batch groups comments that share a corrective action.
type Batch struct {
	BatchKey string `json:"batch_key"`
	Title    string `json:"title"`
}

// Comment is one triaged review comment.
type Comment struct {
	ExternalRefs []ExternalRef `json:"external_refs"`
	Severity     string        `json:"severity"`
	Category     string        `json:"category"`
	File         string        `json:"file"`
	Lines        string        `json:"lines"`
	Source       string        `json:"source"`
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	WhyFix       string        `json:"why_fix"`
	SuggestedFix string        `json:"suggested_fix"`
	Consequences string        `json:"consequences"`
	BatchKey     string        `json:"batch_key,omitempty"`
}

// ExternalRef is one provenance entry attached to a comment. The
// `(Kind, ID)` pair is the natural key the upsert resolution uses.
type ExternalRef struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Reviewer string `json:"reviewer,omitempty"`
}

// ValidationError is one schema violation. Path is a JSON-pointer-style
// path so the surfacing error can point the user (or the slash command)
// at the exact node that failed.
type ValidationError struct {
	Path    string
	Message string
}

func (e ValidationError) String() string {
	return e.Path + ": " + e.Message
}

// Severity enums (mirrors the storage layer's CHECK constraint).
var validSeverity = map[string]struct{}{
	"critical": {}, "major": {}, "minor": {}, "nitpick": {},
}

// Category enums (mirrors the storage layer's CHECK constraint).
var validCategory = map[string]struct{}{
	"security": {}, "correctness": {}, "feature-regression": {},
	"code-quality": {}, "performance": {}, "testing": {},
}

// validKind is the set of accepted Target.Kind values.
var validKind = map[string]struct{}{
	"pr":     {},
	"branch": {},
}

// Decode reads JSON bytes into a Payload using strict decoding (unknown
// fields are rejected). It does NOT validate semantic rules — call
// Validate after a successful Decode.
//
// On parse failure, the returned error carries enough context to be
// shown to the user (line/column when available). Callers should map
// this to IMPORT_INVALID_JSON.
func Decode(r io.Reader) (Payload, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var p Payload
	if err := dec.Decode(&p); err != nil {
		return Payload{}, err
	}
	// Reject trailing content after the first JSON value — payloads are
	// a single object, not a stream.
	if dec.More() {
		return Payload{}, fmt.Errorf("unexpected trailing content after JSON value")
	}
	return p, nil
}

// DecodeBytes is a convenience wrapper around Decode for callers
// already holding the full payload in memory.
func DecodeBytes(b []byte) (Payload, error) {
	return Decode(bytes.NewReader(b))
}

// Validate walks p and collects every schema violation. The returned
// slice is empty when the payload is valid. Validate does NOT short-
// circuit on the first error — surfacing every violation in one pass
// is part of the spec's UX contract (see Requirement: "All validation
// errors reported in one message").
func Validate(p Payload) []ValidationError {
	v := &validator{}
	v.checkRepo(p.Repo)
	v.checkTarget(p.Target)
	knownBatches := v.checkBatches(p.Batches)
	v.checkComments(p.Comments, knownBatches)
	return v.errs
}

// FormatErrors renders a list of ValidationErrors into the multi-line
// "Error:" body the foundation contract expects, alphabetised by path
// for stable, diff-friendly output.
func FormatErrors(errs []ValidationError) string {
	if len(errs) == 0 {
		return ""
	}
	// Sort by path for deterministic output.
	sorted := append([]ValidationError(nil), errs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Path < sorted[j].Path
	})

	var b strings.Builder
	noun := "problems"
	if len(sorted) == 1 {
		noun = "problem"
	}
	fmt.Fprintf(&b, "%d %s with the JSON payload:\n", len(sorted), noun)
	for _, e := range sorted {
		fmt.Fprintf(&b, "  %s: %s\n", e.Path, e.Message)
	}
	return strings.TrimRight(b.String(), "\n")
}

// validator accumulates ValidationErrors while walking a payload.
type validator struct {
	errs []ValidationError
}

func (v *validator) add(path, msg string) {
	v.errs = append(v.errs, ValidationError{Path: path, Message: msg})
}

// checkRepo enforces the canonical `<owner>/<name>` shape. The regex
// is the single source of truth (same shape as repoctx.identityRe) so
// the JSON payload's repo accepts exactly what the --repo flag would.
func (v *validator) checkRepo(repo string) {
	if repo == "" {
		v.add("repo", "required field is empty")
		return
	}
	if !ownerNameRe.MatchString(repo) {
		v.add("repo", fmt.Sprintf("%q does not match <owner>/<name>", repo))
	}
}

func (v *validator) checkTarget(t Target) {
	if t.Kind == "" {
		v.add("target.kind", "required field is empty")
	} else if _, ok := validKind[t.Kind]; !ok {
		v.add("target.kind",
			fmt.Sprintf("%q is not one of (pr, branch)", t.Kind))
	}

	switch t.Kind {
	case "pr":
		if t.Branch != nil {
			v.add("target.branch", "must be absent when target.kind is \"pr\"")
		}
		if t.PR == nil {
			v.add("target.pr", "required when target.kind is \"pr\"")
			return
		}
		v.checkPR(*t.PR)
	case "branch":
		if t.PR != nil {
			v.add("target.pr", "must be absent when target.kind is \"branch\"")
		}
		if t.Branch == nil {
			v.add("target.branch", "required when target.kind is \"branch\"")
			return
		}
		v.checkBranch(*t.Branch)
	default:
		// Unknown kind already reported; still flag the bodies so a payload
		// can't sneak extra structure past the type check.
		if t.PR != nil && t.Branch != nil {
			v.add("target", "target.pr and target.branch are mutually exclusive")
		}
	}
}

func (v *validator) checkPR(pr PR) {
	if pr.Number <= 0 {
		v.add("target.pr.number", "must be a positive integer")
	}
	requireNonEmpty(v, "target.pr.title", pr.Title)
	requireNonEmpty(v, "target.pr.url", pr.URL)
	requireNonEmpty(v, "target.pr.head_branch", pr.HeadBranch)
}

func (v *validator) checkBranch(b Branch) {
	requireNonEmpty(v, "target.branch.name", b.Name)
}

// checkBatches validates each batch and returns the set of known batch
// keys for cross-reference with comments[].batch_key. Duplicate keys
// inside the payload are flagged.
func (v *validator) checkBatches(batches []Batch) map[string]struct{} {
	keys := map[string]struct{}{}
	for i, b := range batches {
		base := fmt.Sprintf("batches[%d]", i)
		requireNonEmpty(v, base+".batch_key", b.BatchKey)
		requireNonEmpty(v, base+".title", b.Title)
		if b.BatchKey == "" {
			continue
		}
		if _, dup := keys[b.BatchKey]; dup {
			v.add(base+".batch_key",
				fmt.Sprintf("duplicate batch_key %q in payload", b.BatchKey))
			continue
		}
		keys[b.BatchKey] = struct{}{}
	}
	return keys
}

func (v *validator) checkComments(comments []Comment, knownBatches map[string]struct{}) {
	for i, c := range comments {
		base := fmt.Sprintf("comments[%d]", i)

		if len(c.ExternalRefs) == 0 {
			v.add(base+".external_refs", "must contain at least one entry")
		}
		for j, ref := range c.ExternalRefs {
			refBase := fmt.Sprintf("%s.external_refs[%d]", base, j)
			requireNonEmpty(v, refBase+".kind", ref.Kind)
			requireNonEmpty(v, refBase+".id", ref.ID)
		}

		if c.Severity == "" {
			v.add(base+".severity", "required field is empty")
		} else if _, ok := validSeverity[c.Severity]; !ok {
			v.add(base+".severity",
				fmt.Sprintf("%q is not one of (critical, major, minor, nitpick)", c.Severity))
		}

		if c.Category == "" {
			v.add(base+".category", "required field is empty")
		} else if _, ok := validCategory[c.Category]; !ok {
			v.add(base+".category",
				fmt.Sprintf("%q is not one of (security, correctness, feature-regression, code-quality, performance, testing)", c.Category))
		}

		requireNonEmpty(v, base+".file", c.File)
		requireNonEmpty(v, base+".lines", c.Lines)
		requireNonEmpty(v, base+".source", c.Source)
		requireNonEmpty(v, base+".title", c.Title)
		requireNonEmpty(v, base+".description", c.Description)
		requireNonEmpty(v, base+".why_fix", c.WhyFix)
		requireNonEmpty(v, base+".suggested_fix", c.SuggestedFix)
		requireNonEmpty(v, base+".consequences", c.Consequences)

		if c.BatchKey != "" {
			if _, ok := knownBatches[c.BatchKey]; !ok {
				v.add(base+".batch_key",
					fmt.Sprintf("references unknown batch %q (not present in payload.batches)", c.BatchKey))
			}
		}
	}
}

func requireNonEmpty(v *validator, path, value string) {
	if value == "" {
		v.add(path, "required field is empty")
	}
}
