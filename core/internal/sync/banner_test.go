package sync

// Internal package tests for the banner-layer extension logic in
// poll.go. The CLI-boundary banner tests live in
// core/internal/cmd/banner_test.go.

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/core/internal/plugins"
)

// stubLatest installs a LatestTagFn that returns the values from a
// map keyed by `<host>/<repo>`, or an error when the key is in
// failures. Not tied to a TC-ID.
func stubLatest(t *testing.T, tags map[string]string, failures map[string]bool) {
	t.Helper()
	LatestTagForTesting(t, func(_ context.Context, host, repo string) (string, error) {
		key := host + "/" + repo
		if failures[key] {
			return "", errors.New("stub failure for " + key)
		}
		if tag, ok := tags[key]; ok {
			return tag, nil
		}
		return "", errors.New("no tag stubbed for " + key)
	})
}

// TestExtendPollWithBannerLayers_skips_dev_version anchors the rule
// that local "dev" builds skip the TAI layer entirely (so the cache
// isn't half-populated with TAICurrent set but TAILatest empty).
//
// Production version.String is unstamped during `go test`, defaulting
// to whatever the package initializes — typically "dev". When that's
// the case, the TAI layer is omitted; tests don't need to stub the
// HTTP path for it.
func TestExtendPollWithBannerLayers_skips_dev_version(t *testing.T) {
	stubLatest(t, map[string]string{}, nil)

	dataDir := t.TempDir()
	state := PollState{}
	extendPollWithBannerLayers(context.Background(), dataDir, "dev", &state)

	if state.TAICurrent != "" || state.TAILatest != "" {
		t.Errorf("dev build should leave TAI layer empty, got Current=%q Latest=%q",
			state.TAICurrent, state.TAILatest)
	}
}

// TestExtendPollWithBannerLayers_plugin_failure_omits_row anchors
// the spec-compliant behaviour caught in review: a per-plugin
// latest-tag failure MUST NOT write a fake "Latest = Current" row.
// The plugin is omitted from PollState.Plugins so the next poll
// retries cleanly.
//
// Both "ok-plugin" and "bad-plugin" are third-party (not in the
// built-in registry), so PluginTagPrefix returns "" and the new
// prefix-aware seam is invoked with an empty prefix.
func TestExtendPollWithBannerLayers_plugin_failure_omits_row(t *testing.T) {
	dataDir := t.TempDir()

	// Seed plugin state so extendPollWithBannerLayers has plugins
	// to iterate over.
	pstate := &plugins.State{Plugins: []plugins.Entry{
		{
			Name:    "ok-plugin",
			Source:  plugins.Source{Host: "github.com", Repo: "acme/ok"},
			Version: "v1.0.0",
		},
		{
			Name:    "bad-plugin",
			Source:  plugins.Source{Host: "github.com", Repo: "acme/bad"},
			Version: "v2.0.0",
		},
	}}
	if err := plugins.SaveState(dataDir, pstate); err != nil {
		t.Fatalf("save plugins state: %v", err)
	}

	stubPrefixed(t,
		map[string]string{"github.com/acme/ok|": "v1.1.0"},
		map[string]bool{"github.com/acme/bad|": true},
	)

	state := PollState{}
	extendPollWithBannerLayers(context.Background(), dataDir, "dev", &state)

	if len(state.Plugins) != 1 {
		t.Fatalf("failed plugin should be omitted, got %d rows: %+v", len(state.Plugins), state.Plugins)
	}
	got := state.Plugins[0]
	if got.Name != "ok-plugin" || got.Current != "v1.0.0" || got.Latest != "v1.1.0" {
		t.Errorf("ok-plugin row: %+v", got)
	}
}

// TestExtendPollWithBannerLayers_no_plugin_state_is_noop verifies
// that the absence of plugins.json is not an error — the function
// just returns having populated only whatever layers it could.
func TestExtendPollWithBannerLayers_no_plugin_state_is_noop(t *testing.T) {
	stubLatest(t, map[string]string{}, nil)

	dataDir := t.TempDir()
	// No plugins.json written. extendPoll should not crash.
	state := PollState{LastCheck: time.Now()}
	extendPollWithBannerLayers(context.Background(), dataDir, "dev", &state)

	if len(state.Plugins) != 0 {
		t.Errorf("no plugin state should yield no rows, got: %+v", state.Plugins)
	}
}

