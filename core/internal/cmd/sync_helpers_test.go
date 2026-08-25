package cmd_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/core/internal/cmd"
	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/sync"
	"github.com/dmastrorillo/tai/pkg/clitest"
	"github.com/dmastrorillo/tai/pkg/datadir"
)

// runRootStdin is the single harness body behind runRoot: it drives
// cmd.NewRoot through the shared pkg/clitest harness (the same one
// the triage plugin's cmdtest wraps), feeding the supplied string as
// stdin (sync prompt tests read it; everything else passes "").
//
// The PreRun hook mirrors main.go's pre-foreground update-banner
// emission so the banner-in-CLI wiring is exercised by every
// harness-based test; fixtures that don't seed update-check.json see
// no banner — EmitBanner silently returns when there is no state.
// Error rendering and the exit code come from cliexec.Exit inside
// clitest, the same translation the shipped binary performs, so this
// harness cannot drift from production behaviour.
//
// Not tied to a TC-ID — it's a test helper.
func runRootStdin(t *testing.T, stdin string, argv ...string) runResult {
	t.Helper()

	r := clitest.RunWith(t, cmd.NewRoot(), clitest.Options{
		Stdin: stdin,
		PreRun: func(stderr io.Writer) {
			if dataDir, err := datadir.Resolve(); err == nil {
				sync.EmitBanner(stderr, dataDir, time.Now())
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
