package sync

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dmastrorillo/tai/core/internal/config"
)

// PollState records the result of the most recent background
// update-check poll across all three update layers: TAI itself, every
// installed plugin, and the configured source repo. The struct is
// the JSON shape written to <TAI_DATA_DIR>/state/update-check.json
// and consumed by the update-banner.
type PollState struct {
	// LastCheck is when the poll completed. Drives the next-run
	// staleness decision in IsStale.
	LastCheck time.Time `json:"last-check"`

	// LastBannerDate is the calendar day (local TZ, formatted
	// YYYY-MM-DD) the banner most recently fired. The banner is
	// suppressed when this equals today.
	LastBannerDate string `json:"last-banner-date,omitempty"`

	// Source-repo layer (Phase 2 — unchanged).
	LocalCommit  string `json:"local-commit,omitempty"`
	RemoteCommit string `json:"remote-commit,omitempty"`
	// HasUpdates is true when LocalCommit and RemoteCommit differ.
	// Preserved for backwards compatibility with the Phase 2 tests.
	HasUpdates bool `json:"has-updates"`

	// TAI-itself layer (Phase 5). Empty strings mean "not checked
	// this poll" — either the version was "dev" (local build) or
	// the HTTP query failed.
	TAICurrent string `json:"tai-current,omitempty"`
	TAILatest  string `json:"tai-latest,omitempty"`

	// Installed-plugins layer (Phase 5). One entry per installed
	// plugin whose latest tag was successfully queried.
	Plugins []PluginUpdate `json:"plugins,omitempty"`
}

// PluginUpdate is one row in PollState.Plugins describing the
// installed-vs-available version for a single plugin.
type PluginUpdate struct {
	// Name is the plugin's directory-name identity, matching the
	// state entry in plugins.json.
	Name string `json:"name"`
	// Current is the version currently installed (from
	// plugins.json's entry.Version at poll time).
	Current string `json:"current"`
	// Latest is the most recent release tag the poll observed. The
	// banner fires for this plugin when Latest != Current AND both
	// are non-empty. When the HTTP query for the plugin's source
	// fails, the row is omitted from PollState.Plugins entirely
	// (rather than written with Latest == Current) so the next
	// poll retries cleanly per the spec.
	Latest string `json:"latest"`
}

// HasPendingUpdate reports whether any layer has a pending update.
// Used by the banner gate. Guards every layer against the empty-
// string-corruption case (a state file with one of the fields
// missing) so a partial cache never spuriously triggers the banner.
func (s PollState) HasPendingUpdate() bool {
	if s.HasUpdates {
		return true
	}
	if s.TAICurrent != "" && s.TAILatest != "" && s.TAICurrent != s.TAILatest {
		return true
	}
	for _, p := range s.Plugins {
		if p.Current != "" && p.Latest != "" && p.Current != p.Latest {
			return true
		}
	}
	return false
}

// StatePath is the canonical location of the poll state file. Lives
// under <TAI_DATA_DIR>/state/ so plugin state can coexist alongside
// without collision.
func StatePath(dataDir string) string {
	return filepath.Join(dataDir, "state", "update-check.json")
}

// LoadState reads the poll state file. Returns (zero-value, nil) when
// the file does not yet exist — first-ever poll, banner has nothing
// to say.
func LoadState(dataDir string) (PollState, error) {
	data, err := os.ReadFile(StatePath(dataDir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return PollState{}, nil
		}
		return PollState{}, err
	}
	var s PollState
	if err := json.Unmarshal(data, &s); err != nil {
		return PollState{}, err
	}
	return s, nil
}

