package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestIsStale_boundary_cases pins the four edge cases of the
// staleness predicate. CLI-boundary coverage for the user-observable
// "cache untouched / cache refreshed" behaviour lives in TC-SYNC-014
// through TC-SYNC-017, but those tests use clearly-stale (year 2000)
// or clearly-fresh (now) timestamps. The boundary semantics — `>=`
// versus `>`, zero-LastCheck handling, zero-interval handling — are
// engine invariants this test pins so they can't drift silently.
//
// Not tied to a TC-ID; engine invariant.
func TestIsStale_boundary_cases(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	hour := time.Hour

	cases := []struct {
		name     string
		state    PollState
		interval time.Duration
		now      time.Time
		want     bool
	}{
		{
			name:     "zero interval disables polling regardless of timestamp",
			state:    PollState{LastCheck: now.Add(-100 * hour)},
			interval: 0,
			now:      now,
			want:     false,
		},
		{
			name:     "negative interval also disables polling",
			state:    PollState{LastCheck: now.Add(-100 * hour)},
			interval: -1,
			now:      now,
			want:     false,
		},
		{
			name:     "zero LastCheck (never polled) is stale",
			state:    PollState{},
			interval: hour,
			now:      now,
			want:     true,
		},
		{
			name:     "exactly at the interval is stale (>= not >)",
			state:    PollState{LastCheck: now.Add(-hour)},
			interval: hour,
			now:      now,
			want:     true,
		},
		{
			name:     "one nanosecond before the interval is fresh",
			state:    PollState{LastCheck: now.Add(-hour + time.Nanosecond)},
			interval: hour,
			now:      now,
			want:     false,
		},
		{
			name:     "well past the interval is stale",
			state:    PollState{LastCheck: now.Add(-2 * hour)},
			interval: hour,
			now:      now,
			want:     true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.state.IsStale(tc.now, tc.interval)
			if got != tc.want {
				t.Errorf("IsStale = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSaveState_cleans_stale_tmp confirms a leftover .tmp from a
// previously-killed goroutine doesn't block the next SaveState. The
// atomic rename pattern leaves the canonical state file intact even
// when the writer is interrupted; the cleanup-on-entry ensures the
// directory doesn't accrete stale .tmp files indefinitely.
//
// Not tied to a TC-ID; this is a maintenance-burden invariant.
func TestSaveState_cleans_stale_tmp(t *testing.T) {
	dataDir := t.TempDir()
	statePath := StatePath(dataDir)
	tmpPath := statePath + ".tmp"
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(tmpPath, []byte("stale leftover"), 0o644); err != nil {
		t.Fatalf("seed stale tmp: %v", err)
	}

	if err := SaveState(dataDir, PollState{LastCheck: time.Now()}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file not written: %v", err)
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("stale tmp should be removed after SaveState, stat returned: %v", err)
	}

	body, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var got PollState
	if err := json.Unmarshal(body, &got); err != nil {
		t.Errorf("state file is not valid JSON: %v\n%s", err, body)
	}
}
