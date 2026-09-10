package cmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/core/internal/notices"
	"github.com/dmastrorillo/tai/core/internal/sync"
)

// expectFirstRun makes the data directory look like a brand-new
// install: the marker is cleared and the harness is told not to write
// one back before each invocation.
func expectFirstRun(t *testing.T, dataDir string) {
	t.Helper()
	t.Setenv(wantFirstRunEnv, "1")
	if err := os.Remove(markerPath(dataDir)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func markerPath(dataDir string) string {
	return filepath.Join(dataDir, "state", "first-run.json")
}

// TC-UB-008 — a brand-new install gets the onboarding hint on stderr
// and a marker on disk.
func TestFirstRun_TCUB008_hint_and_marker(t *testing.T) {
	dataDir := bannerEnv(t)
	expectFirstRun(t, dataDir)

	r := runRoot(t, "--version")
	if r.exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d", r.exitCode)
	}
	if !strings.Contains(r.stderr, notices.FirstRunHint) {
		t.Errorf("stderr must carry the first-run hint, got %q", r.stderr)
	}
	// The hint names no AI tool — it has to read the same for Claude,
	// Cursor, or anything else the user has installed.
	for _, tool := range []string{"Claude", "Cursor", "Cody", "Copilot"} {
		if strings.Contains(notices.FirstRunHint, tool) {
			t.Errorf("hint must stay AI-tool-agnostic, names %q", tool)
		}
	}
	if strings.Contains(r.stdout, "Get started") {
		t.Errorf("the hint belongs on stderr, found it on stdout: %q", r.stdout)
	}

	raw, err := os.ReadFile(markerPath(dataDir))
	if err != nil {
		t.Fatalf("marker must exist after the first run: %v", err)
	}
	var got struct {
		FirstRun string `json:"first-run"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("marker must be JSON: %v (%s)", err, raw)
	}
	stamp, err := time.Parse(time.RFC3339, got.FirstRun)
	if err != nil {
		t.Fatalf("marker timestamp must be ISO-8601: %v (%q)", err, got.FirstRun)
	}
	if loc := stamp.Location(); loc != time.UTC {
		t.Errorf("marker timestamp must be UTC, got %s", loc)
	}
}

// TC-UB-009 — the hint is once-ever, and re-running does not restamp
// the marker.
func TestFirstRun_TCUB009_suppressed_once_marked(t *testing.T) {
	dataDir := bannerEnv(t)
	expectFirstRun(t, dataDir)

	first := runRoot(t, "--version")
	if !strings.Contains(first.stderr, notices.FirstRunHint) {
		t.Fatalf("setup: first run must print the hint, got %q", first.stderr)
	}
	before, err := os.ReadFile(markerPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}

	second := runRoot(t, "--version")
	if strings.Contains(second.stderr, notices.FirstRunHint) {
		t.Errorf("the hint must fire once, got it again: %q", second.stderr)
	}
	after, err := os.ReadFile(markerPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("marker must not be restamped: %q -> %q", before, after)
	}
}

// TC-UB-010 — the first-run hint and the daily update banner never
// stack. The banner stands down for the day and fires tomorrow.
func TestFirstRun_TCUB010_defers_the_update_banner(t *testing.T) {
	dataDir := bannerEnv(t)
	expectFirstRun(t, dataDir)
	seedPollState(t, dataDir, sync.PollState{
		LastCheck:      time.Now(),
		LastBannerDate: time.Now().AddDate(0, 0, -1).Local().Format(time.DateOnly),
		TAICurrent:     "v1.2.0",
		TAILatest:      "v1.3.0",
	})

	r := runRoot(t, "--version")
	if !strings.Contains(r.stderr, notices.FirstRunHint) {
		t.Errorf("stderr must carry the first-run hint, got %q", r.stderr)
	}
	if strings.Contains(r.stderr, "[tai]") {
		t.Errorf("the update banner must stand down on the first run, got %q", r.stderr)
	}

	state := loadStateForAssert(t, dataDir)
	today := time.Now().Local().Format(time.DateOnly)
	if state.LastBannerDate != today {
		t.Errorf("last-banner-date: want %q so the banner is eligible tomorrow, got %q",
			today, state.LastBannerDate)
	}

	// Tomorrow's invocation is the one that shows the banner.
	next := runRoot(t, "--version")
	if strings.Contains(next.stderr, "[tai]") {
		t.Errorf("the banner must still be suppressed later the same day, got %q", next.stderr)
	}
}

// TC-UB-011 — a marker that cannot be written costs the user a
// repeated hint, never a failed command.
func TestFirstRun_TCUB011_unwritable_marker_does_not_fail_the_command(t *testing.T) {
	dataDir := bannerEnv(t)
	expectFirstRun(t, dataDir)

	// Root bypasses directory permission bits, so the read-only
	// directory below would still accept the write and the test would
	// assert the opposite of what it means. Many container-based CI
	// images run as root.
	if os.Geteuid() == 0 {
		t.Skip("permission bits are not enforced for root")
	}

	stateDir := filepath.Join(dataDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stateDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(stateDir, 0o755) })

	r := runRoot(t, "--version")
	if r.exitCode != 0 {
		t.Fatalf("exit code: want 0, got %d (stderr %q)", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, "tai version ") {
		t.Errorf("the foreground command must still deliver its output, got %q", r.stdout)
	}
	if !strings.Contains(r.stderr, notices.FirstRunHint) {
		t.Errorf("the hint is best-effort but still printed, got %q", r.stderr)
	}
	if _, err := os.Stat(markerPath(dataDir)); !os.IsNotExist(err) {
		t.Errorf("no marker can exist when the state dir is read-only: %v", err)
	}
}

// TC-UB-012 — a bare `tai` in a script is a probe, not a session.
// Printing onboarding at it would flood CI logs.
func TestFirstRun_TCUB012_suppressed_for_a_bare_non_tty_invocation(t *testing.T) {
	dataDir := bannerEnv(t)
	expectFirstRun(t, dataDir)

	r := runRoot(t)
	if strings.Contains(r.stderr, notices.FirstRunHint) {
		t.Errorf("a bare non-TTY invocation must not print the hint, got %q", r.stderr)
	}
	if _, err := os.Stat(markerPath(dataDir)); !os.IsNotExist(err) {
		t.Error("suppressing the hint must not consume it — no marker may be written")
	}
}
