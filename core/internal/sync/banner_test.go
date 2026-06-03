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
	extendPollWithBannerLayers(context.Background(), dataDir, &state)

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

	stubLatest(t,
		map[string]string{"github.com/acme/ok": "v1.1.0"},
		map[string]bool{"github.com/acme/bad": true},
	)

	state := PollState{}
	extendPollWithBannerLayers(context.Background(), dataDir, &state)

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
	extendPollWithBannerLayers(context.Background(), dataDir, &state)

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

// Compile-time check that the state path stays under the expected
// directory layout. Not tied to a TC-ID.
func TestStatePath_layout(t *testing.T) {
	got := StatePath("/data")
	want := filepath.Join("/data", "state", "update-check.json")
	if got != want {
		t.Errorf("StatePath: want %q, got %q", want, got)
	}
}
