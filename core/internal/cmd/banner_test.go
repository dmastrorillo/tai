package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/core/internal/sync"
	"github.com/dmastrorillo/tai/pkg/errcode"
)

// loadStateForAssert reads update-check.json and decodes it for a
// test assertion. Avoids brittle substring JSON matching.
//
// Not tied to a TC-ID — test helper.
func loadStateForAssert(t *testing.T, dataDir string) sync.PollState {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dataDir, "state", "update-check.json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var got sync.PollState
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	return got
}

// bannerEnv stages a fresh TAI_DATA_DIR (no config needed). Returns
// the data-dir path. Not tied to a TC-ID — test fixture helper.
func bannerEnv(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("TAI_DATA_DIR", dataDir)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	return dataDir
}

// seedPollState writes a synthetic update-check.json via the
// production write path so the test seed and the production reader
// stay byte-compatible. Not tied to a TC-ID.
func seedPollState(t *testing.T, dataDir string, state sync.PollState) {
	t.Helper()
	if err := sync.SaveState(dataDir, state); err != nil {
		t.Fatalf("seed state: %v", err)
	}
}

// TestBanner_TCUB001_fires_on_first_command exercises TC-UB-001.
func TestBanner_TCUB001_fires_on_first_command(t *testing.T) {
	dataDir := bannerEnv(t)
	yesterday := time.Now().AddDate(0, 0, -1).Local().Format(time.DateOnly)
	seedPollState(t, dataDir, sync.PollState{
		LastCheck:      time.Now(),
		LastBannerDate: yesterday,
		TAICurrent:     "v1.2.0",
		TAILatest:      "v1.3.0",
	})

	var stderr bytes.Buffer
	now := time.Now()
	sync.EmitBanner(&stderr, dataDir, now)

	if !strings.Contains(stderr.String(), "[tai]") {
		t.Fatalf("stderr should contain [tai] banner, got: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "v1.2.0") || !strings.Contains(stderr.String(), "v1.3.0") {
		t.Errorf("banner should name current → latest versions, got: %q", stderr.String())
	}

	// last-banner-date persisted as today.
	got := loadStateForAssert(t, dataDir)
	want := now.Local().Format(time.DateOnly)
	if got.LastBannerDate != want {
		t.Errorf("LastBannerDate: want %q, got %q", want, got.LastBannerDate)
	}
}

// TestBanner_TCUB002_suppressed_same_day exercises TC-UB-002.
func TestBanner_TCUB002_suppressed_same_day(t *testing.T) {
	dataDir := bannerEnv(t)
	now := time.Now()
	today := now.Local().Format(time.DateOnly)
	seedPollState(t, dataDir, sync.PollState{
		LastCheck:      now,
		LastBannerDate: today,
		TAICurrent:     "v1.2.0",
		TAILatest:      "v1.3.0",
	})

	var stderr bytes.Buffer
	sync.EmitBanner(&stderr, dataDir, now)

	if stderr.Len() != 0 {
		t.Errorf("banner should be suppressed today; got stderr:\n%s", stderr.String())
	}
}

// TestBanner_TCUB003_nothing_pending_no_banner exercises TC-UB-003.
func TestBanner_TCUB003_nothing_pending_no_banner(t *testing.T) {
	dataDir := bannerEnv(t)
	seedPollState(t, dataDir, sync.PollState{
		LastCheck: time.Now(),
		// Last-banner-date deliberately left empty so the only
		// reason for suppression is "nothing pending".
		HasUpdates: false,
		TAICurrent: "v1.2.0",
		TAILatest:  "v1.2.0", // current == latest → no update
	})

	var stderr bytes.Buffer
	sync.EmitBanner(&stderr, dataDir, time.Now())

	if stderr.Len() != 0 {
		t.Errorf("banner should not fire when nothing pending, got:\n%s", stderr.String())
	}
}

// TestBanner_TCUB004_stderr_only_prefixed_short exercises TC-UB-004.
func TestBanner_TCUB004_stderr_only_prefixed_short(t *testing.T) {
	dataDir := bannerEnv(t)
	yesterday := time.Now().AddDate(0, 0, -1).Local().Format(time.DateOnly)
	seedPollState(t, dataDir, sync.PollState{
		LastCheck:      time.Now(),
		LastBannerDate: yesterday,
		TAICurrent:     "v1.2.0",
		TAILatest:      "v1.3.0",
		HasUpdates:     true,
		Plugins: []sync.PluginUpdate{
			{Name: "triage", Current: "v0.4.0", Latest: "v0.5.0"},
		},
	})

	var stderr bytes.Buffer
	sync.EmitBanner(&stderr, dataDir, time.Now())

	out := stderr.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > 4 {
		t.Errorf("banner must be ≤4 lines, got %d:\n%s", len(lines), out)
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "[tai]") {
			t.Errorf("line %d missing [tai] prefix: %q", i, line)
		}
	}
}

