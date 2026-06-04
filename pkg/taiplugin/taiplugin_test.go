package taiplugin_test

import (
	"testing"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/pkg/taiplugin"
)

// TestLoad_populates_all_fields verifies the wire contract decode.
// Not tied to a TC-ID — the user-observable assertion lives in
// TC-PLG-005 at the CLI boundary; this is the engine anchor.
func TestLoad_populates_all_fields(t *testing.T) {
	t.Setenv("TAI_DATA_DIR", "/data")
	t.Setenv("TAI_CLONE_DIR", "/data/source")
	t.Setenv("TAI_TARGETS", `[{"root":"/home/u/.claude","skills":"/home/u/.claude/skills","commands":"/home/u/.claude/commands","agents":"/home/u/.claude/agents"}]`)

	ctx, err := taiplugin.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ctx.DataDir != "/data" {
		t.Errorf("DataDir: want /data, got %q", ctx.DataDir)
	}
	if ctx.CloneDir != "/data/source" {
		t.Errorf("CloneDir: want /data/source, got %q", ctx.CloneDir)
	}
	if len(ctx.Targets) != 1 {
		t.Fatalf("want 1 target, got %d", len(ctx.Targets))
	}
	if ctx.Targets[0].Root != "/home/u/.claude" {
		t.Errorf("Targets[0].Root: %q", ctx.Targets[0].Root)
	}
	if ctx.Targets[0].Skills != "/home/u/.claude/skills" {
		t.Errorf("Targets[0].Skills: %q", ctx.Targets[0].Skills)
	}
}

// TestLoad_empty_env_yields_zero_value verifies that absent env vars
// produce an empty Context — plugins are expected to check the
// fields they need.
//
// Not tied to a TC-ID — engine anchor for TC-PLG-005 (the user-
// observable assertion lives at the CLI boundary in
// core/internal/cmd/plugin_invoke_test.go).
func TestLoad_empty_env_yields_zero_value(t *testing.T) {
	t.Setenv("TAI_DATA_DIR", "")
	t.Setenv("TAI_CLONE_DIR", "")
	t.Setenv("TAI_TARGETS", "")

	ctx, err := taiplugin.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ctx.DataDir != "" || ctx.CloneDir != "" || len(ctx.Targets) != 0 {
		t.Errorf("expected zero Context, got %+v", ctx)
	}
}

// TestLoad_TCMIG003_malformed_targets_surfaces_INTERNAL_ERROR locks
// the host-bug contract referenced by TC-MIG-003 (the triage-side
// migration spec): if tai sends malformed JSON for TAI_TARGETS, the
// SDK surfaces a structured *errcode.Error{Code: INTERNAL_ERROR}
// rather than panicking or returning a half-populated Context.
//
// The TC lives in plugins/triage/test-cases.md because Triage was the
// first plugin to consume the wire contract; the underlying contract
// is the SDK's (so the test lives here), and every future plugin
// inherits it.
func TestLoad_TCMIG003_malformed_targets_surfaces_INTERNAL_ERROR(t *testing.T) {
	t.Setenv("TAI_DATA_DIR", "")
	t.Setenv("TAI_CLONE_DIR", "")
	t.Setenv("TAI_TARGETS", "{not json")

	_, err := taiplugin.Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	e, ok := errcode.As(err)
	if !ok {
		t.Fatalf("expected *errcode.Error, got %T %v", err, err)
	}
	if e.Code != errcode.InternalError {
		t.Errorf("Code: want INTERNAL_ERROR, got %s", e.Code)
	}
}

// TestEnvVars_round_trips_through_Load verifies that EnvVars and
// Load are inverses of each other — the wire contract round-trips
// without information loss. This is the engine-level anchor for
// TC-PLG-005's CLI-side assertion.
func TestEnvVars_round_trips_through_Load(t *testing.T) {
	want := []taiplugin.Target{
		{Root: "/h/.claude", Skills: "/h/.claude/skills", Commands: "/h/.claude/commands", Agents: "/h/.claude/agents"},
		{Root: "/h/.opencode", Skills: "/h/.opencode/skills", Commands: "", Agents: "/h/.opencode/agents"},
	}
	env, err := taiplugin.EnvVars("/data", "/data/source", want)
	if err != nil {
		t.Fatalf("EnvVars: %v", err)
	}
	for _, kv := range env {
		// Apply each KEY=VALUE pair into the test environment.
		eq := -1
		for i, r := range kv {
			if r == '=' {
				eq = i
				break
			}
		}
		t.Setenv(kv[:eq], kv[eq+1:])
	}
	got, err := taiplugin.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.DataDir != "/data" {
		t.Errorf("DataDir round-trip: %q", got.DataDir)
	}
	if got.CloneDir != "/data/source" {
		t.Errorf("CloneDir round-trip: %q", got.CloneDir)
	}
	if len(got.Targets) != len(want) {
		t.Fatalf("target count: want %d, got %d", len(want), len(got.Targets))
	}
	for i, w := range want {
		if got.Targets[i] != w {
			t.Errorf("target[%d]: want %+v, got %+v", i, w, got.Targets[i])
		}
	}
}
