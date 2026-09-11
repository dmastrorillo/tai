package cmd_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/dmastrorillo/tai/core/internal/cmd"
	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/notices"
	"github.com/dmastrorillo/tai/core/internal/sync"
	"github.com/dmastrorillo/tai/pkg/clitest"
	"github.com/dmastrorillo/tai/pkg/datadir"
)

// runRootStdin is the single harness body behind runRoot: it drives
// cmd.NewRoot through the shared pkg/clitest harness (the same one
// the triage plugin's cmdtest wraps), feeding the supplied string as
// stdin (sync prompt tests read it; everything else passes "").
//
// The PreRun / PostRun hooks call the same notices pair main.go
// brackets its foreground command with, so the update banner and the
// first-run hint are exercised by every harness-based test rather
// than by a mirrored copy that can drift. Fixtures that don't seed
// update-check.json see no banner, and the marker written below keeps
// the onboarding hint out of unrelated tests' stderr.
// Error rendering and the exit code come from cliexec.Exit inside
// clitest, the same translation the shipped binary performs, so this
// harness cannot drift from production behaviour.
//
// Not tied to a TC-ID — it's a test helper.
func runRootStdin(t *testing.T, stdin string, argv ...string) runResult {
	t.Helper()

	dataDir, dataDirErr := datadir.Resolve()
	if dataDirErr == nil {
		markInstallationEstablished(t, dataDir)
	}

	firstRunOwed := false
	r := clitest.RunWith(t, cmd.NewRoot(), clitest.Options{
		Stdin: stdin,
		PreRun: func(stderr io.Writer) {
			if dataDirErr == nil {
				firstRunOwed = notices.BeforeCommand(stderr, dataDir, time.Now(),
					append([]string{"tai"}, argv...))
			}
		},
		PostRun: func(stderr io.Writer) {
			if dataDirErr == nil {
				notices.AfterCommand(stderr, dataDir, time.Now(), firstRunOwed)
			}
		},
	}, argv...)

	return runResult{
		stdout:   r.Stdout,
		stderr:   r.Stderr,
		exitCode: r.ExitCode,
		err:      r.Err,
	}
}

// wantFirstRunEnv is the opt-in a test sets (via expectFirstRun) to
// tell the harness to leave the data directory looking brand new.
const wantFirstRunEnv = "TAI_TEST_WANT_FIRST_RUN"

// markInstallationEstablished writes the first-run marker so the
// onboarding hint does not land in the stderr of every test that has
// nothing to do with it. Tests that DO exercise the hint call
// expectFirstRun, which opts out of this.
//
// Writing the marker rather than skipping the notices call keeps the
// harness on the same code path main.go runs, which is the point of
// the pair being shared at all.
func markInstallationEstablished(t *testing.T, dataDir string) {
	t.Helper()
	if os.Getenv(wantFirstRunEnv) != "" {
		return
	}
	path := notices.MarkerPath(dataDir)
	if _, err := os.Stat(path); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(`{"first-run":"2020-01-01T00:00:00Z"}`), 0o644)
}

// pollDirect runs sync.Poll synchronously against the current env's
// config. Used by TC-SYNC-014/015/016/017 to avoid the
// fire-and-forget shape of the production Schedule() goroutine.
//
// Not tied to a TC-ID — test fixture helper.
func pollDirect(t *testing.T, _ /*url*/, dataDir string) {
	t.Helper()
	cfgPath, err := config.ResolvePath()
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	// Poll's error is intentionally swallowed in production. In the
	// tests we let it through for diagnostic logging only — we still
	// assert on the state file as the user-observable contract.
	if pollErr := sync.Poll(context.Background(), cfg, dataDir); pollErr != nil {
		t.Logf("sync.Poll returned (non-fatal): %v", pollErr)
	}
}

// runRootTTY drives the assembled root command with a real terminal on
// stdin and `answer` already typed into it.
//
// It exists because the production interactive check is
// cliout.IsTTYReader(stdin), and under `go test` no ordinary reader —
// not a strings.Reader, not even os.Stdin — is a terminal. Without a
// pty, every test takes the non-interactive branch and the prompt
// path is unreachable.
//
// Not tied to a TC-ID — test helper.
func runRootTTY(t *testing.T, answer string, argv ...string) runResult {
	t.Helper()

	primary, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pty available on this platform: %v", err)
	}
	t.Cleanup(func() {
		_ = tty.Close()
		_ = primary.Close()
	})
	if _, err := primary.WriteString(answer); err != nil {
		t.Fatalf("write answer to pty: %v", err)
	}

	dataDir, dataDirErr := datadir.Resolve()
	if dataDirErr == nil {
		markInstallationEstablished(t, dataDir)
	}

	firstRunOwed := false
	r := clitest.RunWith(t, cmd.NewRoot(), clitest.Options{
		StdinReader: tty,
		PreRun: func(stderr io.Writer) {
			if dataDirErr == nil {
				firstRunOwed = notices.BeforeCommand(stderr, dataDir, time.Now(),
					append([]string{"tai"}, argv...))
			}
		},
		PostRun: func(stderr io.Writer) {
			if dataDirErr == nil {
				notices.AfterCommand(stderr, dataDir, time.Now(), firstRunOwed)
			}
		},
	}, argv...)

	return runResult{
		stdout:   r.Stdout,
		stderr:   r.Stderr,
		exitCode: r.ExitCode,
		err:      r.Err,
	}
}