// SaveState writes the poll state file atomically (rename-over to
// avoid torn writes when two TAI invocations race).
//
// We pre-clean any stale `<path>.tmp` left over by a previously-killed
// goroutine — without this, repeated invocations where the goroutine
// is reaped mid-write would slowly accumulate orphaned tmp files in
// the state directory.
func SaveState(dataDir string, s PollState) error {
	path := StatePath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	// Best-effort cleanup of any stale tmp from a prior killed run.
	_ = os.Remove(tmp)

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// IsStale reports whether the cache's last-check is older than
// interval. interval == 0 disables polling — IsStale returns false in
// that case so callers skip the work.
func (s PollState) IsStale(now time.Time, interval time.Duration) bool {
	if interval <= 0 {
		return false
	}
	if s.LastCheck.IsZero() {
		return true
	}
	return now.Sub(s.LastCheck) >= interval
}

// Poll performs one synchronous update-check against the configured
// source repo and writes the result into <TAI_DATA_DIR>/state/
// update-check.json. Errors are returned but callers MUST swallow
// them in production — see Schedule below for the fire-and-forget
// shape.
//
// Skips the work (returns nil) when:
//
//   - cfg is nil or has no repo-url (no source to check)
//   - interval is 0 (poll disabled)
//   - the cache is fresh (within interval since last check)
//
// In each skip case the state file is NOT modified.
func Poll(ctx context.Context, cfg *config.File, dataDir string) error {
	if cfg == nil || strings.TrimSpace(cfg.RepoURL) == "" {
		return nil
	}
	interval, err := cfg.EffectiveUpdateCheckInterval()
	if err != nil {
		return err
	}
	state, _ := LoadState(dataDir) // tolerate unparseable existing state
	if !state.IsStale(time.Now(), interval) {
		return nil
	}

	remote, err := lsRemote(ctx, cfg.RepoURL)
	if err != nil {
		// Whole-poll silent absorb (TC-SYNC-016): when the source-
		// repo query fails, the state file is NOT updated at all.
		// TAI-itself and plugin-version queries are skipped too —
		// the next cadence tick retries the entire poll cleanly.
		// Trade-off: a private or temporarily-unreachable source
		// repo also blocks the TAI / plugin banner layers. A
		// future spec revision could relax this to per-layer
		// independence; today's contract is "whole poll or none"
		// per the Phase 2 cache-invariant assertion.
		return err
	}
	local := localHeadCommit(ctx, dataDir)

	state.LastCheck = time.Now()
	state.LocalCommit = local
	state.RemoteCommit = remote
	state.HasUpdates = remote != "" && local != "" && remote != local

	// Phase 5: layer in TAI's own latest tag + per-plugin latest
	// tags. Per-layer failures are silently absorbed (the affected
	// layer is omitted from state) so the next poll retries cleanly
	// without polluting the cache with fake "no update" rows.
	extendPollWithBannerLayers(ctx, dataDir, &state)

	return SaveState(dataDir, state)
}

// Schedule fires Poll on a background goroutine and returns a
// *Waiter the caller uses to wait briefly (or detach) at exit time.
//
// Production main.go invokes this near startup with the configured
// values and calls Waiter.Wait(timeout) before exiting. Tests can
// call Poll synchronously instead.
type Waiter struct {
	wg sync.WaitGroup
}

// Wait blocks for up to timeout for the goroutine to finish. If the
// goroutine hasn't finished by then, the function returns and the
// goroutine continues in the background (the OS reaps it at process
// exit). Returns true iff the goroutine completed within timeout.
func (w *Waiter) Wait(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// Schedule starts the background poll. Returns the Waiter immediately;
// the goroutine runs concurrently with the foreground command.
func Schedule(ctx context.Context, cfg *config.File, dataDir string) *Waiter {
	w := &Waiter{}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		_ = Poll(ctx, cfg, dataDir)
	}()
	return w
}

// lsRemote runs `git ls-remote <repo-url> HEAD` to fetch the
// default-branch SHA without cloning. Returns the SHA on success.
func lsRemote(ctx context.Context, repoURL string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", repoURL, "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	// Output: "<sha>\tHEAD\n"
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", errors.New("empty ls-remote output")
	}
	return fields[0], nil
}

// localHeadCommit returns the SHA at HEAD of the local clone, or ""
// when no clone exists. Uses exec.CommandContext so the goroutine can
// be cancelled if the caller's context expires.
func localHeadCommit(ctx context.Context, dataDir string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", CloneDir(dataDir), "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
