package sync

import (
	"os"
	"strings"
	"testing"
)

// TestBackgroundGitEnv_TCSYNC018_no_creds_prompt asserts the env
// overlay used by the background update-check path (lsRemote +
// localHeadCommit) carries the three vars that suppress git's
// interactive credential prompt:
//
//   - GIT_TERMINAL_PROMPT=0  → disables git's built-in prompt
//   - GIT_ASKPASS=/bin/echo  → overrides any inherited askpass to a no-op
//   - GCM_INTERACTIVE=Never  → tells Git Credential Manager to skip prompts
//
// The unit-test angle is sufficient to pin TC-SYNC-018: the contract
// is "these env vars are present on background git invocations".
// Stubbing git itself to assert the prompt is never emitted would
// add a PATH-shim binary for marginal extra coverage; the env-var
// overlay's correctness is the load-bearing property.
func TestBackgroundGitEnv_TCSYNC018_no_creds_prompt(t *testing.T) {
	env := withBackgroundGitEnv()

	want := map[string]string{
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_ASKPASS":         "/bin/echo",
		"GCM_INTERACTIVE":     "Never",
	}

	got := map[string]string{}
	for _, kv := range env {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			continue
		}
		got[kv[:i]] = kv[i+1:]
	}

	for k, v := range want {
		if got[k] != v {
			t.Errorf("withBackgroundGitEnv() missing or wrong key %q: got %q, want %q", k, got[k], v)
		}
	}
}

// TestBackgroundGitEnv_overrides_inherited_askpass guards against a
// regression where an inherited GIT_ASKPASS (set by the user's shell
// for some other tool) would survive into the background git
// invocation and re-enable interactive prompting.
func TestBackgroundGitEnv_overrides_inherited_askpass(t *testing.T) {
	t.Setenv("GIT_ASKPASS", "/usr/bin/ssh-askpass")

	env := withBackgroundGitEnv()

	found := false
	for _, kv := range env {
		if kv == "GIT_ASKPASS=/bin/echo" {
			found = true
		}
		if strings.HasPrefix(kv, "GIT_ASKPASS=") && kv != "GIT_ASKPASS=/bin/echo" {
			t.Errorf("inherited GIT_ASKPASS leaked into background env: %q", kv)
		}
	}
	if !found {
		t.Errorf("withBackgroundGitEnv() did not include GIT_ASKPASS=/bin/echo override")
	}
}

// TestBackgroundGitEnv_preserves_unrelated_parent_env asserts the
// overlay only replaces the three credential-prompt keys; unrelated
// inherited env (PATH, HOME, ...) survives into the child process.
func TestBackgroundGitEnv_preserves_unrelated_parent_env(t *testing.T) {
	t.Setenv("TAI_TEST_SENTINEL", "preserved-value")

	env := withBackgroundGitEnv()

	for _, kv := range env {
		if kv == "TAI_TEST_SENTINEL=preserved-value" {
			return
		}
	}
	t.Errorf("unrelated parent env not preserved; missing TAI_TEST_SENTINEL=preserved-value")
}

// TestSyncForeground_TCSYNC019_keeps_interactive_creds asserts the
// foreground git invocations in clone.go do NOT inherit the
// background overlay. Foreground sync is allowed to prompt
// interactively so users can authenticate to a private HTTPS source
// repo on first clone / fetch.
//
// The test runs clone.Clone (or Fetch) against a deliberately-invalid
// URL and inspects the os.Environ snapshot the child process inherits.
// We achieve this without spawning real git by checking that the
// foreground helpers do NOT call withBackgroundGitEnv() — they pass
// no Env override, leaving exec.Cmd's default of os.Environ() in
// place. The check is structural: scan the package's exported clone
// helpers via the exec.Cmd's Env field.
//
// In practice the simplest pin is: assert backgroundGitEnv is NOT
// applied to clone-path commands. Since clone.go doesn't set cmd.Env
// at all, that's already true; this test exists to catch a future
// regression where someone "consistency"-applies the overlay
// project-wide and breaks the foreground prompt path.
func TestSyncForeground_TCSYNC019_keeps_interactive_creds(t *testing.T) {
	// Read clone.go and verify it does not reference
	// withBackgroundGitEnv or backgroundGitEnv. This is a structural
	// guard, not a runtime assertion — the foreground prompt path is
	// only observable with an actual private HTTPS remote, which we
	// cannot stand up in unit tests.
	data, err := os.ReadFile("clone.go")
	if err != nil {
		t.Fatalf("read clone.go: %v", err)
	}
	body := string(data)
	if strings.Contains(body, "withBackgroundGitEnv") {
		t.Error("clone.go must NOT use withBackgroundGitEnv (foreground sync needs interactive creds — TC-SYNC-019)")
	}
	if strings.Contains(body, "backgroundGitEnv") {
		t.Error("clone.go must NOT reference backgroundGitEnv (foreground sync needs interactive creds — TC-SYNC-019)")
	}
}
