package repoctx_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/dmastrorillo/tai/pkg/errcode"
	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdtest"
	"github.com/dmastrorillo/tai/plugins/triage/internal/repoctx"
)

// TestParseOriginURL_TCREPO001_ssh exercises TC-REPO-001: SSH-form
// origin URLs (git@host:owner/name.git) normalise to owner/name.
func TestParseOriginURL_TCREPO001_ssh(t *testing.T) {
	got, err := repoctx.ParseOriginURL("git@github.com:acme/app.git")
	if err != nil {
		t.Fatalf("ParseOriginURL: %v", err)
	}
	if got.String() != "acme/app" {
		t.Fatalf("want acme/app, got %q", got)
	}
}

// TestParseOriginURL_TCREPO002_https_with_dot_git exercises TC-REPO-002:
// HTTPS-form origin URLs with a `.git` suffix normalise to owner/name.
func TestParseOriginURL_TCREPO002_https_with_dot_git(t *testing.T) {
	got, err := repoctx.ParseOriginURL("https://github.com/acme/app.git")
	if err != nil {
		t.Fatalf("ParseOriginURL: %v", err)
	}
	if got.String() != "acme/app" {
		t.Fatalf("want acme/app, got %q", got)
	}
}

// TestParseOriginURL_TCREPO003_https_without_dot_git exercises TC-REPO-003:
// HTTPS-form origin URLs without a `.git` suffix also work.
func TestParseOriginURL_TCREPO003_https_without_dot_git(t *testing.T) {
	got, err := repoctx.ParseOriginURL("https://github.com/acme/app")
	if err != nil {
		t.Fatalf("ParseOriginURL: %v", err)
	}
	if got.String() != "acme/app" {
		t.Fatalf("want acme/app, got %q", got)
	}
}

// TestRead_TCREPO004_not_a_repo exercises TC-REPO-004: Read from a
// directory that is not inside any git repository returns
// REPO_NOT_FOUND.
func TestRead_TCREPO004_not_a_repo(t *testing.T) {
	cmdtest.Chdir(t, t.TempDir())

	_, err := repoctx.Read(context.Background())
	if err == nil {
		t.Fatal("expected REPO_NOT_FOUND, got nil")
	}
	taiErr, ok := errcode.As(err)
	if !ok {
		t.Fatalf("expected *errcode.Error, got %T: %v", err, err)
	}
	if taiErr.Code != errcode.RepoNotFound {
		t.Fatalf("expected REPO_NOT_FOUND, got %s", taiErr.Code)
	}
}

// TestRead_TCREPO005_no_origin exercises TC-REPO-005: a git repo with
// no `origin` remote returns REPO_NOT_FOUND, and the help text
// suggests `git remote add origin` (the user-observable "And" clause).
func TestRead_TCREPO005_no_origin(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q")

	cmdtest.Chdir(t, dir)

	_, err := repoctx.Read(context.Background())
	if err == nil {
		t.Fatal("expected REPO_NOT_FOUND, got nil")
	}
	taiErr, ok := errcode.As(err)
	if !ok {
		t.Fatalf("expected *errcode.Error, got %T: %v", err, err)
	}
	if taiErr.Code != errcode.RepoNotFound {
		t.Fatalf("expected REPO_NOT_FOUND, got %s", taiErr.Code)
	}
	// Spec: the help text MUST suggest `git remote add origin`.
	helpJoined := strings.Join(taiErr.Help, " ")
	if !strings.Contains(helpJoined, "git remote add origin") {
		t.Fatalf("help text should suggest `git remote add origin`, got %v",
			taiErr.Help)
	}
}

// TestResolve_TCREPO006_repo_flag_override exercises TC-REPO-006: when
// the --repo flag is provided, it overrides auto-detection — including
// outside any git repo.
func TestResolve_TCREPO006_repo_flag_override(t *testing.T) {
	cmdtest.Chdir(t, t.TempDir()) // not a repo

	got, err := repoctx.Resolve(context.Background(), "acme/app")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.String() != "acme/app" {
		t.Fatalf("want acme/app, got %q", got)
	}
}

// TestParseIdentity_TCREPO007_malformed exercises TC-REPO-007: a
// malformed --repo value yields REPO_FLAG_INVALID with remediation.
func TestParseIdentity_TCREPO007_malformed(t *testing.T) {
	cases := []string{
		"just-a-name",
		"too/many/slashes",
		"",
		"/name",
		"owner/",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, err := repoctx.ParseIdentity(in)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", in)
			}
			taiErr, ok := errcode.As(err)
			if !ok {
				t.Fatalf("expected *errcode.Error, got %T: %v", err, err)
			}
			if taiErr.Code != errcode.RepoFlagInvalid {
				t.Fatalf("expected REPO_FLAG_INVALID, got %s", taiErr.Code)
			}
		})
	}
}

// TestParseIdentity_accessors is an engine test: it asserts the
// Identity helpers behave correctly. Not tied to a BDD case because the
// accessors are scaffolding for future callers — the user never sees
// Owner()/Name() output directly.
func TestParseIdentity_accessors(t *testing.T) {
	id, err := repoctx.ParseIdentity("acme/app")
	if err != nil {
		t.Fatalf("ParseIdentity: %v", err)
	}
	if id.Owner() != "acme" || id.Name() != "app" || id.String() != "acme/app" {
		t.Fatalf("Owner/Name/String mismatch: %s / %s / %s", id.Owner(), id.Name(), id.String())
	}
}

// mustGit runs `git <args>` in dir, failing the test on any error. Used
// for setting up fixture repos without taking on a git library dep.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}