// TestCallLatestTag_is_concurrent_safe runs a goroutine that
// continuously reads `latestTag` via `callLatestTag` while a parallel
// goroutine swaps the function via `LatestTagForTesting`. Under
// `-race`, an unguarded variable would surface a write/read race
// here. The test exists to lock the mutex contract added in the
// Phase 5 review fix.
func TestCallLatestTag_is_concurrent_safe(t *testing.T) {
	// One swap, one read concurrently. The mutex contract should
	// hold even though both operations target the same package var.
	done := make(chan struct{})
	go func() {
		LatestTagForTesting(t, func(_ context.Context, _, _ string) (string, error) {
			return "v0.0.0", nil
		})
		close(done)
	}()
	_, _ = callLatestTag(context.Background(), "github.com", "x/y")
	<-done
	// Re-read after swap completed; should observe the new value.
	tag, err := callLatestTag(context.Background(), "github.com", "x/y")
	if err != nil || tag != "v0.0.0" {
		t.Errorf("post-swap callLatestTag: tag=%q err=%v", tag, err)
	}
}

// stubPrefixed installs a LatestPrefixedTagFn that returns canned
// (tag, error) values per `<host>/<repo>|<prefix>` key. Mirrors
// stubLatest but for the prefix-aware seam introduced by the
// release-cycle change.
func stubPrefixed(t *testing.T, results map[string]string, failures map[string]bool) {
	t.Helper()
	LatestPrefixedTagForTesting(t, func(_ context.Context, host, repo, prefix string) (string, error) {
		key := host + "/" + repo + "|" + prefix
		if failures[key] {
			return "", errors.New("stub failure for " + key)
		}
		if tag, ok := results[key]; ok {
			return tag, nil
		}
		// Default: no matching release. Empty-string sentinel,
		// nil error — caller must treat as "no update available".
		return "", nil
	})
}

// TestBanner_TCREL006_plugin_row_uses_prefix_aware_lookup verifies
// that the banner's plugin row is populated from the prefix-aware
// lookup, NOT from `/releases/latest`. The fixture has installed
// triage v0.4.0 and a mixed-tag release stream; the row MUST read
// `triage v0.4.0 → v0.5.0` (the highest stable triage tag) and
// MUST NOT pick up a chronologically newer core release.
//
// TC-ID: TC-REL-006 (core/test-cases.md).
func TestBanner_TCREL006_plugin_row_uses_prefix_aware_lookup(t *testing.T) {
	dataDir := t.TempDir()
	pstate := &plugins.State{Plugins: []plugins.Entry{
		{
			Name: "triage",
			// Source matches the monorepo entry in the registry,
			// so PluginTagPrefix returns "plugins/triage/".
			Source:  plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
			Version: "v0.4.0",
		},
	}}
	if err := plugins.SaveState(dataDir, pstate); err != nil {
		t.Fatalf("save plugins state: %v", err)
	}

	stubLatest(t, map[string]string{}, nil) // core lookup is not exercised here (dev build).
	// stubPrefixed returns the FULL tag_name (matching production
	// behaviour after the TC-REL-001 critical fix). The banner
	// layer strips the prefix for display before writing
	// PluginUpdate.Latest.
	stubPrefixed(t,
		map[string]string{
			"github.com/dmastrorillo/tai|plugins/triage/": "plugins/triage/v0.5.0",
		},
		nil,
	)

	state := PollState{}
	extendPollWithBannerLayers(context.Background(), dataDir, "dev", &state)

	if len(state.Plugins) != 1 {
		t.Fatalf("want 1 plugin row, got %d: %+v", len(state.Plugins), state.Plugins)
	}
	got := state.Plugins[0]
	if got.Name != "triage" || got.Current != "v0.4.0" || got.Latest != "v0.5.0" {
		t.Errorf("plugin row: got %+v, want {Name:triage Current:v0.4.0 Latest:v0.5.0}", got)
	}
}

