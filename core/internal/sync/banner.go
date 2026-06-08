// Update-banner rendering and dispatch.
//
// The banner is the user-visible side of the background update-check
// poll: a once-per-day, stderr-only, `[tai]`-prefixed line block that
// names every pending update across TAI itself, installed plugins,
// and the configured source repo. The cadence rule is enforced by
// PollState.LastBannerDate (a `YYYY-MM-DD` string in the user's
// local time zone).
//
// Entry points:
//
//   - EmitBanner — called by main.go before the foreground command
//     runs; renders the banner from the current state file when the
//     once-per-day gate allows.
//   - renderBanner — pure function that turns a PollState into the
//     stderr text. No I/O.
//   - extendPollWithBannerLayers — called by Poll() during the
//     background goroutine; populates the TAI + installed-plugins
//     layers in the state file.
//   - LatestTagForTesting — test-only seam swapping the
//     GitHub-Releases `/releases/latest` query.
//   - LatestPrefixedTagForTesting — test-only seam swapping the
//     prefix-aware GitHub-Releases lookup (plugin rows only).
//
// Spec: openspec/changes/pivot-to-ai-as-code/specs/update-banner/
// spec.md.

package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/core/internal/plugins"
)

// taiReleaseRepo is the hard-coded GitHub repo whose latest release
// represents "the latest TAI". Per spec: "Queries the latest TAI
// release from a hard-coded source URL."
const taiReleaseRepo = "dmastrorillo/tai"

// taiUpgradeCommand is the package-manager hint embedded in the
// banner. The spec says TAI does not self-update — the banner names
// the command the user runs. The hint is platform-agnostic; users on
// a non-brew system will recognize the alternative.
const taiUpgradeCommand = "brew upgrade tai (or `go install github.com/dmastrorillo/tai/core/cmd/tai@latest`)"

// LatestTagFn is the signature of the function Poll uses to query
// GitHub Releases for a `<host>/<repo>`'s latest tag. Production
// binds it to httpLatestTag; tests substitute a stub.
type LatestTagFn func(ctx context.Context, host, repo string) (string, error)

// latestTagMu guards latestTag for concurrent reads/writes between
// the background Poll goroutine and tests that swap the function via
// LatestTagForTesting. Without the mutex, `go test -race` would
// flag a write/read race on the package-level var.
var (
	latestTagMu sync.RWMutex
	latestTag   LatestTagFn = httpLatestTag
)

// callLatestTag reads latestTag under the mutex and invokes it.
// Production code MUST use this accessor rather than reaching for
// the package var directly.
func callLatestTag(ctx context.Context, host, repo string) (string, error) {
	latestTagMu.RLock()
	fn := latestTag
	latestTagMu.RUnlock()
	return fn(ctx, host, repo)
}

// LatestTagForTesting swaps the latest-tag function for the lifetime
// of t. Mirrors the testing-bypass pattern used by
// config.AllowFileURLsForTesting and sync.AutoInstallForTesting.
func LatestTagForTesting(t testing.TB, fn LatestTagFn) {
	t.Helper()
	latestTagMu.Lock()
	prev := latestTag
	latestTag = fn
	latestTagMu.Unlock()
	t.Cleanup(func() {
		latestTagMu.Lock()
		latestTag = prev
		latestTagMu.Unlock()
	})
}

// LatestPrefixedTagFn is the signature of the prefix-aware lookup
// the plugin-row layer calls. Production binds it to
// httpLatestPrefixedTag (which wraps plugins.LatestPrefixedTag with
// the package's timeout-bounded HTTP client). Tests substitute a
// stub via LatestPrefixedTagForTesting.
//
// Return convention: ("", nil) is the "no matching stable release"
// sentinel; the caller MUST treat it as "no update available", NOT
// as an error. Real lookup failures return (_, non-nil) and the
// caller absorbs them per the spec.
type LatestPrefixedTagFn func(ctx context.Context, host, repo, prefix string) (string, error)

// latestPrefixedTagMu guards latestPrefixedTag for concurrent
// reads/writes between the background Poll goroutine and tests
// that swap the function via LatestPrefixedTagForTesting. Same
// race-safety contract as latestTagMu above; without the mutex,
// `go test -race` would flag a write/read race on the package-
// level var.
var (
	latestPrefixedTagMu sync.RWMutex
	latestPrefixedTag   LatestPrefixedTagFn = httpLatestPrefixedTag
)