// TestBanner_TCUB005_names_exact_commands exercises TC-UB-005.
func TestBanner_TCUB005_names_exact_commands(t *testing.T) {
	dataDir := bannerEnv(t)
	yesterday := time.Now().AddDate(0, 0, -1).Local().Format(time.DateOnly)
	seedPollState(t, dataDir, sync.PollState{
		LastCheck:      time.Now(),
		LastBannerDate: yesterday,
		TAICurrent:     "v1.2.0",
		TAILatest:      "v1.3.0",
		HasUpdates:     true,
		Plugins: []sync.PluginUpdate{
			{Name: "triage", Current: "v0.4.0", Latest: "v0.5.0"},
		},
	})

	var stderr bytes.Buffer
	sync.EmitBanner(&stderr, dataDir, time.Now())

	out := stderr.String()
	for _, want := range []string{
		"brew upgrade tai", // TAI package-manager command
		"tai plugins update triage",
		"tai sync",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("banner missing exact command %q\nbanner:\n%s", want, out)
		}
	}
}

// TestBanner_TCUB006_tai_update_is_unknown_subcommand exercises TC-UB-006.
func TestBanner_TCUB006_tai_update_is_unknown_subcommand(t *testing.T) {
	bannerEnv(t)

	r := runRoot(t, "update")
	if r.err == nil {
		t.Fatal("expected error")
	}
	if r.exitCode != 1 {
		t.Errorf("exit code: want 1, got %d", r.exitCode)
	}
	assertCode(t, r.err, errcode.UnknownSubcommand)
	if !strings.Contains(r.stderr, "[exit 1: UNKNOWN_SUBCOMMAND]") {
		t.Errorf("stderr missing footer, got:\n%s", r.stderr)
	}
	// Spec: "what to do" bullets name a package-manager command as
	// the resolution. The user typed `tai update` — the explanation
	// must redirect them to the package-manager path, not the
	// generic "tai plugins list" boilerplate.
	if !strings.Contains(r.stderr, "brew upgrade tai") &&
		!strings.Contains(r.stderr, "go install") {
		t.Errorf("`tai update` help should name a package-manager command, got:\n%s", r.stderr)
	}
}

// TestBanner_TCUB007_fires_at_cli_boundary exercises TC-UB-007: the
// CLI-boundary integration anchor for the banner emitter. TC-UB-001
// through TC-UB-005 stage state and call sync.EmitBanner directly,
// which proves the banner LOGIC works but can't catch a regression
// where the runtime forgets to wire it into the request path. This
// test seeds state, drives a harmless verb through runRoot, and
// confirms the banner reaches stderr (not stdout) without disturbing
// the foreground command's product.
//
// The seed sets HasUpdates: true alongside the TAI-layer mismatch so
// the "pending update" precondition is encoded via the authoritative
// flag — matching TC-UB-004 / TC-UB-005's conventions — and survives
// any future tightening of PollState.HasPendingUpdate().
func TestBanner_TCUB007_fires_at_cli_boundary(t *testing.T) {
	dataDir := bannerEnv(t)
	yesterday := time.Now().AddDate(0, 0, -1).Local().Format(time.DateOnly)
	seedPollState(t, dataDir, sync.PollState{
		LastCheck:      time.Now(),
		LastBannerDate: yesterday,
		TAICurrent:     "v1.2.0",
		TAILatest:      "v1.3.0",
		HasUpdates:     true,
	})

	r := runRoot(t, "--version")
	if r.err != nil {
		t.Fatalf("unexpected error: %v", r.err)
	}
	if !strings.Contains(r.stderr, "[tai]") {
		t.Errorf("banner should appear on stderr via the CLI path, got:\n%s", r.stderr)
	}
	if strings.Contains(r.stdout, "[tai]") {
		t.Errorf("banner must not leak to stdout, got:\n%s", r.stdout)
	}
	// The foreground command's stdout still carries the verb's
	// product unaffected.
	if !strings.Contains(r.stdout, "tai version") {
		t.Errorf("foreground command stdout missing version string, got:\n%s", r.stdout)
	}
}