// TestBanner_TCREL007_plugin_row_skips_prereleases verifies that
// when the only newer release for a plugin is a pre-release, the
// row is OMITTED from PollState.Plugins. Matches the existing
// failure-handling contract (omit, do not write a fake row) so the
// next poll retries cleanly once a stable release appears.
//
// TC-ID: TC-REL-007.
func TestBanner_TCREL007_plugin_row_skips_prereleases(t *testing.T) {
	dataDir := t.TempDir()
	pstate := &plugins.State{Plugins: []plugins.Entry{
		{
			Name:    "triage",
			Source:  plugins.Source{Host: "github.com", Repo: "dmastrorillo/tai"},
			Version: "v0.4.0",
		},
	}}
	if err := plugins.SaveState(dataDir, pstate); err != nil {
		t.Fatalf("save plugins state: %v", err)
	}

	stubLatest(t, map[string]string{}, nil)
	// Empty string for the prefix key means the lookup returned the
	// "no stable release" sentinel — exactly what TC-REL-003 pins
	// for LatestPrefixedTag when the only match is a prerelease.
	stubPrefixed(t,
		map[string]string{"github.com/dmastrorillo/tai|plugins/triage/": ""},
		nil,
	)

	state := PollState{}
	extendPollWithBannerLayers(context.Background(), dataDir, "dev", &state)

	if len(state.Plugins) != 0 {
		t.Errorf("want 0 plugin rows (no stable release available), got %d: %+v",
			len(state.Plugins), state.Plugins)
	}
}

// TestBanner_TCREL008_core_row_uses_releases_latest verifies that
// the TAI core row uses the legacy /releases/latest seam
// (callLatestTag), NOT the prefix-aware lookup. Core tags carry no
// prefix and that endpoint already excludes pre-releases natively.
//
// Threading `currentVersion` as a parameter (rather than reading
// the package-global `version.String`) lets us pin both branches:
// a stamped "v0.6.0" exercises the production path, "dev"
// exercises the local-build skip. Both MUST route through
// callLatestTag, never through callLatestPrefixedTag.
//
// TC-ID: TC-REL-008.
func TestBanner_TCREL008_core_row_uses_releases_latest(t *testing.T) {
	dataDir := t.TempDir()

	LatestPrefixedTagForTesting(t, func(_ context.Context, host, repo, prefix string) (string, error) {
		// Any invocation against taiReleaseRepo through the
		// prefix-aware seam is a routing bug — the core layer MUST
		// use callLatestTag. The recorder fails the test directly.
		if host == "github.com" && repo == taiReleaseRepo {
			t.Errorf("core layer routed through prefix-aware seam (host=%q repo=%q prefix=%q)",
				host, repo, prefix)
		}
		return "", nil
	})
	legacyCalls := 0
	LatestTagForTesting(t, func(_ context.Context, host, repo string) (string, error) {
		if host == "github.com" && repo == taiReleaseRepo {
			legacyCalls++
			return "v0.6.1", nil
		}
		return "", nil
	})

	// Branch 1: stamped version → TAI layer fires; legacy seam runs
	// exactly once; the recorder above proves the prefix-aware seam
	// is NOT touched for the core lookup.
	state := PollState{}
	extendPollWithBannerLayers(context.Background(), dataDir, "v0.6.0", &state)

	if legacyCalls != 1 {
		t.Errorf("stamped core build: legacy /releases/latest must be called once, got %d", legacyCalls)
	}
	if state.TAICurrent != "v0.6.0" || state.TAILatest != "v0.6.1" {
		t.Errorf("stamped core build: state.TAI{Current,Latest} = %q,%q, want v0.6.0,v0.6.1",
			state.TAICurrent, state.TAILatest)
	}

	// Branch 2: "dev" → TAI layer is skipped entirely; legacy seam
	// is NOT called again.
	legacyBefore := legacyCalls
	state2 := PollState{}
	extendPollWithBannerLayers(context.Background(), dataDir, "dev", &state2)

	if legacyCalls != legacyBefore {
		t.Errorf("dev build: legacy seam called %d additional times, want 0 (TAI layer must be skipped)",
			legacyCalls-legacyBefore)
	}
	if state2.TAICurrent != "" || state2.TAILatest != "" {
		t.Errorf("dev build: TAI fields must stay empty, got Current=%q Latest=%q",
			state2.TAICurrent, state2.TAILatest)
	}
}

// Compile-time check that the state path stays under the expected
// directory layout. Not tied to a TC-ID.
func TestStatePath_layout(t *testing.T) {
	got := StatePath("/data")
	want := filepath.Join("/data", "state", "update-check.json")
	if got != want {
		t.Errorf("StatePath: want %q, got %q", want, got)
	}
}
