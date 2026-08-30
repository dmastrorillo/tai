// Command tai is the core CLI entry point.
//
// The root *cli.Command (assembled by core/internal/cmd.NewRoot)
// carries --version, --help, and every user-facing verb (tai config,
// tai repo, tai sync, tai workflow, tai standards,
// tai install-commands, tai plugins).
//
// On every invocation main fires off a non-blocking background
// goroutine that polls the configured source repo for newer commits
// (see core/internal/sync.Schedule). The poll is best-effort —
// failures (no config, no network, malformed YAML) are silently
// absorbed. The Wait deadline at exit keeps the goroutine from being
// killed mid-write when it's about to finish; if it overruns, the OS
// reaps it at process exit and the next invocation retries.
//
// Before dispatching the foreground command, main also calls
// sync.EmitBanner against the data directory. EmitBanner emits the
// once-per-day update banner to stderr based on whatever the most
// recent poll wrote to <TAI_DATA_DIR>/state/update-check.json. The
// banner fires PRE-foreground so the user sees it even if the
// command itself errors. The test harness in
// core/internal/cmd/root_test.go mirrors this call so banner
// behaviour is exercised end-to-end (see TC-UB-007).
//
// main is the single place that calls os.Exit. Subcommands and
// library code under core/internal/ and plugins/<name>/internal/
// MUST return errors; main maps them to exit codes via
// cliexec.Exit, the translation shared with every plugin binary.
package main

import (
	"context"
	"os"
	"time"

	"github.com/dmastrorillo/tai/core/internal/cmd"
	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/sync"
	"github.com/dmastrorillo/tai/pkg/cliexec"
	"github.com/dmastrorillo/tai/pkg/datadir"
)

// pollWaitOnExit is the per-invocation budget we give the background
// update-check goroutine to complete after the foreground command
// exits. 250ms is long enough that a fast write to the state file
// completes, short enough that a slow remote does not visibly delay
// fast commands like `tai --version`. If the goroutine overruns, the
// OS reaps it and the next invocation retries per the cadence rule —
// the spec explicitly contemplates this fallback.
const pollWaitOnExit = 250 * time.Millisecond

func main() {
	ctx := context.Background()

	waiter := schedulePoll(ctx)

	// Update banner: emit once-per-day to stderr based on whatever
	// the most recent poll wrote into the state file. Pre-foreground
	// so the user sees the banner even if the command itself errors.
	// EmitBanner silently absorbs any state-file issues — first-ever
	// invocations and rotated state files just produce no banner.
	//
	// Stream: writes to the real os.Stderr (not the cli.Command's
	// ErrWriter) because the banner fires BEFORE command dispatch.
	// The test harness mirrors this in runRoot by writing to the
	// captured stderr buffer; production tests are end-to-end via
	// the same routing.
	if dataDir, err := datadir.Resolve(); err == nil {
		sync.EmitBanner(os.Stderr, dataDir, time.Now())
	}

	err := cliexec.Run(ctx, cmd.NewRoot(), os.Args)

	// Give the background poll a brief chance to finish writing its
	// state file before we exit. Overruns are reaped by the OS — the
	// next invocation retries per the cadence rule.
	if waiter != nil {
		_ = waiter.Wait(pollWaitOnExit)
	}

	// cliexec.Exit owns the error → exit-code translation (structured
	// template rendering, plugin-subprocess passthrough, INTERNAL
	// fallback) so the rules can't drift between binary entry points.
	os.Exit(cliexec.Exit(os.Stderr, err))
}

// schedulePoll loads the config best-effort and starts the background
// update-check goroutine. Returns nil when there's nothing to do so
// the caller skips the Wait at exit. Cases that short-circuit to nil:
//
//   - No config file / malformed YAML.
//   - No repo-url (nothing to poll).
//   - update-check-interval is 0 (polling explicitly disabled — the
//     spec says the goroutine SHALL NOT run in this case).
//   - No resolvable data dir.
func schedulePoll(ctx context.Context) *sync.Waiter {
	cfgPath, err := config.ResolvePath()
	if err != nil {
		return nil
	}
	cfg, err := config.Load(cfgPath)
	if err != nil || cfg == nil {
		return nil
	}
	interval, err := cfg.EffectiveUpdateCheckInterval()
	if err != nil || interval <= 0 {
		return nil
	}
	dataDir, err := datadir.Resolve()
	if err != nil || dataDir == "" {
		return nil
	}
	return sync.Schedule(ctx, cfg, dataDir)
}
