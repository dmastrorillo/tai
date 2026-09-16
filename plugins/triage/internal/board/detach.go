package board

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/dmastrorillo/tai/pkg/errcode"
)

// ServeChildEnv marks the process as the board's server rather than the
// command that launched it. It is an environment variable and not a
// command-line argument for the same reason the browser is handed a
// secret-free landing path: argv is readable by every other local user
// on macOS and most Linux configurations.
const ServeChildEnv = "TRIAGE_BOARD_SERVE"

// IsServeChild reports whether this process is the detached server.
func IsServeChild() bool { return os.Getenv(ServeChildEnv) == "1" }

// readyTimeout bounds how long the launching command waits for the
// server process to report itself. Binding a loopback socket and
// rendering nothing takes milliseconds; a server that has not spoken in
// ten seconds is not about to.
const readyTimeout = 10 * time.Second

// The server process reports itself on a single line of its stdout,
// which is a pipe held by the command that launched it. Two forms:
//
//	ready <board-url>
//	error <message>
//
// A line rather than a shared file because the pipe is private to the
// two processes: the board URL carries the path prefix that keeps the
// briefing unreadable to anything else on the machine, and a file would
// publish it to every process that can stat the directory.
const (
	readyPrefix = "ready "
	errorPrefix = "error "
)

// FormatReady renders the line the server process writes to announce
// itself. Exported because the announcement is made through Serve's
// announce callback, which the command layer supplies.
func FormatReady(url string) string { return readyPrefix + url + "\n" }

// handOff starts the detached server and returns the board URL it
// reports. It is a package variable so a test can drive the command
// layer without a real process being spawned.
var handOff = spawnServer

// HandOffForTesting replaces the detached-server spawn for the lifetime
// of t. The testing.TB parameter is the guard: a production binary that
// imports `testing` deliberately is a code-review red flag.
func HandOffForTesting(t testing.TB, fn func(briefing []byte) (string, error)) {
	t.Helper()
	prev := handOff
	handOff = fn
	t.Cleanup(func() { handOff = prev })
}

// Launch starts the board and returns once it is serving. The developer
// works the board for as long as they need to and their decisions come
// back through the intents artifact, so the command that calls this has
// nothing left to wait for.
//
// The announcement follows the server reporting itself, never precedes
// it: a URL on stdout is the caller's only handle on the board, so one
// that nothing is serving sends them looking for a failure that left no
// other trace.
func Launch(briefing []byte, announce func(url string)) error {
	url, err := handOff(briefing)
	if err != nil {
		// The cause is repeated into the message because the error
		// template renders Msg and nothing else, and the cause here is
		// the whole diagnosis — a bind refused by a sandbox and a
		// binary that could not be re-executed are different problems
		// with different fixes.
		return errcode.Wrapf(errcode.TriageBoardUnavailable, err,
			"starting the board's server process: %s", err).
			WithHelp(
				"check whether a local firewall or sandbox blocks binding 127.0.0.1",
				"the board needs no outbound network access, only a loopback socket",
			)
	}
	announce(url)
	return nil
}

// spawnServer re-execs this binary as the board's server in a session of
// its own, feeds it the briefing on stdin, and waits for the one line it
// writes back.
//
// It is deliberately not `exec.CommandContext`: the point of the process
// is to outlive this one.
func spawnServer(briefing []byte) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating this binary: %w", err)
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", os.DevNull, err)
	}
	defer func() { _ = devNull.Close() }()

	// An os.Pipe rather than cmd.StdoutPipe: the latter is closed by
	// cmd.Wait, and nothing here ever waits on a process whose whole
	// job is to still be running when this one is gone.
	readyR, readyW, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("opening the readiness pipe: %w", err)
	}
	defer func() { _ = readyR.Close() }()

	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		_ = readyW.Close()
		return "", fmt.Errorf("opening the briefing pipe: %w", err)
	}
	defer func() { _ = stdinR.Close() }()

	cmd := exec.Command(exe, "board", "-")
	cmd.Env = append(os.Environ(), ServeChildEnv+"=1")
	cmd.Stdin = stdinR
	cmd.Stdout = readyW
	cmd.Stderr = devNull
	cmd.SysProcAttr = detachAttr()

	if err := cmd.Start(); err != nil {
		_ = readyW.Close()
		_ = stdinW.Close()
		return "", fmt.Errorf("starting %s: %w", exe, err)
	}
	// This process's copies of the child's ends are closed so the reader
	// below sees EOF when the child exits rather than blocking on a
	// writer only this process still holds.
	_ = readyW.Close()

	// A briefing may run to megabytes and a pipe holds far less, so the
	// write has to proceed while the child is draining it.
	go func() {
		defer func() { _ = stdinW.Close() }()
		_, _ = stdinW.Write(briefing)
	}()

	url, err := readReady(readyR)
	if err != nil {
		_ = cmd.Process.Kill()
		return "", err
	}
	return url, nil
}

// readReady reads the server process's single line of self-report.
func readReady(r io.Reader) (string, error) {
	type result struct {
		line string
		err  error
	}
	lines := make(chan result, 1)
	go func() {
		line, err := bufio.NewReader(r).ReadString('\n')
		lines <- result{line: line, err: err}
	}()

	select {
	case got := <-lines:
		line := strings.TrimRight(got.line, "\r\n")
		switch {
		case strings.HasPrefix(line, readyPrefix):
			return strings.TrimPrefix(line, readyPrefix), nil
		case strings.HasPrefix(line, errorPrefix):
			return "", fmt.Errorf("%s", strings.TrimPrefix(line, errorPrefix))
		case got.err != nil:
			return "", fmt.Errorf("the server process exited before it began serving")
		default:
			return "", fmt.Errorf("the server process reported %q, which is not a readiness line", line)
		}
	case <-time.After(readyTimeout):
		return "", fmt.Errorf("the server process did not report itself within %s", readyTimeout)
	}
}

// ServeDetached is the body of the board's server process: it serves the
// briefing until the developer submits and reports itself on w, which is
// the pipe the launching command reads before it announces the URL.
//
// A bind failure is reported on the same pipe rather than left to this
// process's stderr, which is /dev/null. The launching command is the only
// one with a terminal, so it is the only one that can tell anybody.
func ServeDetached(ctx context.Context, b Briefing, w io.Writer) ([]IntentEntry, error) {
	srv, err := NewServer(b)
	if err != nil {
		reportFailure(w, err)
		return nil, err
	}
	entries, err := srv.Serve(ctx, func(url string) {
		_, _ = io.WriteString(w, FormatReady(url))
	})
	if err != nil {
		reportFailure(w, err)
		return nil, err
	}
	return entries, nil
}

// reportFailure sends one line back up the readiness pipe. The message is
// flattened to a single line because the pipe carries exactly one.
func reportFailure(w io.Writer, err error) {
	msg := strings.Join(strings.Fields(err.Error()), " ")
	_, _ = io.WriteString(w, errorPrefix+msg+"\n")
}
