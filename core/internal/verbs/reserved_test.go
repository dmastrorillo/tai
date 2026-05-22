package verbs_test

import (
	"testing"

	"github.com/dmastrorillo/tai/core/internal/verbs"
)

// TestReserved_matches_spec_list locks the canonical reserved-verbs
// list to the spec's Requirement: Plugin subprocess invocation. Any
// change here MUST also update
// openspec/changes/pivot-to-ai-as-code/specs/plugin-host/spec.md.
//
// Not tied to a TC-ID because plugin-host install behaviour lands in
// later phase tasks; this is a code-level invariant guarding the
// taxonomy.
func TestReserved_matches_spec_list(t *testing.T) {
	want := []string{
		"config",
		"sync",
		"repo",
		"install-commands",
		"workflow",
		"standards",
		"plugins",
		"help",
		"version",
	}
	got := verbs.Reserved()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Reserved()[%d] = %q, want %q", i, got[i], w)
		}
	}
}

// TestReserved_returns_independent_copy confirms Reserved() returns a
// fresh slice each call — callers cannot mutate the package-private
// canonical list.
//
// Not tied to a TC-ID because immutability is a code-level invariant
// the plugin host depends on; install-time PLUGIN_NAME_RESERVED tests
// land with the plugin host in a later phase.
func TestReserved_returns_independent_copy(t *testing.T) {
	a := verbs.Reserved()
	a[0] = "tampered"
	b := verbs.Reserved()
	if b[0] == "tampered" {
		t.Fatal("Reserved() must return a copy; mutation leaked back to canonical store")
	}
}

// TestIsReserved exercises the boolean predicate in both directions.
//
// Not tied to a TC-ID for the same reason TestReserved_matches_spec_list
// isn't: install-time plugin-host TCs land with the plugin host phase.
func TestIsReserved(t *testing.T) {
	yes := []string{"config", "sync", "plugins"}
	no := []string{"triage", "Config", "CONFIG", "", "tai"}

	for _, n := range yes {
		if !verbs.IsReserved(n) {
			t.Errorf("IsReserved(%q) = false, want true", n)
		}
	}
	for _, n := range no {
		if verbs.IsReserved(n) {
			t.Errorf("IsReserved(%q) = true, want false", n)
		}
	}
}
