package board

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dmastrorillo/tai/pkg/datadir"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// Intent is one developer call captured on the board and not yet
// persisted. `unanswered` covers every comment the developer did not
// decide, whether they skipped it deliberately or never reached it.
const (
	IntentAccept     = "accept"
	IntentDismiss    = "dismiss"
	IntentUnanswered = "unanswered"
)

// Intents is the artifact the board writes on submit. It is the board's
// only output: comment status is changed by `tai triage accept` /
// `dismiss` / `complete` after the triage conversation, never here.
type Intents struct {
	Repo        string        `json:"repo"`
	Scope       Scope         `json:"scope"`
	SubmittedAt string        `json:"submitted_at"`
	Entries     []IntentEntry `json:"entries"`
}

// IntentEntry is one comment's captured call. ID is the per-target
// position the triage verbs accept.
type IntentEntry struct {
	ID     int    `json:"id"`
	Intent string `json:"intent"`
	Note   string `json:"note,omitempty"`
}

// NewIntents stamps the submit time in RFC 3339 and returns the artifact
// ready to write.
func NewIntents(repo string, scope Scope, entries []IntentEntry) Intents {
	return Intents{
		Repo:        repo,
		Scope:       scope,
		SubmittedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:     entries,
	}
}

// IntentsDir is the directory holding every scope's artifact.
func IntentsDir() (string, error) {
	base, err := datadir.Resolve()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "plugins", "triage", "state", "intents"), nil
}

// ArtifactPath derives a scope's artifact path. Slashes and every other
// character outside [A-Za-z0-9._-] are replaced, so a branch named
// `feat/oauth` cannot place its file outside the intents directory.
func ArtifactPath(repo string, scope Scope) (string, error) {
	dir, err := IntentsDir()
	if err != nil {
		return "", err
	}
	var suffix string
	if scope.Kind == "branch" {
		suffix = "branch-" + slug(scope.Branch)
	} else {
		n := 0
		if scope.PR != nil {
			n = *scope.PR
		}
		suffix = fmt.Sprintf("pr-%d", n)
	}
	return filepath.Join(dir, slug(repo)+"--"+suffix+".json"), nil
}

// slug reduces a string to characters safe in a single path segment.
func slug(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// WriteIntents replaces the scope's artifact in full.
func WriteIntents(in Intents) (string, error) {
	path, err := ArtifactPath(in.Repo, in.Scope)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", errcode.Wrap(errcode.DataDirUnwritable, err,
			"creating the intents directory")
	}
	body, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return "", errcode.Wrap(errcode.InternalError, err, "encoding intents")
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		return "", errcode.Wrap(errcode.DataDirUnwritable, err,
			"writing the intents artifact")
	}
	return path, nil
}

// ReadIntents returns the scope's artifact. found is false when no
// artifact exists, which callers distinguish from an unreadable file:
// absence is the board's "not submitted yet" signal, not a failure.
func ReadIntents(repo string, scope Scope) (in Intents, found bool, err error) {
	path, err := ArtifactPath(repo, scope)
	if err != nil {
		return Intents{}, false, err
	}
	body, readErr := os.ReadFile(path)
	if os.IsNotExist(readErr) {
		return Intents{}, false, nil
	}
	if readErr != nil {
		return Intents{}, false, errcode.Wrap(errcode.InternalError, readErr,
			"reading the intents artifact")
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return Intents{}, false, errcode.Wrap(errcode.InternalError, err,
			"decoding the intents artifact")
	}
	return in, true, nil
}