// callLatestPrefixedTag reads latestPrefixedTag under the mutex and
// invokes it. Production code MUST use this accessor rather than
// reaching for the package var directly.
func callLatestPrefixedTag(ctx context.Context, host, repo, prefix string) (string, error) {
	latestPrefixedTagMu.RLock()
	fn := latestPrefixedTag
	latestPrefixedTagMu.RUnlock()
	return fn(ctx, host, repo, prefix)
}

// LatestPrefixedTagForTesting swaps the prefix-aware lookup
// function for the lifetime of t. Parallel to LatestTagForTesting.
func LatestPrefixedTagForTesting(t testing.TB, fn LatestPrefixedTagFn) {
	t.Helper()
	latestPrefixedTagMu.Lock()
	prev := latestPrefixedTag
	latestPrefixedTag = fn
	latestPrefixedTagMu.Unlock()
	t.Cleanup(func() {
		latestPrefixedTagMu.Lock()
		latestPrefixedTag = prev
		latestPrefixedTagMu.Unlock()
	})
}

// httpLatestPrefixedTag is the production binding for
// LatestPrefixedTagFn. Wraps plugins.LatestPrefixedTag with the
// package's timeout-bounded client.
func httpLatestPrefixedTag(ctx context.Context, host, repo, prefix string) (string, error) {
	return plugins.LatestPrefixedTag(ctx, releaseHTTPClient, "", host, repo, prefix)
}

// releaseHTTPClient is the timeout-bounded client used for GitHub
// Releases lookups. 10s is generous for a release-metadata call but
// short enough that an unresponsive server doesn't keep the
// background goroutine alive past the parent's exit. Matches the
// pattern in core/internal/plugins/fetch.go which lets callers
// inject a client with its own timeout.
var releaseHTTPClient = &http.Client{Timeout: 10 * time.Second}

// httpLatestTag is the production implementation. Queries
// `https://api.github.com/repos/<repo>/releases/latest` and returns
// the `tag_name`. Authentication is opportunistic (GITHUB_TOKEN env
// var when set); failures return an error which the caller silently
// absorbs (per spec — the cache file is just not updated).
func httpLatestTag(ctx context.Context, host, repo string) (string, error) {
	if host != "github.com" {
		return "", fmt.Errorf("unsupported release host %q", host)
	}
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := releaseHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return payload.TagName, nil
}

// EmitBanner reads the poll state at dataDir, emits a banner to
// stderr when the once-per-day gate allows, and persists the new
// last-banner-date back to the state file.
//
// Returns silently when there is nothing to do (no state file, no
// pending updates, banner already fired today). All errors are
// best-effort absorbed — a missing or unreadable state file is the
// spec's "first-ever invocation" branch.
//
// The `now` parameter is the wall-clock used to compute today's
// calendar day. Production passes time.Now(); tests substitute a
// frozen instant so the banner-suppression assertion is
// deterministic.
func EmitBanner(stderr io.Writer, dataDir string, now time.Time) {
	state, err := LoadState(dataDir)
	if err != nil {
		// Unparseable state — silently absorb. The next poll will
		// rewrite the file.
		return
	}
	if !state.HasPendingUpdate() {
		return
	}
	today := now.Local().Format(time.DateOnly)
	if state.LastBannerDate == today {
		return
	}

	rendered := renderBanner(state)
	if rendered == "" {
		return
	}
	_, _ = io.WriteString(stderr, rendered)

	state.LastBannerDate = today
	_ = SaveState(dataDir, state)
}

