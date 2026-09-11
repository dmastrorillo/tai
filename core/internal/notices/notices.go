// Package notices owns the two stderr messages tai writes around the
// foreground command: the once-per-day update banner and the
// once-ever first-run onboarding hint.
//
// They live together because they are mutually exclusive. Both are
// unsolicited stderr noise, and a brand-new user seeing "here is how
// to get started" next to "here is how to upgrade" learns nothing
// from the second line. The first run wins; the banner is marked as
// already shown for the day so it becomes eligible tomorrow instead
// of stacking here.
//
// The pair is exported rather than inlined into core/cmd/tai/main.go
// so the e2e harness can drive the exact code the binary runs. A
// notice that reaches stderr in production but not in the harness
// (wrong stream, wrong directory, call omitted) is invisible to a
// test that only mirrors the wiring.
//
// Spec: openspec/specs/update-banner/spec.md
// §"First-run onboarding hint".
package notices

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dmastrorillo/tai/core/internal/sync"
	"github.com/dmastrorillo/tai/pkg/cliout"
)

// FirstRunHint is the onboarding line shown once per installation.
//
// It deliberately names no AI tool: tai distributes assets to Claude,
// Cursor, and anything else with a target configured, and the verb it
// points at is the one that asks the user which.
const FirstRunHint = "→ Get started: run `tai install-commands` to make tai's commands available in your AI tool.\n"

// marker is the on-disk shape of first-run.json. Its existence is
// what suppresses the hint; the timestamp is informational.
type marker struct {
	FirstRun string `json:"first-run"`
}

// MarkerPath returns the location of the first-run marker.
func MarkerPath(dataDir string) string {
	return filepath.Join(dataDir, "state", "first-run.json")
}

// BeforeCommand emits whichever pre-command notice is due and reports
// whether the first-run hint is still owed once the command finishes.
//
// The hint itself is deliberately NOT written here: it points at the
// next thing to run, so it belongs below the output of the command
// the user just ran, not above it.
//
// argv is the process argument list, program name included.
func BeforeCommand(stderr io.Writer, dataDir string, now time.Time, argv []string) bool {
	if !firstRunPending(dataDir) || hintSuppressed(stderr, argv) {
		sync.EmitBanner(stderr, dataDir, now)
		return false
	}
	sync.DeferBannerToday(dataDir, now)
	return true
}

// AfterCommand writes the first-run hint when BeforeCommand said it
// was owed, then records the marker.
//
// The marker write is best effort. An unwritable data directory costs
// the user a repeated hint on the next invocation, which is a far
// better outcome than failing a command that has already succeeded.
func AfterCommand(stderr io.Writer, dataDir string, now time.Time, owed bool) {
	if !owed {
		return
	}
	_, _ = io.WriteString(stderr, FirstRunHint)
	body, err := json.Marshal(marker{FirstRun: now.UTC().Format(time.RFC3339)})
	if err != nil {
		return
	}
	path := MarkerPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, body, 0o644)
}

// firstRunPending reports whether the marker is absent. An
// unreadable-but-present marker counts as marked: re-running the hint
// on every invocation is worse than skipping it once.
func firstRunPending(dataDir string) bool {
	_, err := os.Stat(MarkerPath(dataDir))
	return os.IsNotExist(err)
}

// hintSuppressed reports whether this invocation is a script probing
// tai rather than a person using it. A bare `tai` with nothing
// attached to stderr is the shape a CI step takes when it checks the
// binary exists; onboarding advice there is pure log noise, and
// consuming the hint would mean the person never sees it.
func hintSuppressed(stderr io.Writer, argv []string) bool {
	return len(argv) <= 1 && !cliout.IsTTY(stderr)
}
