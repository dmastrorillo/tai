package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/core/internal/config"
)

// TestSchedulePoll_TCSYNC014_real_goroutine_writes_state_within_budget
// exercises TC-SYNC-014 against the production schedulePoll →
// sync.Schedule → goroutine path that the CLI binary actually runs.
// The TC-SYNC-014 case in core/internal/cmd/sync_test.go calls
// sync.Poll synchronously (via pollDirect); this complements it by
// asserting the fire-and-forget wiring itself: schedulePoll returns a
// non-nil Waiter, the goroutine performs the poll concurrently, and
// Waiter.Wait(pollWaitOnExit) blocks long enough for the state file
// to be flushed in the common case.
//
// Slow (involves a real `git ls-remote` against a file:// bare repo).
// Skips on machines without git or with an older git that lacks
// --initial-branch.
func TestSchedulePoll_TCSYNC014_real_goroutine_writes_state_within_budget(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if !modernGit(t) {
		t.Skip("git older than 2.28")
	}

	// Set up a bare git remote with a single seed commit.
	bare := t.TempDir()
	mustRun(t, "", "git", "init", "--bare", "--initial-branch=main", bare)
	work := t.TempDir()
	mustRun(t, "", "git", "clone", bare, work)
	mustRun(t, work, "git", "config", "user.email", "test@local")
	mustRun(t, work, "git", "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	mustRun(t, work, "git", "add", "-A")
	mustRun(t, work, "git", "commit", "-m", "seed")
	mustRun(t, work, "git", "push", "-u", "origin", "main")

	url := "file://" + bare

	// Stage isolated env so the production schedulePoll picks up our
	// fixture config + data dir without touching the developer's
	// machine.
	dataDir := t.TempDir()
	t.Setenv("TAI_DATA_DIR", dataDir)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")

	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	t.Setenv("TAI_CONFIG", cfgPath)
	config.AllowFileURLsForTesting(t)

	body := fmt.Sprintf("repo-url: %s\nupdate-check-interval: 1ns\n", url)
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	// Drive the wiring directly — this is the path main() takes.
	waiter := schedulePoll(context.Background())
	if waiter == nil {
		t.Fatal("schedulePoll returned nil; the goroutine wiring is not engaged")
	}
	// Wait for the goroutine with a budget generous enough that even
	// a slow local file:// fetch completes (production budget is
	// 250ms; tests give 5s so flakiness on a loaded CI box doesn't
	// surface as a false failure of this contract test).
	if !waiter.Wait(5 * time.Second) {
		t.Fatal("background poll did not finish within 5s")
	}

	// State file must now exist and parse.
	statePath := filepath.Join(dataDir, "state", "update-check.json")
	got, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("state file not written: %v", err)
	}
	var state struct {
		LastCheck string `json:"last-check"`
	}
	if err := json.Unmarshal(got, &state); err != nil {
		t.Fatalf("state file is not valid JSON: %v\n%s", err, got)
	}
	if state.LastCheck == "" {
		t.Errorf("state.last-check should be populated after a successful poll, got:\n%s", got)
	}
}

// TestSchedulePoll_returns_nil_when_disabled exercises the
// "interval = 0 → no goroutine" wiring guard. The corresponding
// user-observable check (state file untouched) lives at TC-SYNC-017
// in core/internal/cmd/sync_test.go; this test pins the wiring-level
// contract that schedulePoll itself returns nil (and main.go's Wait
// path is therefore skipped).
//
// Not tied to a TC-ID — wiring invariant complementing TC-SYNC-017.
func TestSchedulePoll_returns_nil_when_disabled(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", t.TempDir())
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	t.Setenv("TAI_CONFIG", cfgPath)
	config.AllowFileURLsForTesting(t)

	body := "repo-url: file:///tmp/whatever\nupdate-check-interval: 0\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	waiter := schedulePoll(context.Background())
	if waiter != nil {
		t.Fatal("schedulePoll should return nil when interval is 0")
	}
}

// mustRun is the wiring-test analogue of the same helper in
// core/internal/cmd/sync_test.go. Lives in this package because the
// test must use schedulePoll, which is package-private here.
//
// Not tied to a TC-ID — test fixture helper.
func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s in %s: %v\n%s", name, strings.Join(args, " "), dir, err, out)
	}
}

// modernGit reports whether `git` is >= 2.28. Skip-guard for tests
// using --initial-branch in fixture setup.
//
// Not tied to a TC-ID — test fixture helper.
func modernGit(t *testing.T) bool {
	t.Helper()
	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		return false
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) < 3 {
		return false
	}
	verParts := strings.SplitN(parts[2], ".", 3)
	if len(verParts) < 2 {
		return false
	}
	major, minor := atoi(verParts[0]), atoi(verParts[1])
	return major > 2 || (major == 2 && minor >= 28)
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