// renderBanner produces the stderr text. Format invariants (per
// spec, locked by TC-UB-004 / TC-UB-005):
//
//   - every line starts with `[tai]`
//   - at most 4 lines total
//   - each pending layer gets one row with its update command
//
// The header is always present; up to three layer rows follow. When
// more than one plugin update is pending, the per-plugin rows
// collapse into a single "N plugins" line so the 4-line cap holds
// even when TAI + plugins + source-repo all have updates.
func renderBanner(s PollState) string {
	rows := []string{}

	if s.TAICurrent != "" && s.TAILatest != "" && s.TAICurrent != s.TAILatest {
		rows = append(rows, fmt.Sprintf("[tai]   tai %s → %s   run: %s",
			s.TAICurrent, s.TAILatest, taiUpgradeCommand))
	}

	pluginsPending := []PluginUpdate{}
	for _, p := range s.Plugins {
		if p.Current != "" && p.Latest != "" && p.Current != p.Latest {
			pluginsPending = append(pluginsPending, p)
		}
	}
	switch {
	case len(pluginsPending) == 0:
		// no plugin row
	case len(pluginsPending) == 1:
		p := pluginsPending[0]
		rows = append(rows, fmt.Sprintf("[tai]   %s %s → %s   run: tai plugins update %s",
			p.Name, p.Current, p.Latest, p.Name))
	default:
		// Collapse to keep the 4-line cap. Lists all names in one
		// row; the user runs `tai plugins update <name>` per entry.
		names := make([]string, len(pluginsPending))
		for i, p := range pluginsPending {
			names[i] = p.Name
		}
		rows = append(rows, fmt.Sprintf("[tai]   %d plugins have updates (%s)   run: tai plugins update <name>",
			len(pluginsPending), strings.Join(names, ", ")))
	}

	if s.HasUpdates {
		rows = append(rows, "[tai]   source repo has new commits   run: tai sync")
	}

	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("[tai] Updates available:\n")
	for _, r := range rows {
		b.WriteString(r)
		b.WriteString("\n")
	}
	return b.String()
}

// extendPollWithBannerLayers is called by Poll() after the source-
// repo check completes; it queries the TAI release and each
// installed plugin's release, populating the new state fields.
// Failures (network, 5xx) are silently absorbed per spec: the
// affected layer is NOT written to the state file, so the next
// poll retries cleanly per the cadence rule. (Pre-fix behaviour
// wrote "Latest = Current" on failure, which suppressed the next
// retry until the cache expired — a violation surfaced in review.)
//
// The `currentVersion` parameter is the running binary's
// `version.String`. Production passes `version.String` directly;
// tests pass concrete values like "v0.6.0" or "dev" so the routing
// rule ("dev → skip TAI layer; stamped → query the legacy
// /releases/latest seam") can be exercised without -ldflags.
// Threading it as a parameter avoids violating CLAUDE.md's "tests
// MUST NOT mutate version.String" rule.
func extendPollWithBannerLayers(ctx context.Context, dataDir, currentVersion string, state *PollState) {
	// TAI itself. Only meaningful when the running binary has a
	// concrete (non-"dev") version stamped via -ldflags="-X
	// version.String=…". Local builds skip the check entirely so
	// the field stays empty rather than being half-populated.
	if v := strings.TrimSpace(currentVersion); v != "" && v != "dev" {
		if tag, err := callLatestTag(ctx, "github.com", taiReleaseRepo); err == nil {
			state.TAICurrent = v
			state.TAILatest = tag
		}
	}

	// Installed plugins. Empty plugin state is a non-error. A
	// per-plugin tag-query failure is silently absorbed: that
	// plugin's row is omitted from the state, so the next poll
	// will retry rather than seeing a fresh-but-stale "no update"
	// entry.
	pluginsState, err := plugins.LoadState(dataDir)
	if err != nil || pluginsState == nil {
		return
	}
	state.Plugins = state.Plugins[:0]
	for _, e := range pluginsState.Plugins {
		// Prefix-aware lookup. For first-party plugins from this
		// monorepo, PluginTagPrefix returns "plugins/<name>/" — the
		// algorithm filters out unprefixed (core) tags and other
		// plugin streams. For third-party plugins, it returns ""
		// and the algorithm matches every tag from the foreign
		// source repo. Either way, the legacy /releases/latest
		// endpoint is no longer used for plugin lookups — see
		// release-cycle spec D5.
		prefix := plugins.PluginTagPrefix(e.Name, e.Source)
		fullTag, err := callLatestPrefixedTag(ctx, e.Source.Host, e.Source.Repo, prefix)
		if err != nil {
			// Real failure (network, 5xx). Omit the row so the
			// next poll retries cleanly — same contract as the
			// pre-release-cycle behaviour.
			continue
		}
		if fullTag == "" {
			// "No matching stable release" sentinel. Omit the row;
			// callers MUST treat this as "no update available" and
			// the user will see a normal banner-less invocation.
			continue
		}
		// LatestPrefixedTag returns the FULL tag_name (e.g.
		// "plugins/triage/v0.5.0"). Strip the prefix for display
		// and version-comparison; `e.Version` stored in plugins.json
		// is the stripped form (`v0.5.0`), so the Latest field must
		// match that shape for `Current != Latest` to be meaningful.
		state.Plugins = append(state.Plugins, PluginUpdate{
			Name:    e.Name,
			Current: e.Version,
			Latest:  strings.TrimPrefix(fullTag, prefix),
		})
	}
}
