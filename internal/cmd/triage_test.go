package cmd_test

// Shared scaffolding for triage-verb E2E tests. Each TC-TRG-NNN
// scenario assembles a payload, pipes it through `tai import -` to
// seed the database under a per-test isolated env, then exercises
// the verb under test. Test helpers live here to keep individual
// test files focused on assertions.

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/internal/cmd"
	"github.com/dmastrorillo/tai/internal/cmdtest"
)

// initGitRepoOnBranch creates a fresh git repo in the test's tmp
// area, sets `origin` to a fixture URL whose owner_name matches the
// seeded fixture repo, checks out the named branch, and chdirs into
// it. Used by auto-detect TC tests so `git rev-parse --abbrev-ref
// HEAD` returns the branch the test seeded comments against.
func initGitRepoOnBranch(t *testing.T, branch string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "--quiet", "--initial-branch="+branch)
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "tai test")
	run("remote", "add", "origin", "https://github.com/acme/app.git")
	// An empty repo's HEAD points at the configured branch but the
	// ref doesn't exist; `git rev-parse --abbrev-ref HEAD` fails. Make
	// one empty commit so the branch is real and resolvable.
	run("commit", "--quiet", "--allow-empty", "-m", "init")
	cmdtest.Chdir(t, dir)
	return filepath.Clean(dir)
}

// triage runs a triage verb scoped to the canonical test repo. Tests
// run inside the tai repo itself, so without an explicit --repo flag
// the repoctx auto-detector resolves to dmastrorillo/tai (the real
// origin) rather than the seeded acme/app fixture. Prepending --repo
// pins every test to the seeded repo regardless of the host repo.
func triage(t *testing.T, args ...string) cmdtest.Result {
	t.Helper()
	full := append([]string{"--repo", "acme/app"}, args...)
	return cmdtest.Run(t, cmd.NewRoot(), full...)
}

// seedPR pipes a payload importing one PR with the given comments
// into a tai store rooted at the active isolation. Returns the import
// result so callers can assert if needed (most don't).
func seedPR(t *testing.T, prNumber int, comments string) {
	t.Helper()
	payload := buildPRPayload(prNumber, "feat: x", "feat/x", comments)
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)
}

func seedBranch(t *testing.T, name string, comments string) {
	t.Helper()
	payload := buildBranchPayload(name, comments)
	r := cmdtest.RunWithStdin(t, cmd.NewRoot(), payload, "import", "-")
	cmdtest.AssertNoError(t, r)
}

// commentJSON is a small DSL helper: produce a comment JSON snippet
// keyed by ref id so the test reads as a one-liner.
//
// Status is set via the corresponding mutation verb after import (the
// importer always seeds rows as 'pending'). The status argument here
// is retained for caller-side readability only; it is not embedded in
// the produced JSON.
func commentJSON(refID, title, severity, status string) string {
	_ = status
	return `{
      "external_refs": [{ "kind": "github-pr-comment", "id": "` + refID + `" }],
      "severity": "` + severity + `",
      "category": "code-quality",
      "file": "src/x.ts",
      "lines": "1-5",
      "source": "test",
      "title": "` + title + `",
      "description": "d",
      "why_fix": "w",
      "suggested_fix": "s",
      "consequences": "c"
    }`
}

// commentInBatch is commentJSON with a batch_key field.
func commentInBatch(refID, title, severity, batch string) string {
	return `{
      "external_refs": [{ "kind": "github-pr-comment", "id": "` + refID + `" }],
      "severity": "` + severity + `",
      "category": "code-quality",
      "file": "src/x.ts",
      "lines": "1-5",
      "source": "test",
      "title": "` + title + `",
      "description": "d",
      "why_fix": "w",
      "suggested_fix": "s",
      "consequences": "c",
      "batch_key": "` + batch + `"
    }`
}

func buildPRPayload(number int, title, head, comments string) string {
	return buildPRPayloadWithBatches(number, title, head, "[]", comments)
}

func buildPRPayloadWithBatches(number int, title, head, batches, comments string) string {
	return strings.NewReplacer(
		"$NUMBER", strconv.Itoa(number),
		"$TITLE", title,
		"$HEAD", head,
		"$BATCHES", batches,
		"$COMMENTS", comments,
	).Replace(`{
  "repo": "acme/app",
  "target": {
    "kind": "pr",
    "pr": { "number": $NUMBER, "title": "$TITLE", "url": "https://x", "head_branch": "$HEAD" }
  },
  "batches": $BATCHES,
  "comments": [$COMMENTS]
}`)
}

func buildBranchPayload(name, comments string) string {
	return strings.NewReplacer(
		"$NAME", name,
		"$COMMENTS", comments,
	).Replace(`{
  "repo": "acme/app",
  "target": {
    "kind": "branch",
    "branch": { "name": "$NAME" }
  },
  "comments": [$COMMENTS]
}`)
}
