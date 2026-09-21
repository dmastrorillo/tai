package cmd_test

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/plugins/triage/internal/cmdtest"
)

// TestBoard_TCBRD035_the_binary_detaches_and_the_board_outlives_it is the
// only board case that runs a real process. Every other one drives the
// command tree in-process, where the spawn is a package-variable seam —
// so a re-exec that never produced a serving board would leave all of
// them green. That is the exact shape of the failure this case exists
// for: correct from inside the process, and no board outside it.
//
// The browser is neutralised with a PATH shim rather than the in-process
// NoBrowserForTesting seam, because the process under test is a real one
// and cannot see this one's package variables.
func TestBoard_TCBRD035_the_binary_detaches_and_the_board_outlives_it(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the triage binary")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the browser shim below is a POSIX shell script")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// Built before the environment is isolated: `go build` reads HOME to
	// locate its build cache, and Isolate points HOME at an empty
	// directory.
	bin := buildTriageBinary(t)

	env := cmdtest.Isolate(t)
	t.Setenv("PATH", browserShim(t)+string(os.PathListSeparator)+os.Getenv("PATH"))

	launch := exec.Command(bin, "board", "-")
	launch.Stdin = strings.NewReader(validBriefing)
	out, err := launch.Output()
	if err != nil {
		t.Fatalf("`triage board -` failed: %v", err)
	}

	// A detached server is running from here on, and nothing reaps it
	// but a submit. Its shutdown is registered before the first step
	// that can abort the test — the URL parse below is the likeliest,
	// because it matches a literal this change reformatted — so a
	// failure there leaves no stray server holding a port.
	t.Cleanup(func() { shutdownBoard(string(out)) })

	url := boardURLFrom(t, string(out))
	if launch.ProcessState == nil || !launch.ProcessState.Exited() {
		t.Fatal("the launching process did not exit")
	}

	// The launching process is gone; the board is not.
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("the board did not outlive the command that launched it: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET the board after the launcher exited: want 200, got %d", resp.StatusCode)
	}

	post, err := http.Post(url+"submit", "application/json",
		strings.NewReader(`[{"id":1,"intent":"accept","note":"from a real detached process"}]`))
	if err != nil {
		t.Fatalf("POST submit: %v", err)
	}
	_ = post.Body.Close()

	artifact := filepath.Join(env.DataDir, "plugins", "triage", "state", "intents",
		"acme-app--pr-142.json")
	waitForFile(t, artifact)
	body, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatalf("reading the artifact: %v", err)
	}
	if !strings.Contains(string(body), "from a real detached process") {
		t.Errorf("the submitted note is not in the artifact: %s", body)
	}

	waitForBoardGone(t, url)

	// TC-BRD-037: the server process has no terminal, so its stderr is
	// a file. Asserted here rather than in its own test because it is a
	// property of a real detached process and this is the only case
	// that starts one.
	logPath := filepath.Join(env.DataDir, "plugins", "triage", "state", "board.log")
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("the server process's log was not created at %s: %v", logPath, err)
	}
}

func buildTriageBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "triage")
	build := exec.Command("go", "build", "-o", bin, "./plugins/triage/cmd/triage")
	build.Dir = filepath.Join("..", "..", "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the triage binary: %v\n%s", err, out)
	}
	return bin
}

// browserShim returns a directory holding no-op stand-ins for the
// commands launchBrowser execs, so a test run never opens a window.
func browserShim(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"open", "xdg-open"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("writing the %s shim: %v", name, err)
		}
	}
	return dir
}

func boardURLFrom(t *testing.T, stdout string) string {
	t.Helper()
	url, ok := findBoardURL(stdout)
	if !ok {
		t.Fatalf("stdout announced no board URL:\n%s", stdout)
	}
	return url
}

// findBoardURL is boardURLFrom without the testing.T, so the cleanup
// path can look for the URL without being able to fail the test it is
// cleaning up after.
func findBoardURL(stdout string) (string, bool) {
	const marker = "Board ready at "
	for _, line := range strings.Split(stdout, "\n") {
		if after, ok := strings.CutPrefix(line, marker); ok {
			return strings.TrimSpace(after), true
		}
	}
	return "", false
}

// shutdownBoard submits an empty board, which is how a detached server
// is asked to exit. Best-effort by design: it runs on the way out of a
// test that may already have failed, and a server that has gone is the
// outcome it wants anyway.
func shutdownBoard(stdout string) {
	url, ok := findBoardURL(stdout)
	if !ok {
		return
	}
	resp, err := http.Post(url+"submit", "application/json", strings.NewReader("[]"))
	if err == nil {
		_ = resp.Body.Close()
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", path)
}

func waitForBoardGone(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err != nil {
			return
		}
		_ = resp.Body.Close()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the server process kept serving after the board was submitted")
}
