// Command tai is the core CLI entry point.
//
// The root *cli.Command (assembled by core/internal/cmd.NewRoot)
// carries --version, --help, and the present user-facing verbs (tai
// config, tai repo, tai sync). Subsequent phases (workflows,
// standards, plugin host, install-commands, update banner) graft
// onto the same root as their OpenSpec proposals land.
//
// On every invocation main fires off a non-blocking background
// goroutine that polls the configured source repo for newer commits
// (see core/internal/sync.Schedule). The poll is best-effort —
// failures (no config, no network, malformed YAML) are silently
// absorbed. The Wait deadline at exit keeps the goroutine from being
// killed mid-write when it's about to finish; if it overruns, the OS
// reaps it at process exit and the next invocation retries.
//
// main is the single place that calls os.Exit. Subcommands and
// library code under core/internal/ and plugins/<name>/internal/
// MUST return errors; main maps them via errcode.Code.ExitCode().
package main

import (
	"context"
	"os"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/dmastrorillo/tai/core/internal/cmd"
	"github.com/dmastrorillo/tai/core/internal/config"
	"github.com/dmastrorillo/tai/core/internal/sync"
	"github.com/dmastrorillo/tai/pkg/cliexec"
	"github.com/dmastrorillo/tai/pkg/cliout"
	"github.com/dmastrorillo/tai/pkg/datadir"
	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/pkg/exitcode"
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

	err := cliexec.Run(ctx, cmd.NewRoot(), os.Args)

	// Give the background poll a brief chance to finish writing its
	// state file before we exit. Overruns are reaped by the OS — the
	// next invocation retries per the cadence rule.
	if waiter != nil {
		_ = waiter.Wait(pollWaitOnExit)
	}

	if err == nil {
		os.Exit(exitcode.Success)
	}

	// Structured *errcode.Error: render the template and exit with
	// the code's mapped exit.
	if e, ok := errcode.As(err); ok {
		cliout.WriteError(os.Stderr, err)
		os.Exit(e.Code.ExitCode())
	}

	// Plugin subprocess exit: the child process has already written
	// its own stderr template. The error here is a cli.ExitCoder
	// (e.g. core/internal/cmd.pluginExitError) carrying only the
	// child's exit code. Don't render an INTERNAL_ERROR template
	// over the plugin's real output — just propagate the code.
	if ec, ok := err.(cli.ExitCoder); ok {
		os.Exit(ec.ExitCode())
	}

	// Truly unstructured error (unwrapped panic, third-party leak).
	// Surface as INTERNAL_ERROR so the OS exit and the rendered
	// footer agree.
	cliout.WriteError(os.Stderr, err)
	os.Exit(exitcode.Internal)
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
