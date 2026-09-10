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
// main also brackets the foreground command with
// notices.BeforeCommand / notices.AfterCommand, which own the
// once-per-day update banner and the once-ever first-run onboarding
// hint. The e2e harness in core/internal/cmd calls the same pair, so
// a wiring regression (wrong stream, wrong directory, call omitted)
// fails a test rather than shipping (see TC-UB-007, TC-UB-008).
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
	"github.com/dmastrorillo/tai/core/internal/notices"
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

	// Unsolicited stderr notices bracket the foreground command: the
	// once-per-day update banner before it (so it shows even when the
	// command errors), the once-ever first-run hint after it (so the
	// "run this next" line lands below the output of what just ran).
	// notices owns which of the two fires — they never stack.
	//
	// Stream: the real os.Stderr, not the cli.Command's ErrWriter,
	// because neither notice belongs to a command. Both calls absorb
	// their own failures; a broken state file costs a notice, never
	// the command.
	dataDir, dataDirErr := datadir.Resolve()
	firstRunOwed := false
	if dataDirErr == nil {
		firstRunOwed = notices.BeforeCommand(os.Stderr, dataDir, time.Now(), os.Args)
	}

	err := cliexec.Run(ctx, cmd.NewRoot(), os.Args)

	if dataDirErr == nil {
		notices.AfterCommand(os.Stderr, dataDir, time.Now(), firstRunOwed)
	}

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