// TestBanner_multi_plugin_collapses_to_one_row exercises renderBanner's
// `>1 plugin pending` collapse branch. Two plugins pending plus TAI
// plus source-repo would otherwise produce 5 lines (header + 4 rows);
// the collapse keeps the line cap. Not tied to a TC-ID — locks the
// regression caught in Phase 5 review.
// TestBanner_TCREL006_plugin_row_appears_in_stderr is the CLI-boundary
// anchor for TC-REL-006: a plugin's latest version, resolved via the
// prefix-aware lookup, MUST appear in the rendered stderr banner.
// The poll-layer unit test in core/internal/sync/banner_test.go pins
// the PollState contract; this test pins the user-observable stderr
// output, closing the "internal struct, not user-observable" gap.
//
// TC-ID: TC-REL-006 (core/test-cases.md).
func TestBanner_TCREL006_plugin_row_appears_in_stderr(t *testing.T) {
	dataDir := bannerEnv(t)
	yesterday := time.Now().AddDate(0, 0, -1).Local().Format(time.DateOnly)
	seedPollState(t, dataDir, sync.PollState{
		LastCheck:      time.Now(),
		LastBannerDate: yesterday,
		// Mirrors the BDD fixture: triage installed at v0.4.0, latest
		// stable plugins/triage/v0.5.0 already stripped for display.
		Plugins: []sync.PluginUpdate{
			{Name: "triage", Current: "v0.4.0", Latest: "v0.5.0"},
		},
	})

	var stderr bytes.Buffer
	sync.EmitBanner(&stderr, dataDir, time.Now())

	out := stderr.String()
	for _, want := range []string{
		"triage",
		"v0.4.0",
		"v0.5.0",
		"tai plugins update triage",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("banner missing %q\nbanner:\n%s", want, out)
		}
	}
}

// TestBanner_TCREL007_no_plugin_row_when_only_prereleases is the
// CLI-boundary anchor for TC-REL-007: when the plugin's prefix-aware
// lookup returns the "no stable release" sentinel, the poll layer
// omits the row from PollState.Plugins, and the rendered stderr
// banner contains no plugin row. The "no banner at all" case
// triggers because there is nothing else pending.
//
// TC-ID: TC-REL-007.
func TestBanner_TCREL007_no_plugin_row_when_only_prereleases(t *testing.T) {
	dataDir := bannerEnv(t)
	yesterday := time.Now().AddDate(0, 0, -1).Local().Format(time.DateOnly)
	// Empty Plugins slice mimics the post-poll state when
	// LatestPrefixedTag returned ("", nil) for the only installed
	// plugin (TC-REL-003 sentinel). No other pending updates.
	seedPollState(t, dataDir, sync.PollState{
		LastCheck:      time.Now(),
		LastBannerDate: yesterday,
		Plugins:        nil,
	})

	var stderr bytes.Buffer
	sync.EmitBanner(&stderr, dataDir, time.Now())

	out := stderr.String()
	if out != "" {
		t.Errorf("expected empty stderr (no pending updates), got:\n%s", out)
	}
	if strings.Contains(out, "triage") {
		t.Errorf("banner must not name triage when no stable release is available")
	}
}

func TestBanner_multi_plugin_collapses_to_one_row(t *testing.T) {
	dataDir := bannerEnv(t)
	yesterday := time.Now().AddDate(0, 0, -1).Local().Format(time.DateOnly)
	seedPollState(t, dataDir, sync.PollState{
		LastCheck:      time.Now(),
		LastBannerDate: yesterday,
		TAICurrent:     "v1.2.0",
		TAILatest:      "v1.3.0",
		HasUpdates:     true,
		Plugins: []sync.PluginUpdate{
			{Name: "triage", Current: "v0.4.0", Latest: "v0.5.0"},
			{Name: "other", Current: "v1.0.0", Latest: "v1.1.0"},
		},
	})

	var stderr bytes.Buffer
	sync.EmitBanner(&stderr, dataDir, time.Now())

	out := stderr.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > 4 {
		t.Errorf("multi-plugin banner must hold ≤4 lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(out, "2 plugins have updates") {
		t.Errorf("collapse row should name `2 plugins have updates`, got:\n%s", out)
	}
	for _, want := range []string{"triage", "other"} {
		if !strings.Contains(out, want) {
			t.Errorf("collapse row should list plugin name %q, got:\n%s", want, out)
		}
	}
}
