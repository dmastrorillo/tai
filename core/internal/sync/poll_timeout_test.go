package sync

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// hangingGitShim prepends a fake `git` to PATH that sleeps far longer
// than any test timeout, simulating a stalled network (unreachable
// host, silent packet drop) where `git ls-remote` blocks instead of
// failing fast.
func hangingGitShim(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script PATH shim is not portable to windows")
	}
	dir := t.TempDir()
	shim := filepath.Join(dir, "git")
	// exec, not a plain command: without it the shell spawns sleep as
	// a grandchild that inherits the stdout pipe, and killing the
	// direct child (the shell) leaves the pipe open — Output() would
	// then block for the full sleep on platforms whose sh doesn't
	// exec-optimise the last command.
	script := "#!/bin/sh\nexec sleep 30\n"
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// The background poll runs with main's never-cancelled context, so
// lsRemote must bound its own subprocess: a hung `git ls-remote` must
// be killed at backgroundGitTimeout rather than outliving the tai
// process as an orphan.
func TestLsRemote_kills_hanging_git_at_timeout(t *testing.T) {
	hangingGitShim(t)
	BackgroundGitTimeoutForTesting(t, 100*time.Millisecond)

	start := time.Now()
	_, err := lsRemote(context.Background(), "https://example.invalid/repo.git")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("want error from timed-out ls-remote, got nil")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("ls-remote not bounded by timeout: took %v", elapsed)
	}
}

// localHeadCommit shares the same bound: a wedged local-clone
// rev-parse (e.g. clone on an unresponsive network mount) must not
// hang the poll goroutine.
func TestLocalHeadCommit_kills_hanging_git_at_timeout(t *testing.T) {
	hangingGitShim(t)
	BackgroundGitTimeoutForTesting(t, 100*time.Millisecond)

	start := time.Now()
	got := localHeadCommit(context.Background(), t.TempDir())
	elapsed := time.Since(start)

	if got != "" {
		t.Fatalf("want empty SHA from timed-out rev-parse, got %q", got)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("rev-parse not bounded by timeout: took %v", elapsed)
	}
}
